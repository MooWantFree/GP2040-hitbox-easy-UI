package app

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Reserve neighboring ports so the fallback tests do not rely on fixed free ports.
func reserveLocalPorts(t *testing.T, count int) (int, []net.Listener) {
	t.Helper()
	for attempt := 0; attempt < 40; attempt++ {
		first, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		start := first.Addr().(*net.TCPAddr).Port
		listeners := []net.Listener{first}
		for next := start + 1; next < start+count && next <= 65535; next++ {
			listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", next))
			if err != nil {
				break
			}
			listeners = append(listeners, listener)
		}
		if len(listeners) == count {
			t.Cleanup(func() {
				for _, listener := range listeners {
					listener.Close()
				}
			})
			return start, listeners
		}
		for _, listener := range listeners {
			listener.Close()
		}
	}
	t.Fatal("could not reserve adjacent test ports")
	return 0, nil
}

func servePort(t *testing.T, listener net.Listener, handler http.Handler) {
	t.Helper()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	t.Cleanup(func() { server.Close() })
	go server.Serve(listener)
}

func foreignApp(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"app":"another-app","protocol":2}`)
}

func TestFreePortIsReservedUntilServerStarts(t *testing.T) {
	start, ports := reserveLocalPorts(t, 1)
	ports[0].Close()
	listener, url, err := listenLocal(start)
	if err != nil || listener == nil {
		t.Fatalf("listen: %s %v", url, err)
	}
	defer listener.Close()
	if url != fmt.Sprintf("http://127.0.0.1:%d", start) {
		t.Fatal("wrong URL", url)
	}
	other, err := net.Listen("tcp4", listener.Addr().String())
	if err == nil {
		other.Close()
		t.Fatal("port released before startup")
	}
}

func TestForeignPortsAdvanceAndDesktopUsesActualURL(t *testing.T) {
	start, ports := reserveLocalPorts(t, 3)
	servePort(t, ports[0], http.HandlerFunc(foreignApp))
	servePort(t, ports[1], http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "not this app", 404) }))
	ports[2].Close()
	want := fmt.Sprintf("http://127.0.0.1:%d", start+2)
	called := false
	desktop := func(url string, open bool, _ func(), _ <-chan struct{}) error {
		called = true
		if url != want || !open {
			t.Errorf("desktop URL/options: %s %t", url, open)
		}
		client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
		if !existingInstance(client, url) {
			t.Error("new server is not serving on selected port")
		}
		return nil
	}
	if err := runManager(start, t.TempDir(), true, func(string) error { t.Error("unexpected reuse"); return nil }, desktop); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("desktop did not start")
	}
}

func TestExistingInstanceReusedAfterForeignPort(t *testing.T) {
	for _, offset := range []int{0, 1} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			start, ports := reserveLocalPorts(t, offset+1)
			if offset == 1 {
				servePort(t, ports[0], http.HandlerFunc(foreignApp))
			}
			servePort(t, ports[offset], newApp(t.TempDir(), nil))
			dataDir := filepath.Join(t.TempDir(), "must-not-be-created")
			want := fmt.Sprintf("http://127.0.0.1:%d", start+offset)
			opened := []string{}
			desktop := func(string, bool, func(), <-chan struct{}) error { t.Error("created a duplicate desktop"); return nil }
			open := func(url string) error { opened = append(opened, url); return nil }
			if err := runManager(start, dataDir, true, open, desktop); err != nil {
				t.Fatal(err)
			}
			if len(opened) != 1 || opened[0] != want {
				t.Fatal("did not open existing webpage", opened)
			}
			if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("reuse touched data directory")
			}
			if err := runManager(start, dataDir, false, open, desktop); err != nil {
				t.Fatal(err)
			}
			if len(opened) != 1 {
				t.Fatal("ignored explicit no-browser option")
			}
			expected := errors.New("browser unavailable")
			if err := runManager(start, dataDir, true, func(string) error { return expected }, desktop); !errors.Is(err, expected) {
				t.Fatal("lost browser error")
			}
		})
	}
}

func TestForeignHealthCannotImpersonateByRedirectOrInvalidResponse(t *testing.T) {
	for _, response := range []string{"redirect", "html", "wrong-protocol", "oversized", "trailing-json"} {
		t.Run(response, func(t *testing.T) {
			start, ports := reserveLocalPorts(t, 3)
			var redirectRequests atomic.Int32
			servePort(t, ports[2], http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				redirectRequests.Add(1)
				_, _ = io.WriteString(w, `{"app":"hitbox-map","protocol":2}`)
			}))
			servePort(t, ports[0], http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch response {
				case "redirect":
					http.Redirect(w, r, fmt.Sprintf("http://127.0.0.1:%d/api/health", start+2), http.StatusFound)
				case "html":
					_, _ = io.WriteString(w, "<html>Hitbox Map</html>")
				case "wrong-protocol":
					_, _ = io.WriteString(w, `{"app":"hitbox-map","protocol":999}`)
				case "oversized":
					_, _ = io.WriteString(w, `{"app":"hitbox-map","protocol":2,"extra":"`+strings.Repeat("x", 4096)+`"}`)
				case "trailing-json":
					_, _ = io.WriteString(w, `{"app":"hitbox-map","protocol":2}{}`)
				}
			}))
			ports[1].Close()
			listener, url, err := listenLocal(start)
			if listener != nil {
				defer listener.Close()
			}
			if err != nil || listener == nil || url != fmt.Sprintf("http://127.0.0.1:%d", start+1) {
				t.Fatalf("wrong selection: %s %v", url, err)
			}
			if redirectRequests.Load() != 0 {
				t.Fatal("followed foreign redirect")
			}
		})
	}
}

func TestPortProbeWaitsForStartingInstanceAndTimesOutOnSilentService(t *testing.T) {
	t.Run("starting", func(t *testing.T) {
		start, ports := reserveLocalPorts(t, 1)
		server := &http.Server{Handler: newApp(t.TempDir(), nil), ReadHeaderTimeout: time.Second}
		t.Cleanup(func() { server.Close() })
		go func() { time.Sleep(100 * time.Millisecond); server.Serve(ports[0]) }()
		listener, url, err := listenLocal(start)
		if listener != nil {
			listener.Close()
		}
		if err != nil || listener != nil || url != fmt.Sprintf("http://127.0.0.1:%d", start) {
			t.Fatalf("duplicated starting instance: %s %v", url, err)
		}
	})
	t.Run("silent", func(t *testing.T) {
		start, ports := reserveLocalPorts(t, 2)
		// Leave the first listener open without serving HTTP.
		ports[1].Close()
		began := time.Now()
		listener, url, err := listenLocal(start)
		if listener != nil {
			defer listener.Close()
		}
		if err != nil || listener == nil || url != fmt.Sprintf("http://127.0.0.1:%d", start+1) {
			t.Fatalf("silent service blocked fallback: %s %v", url, err)
		}
		if time.Since(began) > 3*time.Second {
			t.Fatal("health probe did not time out")
		}
	})
}

func TestPortRangeBoundaries(t *testing.T) {
	for _, port := range []int{-1, 0, 65536} {
		listener, _, err := listenLocal(port)
		if listener != nil {
			listener.Close()
		}
		if err == nil {
			t.Fatalf("accepted invalid port %d", port)
		}
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:65535")
	if err != nil {
		t.Skip("last port unavailable for isolated boundary test")
	}
	servePort(t, listener, http.HandlerFunc(foreignApp))
	got, url, err := listenLocal(65535)
	if got != nil {
		got.Close()
	}
	if err == nil || got != nil || url != "" {
		t.Fatal("port range wrapped or exhaustion was hidden")
	}
}
