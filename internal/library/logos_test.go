package library

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

func logoStore(t *testing.T) *Store {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return &Store{DB: d}
}

func pngOf(t *testing.T, m image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A logo on a clear background keeps its shape, painted one colour.
func TestLogoVariantsFromTransparency(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := 5; y < 15; y++ {
		for x := 10; x < 30; x++ {
			m.SetNRGBA(x, y, color.NRGBA{200, 20, 20, 255})
		}
	}
	set, err := makeLogoVariants(pngOf(t, m))
	if err != nil {
		t.Fatal(err)
	}
	if set.opaque {
		t.Fatal("a transparent picture was treated as opaque")
	}
	w, err := png.Decode(bytes.NewReader(set.white))
	if err != nil {
		t.Fatal(err)
	}
	if b := w.Bounds(); b.Dx() != 20 || b.Dy() != 10 {
		t.Errorf("the empty margin wasn't trimmed: %v", b)
	}
	if r, g, bl, a := w.At(5, 5).RGBA(); a == 0 || r>>8 != 255 || g>>8 != 255 || bl>>8 != 255 {
		t.Errorf("the white version isn't white: %d %d %d %d", r, g, bl, a)
	}
	d, _ := png.Decode(bytes.NewReader(set.dark))
	if r, _, _, a := d.At(5, 5).RGBA(); a == 0 || r>>8 > 40 {
		t.Errorf("the dark version isn't dark: %d %d", r, a)
	}
}

// Black letters on a white rectangle are cut out of the rectangle.
func TestLogoVariantsFromPlainBackground(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			c := color.NRGBA{255, 255, 255, 255}
			if x >= 10 && x < 30 && y >= 5 && y < 15 {
				c = color.NRGBA{0, 0, 0, 255}
			}
			m.SetNRGBA(x, y, c)
		}
	}
	set, err := makeLogoVariants(pngOf(t, m))
	if err != nil {
		t.Fatal(err)
	}
	if set.opaque {
		t.Fatal("a plain background should come off")
	}
	w, _ := png.Decode(bytes.NewReader(set.white))
	if b := w.Bounds(); b.Dx() != 20 || b.Dy() != 10 {
		t.Errorf("wanted only the letters, got %v", b)
	}
}

// A photo has no background to take off: it stays whole.
func TestLogoVariantsOpaquePhoto(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			m.SetNRGBA(x, y, color.NRGBA{uint8(x * 6), uint8(y * 6), uint8(x * y % 255), 255})
		}
	}
	set, err := makeLogoVariants(pngOf(t, m))
	if err != nil {
		t.Fatal(err)
	}
	if !set.opaque || !bytes.Equal(set.white, set.original) {
		t.Fatal("a picture that can't be cut out should stay as it is")
	}
}

// Whatever comes in, only pixels come out; anything that isn't a picture is refused.
func TestLogoRefusesWhatIsNotAPicture(t *testing.T) {
	for name, b := range map[string][]byte{
		"svg":   []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"html":  []byte(`<html><script>alert(1)</script></html>`),
		"empty": {},
		"huge":  bytes.Repeat([]byte{0}, maxLogoBytes+1),
	} {
		if _, err := makeLogoVariants(b); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// Trailing data on a real picture doesn't survive being written again.
	m := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	m.SetNRGBA(3, 3, color.NRGBA{9, 9, 9, 255})
	evil := append(pngOf(t, m), []byte("<script>alert(1)</script>")...)
	set, err := makeLogoVariants(evil)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range [][]byte{set.original, set.white, set.dark} {
		if bytes.Contains(out, []byte("script")) {
			t.Fatal("trailing data survived")
		}
	}
}

func TestFreeLogoLicense(t *testing.T) {
	for l, want := range map[string]bool{
		"Public domain": true, "PD-textlogo": true, "CC0": true, "CC BY 4.0": true, "CC BY-SA 3.0": true, "CC-BY-SA-4.0": true,
		"CC BY-NC 4.0": false, "CC BY-ND 4.0": false, "CC BY-NC-SA 4.0": false, "Fair use": false, "Non-free logo": false, "": false, "All rights reserved": false,
	} {
		if FreeLogoLicense(l) != want {
			t.Errorf("licence %q: want %v", l, want)
		}
	}
}

func TestBrandKeyTreatsSpellingsAlike(t *testing.T) {
	if a, b := BrandKey("Warner Bros."), BrandKey("warner  bros"); a != b {
		t.Errorf("%q and %q", a, b)
	}
	if a, b := BrandKey("Acme, Inc."), BrandKey("ACME"); a != b {
		t.Errorf("%q and %q", a, b)
	}
	if BrandKey("  --  ") != "" {
		t.Error("punctuation alone isn't a name")
	}
}

// fakeWikimedia answers as Wikidata and Commons do.
func fakeWikimedia(t *testing.T, license string) (*WikimediaLogos, *int) {
	t.Helper()
	calls := 0
	m := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := 5; y < 15; y++ {
		for x := 10; x < 30; x++ {
			m.SetNRGBA(x, y, color.NRGBA{10, 10, 10, 255})
		}
	}
	pic := pngOf(t, m)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/pic.png":
			w.Write(pic)
		case q.Get("action") == "wbsearchentities":
			json.NewEncoder(w).Encode(map[string]any{"search": []map[string]string{{"id": "Q1"}, {"id": "Q2"}}})
		case q.Get("action") == "wbgetentities":
			json.NewEncoder(w).Encode(map[string]any{"entities": map[string]any{
				"Q1": map[string]any{"labels": map[string]any{"en": map[string]string{"value": "Acme Studio"}}, "descriptions": map[string]any{"en": map[string]string{"value": "American film studio"}},
					"claims": map[string]any{"P154": []map[string]any{{"rank": "normal", "mainsnak": map[string]any{"datavalue": map[string]any{"value": "Acme logo.svg"}}}}}},
				// A film with the same name: the right word is missing from what it is.
				"Q2": map[string]any{"labels": map[string]any{"en": map[string]string{"value": "Acme Studio"}}, "descriptions": map[string]any{"en": map[string]string{"value": "1999 album"}},
					"claims": map[string]any{"P154": []map[string]any{{"rank": "normal", "mainsnak": map[string]any{"datavalue": map[string]any{"value": "Album cover.png"}}}}}},
			}})
		case q.Get("action") == "query":
			json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": map[string]any{"1": map[string]any{"title": "File:Acme logo.svg", "imageinfo": []map[string]any{{
				"thumburl": "https://upload.wikimedia.org/acme.png", "descriptionurl": "https://commons.wikimedia.org/wiki/File:Acme_logo.svg",
				"extmetadata": map[string]any{"LicenseShortName": map[string]string{"value": license}, "Artist": map[string]string{"value": "<a href='x'>Some Author</a>"}},
			}}}}}})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &WikimediaLogos{Client: &http.Client{Transport: rewriteTo{srv.URL}}, WikidataAPI: srv.URL + "/wd", CommonsAPI: srv.URL + "/c"}, &calls
}

// rewriteTo sends a picture's address to the fake server.
type rewriteTo struct{ base string }

func (rt rewriteTo) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.Contains(r.URL.Host, "wikimedia") {
		r2 := r.Clone(r.Context())
		u, _ := r.URL.Parse(rt.base + "/pic.png")
		r2.URL = u
		r2.Host = ""
		return http.DefaultTransport.RoundTrip(r2)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func TestLogoServiceAsksNobodyWithoutConsent(t *testing.T) {
	s := logoStore(t)
	src, calls := fakeWikimedia(t, "PD-textlogo")
	l := &Logos{Store: s, Dir: t.TempDir(), Source: src, Gap: 1}
	ctx := context.Background()
	got, err := l.LookupLogos(ctx, []LogoRef{{LogoStudio, "Acme Studio"}}, true)
	if err != nil || len(got) != 1 || got[0].Has {
		t.Fatalf("%+v %v", got, err)
	}
	l.work(ctx) // what Kick runs
	if *calls != 0 {
		t.Fatalf("%d requests before anyone agreed", *calls)
	}
	if _, err := l.Candidates(ctx, &got[0], ""); err != ErrLogosNotAgreed {
		t.Fatalf("candidates without consent: %v", err)
	}
}

func TestLogoServiceDownloadsAfterConsentAndCredits(t *testing.T) {
	s := logoStore(t)
	src, _ := fakeWikimedia(t, "PD-textlogo")
	dir := t.TempDir()
	l := &Logos{Store: s, Dir: dir, Source: src, Gap: 1}
	ctx := context.Background()
	if _, err := s.EnsureLogo(ctx, LogoStudio, "Acme Studio"); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.SetSetting(ctx, "logoConsent", LogoConsentVersion); err != nil {
		t.Fatal(err)
	}
	l.work(ctx)
	g, err := s.LogoFor(ctx, LogoStudio, "acme studio")
	if err != nil || g == nil || !g.Has {
		t.Fatalf("no logo: %+v %v", g, err)
	}
	if g.License != "PD-textlogo" || g.Credit != "Some Author" || !strings.HasPrefix(g.SourceURL, "https://commons") {
		t.Errorf("credit: %+v", g)
	}
	for _, v := range []string{"white", "dark", "original"} {
		if _, err := os.Stat(l.ImagePath(g, v)); err != nil {
			t.Errorf("%s: %v", v, err)
		}
	}
	// Declining and asking to remove them takes them away again.
	if err := l.SetLogoConsent(ctx, false, true); err != nil {
		t.Fatal(err)
	}
	after, _ := s.LogoFor(ctx, LogoStudio, "Acme Studio")
	if after.Has {
		t.Error("still has a logo after declining")
	}
	if _, err := os.Stat(l.ImagePath(g, "white")); err == nil {
		t.Error("the file is still there")
	}
}

// A file with a licence that doesn't allow this is never taken.
func TestLogoServiceSkipsUnfreeFiles(t *testing.T) {
	s := logoStore(t)
	src, _ := fakeWikimedia(t, "CC BY-NC 4.0")
	l := &Logos{Store: s, Dir: t.TempDir(), Source: src, Gap: 1}
	ctx := context.Background()
	s.EnsureLogo(ctx, LogoStudio, "Acme Studio")
	s.DB.SetSetting(ctx, "logoConsent", LogoConsentVersion)
	l.work(ctx)
	g, _ := s.LogoFor(ctx, LogoStudio, "Acme Studio")
	if g.Has || g.Status != LogoNone {
		t.Fatalf("%+v", g)
	}
}

// The owner's own logo needs no consent and survives declining.
func TestOwnLogoStaysAfterDeclining(t *testing.T) {
	s := logoStore(t)
	l := &Logos{Store: s, Dir: t.TempDir(), Gap: 1}
	ctx := context.Background()
	g, _ := s.EnsureLogo(ctx, LogoNetwork, "Local Access")
	m := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	m.SetNRGBA(3, 3, color.NRGBA{9, 9, 9, 255})
	got, err := l.Upload(ctx, g, pngOf(t, m))
	if err != nil || !got.Has || got.Source != "you" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := l.SetLogoConsent(ctx, false, true); err != nil {
		t.Fatal(err)
	}
	after, _ := s.LogoFor(ctx, LogoNetwork, "Local Access")
	if !after.Has {
		t.Error("the owner's own logo was removed")
	}
}
