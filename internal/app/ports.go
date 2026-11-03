package app

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const defaultPort = 29301

// A nil listener means an existing Hitbox Map owns the returned URL.
// Keep a newly acquired listener open so another launch cannot claim its port.
func listenLocal(startPort int) (net.Listener, string, error) {
	if startPort < 1 || startPort > 65535 {
		return nil, "", messageError("error.portRange")
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Timeout:       time.Second,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	for port := startPort; port <= 65535; port++ {
		address := fmt.Sprintf("127.0.0.1:%d", port)
		localURL := "http://" + address
		listener, err := net.Listen("tcp4", address)
		if err == nil {
			return listener, localURL, nil
		}
		if !portUnavailable(err) {
			return nil, "", messageError("error.listen", port, err)
		}
		if existingInstance(client, localURL) {
			return nil, localURL, nil
		}
	}
	return nil, "", messageError("error.noPort", startPort)
}

func existingInstance(client *http.Client, localURL string) bool {
	res, err := client.Get(localURL + "/api/health")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 4097))
	if err != nil || len(data) > 4096 {
		return false
	}
	var health struct {
		App      string `json:"app"`
		Protocol int    `json:"protocol"`
	}
	return decodeJSON(data, &health) == nil && health.App == "hitbox-map" && health.Protocol == 2
}
