// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

const plantedGuideKey = "Gu1de+Key/Planted=42"

// guideSite is a guide's site over HTTPS that counts what it is asked, and whether the key came along.
type guideSite struct {
	*httptest.Server
	hits, keyed atomic.Int32
}

func newGuideSite(t *testing.T, h http.HandlerFunc) *guideSite {
	t.Helper()
	g := &guideSite{}
	g.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.hits.Add(1)
		if strings.Contains(r.URL.RawQuery+r.URL.Path, url.QueryEscape(plantedGuideKey)) || strings.Contains(r.URL.String(), plantedGuideKey) {
			g.keyed.Add(1)
		}
		if h != nil {
			h(w, r)
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><tv><channel id="one"><display-name>One</display-name></channel></tv>`))
	}))
	t.Cleanup(g.Close)
	return g
}

// guideKeySetup is a test server with the guide key saved through the settings API, and every
// test site's certificate trusted (they share one).
func guideKeySetup(t *testing.T, site *guideSite) (*testServer, string) {
	t.Helper()
	old := *library.SafeClient
	*library.SafeClient = *site.Client()
	t.Cleanup(func() { *library.SafeClient = old })
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	if w := ts.do(t, "PUT", "/api/settings", tok, map[string]string{"guideKey": plantedGuideKey}); w.Code != 200 {
		t.Fatalf("saving the guide key: %d %s", w.Code, w.Body)
	}
	return ts, tok
}

func stations(t *testing.T, ts *testServer, tok, link string) *httptest.ResponseRecorder {
	t.Helper()
	return ts.do(t, "POST", "/api/channels/clone/stations", tok, map[string]any{
		"source": map[string]string{"kind": "xmltv", "url": link}, "from": "2026-11-20", "days": 1})
}

// The guide key is masked when read back; a "{key}" link mustn't be a way to read it anyway,
// by sending it to a site of one's own.
func TestGuideKeyGoesToOneSiteOnly(t *testing.T) {
	first := newGuideSite(t, nil)
	other := newGuideSite(t, nil)
	ts, tok := guideKeySetup(t, first)

	if w := stations(t, ts, tok, first.URL+"/guide.xml?key={key}"); w.Code != 200 {
		t.Fatalf("the first site: %d %s", w.Code, w.Body)
	}
	if first.keyed.Load() != 1 {
		t.Fatalf("the first site wasn't sent the key (%d requests)", first.hits.Load())
	}
	w := stations(t, ts, tok, other.URL+"/steal?k={key}")
	if w.Code == 200 || other.hits.Load() != 0 {
		t.Fatalf("the key went to a second site: %d, %d requests", w.Code, other.hits.Load())
	}
	if strings.Contains(w.Body.String(), plantedGuideKey) || strings.Contains(w.Body.String(), url.QueryEscape(plantedGuideKey)) {
		t.Fatalf("the refusal shows the key: %s", w.Body)
	}

	// Entering the key again lets it go to another site.
	if w := ts.do(t, "PUT", "/api/settings", tok, map[string]string{"guideKey": plantedGuideKey}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := stations(t, ts, tok, other.URL+"/guide.xml?k={key}"); w.Code != 200 || other.keyed.Load() != 1 {
		t.Fatalf("after the key was entered again: %d %s", w.Code, w.Body)
	}
	// The masked value sent back unchanged (as the settings page does) doesn't move it.
	if w := ts.do(t, "PUT", "/api/settings", tok, map[string]string{"guideKey": "••••••••"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := stations(t, ts, tok, first.URL+"/again.xml?key={key}"); w.Code == 200 {
		t.Fatal("a masked key sent back let the key go to another site")
	}
	// Nor can the settings API set where it goes.
	ts.do(t, "PUT", "/api/settings", tok, map[string]string{guideKeyHostSetting: "evil.example"})
	if got := ts.DB.GetSetting(t.Context(), guideKeyHostSetting, ""); got == "evil.example" {
		t.Fatal("the settings API set the guide key's site")
	}
}

func TestGuideKeyLinkRules(t *testing.T) {
	site := newGuideSite(t, nil)
	ts, tok := guideKeySetup(t, site)
	host := strings.TrimPrefix(site.URL, "https://")
	for _, link := range []string{
		"http://" + host + "/guide.xml?key={key}", // in the clear
		"https://{key}." + host + "/guide.xml",    // in the site's name
		"https://{key}@" + host + "/guide.xml",    // as a user name
	} {
		w := stations(t, ts, tok, link)
		if w.Code == 200 {
			t.Errorf("%s was read", link)
		}
		if strings.Contains(w.Body.String(), plantedGuideKey) || strings.Contains(w.Body.String(), url.QueryEscape(plantedGuideKey)) {
			t.Errorf("%s: the refusal shows the key: %s", link, w.Body)
		}
	}
	if site.keyed.Load() != 0 {
		t.Fatal("a refused link sent the key")
	}
}

// A redirect would carry the link, key and all, to another site as the Referer.
func TestGuideKeyNotRedirectedElsewhere(t *testing.T) {
	var seenReferer atomic.Bool
	sink := newGuideSite(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Referer() != "" {
			seenReferer.Store(true)
		}
		_, _ = w.Write([]byte(`<tv></tv>`))
	})
	site := newGuideSite(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL+"/elsewhere.xml", http.StatusFound)
	})
	ts, tok := guideKeySetup(t, site)
	w := stations(t, ts, tok, site.URL+"/guide.xml?key={key}")
	if w.Code == 200 || sink.hits.Load() != 0 || seenReferer.Load() {
		t.Fatalf("followed a redirect to another site: %d, %d requests there", w.Code, sink.hits.Load())
	}
	if strings.Contains(w.Body.String(), plantedGuideKey) || strings.Contains(w.Body.String(), url.QueryEscape(plantedGuideKey)) {
		t.Fatalf("the error shows the key: %s", w.Body)
	}
}

// A link without the key is unchanged: any site on the internet, over HTTP or HTTPS.
func TestGuideLinkWithoutKeyUnchanged(t *testing.T) {
	a := newGuideSite(t, nil)
	b := newGuideSite(t, nil)
	ts, tok := guideKeySetup(t, a)
	for _, s := range []*guideSite{a, b} {
		if w := stations(t, ts, tok, s.URL+"/open.xml"); w.Code != 200 {
			t.Fatalf("%s: %d %s", s.URL, w.Code, w.Body)
		}
	}
}
