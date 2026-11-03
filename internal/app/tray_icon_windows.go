//go:build windows

package app

import (
	"encoding/binary"
	"runtime"
	"unsafe"
)

// A 32-bit icon resource, generated in memory so the exe has no external assets.
func createTrayIcon() (uintptr, error) {
	const size = 32
	pixels := make([]byte, 40+size*size*4+size*4)
	binary.LittleEndian.PutUint32(pixels[0:4], 40)
	binary.LittleEndian.PutUint32(pixels[4:8], size)
	binary.LittleEndian.PutUint32(pixels[8:12], size*2)
	binary.LittleEndian.PutUint16(pixels[12:14], 1)
	binary.LittleEndian.PutUint16(pixels[14:16], 32)
	binary.LittleEndian.PutUint32(pixels[20:24], size*size*4)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var red, green, blue, alpha int
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					px, py := float64(x)+(float64(sx)+.5)/4, float64(y)+(float64(sy)+.5)/4
					cx, cy := max(9.0, min(23.0, px)), max(12.0, min(20.0, py))
					if (px-cx)*(px-cx)+(py-cy)*(py-cy) > 49 {
						continue
					}
					r, g, b := 24, 24, 27
					cross := (px >= 6 && px <= 14 && py >= 14 && py <= 17) || (px >= 9 && px <= 11 && py >= 11 && py <= 20)
					dots := (px-21)*(px-21)+(py-12.5)*(py-12.5) <= 4 || (px-25)*(px-25)+(py-17)*(py-17) <= 4
					if cross || dots {
						r, g, b = 250, 250, 250
					}
					red += r
					green += g
					blue += b
					alpha += 255
				}
			}
			offset := 40 + ((size-1-y)*size+x)*4
			pixels[offset] = byte(blue / 16)
			pixels[offset+1] = byte(green / 16)
			pixels[offset+2] = byte(red / 16)
			pixels[offset+3] = byte(alpha / 16)
			if alpha == 0 {
				pixels[40+size*size*4+(size-1-y)*4+x/8] |= 1 << uint(7-x%8)
			}
		}
	}
	icon, _, err := trayUser32.NewProc("CreateIconFromResourceEx").Call(uintptr(unsafe.Pointer(&pixels[0])), uintptr(len(pixels)), 1, 0x00030000, size, size, 0)
	runtime.KeepAlive(pixels)
	if icon == 0 {
		return 0, winError(nativeText("tray.icon"), err)
	}
	return icon, nil
}
