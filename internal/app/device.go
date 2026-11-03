package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type deviceFunc func(context.Context, string, string, any) (object, error)

func normalizeTarget(input string) (string, error) {
	raw := strings.TrimSpace(input)
	if !strings.HasPrefix(raw, "http://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", messageError("error.address")
	}
	addr, err := netip.ParseAddr(u.Hostname())
	if err != nil || !addr.Is4() || (!addr.IsPrivate() && addr.String() != "127.0.0.1") {
		return "", messageError("error.address")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", messageError("error.port")
		}
	}
	host := u.Host
	if u.Port() == "80" {
		host = u.Hostname()
	}
	return "http://" + host, nil
}

func decodeJSON(data []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return messageError("error.jsonExtra")
	}
	return nil
}

func newDeviceClient() deviceFunc {
	// USB network traffic must go directly to the device, not through a proxy.
	client := &http.Client{
		Timeout:       9 * time.Second,
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return func(ctx context.Context, target, endpoint string, payload any) (object, error) {
		var data []byte
		method := http.MethodGet
		if payload != nil {
			var err error
			data, err = json.Marshal(payload)
			if err != nil {
				return nil, err
			}
			method = http.MethodPost
		}
		req, err := http.NewRequestWithContext(ctx, method, target+"/api/"+endpoint, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return nil, messageError("error.deviceConnection", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, messageError("error.deviceStatus", endpoint, res.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(res.Body, 4_000_001))
		if err != nil {
			return nil, err
		}
		if len(data) > 4_000_000 {
			return nil, messageError("error.deviceLarge")
		}
		var result object
		if err = decodeJSON(data, &result); err != nil || result == nil {
			return nil, messageError("error.deviceConfig", endpoint)
		}
		return result, nil
	}
}

func readSnapshot(ctx context.Context, device deviceFunc, target string) (*snapshot, error) {
	firmware, err := device(ctx, target, "getFirmwareVersion", nil)
	if err != nil {
		return nil, err
	}
	if _, ok := firmware["version"].(string); !ok {
		return nil, messageError("error.firmware")
	}
	base, err := device(ctx, target, "getPinMappings", nil)
	if err != nil {
		return nil, err
	}
	alternatives, err := device(ctx, target, "getProfileOptions", nil)
	if err != nil {
		return nil, err
	}
	options, err := device(ctx, target, "getGamepadOptions", nil)
	if err != nil {
		return nil, err
	}
	alts, ok := alternatives["alternativePinMappings"].([]any)
	if !ok {
		return nil, messageError("error.profileFormat")
	}
	s := &snapshot{Firmware: firmware, Options: options, Alternatives: alternatives, Profiles: []object{base}}
	for _, alt := range alts {
		p, ok := alt.(object)
		if !ok {
			return nil, messageError("error.profileFormat")
		}
		s.Profiles = append(s.Profiles, p)
	}
	if err := validateSnapshot(s); err != nil {
		return nil, err
	}
	return s, nil
}
