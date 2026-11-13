// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Console photos for WittyPlay's Consoles tab: a picture of each console's hardware from Wikimedia Commons (by its Wikidata
// entity), fetched by this server once and kept, never linked from the browser, with its author and licence to show beside it.
// It asks Wikimedia only while Wikidata lookups are on (Settings, Metadata) and the owner has agreed to downloads from Wikimedia
// (the logo service's consent); a picture kept before is shown either way. With none, the app draws the console's own mark.
//
// GET /api/console-photos/{id}        → {imageUrl, credit, license, licenseUrl, pageUrl, source}, or 404
// GET /api/console-photos/{id}/image  → the picture

// consolePhotoSource finds a console's picture; a fake in tests.
type consolePhotoSource interface {
	ConsolePhoto(ctx context.Context, console, name string) (library.LogoCandidate, error)
	Fetch(ctx context.Context, c library.LogoCandidate) ([]byte, error)
}

// consolePhotoRetry: how long a console with no free picture is left before Commons is asked again.
const consolePhotoRetry = 7 * 24 * time.Hour

var consoleIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,47}$`)

// consolePhotoFacts is what is kept beside a picture.
type consolePhotoFacts struct {
	None       bool   `json:"none,omitempty"`
	At         int64  `json:"at"`
	File       string `json:"file,omitempty"`
	Credit     string `json:"credit,omitempty"`
	License    string `json:"license,omitempty"`
	LicenseURL string `json:"licenseUrl,omitempty"`
	PageURL    string `json:"pageUrl,omitempty"`
}

type consolePhotos struct {
	dir    string
	source consolePhotoSource
	// allowed says whether Wikimedia may be asked now.
	allowed func(ctx context.Context) bool
	mu      sync.Mutex
	busy    map[string]*sync.Mutex
}

func (s *Server) consolePhotoService() *consolePhotos {
	s.consolePhotoOnce.Do(func() {
		s.consolePhotoSvc = &consolePhotos{dir: filepath.Join(s.Cfg.ImagesDir, "console-photos"), source: &library.WikimediaLogos{},
			allowed: func(ctx context.Context) bool {
				return s.DB.GetSetting(ctx, "wikidata", "1") != "0" && s.Store.LogoConsent(ctx) == "yes"
			}}
	})
	return s.consolePhotoSvc
}

func (p *consolePhotos) lock(id string) *sync.Mutex {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.busy == nil {
		p.busy = map[string]*sync.Mutex{}
	}
	if p.busy[id] == nil {
		p.busy[id] = &sync.Mutex{}
	}
	return p.busy[id]
}

// get is the console's picture's facts, asking Commons when there are none yet (and it may), or ok false for none.
func (p *consolePhotos) get(ctx context.Context, id string) (consolePhotoFacts, bool) {
	l := p.lock(id)
	l.Lock()
	defer l.Unlock()
	facts, kept := p.read(id)
	if kept && !facts.None {
		return facts, true
	}
	if kept && facts.None && time.Since(time.Unix(facts.At, 0)) < consolePhotoRetry {
		return facts, false
	}
	if !p.allowed(ctx) {
		return consolePhotoFacts{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c, err := p.source.ConsolePhoto(ctx, id, library.ConsoleLogoName(id))
	if errors.Is(err, library.ErrNoPhoto) {
		p.write(id, consolePhotoFacts{None: true, At: time.Now().Unix()}, nil)
		return consolePhotoFacts{}, false
	}
	if err != nil {
		return consolePhotoFacts{}, false // offline, or Wikimedia busy: asked again next time
	}
	body, err := p.source.Fetch(ctx, c)
	if err != nil || len(body) == 0 || !isPicture(body) {
		return consolePhotoFacts{}, false
	}
	facts = consolePhotoFacts{At: time.Now().Unix(), File: c.SourceID, Credit: c.Credit, License: c.License, LicenseURL: c.LicenseURL, PageURL: c.PageURL}
	if p.write(id, facts, body) != nil {
		return consolePhotoFacts{}, false
	}
	return facts, true
}

// isPicture: a JPEG, PNG or WebP, by its first bytes (what Commons' thumbnails are).
func isPicture(b []byte) bool {
	switch ct := http.DetectContentType(b); ct {
	case "image/jpeg", "image/png", "image/webp":
		return true
	}
	return false
}

func (p *consolePhotos) read(id string) (consolePhotoFacts, bool) {
	var f consolePhotoFacts
	b, err := os.ReadFile(filepath.Join(p.dir, id+".json"))
	if err != nil || json.Unmarshal(b, &f) != nil {
		return f, false
	}
	if !f.None {
		if _, err := os.Stat(filepath.Join(p.dir, id+".img")); err != nil {
			return f, false
		}
	}
	return f, true
}

func (p *consolePhotos) write(id string, f consolePhotoFacts, body []byte) error {
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return err
	}
	if body != nil {
		if err := writeFileAtomic(filepath.Join(p.dir, id+".img"), body); err != nil {
			return err
		}
	}
	b, _ := json.Marshal(f)
	return writeFileAtomic(filepath.Join(p.dir, id+".json"), b)
}

func (s *Server) handleConsolePhoto(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !consoleIDRe.MatchString(id) {
		writeErr(w, 400, "bad console")
		return
	}
	f, ok := s.consolePhotoService().get(r.Context(), id)
	if !ok {
		writeErr(w, 404, "no picture of this console")
		return
	}
	writeJSON(w, 200, map[string]any{"imageUrl": "/api/console-photos/" + id + "/image?v=" + time.Unix(f.At, 0).UTC().Format("20060102"), "credit": f.Credit, "license": f.License,
		"licenseUrl": f.LicenseURL, "pageUrl": f.PageURL, "source": "Wikimedia Commons"})
}

func (s *Server) handleConsolePhotoImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !consoleIDRe.MatchString(id) {
		writeErr(w, 400, "bad console")
		return
	}
	p := s.consolePhotoService()
	if f, ok := p.read(id); !ok || f.None {
		writeErr(w, 404, "no picture of this console")
		return
	}
	b, err := os.ReadFile(filepath.Join(p.dir, id+".img"))
	if err != nil {
		writeErr(w, 404, "no picture of this console")
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(b))
	w.Header().Set("Cache-Control", "private, max-age=604800")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}
