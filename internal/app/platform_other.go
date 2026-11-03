//go:build !windows

package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

func portUnavailable(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }

func systemLanguage() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(name); value != "" {
			return strings.Split(strings.Split(value, ".")[0], "@")[0]
		}
	}
	return "en"
}

func openBrowser(url string) error {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	}
	return exec.Command(command, url).Run()
}
func showError(message string) { fmt.Fprintln(os.Stderr, message) }

// Non-Windows source builds keep the foreground server and Ctrl+C exit path.
func runDesktop(url string, openOnStart bool, _ func(), stopped <-chan struct{}) error {
	if openOnStart {
		go func() {
			if err := openBrowser(url); err != nil {
				showError(err.Error())
			}
		}()
	}
	<-stopped
	return nil
}
