// Generates the executable icon using the same controller artwork as the tray.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

func drawIcon(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const samples = 4
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			red, green, blue, covered := 0, 0, 0, 0
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					px := (float64(x) + (float64(sx)+.5)/samples) * 32 / float64(size)
					py := (float64(y) + (float64(sy)+.5)/samples) * 32 / float64(size)
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
					covered++
				}
			}
			if covered != 0 {
				img.SetNRGBA(x, y, color.NRGBA{R: byte(red / covered), G: byte(green / covered), B: byte(blue / covered), A: byte(255 * covered / (samples * samples))})
			}
		}
	}
	return img
}

func generate(output, preview string) error {
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	images := make([][]byte, len(sizes))
	for i, size := range sizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, drawIcon(size)); err != nil {
			return err
		}
		images[i] = buf.Bytes()
	}
	// ICONDIR followed by ICONDIRENTRY records and their PNG image payloads.
	header := make([]byte, 6+16*len(sizes))
	binary.LittleEndian.PutUint16(header[2:4], 1)
	binary.LittleEndian.PutUint16(header[4:6], uint16(len(sizes)))
	offset := len(header)
	for i, size := range sizes {
		entry := header[6+16*i : 6+16*(i+1)]
		entry[0], entry[1] = byte(size), byte(size) // 0 represents 256 pixels.
		binary.LittleEndian.PutUint16(entry[4:6], 1)
		binary.LittleEndian.PutUint16(entry[6:8], 32)
		binary.LittleEndian.PutUint32(entry[8:12], uint32(len(images[i])))
		binary.LittleEndian.PutUint32(entry[12:16], uint32(offset))
		offset += len(images[i])
	}
	data := header
	for _, img := range images {
		data = append(data, img...)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(output, data, 0644); err != nil {
		return err
	}
	if preview != "" {
		return os.WriteFile(preview, images[len(images)-1], 0644)
	}
	return nil
}

func main() {
	output := flag.String("out", "assets/HitboxMap.ico", "output icon path")
	preview := flag.String("preview", "", "optional PNG preview path")
	flag.Parse()
	if err := generate(*output, *preview); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
