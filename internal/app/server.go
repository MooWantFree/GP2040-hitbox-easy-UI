package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"hitbox-map/web"
)

type session struct {
	Token     string    `json:"token"`
	Target    string    `json:"target"`
	DeviceKey string    `json:"deviceKey"`
	Snapshot  *snapshot `json:"snapshot"`
}

type app struct {
	dataDir   string
	localeDir string
	device    deviceFunc
	deviceMu  sync.Mutex
	layoutMu  sync.Mutex
	session   *session
}

func newApp(dataDir string, device deviceFunc) *app {
	if device == nil {
		device = newDeviceClient()
	}
	return &app{dataDir: dataDir, device: device, localeDir: localeDirectory()}
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sendJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func sendError(w http.ResponseWriter, status int, err error) {
	sendJSON(w, status, errorResponse(err))
}

func readBody(w http.ResponseWriter, r *http.Request, out any, limit int64) error {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		return messageError("error.body")
	}
	if err := decodeJSON(data, out); err != nil {
		return messageError("error.json")
	}
	return nil
}

func writeExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".layout-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (a *app) backup(ctx context.Context, target string, current *snapshot) (string, error) {
	config, err := a.device(ctx, target, "getConfig", nil)
	if err != nil {
		return "", err
	}
	if len(config) == 0 {
		return "", messageError("error.fullConfig")
	}
	id, err := randomID()
	if err != nil {
		return "", err
	}
	base := strings.ReplaceAll(time.Now().UTC().Format("2006-01-02T15-04-05.000Z"), ".", "-") + "-" + id[:8]
	dir := filepath.Join(a.dataDir, "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	files := []struct {
		ext   string
		value any
	}{
		{".json", config},
		{".meta.json", object{"target": target, "firmware": current.Firmware, "profiles": current.Profiles}},
		{".gp2040", object{"pins": current.Profiles[0], "profiles": current.Profiles[1:]}},
	}
	for _, file := range files {
		data, err := json.MarshalIndent(file.value, "", "  ")
		if err != nil {
			return "", err
		}
		if err := writeExclusive(filepath.Join(dir, base+file.ext), data); err != nil {
			return "", err
		}
	}
	return base + ".json", nil
}

var backupPattern = regexp.MustCompile(`^[\dTZ-]+-[a-f0-9]{8}\.(json|gp2040)$`)

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || port == "" || (host != "127.0.0.1" && host != "localhost") {
		sendError(w, 403, messageError("error.localOnly"))
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		sendError(w, 403, messageError("error.crossOrigin"))
		return
	}
	path := r.URL.Path
	if r.Method == http.MethodGet {
		if path == "/api/locales" {
			sendJSON(w, 200, loadLanguages(a.localeDir))
			return
		}
		if path == "/api/health" {
			sendJSON(w, 200, object{"app": "hitbox-map", "protocol": 2})
			return
		}
		if path == "/favicon.ico" {
			w.WriteHeader(204)
			return
		}
		static := map[string]string{"/": "index.html", "/style.css": "style.css", "/app.js": "app.js", "/model.js": "model.js", "/i18n.js": "i18n.js", "/layout-editor.js": "layout-editor.js"}
		if name, ok := static[path]; ok {
			data, err := web.Files.ReadFile(name)
			if err != nil {
				sendError(w, 500, err)
				return
			}
			contentType := "text/javascript; charset=utf-8"
			if strings.HasSuffix(name, ".html") {
				contentType = "text/html; charset=utf-8"
			}
			if strings.HasSuffix(name, ".css") {
				contentType = "text/css; charset=utf-8"
			}
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(data)
			return
		}
	}
	if !strings.HasPrefix(path, "/api/") || r.Header.Get("X-Hitbox-Local") != "1" {
		sendError(w, 403, messageError("error.origin"))
		return
	}
	if path == "/api/layout" {
		a.handleLayout(w, r)
		return
	}
	if r.Method == http.MethodGet && strings.HasPrefix(path, "/api/backups/") {
		name := strings.TrimPrefix(path, "/api/backups/")
		if !backupPattern.MatchString(name) {
			sendError(w, 400, messageError("error.backupName"))
			return
		}
		data, err := os.ReadFile(filepath.Join(a.dataDir, "backups", name))
		if err != nil {
			sendError(w, 404, messageError("error.backupMissing"))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
		return
	}
	if r.Method != http.MethodPost {
		sendError(w, 404, messageError("error.endpoint"))
		return
	}
	switch path {
	case "/api/connect", "/api/capture", "/api/backup", "/api/save":
	default:
		sendError(w, 404, messageError("error.endpoint"))
		return
	}
	var input struct {
		Target       string            `json:"target"`
		Token        string            `json:"token"`
		ProfileIndex *int              `json:"profileIndex"`
		Changes      map[string]object `json:"changes"`
	}
	if err := readBody(w, r, &input, 200_000); err != nil {
		sendError(w, 400, err)
		return
	}
	if !a.deviceMu.TryLock() {
		sendError(w, 409, messageError("error.deviceBusy"))
		return
	}
	defer a.deviceMu.Unlock()
	// Complete backup/write/readback even when the browser closes mid-request.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if path == "/api/connect" {
		a.session = nil
		target, err := normalizeTarget(input.Target)
		if err != nil {
			sendError(w, 400, err)
			return
		}
		current, err := readSnapshot(ctx, a.device, target)
		if err != nil {
			sendError(w, 502, err)
			return
		}
		board, _ := current.Firmware["boardBuild"].(string)
		config := current.Firmware["boardConfig"]
		n, isNumber := integer(config)
		if config == nil || config == "" || config == false || (isNumber && n == 0) {
			config = current.Firmware["boardConfigLabel"]
		}
		if config == nil {
			config = ""
		}
		key, err := json.Marshal([]any{target, board, config})
		if err != nil {
			sendError(w, 502, err)
			return
		}
		token, err := randomID()
		if err != nil {
			sendError(w, 500, err)
			return
		}
		a.session = &session{Token: token, Target: target, DeviceKey: string(key), Snapshot: current}
		sendJSON(w, 200, a.session)
		return
	}
	if a.session == nil || input.Token != a.session.Token {
		sendError(w, 409, messageError("error.session"))
		return
	}
	s := a.session
	switch path {
	case "/api/capture":
		result, err := a.device(ctx, s.Target, "getHeldPins", nil)
		if err != nil {
			sendError(w, 502, err)
			return
		}
		pins, ok := result["heldPins"].([]any)
		if !ok {
			sendError(w, 502, messageError("error.captureUnsupported"))
			return
		}
		seen := map[int64]bool{}
		held := []int64{}
		for _, value := range pins {
			pin, ok := integer(value)
			if !ok || pin < 0 || s.Snapshot.Profiles[0][fmt.Sprintf("pin%02d", pin)] == nil {
				sendError(w, 502, messageError("error.captureInvalid"))
				return
			}
			if !seen[pin] {
				held = append(held, pin)
				seen[pin] = true
			}
		}
		sendJSON(w, 200, object{"heldPins": held})
	case "/api/backup":
		current, err := readSnapshot(ctx, a.device, s.Target)
		if err != nil {
			sendError(w, 502, err)
			return
		}
		name, err := a.backup(ctx, s.Target, current)
		if err != nil {
			sendError(w, 500, messageError("error.backup", err))
			return
		}
		sendJSON(w, 200, object{"name": name})
	case "/api/save":
		if input.ProfileIndex == nil {
			sendError(w, 400, messageError("error.chooseProfile"))
			return
		}
		current, err := readSnapshot(ctx, a.device, s.Target)
		if err != nil {
			sendError(w, 502, err)
			return
		}
		if !same(current.Firmware, s.Snapshot.Firmware) || !same(current.Profiles, s.Snapshot.Profiles) {
			sendError(w, 409, messageError("error.conflict"))
			return
		}
		next, err := applyChanges(current, *input.ProfileIndex, input.Changes)
		if err != nil {
			sendError(w, 400, err)
			return
		}
		name, err := a.backup(ctx, s.Target, current)
		if err != nil {
			sendError(w, 500, messageError("error.backupAborted", err))
			return
		}
		if *input.ProfileIndex == 0 {
			_, err = a.device(ctx, s.Target, "setPinMappings", next.Profiles[0])
		} else {
			payload := cloneObject(current.Alternatives)
			payload["alternativePinMappings"] = next.Profiles[1:]
			_, err = a.device(ctx, s.Target, "setProfileOptions", payload)
		}
		var verified *snapshot
		if err == nil {
			verified, err = readSnapshot(ctx, a.device, s.Target)
		}
		if err == nil && !same(verified.Profiles, next.Profiles) {
			err = messageError("error.readback")
		}
		if err != nil {
			a.session = nil
			response := errorResponse(messageError("error.uncertain", err))
			response["backup"], response["uncertain"] = name, true
			sendJSON(w, 502, response)
			return
		}
		s.Snapshot = verified
		sendJSON(w, 200, object{"snapshot": verified, "backup": name})
	default:
		sendError(w, 404, messageError("error.endpoint"))
	}
}

func (a *app) handleLayout(w http.ResponseWriter, r *http.Request) {
	a.layoutMu.Lock()
	defer a.layoutMu.Unlock()
	path := filepath.Join(a.dataDir, "layout.json")
	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			sendJSON(w, 200, nil)
			return
		}
		if err != nil {
			sendError(w, 500, messageError("error.layoutRead", err))
			return
		}
		var l layout
		if err := decodeJSON(data, &l); err != nil {
			sendError(w, 500, messageError("error.layoutCorrupt"))
			return
		}
		if err := normalizeLayout(&l); err != nil {
			sendError(w, 500, err)
			return
		}
		sendJSON(w, 200, l)
	case http.MethodPut:
		var l layout
		if err := readBody(w, r, &l, 16_000_000); err != nil {
			sendError(w, 400, err)
			return
		}
		if err := normalizeLayout(&l); err != nil {
			sendError(w, 400, err)
			return
		}
		if data, err := os.ReadFile(path); err == nil {
			var old struct {
				Version int `json:"version"`
			}
			if json.Unmarshal(data, &old) == nil && old.Version == 1 {
				if err := writeExclusive(filepath.Join(a.dataDir, "layout.before-go.json"), data); err != nil && !os.IsExist(err) {
					sendError(w, 500, messageError("error.legacyBackup", err))
					return
				}
			}
		} else if !os.IsNotExist(err) {
			sendError(w, 500, err)
			return
		}
		data, err := json.MarshalIndent(&l, "", "  ")
		if err == nil {
			err = writeAtomic(path, data)
		}
		if err != nil {
			sendError(w, 500, messageError("error.layoutSave", err))
			return
		}
		sendJSON(w, 200, object{"saved": true})
	default:
		sendError(w, 405, messageError("error.method"))
	}
}
