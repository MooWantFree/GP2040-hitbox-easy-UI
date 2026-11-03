package app

import (
	"context"
	"io"
	"testing"
)

func TestRebootUsesConnectedDeviceAndControllerMode(t *testing.T) {
	f := newFixture(t, 30)
	token := f.connect(t)
	calls := 0
	f.a.device = func(_ context.Context, target, endpoint string, payload any) (object, error) {
		calls++
		if target != "http://192.168.7.1" || endpoint != "reboot" || !same(payload, object{"bootMode": 0}) {
			t.Fatalf("unexpected reboot: %s %s %#v", target, endpoint, payload)
		}
		return object{"success": true}, nil
	}
	status, out := f.call("reboot", object{"token": token, "target": "127.0.0.1:9999", "bootMode": 2}, "POST", "")
	if status != 200 || out["rebooting"] != true || f.a.session != nil || calls != 1 || len(f.writes) != 0 {
		t.Fatalf("reboot: %d %#v; calls=%d", status, out, calls)
	}
	for _, endpoint := range []string{"reboot", "save", "capture", "backup"} {
		status, _ = f.call(endpoint, object{"token": token}, "POST", "")
		if status != 409 || calls != 1 {
			t.Fatalf("old session allowed %s: %d", endpoint, status)
		}
	}
}

func TestRebootRejectsInvalidRequestsBeforeContactingDevice(t *testing.T) {
	f := newFixture(t, 30)
	token := f.connect(t)
	f.a.device = func(context.Context, string, string, any) (object, error) {
		t.Fatal("invalid request reached device")
		return nil, nil
	}
	for _, input := range []struct {
		token, method, origin string
		status                int
	}{
		{"", "POST", "", 409},
		{"expired", "POST", "", 409},
		{token, "GET", "", 404},
		{token, "POST", "https://example.com", 403},
	} {
		status, _ := f.call("reboot", object{"token": input.token}, input.method, input.origin)
		if status != input.status || f.a.session == nil {
			t.Fatalf("invalid request: got %d, want %d", status, input.status)
		}
	}
	f.a.deviceMu.Lock()
	status, _ := f.call("reboot", object{"token": token}, "POST", "")
	f.a.deviceMu.Unlock()
	if status != 409 || f.a.session == nil {
		t.Fatalf("busy device: %d", status)
	}
}

func TestUnconfirmedRebootInvalidatesSession(t *testing.T) {
	for _, name := range []string{"connection-closed", "rejected", "missing-confirmation"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, 30)
			token := f.connect(t)
			f.a.device = func(context.Context, string, string, any) (object, error) {
				switch name {
				case "connection-closed":
					return nil, io.EOF
				case "rejected":
					return object{"success": false}, nil
				default:
					return object{}, nil
				}
			}
			status, out := f.call("reboot", object{"token": token}, "POST", "")
			if status != 502 || out["uncertain"] != true || f.a.session != nil {
				t.Fatalf("unconfirmed reboot: %d %#v", status, out)
			}
		})
	}
}
