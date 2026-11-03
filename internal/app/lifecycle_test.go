package app

import (
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestTrayExitDrainsInFlightRequest(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = w.Write([]byte("saved"))
	})}
	defer server.Close()
	result := make(chan error, 1)
	url := "http://" + listener.Addr().String()
	desktop := func(actual string, open bool, shutdown func(), stopped <-chan struct{}) error {
		if actual != url || open {
			t.Error("desktop received wrong startup options")
		}
		client := &http.Client{Timeout: 3 * time.Second}
		go func() {
			res, err := client.Get(url)
			if err == nil {
				defer res.Body.Close()
				var data []byte
				data, err = io.ReadAll(res.Body)
				if string(data) != "saved" {
					err = errors.New("request interrupted")
				}
			}
			result <- err
		}()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			return errors.New("request did not start")
		}
		go shutdown()
		early := false
		select {
		case <-stopped:
			early = true
		case <-time.After(100 * time.Millisecond):
		}
		close(release)
		if early {
			return errors.New("tray stopped before save finished")
		}
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			return errors.New("shutdown did not finish")
		}
		return <-result
	}
	if err := serveWithDesktop(server, listener, url, false, desktop); err != nil {
		t.Fatal(err)
	}
	replacement, err := net.Listen("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal("port was not released:", err)
	}
	replacement.Close()
}

func TestTrayStartupFailureStopsServer(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	expected := errors.New("tray unavailable")
	desktop := func(string, bool, func(), <-chan struct{}) error { return expected }
	if err := serveWithDesktop(&http.Server{}, listener, "http://"+listener.Addr().String(), false, desktop); !errors.Is(err, expected) {
		t.Fatalf("startup error: %v", err)
	}
	replacement, err := net.Listen("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal("orphaned server:", err)
	}
	replacement.Close()
}
