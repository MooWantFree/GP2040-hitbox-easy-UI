package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testProfile(label string, count int) object {
	p := object{"profileLabel": label, "enabled": true, "pinInversionEnabled": true, "invertedPins": []int{22}, "futureField": object{"keep": "untouched"}}
	for i := 0; i < count; i++ {
		action := i + 1
		if i == 0 {
			action = -5
		}
		if i == 1 {
			action = 0
		}
		p[fmt.Sprintf("pin%02d", i)] = object{"action": action, "customButtonMask": 0, "customDpadMask": 0, "futurePinField": "keep"}
	}
	return p
}
func deepCopy(value any) object {
	data, _ := json.Marshal(value)
	var out object
	_ = decodeJSON(data, &out)
	return out
}
func mapping(action int) object {
	return object{"action": action, "customButtonMask": 0, "customDpadMask": 0}
}

type fixture struct {
	a                       *app
	base, alts              object
	writes                  []string
	capture                 []any
	failBackup, ignoreWrite bool
}

func newFixture(t *testing.T, count int) *fixture {
	t.Helper()
	f := &fixture{base: testProfile("Base", count), alts: object{"alternativePinMappings": []any{testProfile("Alt", count), testProfile("Other", count)}, "futureOption": 123}, capture: []any{5}}
	f.a = newApp(t.TempDir(), func(_ context.Context, _ string, endpoint string, payload any) (object, error) {
		switch endpoint {
		case "getFirmwareVersion":
			return object{"version": "test-only", "boardBuild": "TEST", "boardConfig": 0, "boardConfigLabel": "TEST"}, nil
		case "getPinMappings":
			return deepCopy(f.base), nil
		case "getProfileOptions":
			return deepCopy(f.alts), nil
		case "getGamepadOptions":
			return object{"profileNumber": 1, "inputMode": 0}, nil
		case "getConfig":
			if f.failBackup {
				return nil, errors.New("backup unavailable")
			}
			return deepCopy(object{"marker": "full-config", "gpioMappings": f.base, "extra": "keep"}), nil
		case "getHeldPins":
			return object{"heldPins": f.capture}, nil
		case "setPinMappings":
			f.writes = append(f.writes, endpoint)
			if !f.ignoreWrite {
				f.base = deepCopy(payload)
			}
			return object{"ok": true}, nil
		case "setProfileOptions":
			f.writes = append(f.writes, endpoint)
			if !f.ignoreWrite {
				f.alts = deepCopy(payload)
			}
			return object{"ok": true}, nil
		}
		return nil, fmt.Errorf("unexpected endpoint %s", endpoint)
	})
	return f
}
func (f *fixture) call(path string, body any, method string, origin string) (int, object) {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(method, "http://127.0.0.1:8097/api/"+path, bytes.NewReader(data))
	req.Header.Set("X-Hitbox-Local", "1")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	f.a.ServeHTTP(w, req)
	var out object
	_ = decodeJSON(w.Body.Bytes(), &out)
	return w.Code, out
}
func (f *fixture) connect(t *testing.T) string {
	t.Helper()
	status, out := f.call("connect", object{"target": "192.168.7.1"}, "POST", "")
	if status != 200 {
		t.Fatalf("connect: %d %v", status, out)
	}
	return out["token"].(string)
}
func (f *fixture) save(token string, index int, changes object) (int, object) {
	return f.call("save", object{"token": token, "profileIndex": index, "changes": changes}, "POST", "")
}

func TestFullTableSaveAndRestorableBackup(t *testing.T) {
	f := newFixture(t, 30)
	token := f.connect(t)
	before := deepCopy(f.base)
	status, out := f.save(token, 0, object{"pin05": mapping(7)})
	if status != 200 {
		t.Fatalf("save %d %v", status, out)
	}
	expected := deepCopy(before)
	expected["pin05"].(object)["action"] = 7
	if !same(f.base, expected) || len(f.writes) != 1 {
		t.Fatal("lost fields or wrong writes")
	}
	name := out["backup"].(string)
	status, backup := f.call("backups/"+name, nil, "GET", "")
	if status != 200 || backup["marker"] != "full-config" || !same(backup["gpioMappings"], before) {
		t.Fatal("full backup lost original")
	}
	status, backup = f.call("backups/"+strings.TrimSuffix(name, ".json")+".gp2040", nil, "GET", "")
	if status != 200 || !same(backup["pins"], before) || !same(backup["profiles"], f.alts["alternativePinMappings"]) {
		t.Fatal("official restoration shape incorrect")
	}
}
func Test48PinAlternativePreservesWholeArray(t *testing.T) {
	f := newFixture(t, 48)
	token := f.connect(t)
	expected := deepCopy(f.alts)
	expected["alternativePinMappings"].([]any)[0].(object)["pin47"].(object)["action"] = 11
	status, out := f.save(token, 1, object{"pin47": mapping(11)})
	if status != 200 || !same(f.alts, expected) || f.writes[0] != "setProfileOptions" {
		t.Fatalf("alternative save %d %v", status, out)
	}
}
func TestStaleSnapshotStopsBeforeBackup(t *testing.T) {
	f := newFixture(t, 30)
	token := f.connect(t)
	f.base["pin09"] = mapping(18)
	status, _ := f.save(token, 0, object{"pin05": mapping(7)})
	files, _ := os.ReadDir(f.a.dataDir)
	if status != 409 || len(f.writes) != 0 || len(files) != 0 {
		t.Fatal("stale state wrote data")
	}
}
func TestProtectedPinsAndMalformedChanges(t *testing.T) {
	f := newFixture(t, 30)
	token := f.connect(t)
	for _, change := range []object{{"pin00": mapping(5)}, {"pin01": mapping(5)}, {"pin49": mapping(5)}, {"pin05": object{"action": 5}}, {"pin05": object{"action": 40, "customButtonMask": -1, "customDpadMask": 0}}, {"pin05": mapping(20)}} {
		status, _ := f.save(token, 0, change)
		if status != 400 {
			t.Fatalf("accepted invalid change %v", change)
		}
	}
	if len(f.writes) != 0 {
		t.Fatal("protected write")
	}
}
func TestBackupFailurePreventsWrite(t *testing.T) {
	f := newFixture(t, 30)
	token := f.connect(t)
	f.failBackup = true
	status, _ := f.save(token, 0, object{"pin05": mapping(7)})
	if status != 500 || len(f.writes) != 0 {
		t.Fatal("write without backup")
	}
}
func TestReadbackMismatchInvalidatesSession(t *testing.T) {
	f := newFixture(t, 30)
	token := f.connect(t)
	f.ignoreWrite = true
	status, out := f.save(token, 0, object{"pin05": mapping(7)})
	if status != 502 || out["uncertain"] != true || out["backup"] == nil {
		t.Fatal("missing uncertain state and backup")
	}
	status, _ = f.save(token, 0, object{"pin05": mapping(7)})
	if status != 409 || len(f.writes) != 1 {
		t.Fatal("allowed blind retry")
	}
}
func TestCaptureKeepsMultiplePinsAndGPIOZero(t *testing.T) {
	f := newFixture(t, 30)
	token := f.connect(t)
	f.capture = []any{0, 5, 6, 5}
	status, out := f.call("capture", object{"token": token}, "POST", "")
	if status != 200 || !same(out["heldPins"], []int{0, 5, 6}) {
		t.Fatal("capture changed pins")
	}
	f.capture = []any{999}
	status, _ = f.call("capture", object{"token": token}, "POST", "")
	if status != 502 {
		t.Fatal("invalid pin accepted")
	}
}
func TestLegacyFirmwareRejected(t *testing.T) {
	f := newFixture(t, 30)
	f.base["pin02"] = 5
	status, _ := f.call("connect", object{"target": "192.168.7.1"}, "POST", "")
	if status != 502 || len(f.writes) != 0 {
		t.Fatal("legacy scalar accepted")
	}
}
func TestLayoutMigrationRetainsBindingsAndOriginal(t *testing.T) {
	f := newFixture(t, 30)
	old := []byte(`{"version":1,"name":"旧布局","image":"data:image/png;base64,AA==","deviceKey":"device","buttons":[{"id":"left","name":"左手","x":20,"y":40,"size":7,"pin":0},{"id":"right","name":"右手","x":60,"y":40,"size":8,"pin":47}]}`)
	path := filepath.Join(f.a.dataDir, "layout.json")
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	status, l := f.call("layout", nil, "GET", "")
	if status != 200 || l["version"] != json.Number("2") || l["image"] != nil {
		t.Fatal("migration failed")
	}
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, old) {
		t.Fatal("GET modified existing data")
	}
	for i := 0; i < 2; i++ {
		l["name"] = fmt.Sprintf("更名%d", i)
		status, out := f.call("layout", l, "PUT", "")
		if status != 200 {
			t.Fatalf("layout overwrite %d %v", status, out)
		}
	}
	backup, _ := os.ReadFile(filepath.Join(f.a.dataDir, "layout.before-go.json"))
	if !bytes.Equal(backup, old) {
		t.Fatal("original backup missing")
	}
	_, got := f.call("layout", nil, "GET", "")
	if !same(got["buttons"], l["buttons"]) {
		t.Fatal("lost bindings")
	}
	buttons := l["buttons"].([]any)
	buttons[1].(object)["pin"] = 0
	status, _ = f.call("layout", l, "PUT", "")
	if status != 400 {
		t.Fatal("duplicate pin accepted")
	}
}
func TestLayoutShapesPersistAndRejectUnknownTypes(t *testing.T) {
	f := newFixture(t, 30)
	l := object{"version": 2, "name": "形状布局", "deviceKey": nil, "labelSet": "xbox", "buttons": []any{
		object{"id": "small", "name": "小圆点", "x": 20, "y": 30, "size": 4, "shape": "dot", "pin": nil},
		object{"id": "normal", "name": "圆形", "x": 50, "y": 50, "size": 7, "shape": "circle", "pin": 5},
		object{"id": "long", "name": "胶囊", "x": 70, "y": 60, "size": 11, "shape": "capsule", "pin": nil},
	}}
	status, out := f.call("layout", l, "PUT", "")
	if status != 200 {
		t.Fatalf("save shapes: %d %v", status, out)
	}
	// A new server instance must recover the shape and binding from disk.
	f.a = newApp(f.a.dataDir, nil)
	status, out = f.call("layout", nil, "GET", "")
	if status != 200 || !same(out, l) {
		t.Fatalf("shape roundtrip: %d %v", status, out)
	}
	buttons := l["buttons"].([]any)
	buttons[0].(object)["shape"] = "triangle"
	status, _ = f.call("layout", l, "PUT", "")
	if status != 400 {
		t.Fatal("unknown shape accepted")
	}
	delete(buttons[0].(object), "shape")
	status, _ = f.call("layout", l, "PUT", "")
	_, out = f.call("layout", nil, "GET", "")
	legacy := out["buttons"].([]any)[0].(object)
	if status != 200 || legacy["shape"] != "circle" || !same(legacy["size"], 4) {
		t.Fatal("legacy geometry was changed")
	}
}

func TestSourceTokenAndConcurrentRequestGuards(t *testing.T) {
	f := newFixture(t, 30)
	status, _ := f.call("connect", object{"target": "192.168.7.1"}, "POST", "https://example.com")
	if status != 403 {
		t.Fatal("cross origin allowed")
	}
	token := f.connect(t)
	f.connect(t)
	status, _ = f.call("capture", object{"token": token}, "POST", "")
	if status != 409 {
		t.Fatal("old token accepted")
	}
	f.a.deviceMu.Lock()
	status, _ = f.call("connect", object{"target": "192.168.7.1"}, "POST", "")
	f.a.deviceMu.Unlock()
	if status != 409 {
		t.Fatal("concurrent device request accepted")
	}
	req := httptest.NewRequest("POST", "http://evil.example:8097/api/quit", strings.NewReader("{}"))
	req.Header.Set("X-Hitbox-Local", "1")
	w := httptest.NewRecorder()
	f.a.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("foreign host accepted")
	}
}
func TestEmbeddedAssetsAndHealth(t *testing.T) {
	f := newFixture(t, 30)
	for _, path := range []string{"/", "/app.js", "/model.js", "/style.css", "/api/health"} {
		req := httptest.NewRequest("GET", "http://127.0.0.1:8097"+path, nil)
		w := httptest.NewRecorder()
		f.a.ServeHTTP(w, req)
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("missing embed %s", path)
		}
		if path == "/" && (strings.Contains(w.Body.String(), "imageInput") || strings.Contains(w.Body.String(), "上传")) {
			t.Fatal("image upload remains")
		}
	}
}
func TestDeviceHTTPTransport(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	payload := object{"profileLabel": "我的配置", "pin02": mapping(5)}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/redirect" {
			http.Redirect(w, r, "/api/setPinMappings", 302)
			return
		}
		data, _ := io.ReadAll(r.Body)
		var got object
		_ = decodeJSON(data, &got)
		if r.Method != "POST" || r.URL.Path != "/api/setPinMappings" || r.ContentLength != int64(len(data)) || !same(got, payload) {
			t.Error("wrong HTTP payload")
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer fake.Close()
	device := newDeviceClient()
	out, err := device(context.Background(), fake.URL, "setPinMappings", payload)
	if err != nil || out["ok"] != true {
		t.Fatal(err)
	}
	if _, err = device(context.Background(), fake.URL, "redirect", nil); err == nil {
		t.Fatal("redirect followed")
	}
}
func TestTargetAndCombinationValidation(t *testing.T) {
	for _, bad := range []string{"https://example.com", "8.8.8.8", "192.168.7.1/path", "user@192.168.7.1", "192.168.7.1:0"} {
		if _, err := normalizeTarget(bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	if got, err := normalizeTarget(" 192.168.7.1:80/ "); err != nil || got != "http://192.168.7.1" {
		t.Fatal("normalization")
	}
	if err := validateMapping(object{"action": 40, "customButtonMask": 65, "customDpadMask": 1}); err != nil {
		t.Fatal(err)
	}
	for _, mask := range []int{-1, 16384} {
		if validateMapping(object{"action": 40, "customButtonMask": mask, "customDpadMask": 0}) == nil {
			t.Fatal("invalid mask")
		}
	}
}
