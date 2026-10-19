// Command generate-icons exports native icon assets from the shared SVG.
// It uses MyGO's software renderer without opening a window or fetching tools.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"hash/crc32"
	"image/png"
	"os"
	"path/filepath"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/assets"
)

func main() {
	output := flag.String("output", "assets", "output directory for generated PNG and ICO resources")
	flag.Parse()
	if err := generate(*output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(output string) error {
	logo, err := nativeui.ParseSVG(assets.LogoSVG())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	sizes := []int{16, 24, 32, 48, 64, 128, 256, 512, 1024}
	icons := make(map[int][]byte, len(sizes))
	theme := nativeui.LightTheme()
	theme.Background = nativeui.Color{}
	for _, size := range sizes {
		img := nativeui.Render(func(c *nativeui.Context) {
			c.SetTheme(theme)
			nativeui.Image(c, logo).Size(float32(size), float32(size)).Fit(nativeui.Contain)
		}, size, size, 1)
		if img.RGBAAt(0, 0).A != 0 {
			return fmt.Errorf("%dpx icon did not preserve transparent corners", size)
		}
		var raw bytes.Buffer
		if err := png.Encode(&raw, img); err != nil {
			return err
		}
		encoded := withProvenance(raw.Bytes())
		icons[size] = encoded
		if size != 24 && size != 48 {
			name := filepath.Join(output, fmt.Sprintf("proxy-start-%d.png", size))
			if err := os.WriteFile(name, encoded, 0o644); err != nil {
				return err
			}
		}
	}
	var ico bytes.Buffer
	iconSizes := sizes[:7]
	_ = binary.Write(&ico, binary.LittleEndian, [3]uint16{0, 1, uint16(len(iconSizes))})
	offset := uint32(6 + 16*len(iconSizes))
	for _, size := range iconSizes {
		ico.Write([]byte{byte(size % 256), byte(size % 256), 0, 0})
		_ = binary.Write(&ico, binary.LittleEndian, [2]uint16{1, 32})
		_ = binary.Write(&ico, binary.LittleEndian, uint32(len(icons[size])))
		_ = binary.Write(&ico, binary.LittleEndian, offset)
		offset += uint32(len(icons[size]))
	}
	for _, size := range iconSizes {
		ico.Write(icons[size])
	}
	return os.WriteFile(filepath.Join(output, "proxy-start.ico"), ico.Bytes(), 0o644)
}

// Keep the origin in each generated PNG, including the PNG entries in the ICO.
// png.Encode always ends with a twelve-byte IEND chunk.
func withProvenance(pngData []byte) []byte {
	const origin = "Origin: authored assets/proxy-start.svg; geometric P with forward play cutout; rendered locally with MyGO v0.5.5 by cmd/generate-icons."
	text := []byte("impeccable:prompt\x00" + origin)
	var chunk bytes.Buffer
	_ = binary.Write(&chunk, binary.BigEndian, uint32(len(text)))
	chunk.WriteString("tEXt")
	chunk.Write(text)
	_ = binary.Write(&chunk, binary.BigEndian, crc32.ChecksumIEEE(chunk.Bytes()[4:]))
	result := make([]byte, 0, len(pngData)+chunk.Len())
	result = append(result, pngData[:len(pngData)-12]...)
	result = append(result, chunk.Bytes()...)
	return append(result, pngData[len(pngData)-12:]...)
}
