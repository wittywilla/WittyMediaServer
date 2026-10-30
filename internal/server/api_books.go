package server

import (
	"archive/zip"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Books: what the reader needs to show a comic page by page or an EPUB
// chapter by chapter. PDFs open in the browser's own viewer (/file).

// GET /api/items/{id}/book
func (s *Server) handleBookInfo(w http.ResponseWriter, r *http.Request) {
	it, ok := s.bookItem(w, r)
	if !ok {
		return
	}
	out := map[string]any{"format": it.Container, "reading": it.Reading, "title": it.Title}
	switch it.Container {
	case "cbz":
		pages, err := library.ArchivePages(it.Path)
		if err != nil {
			writeErr(w, 500, "cannot open this comic")
			return
		}
		out["pages"] = len(pages)
	case "epub":
		info, words, err := library.ArchiveEPUB(it.Path)
		if err != nil {
			writeErr(w, 422, err.Error())
			return
		}
		out["spine"], out["toc"] = info.Spine, info.TOC
		out["words"] = words // for "time left" in the chapter and the book
	}
	writeJSON(w, 200, out)
}

func (s *Server) bookItem(w http.ResponseWriter, r *http.Request) (*library.Item, bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil, false
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "book" {
		writeErr(w, 404, "book not found")
		return nil, false
	}
	return it, true
}

// GET /api/items/{id}/page/{n}: page n (0-based) of a comic archive.
func (s *Server) handleComicPage(w http.ResponseWriter, r *http.Request) {
	it, ok := s.bookItem(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	pages, perr := library.ArchivePages(it.Path)
	if err != nil || perr != nil || n < 0 || n >= len(pages) {
		writeErr(w, 404, "no such page")
		return
	}
	zr, done, err := library.OpenArchive(it.Path)
	if err != nil {
		writeErr(w, 500, "cannot open this comic")
		return
	}
	defer done()
	s.serveArchiveFile(w, zr, pages[n])
}

// GET /api/items/{id}/epub/{file...}: a file inside an EPUB (chapters,
// styles, images). Book HTML is untrusted: a Content-Security-Policy keeps
// any script, form or outside request in it from running.
func (s *Server) handleEPUBFile(w http.ResponseWriter, r *http.Request) {
	it, ok := s.bookItem(w, r)
	if !ok || it.Container != "epub" {
		if ok {
			writeErr(w, 404, "not an EPUB")
		}
		return
	}
	name := path.Clean(r.PathValue("file"))
	if strings.HasPrefix(name, "..") || strings.HasPrefix(name, "/") {
		writeErr(w, 400, "bad path")
		return
	}
	zr, done, err := library.OpenArchive(it.Path)
	if err != nil {
		writeErr(w, 500, "cannot open this book")
		return
	}
	defer done()
	s.serveArchiveFile(w, zr, name)
}

// serveArchiveFile serves a file from a book or comic. Their files are
// untrusted (a comic "page" may be an SVG with a script), so none of them
// can run scripts, submit forms or reach outside this server, even when
// opened on their own.
func (s *Server) serveArchiveFile(w http.ResponseWriter, zr *zip.Reader, name string) {
	f, err := zr.Open(name)
	if err != nil {
		writeErr(w, 404, "not in this book")
		return
	}
	defer f.Close()
	ct := mime.TypeByExtension(strings.ToLower(path.Ext(name)))
	switch strings.ToLower(path.Ext(name)) {
	case ".xhtml", ".xht":
		ct = "application/xhtml+xml"
	case ".webp":
		ct = "image/webp"
	case ".avif":
		ct = "image/avif"
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	w.Header().Set("Content-Security-Policy", "default-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'none'; object-src 'none'; form-action 'none'; frame-ancestors 'self'; base-uri 'none'; sandbox allow-same-origin")
	_, _ = io.Copy(w, f)
}
