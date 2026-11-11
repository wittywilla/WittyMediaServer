// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command genicon writes the WittyMediaServer icon as PNG/ICO files for packaging (desktop
// entries, web manifest, installers), and the pictures the Windows installer's wizard shows.
//
//	go run ./scripts/genicon out.png [size]
//	go run ./scripts/genicon out.ico
//	go run ./scripts/genicon -bitmaps <folder>   writes banner.bmp and welcome.bmp
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/trayicon"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: genicon <out.png|out.ico> [size] | -bitmaps <folder>")
		os.Exit(2)
	}
	if os.Args[1] == "-bitmaps" && len(os.Args) == 3 {
		check(bitmaps(os.Args[2]))
		return
	}
	out := os.Args[1]
	size := 256
	if len(os.Args) > 2 {
		size, _ = strconv.Atoi(os.Args[2])
	}
	var data []byte
	if strings.EqualFold(filepath.Ext(out), ".ico") {
		data = trayicon.ICO(false)
	} else {
		data = trayicon.PNG(size, false)
	}
	check(os.WriteFile(out, data, 0o644))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// bitmaps draws the two pictures in the sizes Windows Installer's wizard wants: a banner along
// the top of a page, and the first and last pages' panel down the left.
func bitmaps(dir string) error {
	white := image.NewUniform(color.White)
	panel := image.NewUniform(color.RGBA{0x1b, 0x1f, 0x2e, 0xff})

	banner := image.NewRGBA(image.Rect(0, 0, 493, 58))
	draw.Draw(banner, banner.Bounds(), white, image.Point{}, draw.Src)
	if err := drawIcon(banner, 44, 493-44-8, 7); err != nil {
		return err
	}

	welcome := image.NewRGBA(image.Rect(0, 0, 493, 312))
	draw.Draw(welcome, welcome.Bounds(), white, image.Point{}, draw.Src)
	draw.Draw(welcome, image.Rect(0, 0, 164, 312), panel, image.Point{}, draw.Src)
	if err := drawIcon(welcome, 96, (164-96)/2, 40); err != nil {
		return err
	}

	if err := writeBMP(filepath.Join(dir, "banner.bmp"), banner); err != nil {
		return err
	}
	return writeBMP(filepath.Join(dir, "welcome.bmp"), welcome)
}

func drawIcon(dst draw.Image, size, x, y int) error {
	icon, err := png.Decode(bytes.NewReader(trayicon.PNG(size, false)))
	if err != nil {
		return err
	}
	draw.Draw(dst, image.Rect(x, y, x+size, y+size), icon, image.Point{}, draw.Over)
	return nil
}

// writeBMP writes the 24-bit, bottom-up form Windows Installer takes.
func writeBMP(path string, img *image.RGBA) error {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	stride := (w*3 + 3) &^ 3
	var out bytes.Buffer
	size := 54 + stride*h
	out.WriteString("BM")
	le := binary.LittleEndian
	for _, v := range []any{uint32(size), uint32(0), uint32(54), uint32(40), int32(w), int32(h), uint16(1), uint16(24), uint32(0), uint32(stride * h), int32(2835), int32(2835), uint32(0), uint32(0)} {
		if err := binary.Write(&out, le, v); err != nil {
			return err
		}
	}
	row := make([]byte, stride)
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(x, y)
			row[x*3], row[x*3+1], row[x*3+2] = c.B, c.G, c.R
		}
		out.Write(row)
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}
