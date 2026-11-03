//go:build windows

package app

import (
	"errors"
	"syscall"
	"unsafe"
)

func portUnavailable(err error) bool {
	// Winsock also reports WSAEACCES when another socket has exclusive use.
	return errors.Is(err, syscall.Errno(10048)) || errors.Is(err, syscall.WSAEACCES)
}

func systemLanguage() string {
	var name [85]uint16
	result, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName").Call(uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	if result == 0 {
		return "en"
	}
	return syscall.UTF16ToString(name[:])
}

func openBrowser(url string) error {
	verb, _ := syscall.UTF16PtrFromString("open")
	file, err := syscall.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	result, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, 1)
	if result <= 32 {
		return messageError("error.browser", result)
	}
	return nil
}

func showError(message string) {
	text, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString("Hitbox Map")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
