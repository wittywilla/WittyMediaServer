// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wittycd "github.com/wittywilla/wittymediaserver/internal/cd"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// epubFile writes an EPUB with these extra files, and adds it to a books library.
func epubFile(t *testing.T, ts *testServer, name string, extra map[string]string) *library.Item {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	files := map[string]string{
		"META-INF/container.xml": `<container><rootfiles><rootfile full-path="OEBPS/book.opf"/></rootfiles></container>`,
		"OEBPS/book.opf":         `<package><manifest><item id="c" href="c.xhtml"/></manifest><spine><itemref idref="c"/></spine></package>`,
		"OEBPS/c.xhtml":          `<html><body>some words</body></html>`,
	}
	for k, v := range extra {
		files[k] = v
	}
	for k, v := range files {
		w, err := zw.Create(k)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(v))
	}
	zw.Close()
	f.Close()
	lib := ts.library(t, "books", dir)
	return ts.item(t, &library.Item{LibraryID: lib, Kind: "book", Path: p, Title: "A Made-up Book", Container: "epub"})
}

// A locked book is refused in plain words at every way in, and none of its files go out. A book
// whose fonts are merely scrambled reads as usual.
func TestProtectedBooksAreRefusedOverTheAPI(t *testing.T) {
	ts := newTestServer(t)
	_, session := ts.user(t, "reader", "adult", false, "")
	const list = `<encryption xmlns:enc="http://www.w3.org/2001/04/xmlenc#"><enc:EncryptedData><enc:EncryptionMethod Algorithm="%s"/><enc:CipherData><enc:CipherReference URI="%s"/></enc:CipherData></enc:EncryptedData></encryption>`
	locked := epubFile(t, ts, "locked.epub", map[string]string{"META-INF/encryption.xml": fmt.Sprintf(list, "http://www.w3.org/2001/04/xmlenc#aes128-cbc", "OEBPS/c.xhtml")})
	for _, path := range []string{"book", "epub/OEBPS/c.xhtml", "search?q=words"} {
		w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/%s", locked.ID, path), session, nil)
		if w.Code != 422 || !strings.Contains(w.Body.String(), "copy protection") || strings.Contains(w.Body.String(), "words") {
			t.Errorf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	fonts := epubFile(t, ts, "fonts.epub", map[string]string{"META-INF/encryption.xml": fmt.Sprintf(list, "http://www.idpf.org/2008/embedding", "OEBPS/a.otf")})
	for _, path := range []string{"book", "epub/OEBPS/c.xhtml"} {
		if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/%s", fonts.ID, path), session, nil); w.Code != 200 {
			t.Errorf("%s: %d %s", path, w.Code, w.Body)
		}
	}
}

// A library set to read-only takes nothing into its folders, from anyone.
func TestReadOnlyLibraryRefusesUploads(t *testing.T) {
	ts := newGalleryServer(t)
	if err := ts.s.Store.SetLibraryReadOnly(t.Context(), ts.lib.ID, true); err != nil {
		t.Fatal(err)
	}
	w := ts.upload(ts.admin, "picture.jpg", tinyJPEG(t))
	if w.Code != 403 || !strings.Contains(w.Body.String(), "read-only") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if err := ts.s.Store.SetLibraryReadOnly(t.Context(), ts.lib.ID, false); err != nil {
		t.Fatal(err)
	}
	ts.uploadOK(ts.adult, "picture.jpg")
}

// A rule Settings shows a choice for needs to answer that choice.
func TestStatisticsChoiceTakesWhatSettingsOffers(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	for _, v := range []string{"admins", "adults", "everyone"} {
		if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"statsViewers": v}); w.Code != 200 {
			t.Errorf("%s: %d %s", v, w.Code, w.Body)
		}
		var got map[string]string
		_ = json.Unmarshal(ts.do(t, "GET", "/api/settings", admin, nil).Body.Bytes(), &got)
		if got["statsViewers"] != v {
			t.Errorf("settings say %q for %s", got["statsViewers"], v)
		}
	}
}

// Settings' 2160p choice is one the server takes, and the 4K discs' rule depends on it.
func TestMaximumResolutionTakes2160(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"maxHeight": "2160"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

// With no WittyCD to read it, nothing is wrong with the disc: the answer is a missing module,
// which the page knows how to offer, not a claim that the disc may be damaged.
func TestDiscWithoutWittyCDSaysTheModuleIsMissing(t *testing.T) {
	ts := newContractServer(t)
	_, session := ts.account(t, "listener", "adult", false)
	dir := t.TempDir()
	cue := filepath.Join(dir, "a.cue")
	if err := os.WriteFile(cue, []byte("FILE \"a.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := ts.item(t, "cd-image", cue)
	saved := wittycd.Open
	wittycd.Open = func(string) (*wittycd.Disc, error) { return nil, wittycd.ErrNoEngine }
	t.Cleanup(func() { wittycd.Open = saved })
	w := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d", id), session, nil)
	if w.Code != http.StatusFailedDependency || strings.Contains(w.Body.String(), "damaged") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var got struct {
		Module string `json:"module"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Module != "wittycd" {
		t.Fatalf("%s %v", w.Body, err)
	}
}

// mxmTransport answers for the public service, wherever a request was going.
type mxmTransport struct{ to *url.URL }

func (m mxmTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = m.to.Scheme, m.to.Host
	return http.DefaultTransport.RoundTrip(r)
}

// A Musixmatch answer under Other versions can be chosen, as any other can.
func TestLyricsChoosingAMusixmatchResult(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	it, _ := musicSong(t, ts, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Get("q_track") == "" || q.Get("apikey") != "a-key" {
			t.Errorf("asked with %s", r.URL.RawQuery)
		}
		body := `{}`
		if strings.HasSuffix(r.URL.Path, "matcher.subtitle.get") {
			body = `{"subtitle":{"subtitle_body":"[00:01.00]From the keyed source"}}`
		}
		_, _ = w.Write([]byte(`{"message":{"header":{"status_code":200},"body":` + body + `}}`))
	}))
	t.Cleanup(srv.Close)
	to, _ := url.Parse(srv.URL)
	saved := library.SafeClient
	library.SafeClient = &http.Client{Transport: mxmTransport{to}}
	t.Cleanup(func() { library.SafeClient = saved })
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"lyricsOnline": "1", "lyricsLrclib": "0", "lyricsMusixmatchKey": "a-key"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	base := fmt.Sprintf("/api/music/lyrics/%d", it.ID)
	w := ts.do(t, "POST", base+"/search", admin, map[string]string{"q": "anything"})
	var found []struct {
		Provider string `json:"provider"`
		ID       string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &found); err != nil || len(found) != 1 || found[0].Provider != "musixmatch" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", base, admin, map[string]string{"provider": found[0].Provider, "id": found[0].ID}); w.Code != 200 {
		t.Fatalf("choosing it: %d %s", w.Code, w.Body)
	}
	if got := decodeLyrics(t, ts.do(t, "GET", base, admin, nil)); len(got.Lines) != 1 || got.Lines[0].Text != "From the keyed source" {
		t.Fatalf("%+v", got)
	}
	// Turned off, nothing can be chosen from it.
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"lyricsMusixmatchKey": ""}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", base, admin, map[string]string{"provider": found[0].Provider, "id": found[0].ID}); w.Code != 409 {
		t.Fatalf("with no key: %d %s", w.Code, w.Body)
	}
}
