package wittyphotos

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/gif" // registered for DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
)

// Dimensions reads a picture's size from its header, without decoding it: JPEG,
// PNG, GIF, WebP and BMP. It returns 0, 0 for anything else.
func Dimensions(path string) (w, h int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	head := make([]byte, 64)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if w, h = webpSize(head); w > 0 {
		return w, h
	}
	if w, h = bmpSize(head); w > 0 {
		return w, h
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, 0
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func webpSize(b []byte) (w, h int) {
	if len(b) < 30 || !bytes.HasPrefix(b, []byte("RIFF")) || string(b[8:12]) != "WEBP" {
		return 0, 0
	}
	switch string(b[12:16]) {
	case "VP8 ":
		if b[23] == 0x9D && b[24] == 0x01 && b[25] == 0x2A {
			return int(binary.LittleEndian.Uint16(b[26:]) & 0x3FFF), int(binary.LittleEndian.Uint16(b[28:]) & 0x3FFF)
		}
	case "VP8L":
		if b[20] == 0x2F {
			v := binary.LittleEndian.Uint32(b[21:])
			return int(v&0x3FFF) + 1, int((v>>14)&0x3FFF) + 1
		}
	case "VP8X":
		return int(b[24]) | int(b[25])<<8 | int(b[26])<<16 + 1, int(b[27]) | int(b[28])<<8 | int(b[29])<<16 + 1
	}
	return 0, 0
}

func bmpSize(b []byte) (w, h int) {
	if len(b) < 26 || b[0] != 'B' || b[1] != 'M' {
		return 0, 0
	}
	if int32(binary.LittleEndian.Uint32(b[14:])) < 40 { // old OS/2 header
		return 0, 0
	}
	w, h = int(int32(binary.LittleEndian.Uint32(b[18:]))), int(int32(binary.LittleEndian.Uint32(b[22:])))
	if h < 0 {
		h = -h // stored top-down
	}
	if w <= 0 {
		return 0, 0
	}
	return w, h
}
