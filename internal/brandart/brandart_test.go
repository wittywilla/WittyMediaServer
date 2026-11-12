// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/brandart/fonts"
)

func TestMain(m *testing.M) {
	if err := UseFonts(fonts.InterBold, fonts.InterRegular); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// Inter's own GPOS kerning is applied: "Wo" sits closer than its plain advances, as in a browser.
func TestInterKerning(t *testing.T) {
	plain := TextWidth([]Run{{"W", Colour{}}}, 100, false) + TextWidth([]Run{{"o", Colour{}}}, 100, false)
	if pair := TextWidth([]Run{{"Wo", Colour{}}}, 100, false); pair >= plain-0.5 {
		t.Errorf("no kerning between W and o: %.2f vs %.2f", pair, plain)
	}
}

// The web client's icons in the repository are what the generator draws now.
func TestCommittedWebIconsAreCurrent(t *testing.T) {
	files, err := NewKit(DefaultConcept).Build("server", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range files.Paths() {
		have, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(have, files[p]) {
			t.Errorf("%s is out of date: run scripts/sync-brand.sh", p)
		}
	}
}

func decode(t *testing.T, name string, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return img
}

// The design in use builds every target, and each picture is the size its platform requires;
// every other concept builds the cheaper targets (the client's set is drawn the same way).
func TestEveryTargetInEveryConcept(t *testing.T) {
	modules := map[string]string{"wittyplay": "games", "wittydvd": "discs"}
	sizes := map[string][2]int{
		"web/public/icon-192.png":                                    {192, 192},
		"web/public/icon-512.png":                                    {512, 512},
		"web/public/apple-touch-icon.png":                            {180, 180},
		"android/app/src/main/res/mipmap-xxxhdpi/ic_launcher.png":    {192, 192},
		"android/app/src/main/res/mipmap-mdpi/ic_launcher_round.png": {48, 48},
		"android/app/src/main/res/drawable-xhdpi/banner.png":         {640, 360},
		"apple/Brand/iOS.xcassets/AppIcon.appiconset/AppIcon.png":    {1024, 1024},
		"apple/Brand/tvOS.xcassets/App Icon & Top Shelf Image.brandassets/App Icon.imagestack/Back.imagestacklayer/Content.imageset/Back@1x.png":                 {400, 240},
		"apple/Brand/tvOS.xcassets/App Icon & Top Shelf Image.brandassets/App Icon.imagestack/Front.imagestacklayer/Content.imageset/Front@2x.png":               {800, 480},
		"apple/Brand/tvOS.xcassets/App Icon & Top Shelf Image.brandassets/App Icon - App Store.imagestack/Middle.imagestacklayer/Content.imageset/Middle@1x.png": {1280, 768},
		"apple/Brand/tvOS.xcassets/App Icon & Top Shelf Image.brandassets/Top Shelf Image.imageset/Top Shelf Image@2x.png":                                       {3840, 1440},
		"apple/Brand/tvOS.xcassets/App Icon & Top Shelf Image.brandassets/Top Shelf Image Wide.imageset/Top Shelf Image Wide@1x.png":                             {2320, 720},
		"desktop/build/background@2x.png": {1080, 800},
		"icons/wittyplay.png":             {128, 128},
		"images/brand/social.png":         {1200, 630},
		"pkg-background.png":              {1240, 836},
	}
	for _, c := range Concepts {
		k := NewKit(c)
		seen := map[string]bool{}
		targets := Targets
		if c != DefaultConcept {
			targets = []string{"server", "player", "website"}
		}
		for _, target := range targets {
			if target == "kit" {
				continue // the same drawings as the other targets; docs/brand is checked by -check
			}
			files, err := k.Build(target, modules)
			if err != nil {
				t.Fatalf("%s %s: %v", c, target, err)
			}
			for _, p := range files.Paths() {
				data := files[p]
				switch {
				case strings.HasSuffix(p, ".png"):
					img := decode(t, p, data)
					if want, ok := sizes[p]; ok {
						seen[p] = true
						if b := img.Bounds(); b.Dx() != want[0] || b.Dy() != want[1] {
							t.Errorf("%s %s is %dx%d, want %dx%d", c, p, b.Dx(), b.Dy(), want[0], want[1])
						}
					}
				case strings.HasSuffix(p, ".json"):
					var v map[string]any
					if err := json.Unmarshal(data, &v); err != nil {
						t.Errorf("%s: %v", p, err)
					}
				case strings.HasSuffix(p, ".svg"), strings.HasSuffix(p, ".xml"):
					if err := xml.Unmarshal(data, new(struct{})); err != nil {
						t.Errorf("%s: not well-formed XML: %v", p, err)
					}
				}
			}
		}
		for p := range sizes {
			if c == DefaultConcept && !seen[p] {
				t.Errorf("%s: no target wrote %s", c, p)
			}
		}
	}
}

// Store and home-screen icons may not be transparent; launcher layers above the back must be.
func TestOpacity(t *testing.T) {
	k := NewKit(DefaultConcept)
	files, _ := k.Build("client", nil)
	opaque := func(p string) bool {
		img := decode(t, p, files[p])
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
					return false
				}
			}
		}
		return true
	}
	for _, p := range []string{
		"apple/Brand/iOS.xcassets/AppIcon.appiconset/AppIcon.png",
		"apple/Brand/iOS.xcassets/AppIcon.appiconset/AppIcon-Dark.png",
		"apple/Brand/tvOS.xcassets/App Icon & Top Shelf Image.brandassets/App Icon.imagestack/Back.imagestacklayer/Content.imageset/Back@1x.png",
		"android/brand/play-icon-512.png",
	} {
		if !opaque(p) {
			t.Errorf("%s has transparent pixels", p)
		}
	}
	if opaque("apple/Brand/tvOS.xcassets/App Icon & Top Shelf Image.brandassets/App Icon.imagestack/Front.imagestacklayer/Content.imageset/Front@1x.png") {
		t.Error("the tvOS front layer must be transparent around the face")
	}
}

// Every mark keeps inside the circle every Android launcher mask shows: 72 of 108 across.
func TestMarksFitTheSafeZone(t *testing.T) {
	for _, c := range Concepts {
		for _, p := range Products {
			m := Design(c, p, HousePalette(), false)
			for _, s := range m.MonoPath().subs {
				for _, q := range s.flatten(8) {
					dx, dy := q.X-Page/2, q.Y-Page/2
					if r := dx*dx + dy*dy; r > (Page*36/108)*(Page*36/108) {
						t.Errorf("%s %s reaches %.0f from the centre, outside the safe zone", c, p, r)
						break
					}
				}
			}
		}
	}
}

// The server and client marks differ: their one-colour silhouettes overlap well under 85%.
func TestServerAndClientDiffer(t *testing.T) {
	for _, c := range Concepts {
		k := NewKit(c)
		a := k.Mono(Server, 64, Hex("#000000"), 1).Image(64, 64)
		b := k.Mono(Client, 64, Hex("#000000"), 1).Image(64, 64)
		var both, either float64
		for i := 3; i < len(a.Pix); i += 4 {
			x, y := float64(a.Pix[i])/255, float64(b.Pix[i])/255
			both += min(x, y)
			either += max(x, y)
		}
		if both/either > 0.85 {
			t.Errorf("%s: server and client silhouettes overlap %.0f%%", c, 100*both/either)
		}
	}
}

// The cuts are holes in the one-colour mark: it covers clearly less than the body alone.
func TestMonoHasHoles(t *testing.T) {
	m := Design(DefaultConcept, Client, HousePalette(), false)
	cover := func(p Path) float64 {
		img := Art{W: Page, H: Page, Layers: []Layer{{p, Fill(Hex("#000000"))}}}.Image(128, 128)
		n := 0.0
		for i := 3; i < len(img.Pix); i += 4 {
			n += float64(img.Pix[i]) / 255
		}
		return n
	}
	if with, without := cover(Cut(m.Body.Path, m.Ink.Path)), cover(m.Body.Path); with > without*0.95 {
		t.Errorf("the cuts don't show in the one-colour mark (%.0f of %.0f)", with, without)
	}
}

func TestICOAndICNS(t *testing.T) {
	k := NewKit(DefaultConcept)
	ico := ICO([]int{16, 32, 256}, func(px int) Art { return k.Tile(Server, px) })
	le := binary.LittleEndian
	if le.Uint16(ico[2:]) != 1 || le.Uint16(ico[4:]) != 3 {
		t.Fatal("not a three-picture icon")
	}
	for i, want := range []int{16, 32, 256} {
		e := ico[6+16*i:]
		img := decode(t, "ico", ico[le.Uint32(e[12:]):][:le.Uint32(e[8:])])
		if img.Bounds().Dx() != want {
			t.Errorf("frame %d is %d wide, want %d", i, img.Bounds().Dx(), want)
		}
	}
	icns := ICNS(func(px int) Art { return k.MacTile(Server, px) })
	if string(icns[:4]) != "icns" || int(binary.BigEndian.Uint32(icns[4:])) != len(icns) {
		t.Fatal("bad icns header")
	}
	n := 0
	for at := 8; at < len(icns); n++ {
		at += int(binary.BigEndian.Uint32(icns[at+4:]))
	}
	if n != 11 {
		t.Errorf("icns has %d pictures, want 11", n)
	}
}

func TestBMPHeader(t *testing.T) {
	b := BMP(image.NewRGBA(image.Rect(0, 0, 493, 58)))
	if string(b[:2]) != "BM" || binary.LittleEndian.Uint32(b[2:]) != uint32(len(b)) {
		t.Fatal("bad bitmap header")
	}
}

// The generated Kotlin and Swift hold every drawing.
func TestCodegen(t *testing.T) {
	k := NewKit(DefaultConcept)
	kt := string(KotlinFile("x.y", []string{"Server"}, []Art{k.Tile(Server, 64)}, 64))
	if !strings.Contains(kt, "val Server: ImageVector") || !strings.Contains(kt, "addPathNodes(\"M") {
		t.Error("Kotlin is missing its vector")
	}
	sw := string(SwiftFile([]string{"server"}, []Art{k.Tile(Server, 64)}))
	if !strings.Contains(sw, "static func server(") || !strings.Contains(sw, "p.addCurve(") {
		t.Error("Swift is missing its drawing")
	}
}

func TestPathReverseKeepsShape(t *testing.T) {
	c := Circle(0, 0, 10)
	if a, b := c.subs[0].area(), c.subs[0].reversed().area(); a+b > 1e-6 || a == 0 {
		t.Errorf("reversing a circle should flip its area: %v %v", a, b)
	}
}
