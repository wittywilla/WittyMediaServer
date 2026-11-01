package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The logo service's routes (docs/METADATA.md#the-logo-service). Anyone signed in can
// see a logo; only an administrator can agree to downloads or change one.

// logos is the service, made when first wanted.
func (s *Server) logos() *library.Logos {
	s.logoOnce.Do(func() {
		s.logoSvc = &library.Logos{Store: s.Store, Dir: filepath.Join(s.Cfg.ImagesDir, "logos"), Source: &library.WikimediaLogos{}, Log: s.Log, Tasks: s.Tasks}
	})
	return s.logoSvc
}

func (s *Server) handleLogoStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts, err := s.Store.LogoCounts(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"consent": s.Store.LogoConsent(ctx), "counts": counts, "source": "Wikidata and Wikimedia Commons",
		"sourceUrl": "https://commons.wikimedia.org/wiki/Commons:Licensing",
	})
}

// PUT /api/logos/consent {accepted, removeDownloaded}
func (s *Server) handleLogoConsent(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Accepted         bool `json:"accepted"`
		RemoveDownloaded bool `json:"removeDownloaded"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.logos().SetLogoConsent(r.Context(), b.Accepted, b.RemoveDownloaded); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.handleLogoStatus(w, r)
}

const maxLogoLookup = 200

// POST /api/logos/lookup {logos: [{kind, name}]}: what to show for many names at once.
func (s *Server) handleLogoLookup(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Logos []library.LogoRef `json:"logos"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(b.Logos) > maxLogoLookup {
		writeErr(w, 400, "at most 200 names at a time")
		return
	}
	// A name comes from a page, and looking it up asks the internet, so only an
	// administrator's page can add one. Everyone else sees what's known already.
	out, err := s.logos().LookupLogos(r.Context(), b.Logos, userFrom(r).IsAdmin)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"logos": out})
}

// GET /api/logos/{id}/image?variant=white|dark|original
func (s *Server) handleLogoImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	g, err := s.Store.LogoByID(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "no logo")
		return
	}
	p := s.logos().ImagePath(g, r.URL.Query().Get("variant"))
	if p == "" {
		writeErr(w, 404, "no logo")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		writeErr(w, 404, "no logo")
		return
	}
	s.serveArtFile(w, r, f, p)
}

// GET /api/logos/credits: where every logo came from, as their licences ask.
func (s *Server) handleLogoCredits(w http.ResponseWriter, r *http.Request) {
	out, err := s.Store.LogoCredits(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"logos": out})
}

// GET /api/logos/{id}/candidates?q=: what a source has, to choose from.
func (s *Server) handleLogoCandidates(w http.ResponseWriter, r *http.Request) {
	g, ok := s.logoFromPath(w, r)
	if !ok {
		return
	}
	out, err := s.logos().Candidates(r.Context(), g, r.URL.Query().Get("q"))
	if err != nil {
		if errors.Is(err, library.ErrLogosNotAgreed) {
			writeErr(w, 409, err.Error())
			return
		}
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"candidates": out})
}

// GET /api/logos/preview?src=: a candidate's picture, fetched by the server and kept
// in the image cache like any other artwork, so a browser never talks to the source.
func (s *Server) handleLogoPreview(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	if !library.IsLogoPreview(src) {
		writeErr(w, 400, "not a logo address")
		return
	}
	s.serveArtSrc(w, r, src)
}

// PUT /api/logos/{id} {sourceId, q} picks a candidate; {status: "text"|"pending"}
// means the name alone, or look again.
func (s *Server) handleLogoChange(w http.ResponseWriter, r *http.Request) {
	g, ok := s.logoFromPath(w, r)
	if !ok {
		return
	}
	var b struct {
		SourceID string `json:"sourceId"`
		Query    string `json:"q"`
		Status   string `json:"status"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var out *library.Logo
	var err error
	if b.SourceID != "" {
		out, err = s.logos().Pick(r.Context(), g, b.Query, b.SourceID)
	} else {
		out, err = s.logos().SetLogoStatus(r.Context(), g, b.Status)
	}
	switch {
	case errors.Is(err, library.ErrLogosNotAgreed):
		writeErr(w, 409, err.Error())
	case errors.Is(err, library.ErrNotFound):
		writeErr(w, 404, "that picture isn't offered any more")
	case err != nil:
		writeErr(w, 400, err.Error())
	default:
		writeJSON(w, 200, out)
	}
}

// PUT /api/logos/{id}/upload, with the picture as the body.
func (s *Server) handleLogoUpload(w http.ResponseWriter, r *http.Request) {
	g, ok := s.logoFromPath(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		writeErr(w, 413, "the picture is too big: the limit is 8 MB")
		return
	}
	out, err := s.logos().Upload(r.Context(), g, body)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) logoFromPath(w http.ResponseWriter, r *http.Request) (*library.Logo, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad id")
		return nil, false
	}
	g, err := s.Store.LogoByID(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "no such logo")
		return nil, false
	}
	return g, true
}

// GET /api/brands?library=&kind=studio|network: the studios or networks of a library.
func (s *Server) handleBrands(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind != library.LogoStudio && kind != library.LogoNetwork {
		writeErr(w, 400, "kind must be studio or network")
		return
	}
	out, err := s.Store.Brands(r.Context(), int64(qInt(r, "library", 0)), kind, s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"brands": out})
}
