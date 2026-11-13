// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

type fakePhotos struct {
	asked int
	none  bool
	pic   []byte
}

func (f *fakePhotos) ConsolePhoto(ctx context.Context, console, name string) (library.LogoCandidate, error) {
	f.asked++
	if f.none {
		return library.LogoCandidate{}, library.ErrNoPhoto
	}
	return library.LogoCandidate{SourceID: "SNES.jpg", Credit: "Evan Amos", License: "Public domain", PageURL: "https://commons.wikimedia.org/wiki/File:SNES.jpg"}, nil
}

func (f *fakePhotos) Fetch(ctx context.Context, c library.LogoCandidate) ([]byte, error) {
	return f.pic, nil
}

func tinyPNG(t *testing.T) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestConsolePhotosAreKeptAndAskedForOnce(t *testing.T) {
	src := &fakePhotos{pic: tinyPNG(t)}
	allowed := true
	p := &consolePhotos{dir: t.TempDir(), source: src, allowed: func(context.Context) bool { return allowed }}
	f, ok := p.get(context.Background(), "snes")
	if !ok || f.Credit != "Evan Amos" || f.License != "Public domain" {
		t.Fatalf("got %+v %v", f, ok)
	}
	// Kept: shown with lookups off, and Commons isn't asked again.
	allowed = false
	if _, ok := p.get(context.Background(), "snes"); !ok || src.asked != 1 {
		t.Errorf("the kept picture: ok %v, asked %d", ok, src.asked)
	}
}

func TestConsolePhotosRespectLookupsOffAndRememberNone(t *testing.T) {
	src := &fakePhotos{pic: tinyPNG(t)}
	allowed := false
	p := &consolePhotos{dir: t.TempDir(), source: src, allowed: func(context.Context) bool { return allowed }}
	if _, ok := p.get(context.Background(), "nes"); ok || src.asked != 0 {
		t.Errorf("asked Wikimedia with lookups off: ok %v, asked %d", ok, src.asked)
	}
	allowed, src.none = true, true
	p.get(context.Background(), "nes")
	p.get(context.Background(), "nes")
	if src.asked != 1 {
		t.Errorf("a console with no free picture was asked about %d times", src.asked)
	}
	// Something that isn't a picture is never kept.
	src2 := &fakePhotos{pic: []byte("<html>")}
	p2 := &consolePhotos{dir: t.TempDir(), source: src2, allowed: func(context.Context) bool { return true }}
	if _, ok := p2.get(context.Background(), "gba"); ok {
		t.Error("kept something that isn't a picture")
	}
	if consoleIDRe.MatchString("../x") || !consoleIDRe.MatchString("custom-zeta_2") {
		t.Error("console ids")
	}
}
