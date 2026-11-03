package app

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"
)

// Main starts the local server and desktop tray.
func Main() {
	if err := run(); err != nil {
		showError(nativeError(err))
		os.Exit(1)
	}
}

func run() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	nativeCatalog = loadLanguages(localeDirectory())
	nativeLocale = resolveLanguage([]string{systemLanguage()}, nativeCatalog.Languages)
	port := flag.Int("port", defaultPort, "first local port to try")
	dataDir := flag.String("data-dir", filepath.Join(filepath.Dir(executable), "data"), "layout and backup directory")
	noBrowser := flag.Bool("no-browser", false, "do not open a browser")
	flag.Parse()
	return runManager(*port, *dataDir, !*noBrowser, openBrowser, runDesktop)
}

func runManager(port int, dataDir string, openOnStart bool, openURL func(string) error, desktop desktopRunner) error {
	listener, localURL, err := listenLocal(port)
	if err != nil {
		return err
	}
	if listener == nil {
		if openOnStart {
			return openURL(localURL)
		}
		return nil
	}
	defer listener.Close()
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return messageError("error.dataDirectory", err)
	}
	a := newApp(absolute, nil)
	server := &http.Server{Handler: a, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 100 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	fmt.Println("Hitbox Map:", localURL)
	return serveWithDesktop(server, listener, localURL, openOnStart, desktop)
}

type desktopRunner func(string, bool, func(), <-chan struct{}) error

func serveWithDesktop(server *http.Server, listener net.Listener, localURL string, openOnStart bool, desktop desktopRunner) error {
	shutdownDone := make(chan struct{})
	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			defer close(shutdownDone)
			ctx, cancel := context.WithTimeout(context.Background(), 95*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
		})
	}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	go func() {
		select {
		case <-interrupt:
			shutdown()
		case <-shutdownDone:
		}
	}()
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
		shutdown()
	}()
	desktopErr := desktop(localURL, openOnStart, shutdown, shutdownDone)
	shutdown()
	serveErr := <-serveResult
	if desktopErr != nil {
		return desktopErr
	}
	if serveErr == http.ErrServerClosed {
		return nil
	}
	return serveErr
}
