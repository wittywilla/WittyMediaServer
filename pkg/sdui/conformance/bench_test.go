// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package conformance

import (
	"testing"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// What rendering costs apart from the library's own queries: the built-in Home
// with something in every row, for the smallest device and the largest.
//
//	go test ./pkg/sdui/conformance -bench . -benchmem
func benchRender(b *testing.B, in Input) {
	b.ReportAllocs()
	var bytes int
	for b.Loop() {
		doc, err := sdui.Render(sdui.ClonePage(in.Page), in.Context())
		if err != nil {
			b.Fatal(err)
		}
		out, err := sdui.Marshal(doc)
		if err != nil {
			b.Fatal(err)
		}
		bytes = len(out)
	}
	b.ReportMetric(float64(bytes), "bytes/page")
}

func BenchmarkRenderHomeTVHD(b *testing.B) { benchRender(b, home("tv", nil)) }
func BenchmarkRenderHomeDesktop(b *testing.B) {
	benchRender(b, home("desktop", func(in *Input) { in.Request.Viewport = "1440x900" }))
}
func BenchmarkRenderLibraryGridDesktop(b *testing.B) {
	benchRender(b, library("library-movies", "desktop", "1", "movies", "Films", func(in *Input) { in.Request.Viewport = "1440x900" }))
}

func BenchmarkValidateHome(b *testing.B) {
	p := stock("home")
	b.ReportAllocs()
	for b.Loop() {
		if sdui.HasErrors(sdui.ValidatePage(&p)) {
			b.Fatal("invalid")
		}
	}
}

func BenchmarkDiffHome(b *testing.B) {
	in := home("desktop", nil)
	a, _ := sdui.Render(sdui.ClonePage(in.Page), in.Context())
	in.Request.Personal = &sdui.Personal{Order: []string{"nextup", "jump"}, Hidden: []string{"foryou"}}
	c, _ := sdui.Render(sdui.ClonePage(in.Page), in.Context())
	b.ReportAllocs()
	for b.Loop() {
		if len(sdui.Diff(a, c)) == 0 {
			b.Fatal("no patches")
		}
	}
}
