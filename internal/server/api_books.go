// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ebook"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
)

// Books: what the reader needs to show a comic page by page or an EPUB
// chapter by chapter. PDFs open in the browser's own viewer (/file).

// GET /api/items/{id}/book
func (s *Server) handleBookInfo(w http.ResponseWriter, r *http.Request) {
	it, ok := s.bookItem(w, r)
	if !ok {
		return
	}
	reader := mediatypes.ReaderFor(it.Container)
	out := map[string]any{"format": it.Container, "reader": reader, "reading": it.Reading, "title": it.Title}
	switch reader {
	case "cbz":
		pages, err := s.comicPages(it)
		if err != nil {
			s.Log.WarnContext(r.Context(), "could not open a comic", "cat", "library", "item", it.ID, "file", it.Path, "err", err)
			writeErr(w, 422, err.Error())
			return
		}
		out["pages"] = len(pages)
	case "epub":
		book, err := s.Scanner.BookEPUB(it)
		if err != nil {
			writeBookError(w, err)
			return
		}
		info, words, err := s.Store.ArchiveEPUBOf(r.Context(), it, book)
		if bookRefused(err) {
			writeBookError(w, err)
			return
		}
		if err != nil {
			writeErr(w, 422, s.viewerError(r, err))
			return
		}
		out["spine"], out["toc"] = info.Spine, info.TOC
		out["words"] = words // for "time left" in the chapter and the book
	case "pdf":
	default:
		writeErr(w, 422, "this kind of book can't be opened here. You can download it.")
		return
	}
	writeJSON(w, 200, out)
}

// writeBookError says why a book can't be opened. What's wrong with the book is said
// plainly; anything else about the server stays in the log.
func writeBookError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ebook.ErrProtected), errors.Is(err, ebook.ErrEncrypted), errors.Is(err, ebook.ErrTooBig):
		writeErr(w, 422, err.Error())
	case errors.Is(err, library.ErrNoReader):
		writeErr(w, 422, err.Error())
	default:
		writeErr(w, 422, "this book could not be read")
	}
}

// bookRefused: the book's own locks are in the way, which is worth saying as it is.
func bookRefused(err error) bool {
	return errors.Is(err, ebook.ErrProtected) || errors.Is(err, ebook.ErrEncrypted)
}

// comicPages lists the pages of anything that opens page by page.
func (s *Server) comicPages(it *library.Item) ([]string, error) {
	if it.Container == "djvu" {
		if !library.HasDjVu() {
			return nil, library.ErrNeedsDjVu
		}
		return library.DjvuPageList(it.Path), nil
	}
	return s.Scanner.ComicPageList(it.Path)
}

func (s *Server) bookItem(w http.ResponseWriter, r *http.Request) (*library.Item, bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil, false
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "book" || !s.Store.LibraryFile(r.Context(), it.Path) {
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
	pages, perr := s.comicPages(it)
	if err != nil || perr != nil || n < 0 || n >= len(pages) {
		writeErr(w, 404, "no such page")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	switch {
	case it.Container == "djvu":
		p, err := s.Scanner.DjvuPageFile(ctx, it, n)
		if err != nil {
			s.Log.WarnContext(ctx, "could not draw a page", "cat", "library", "item", it.ID, "page", n, "err", err)
			writeErr(w, 422, "this page can't be drawn")
			return
		}
		s.servePageFile(w, r, p)
	case library.IsZipComic(it.Path):
		zr, done, err := s.Store.OpenArchive(ctx, it.Path)
		if err != nil {
			writeErr(w, 500, "cannot open this comic")
			return
		}
		defer done()
		if library.ComicPageWeb(pages[n]) {
			s.serveArchiveFile(w, zr, pages[n])
			return
		}
		p, err := s.Scanner.ComicPageImage(ctx, it.Path, pages[n], func() (io.ReadCloser, error) { return zr.Open(pages[n]) })
		if err != nil {
			writeErr(w, 422, "this page can't be read")
			return
		}
		s.servePageFile(w, r, p)
	default:
		p, err := s.Scanner.ComicPageFile(ctx, it.Path, pages, n)
		if err != nil {
			s.Log.WarnContext(ctx, "could not unpack a page", "cat", "library", "item", it.ID, "page", n, "err", err)
			writeErr(w, 422, err.Error())
			return
		}
		if !library.ComicPageWeb(pages[n]) {
			if p, err = s.Scanner.ComicPageImage(ctx, it.Path, pages[n], func() (io.ReadCloser, error) { return os.Open(p) }); err != nil {
				writeErr(w, 422, "this page can't be read")
				return
			}
		}
		s.servePageFile(w, r, p)
	}
}

// servePageFile sends a page out of the cache: its type comes from its name, which the
// server gave it, and it can't run anything even when opened on its own.
func (s *Server) servePageFile(w http.ResponseWriter, r *http.Request, p string) {
	ct := mediatypes.MIME(p)
	if !strings.HasPrefix(ct, "image/") {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeFile(w, r, p)
}

// GET /api/items/{id}/epub/{file...}: a file inside an EPUB (chapters,
// styles, images). Book HTML is untrusted: a Content-Security-Policy keeps
// any script, form or outside request in it from running.
func (s *Server) handleEPUBFile(w http.ResponseWriter, r *http.Request) {
	it, ok := s.bookItem(w, r)
	if !ok || mediatypes.ReaderFor(it.Container) != "epub" {
		if ok {
			writeErr(w, 404, "not a book that opens this way")
		}
		return
	}
	name := path.Clean(r.PathValue("file"))
	if strings.HasPrefix(name, "..") || strings.HasPrefix(name, "/") {
		writeErr(w, 400, "bad path")
		return
	}
	book, err := s.Scanner.BookEPUB(it)
	if err != nil {
		writeBookError(w, err)
		return
	}
	zr, done, err := s.Store.OpenBook(r.Context(), it, book)
	if bookRefused(err) {
		writeBookError(w, err)
		return
	}
	if err != nil {
		writeErr(w, 500, "cannot open this book")
		return
	}
	defer done()
	s.serveArchiveFile(w, zr, name)
}

// scriptType: a browser would run a file of this type as a script. The app's policy allows
// scripts from this site, so a file from a library must never go out as one: a page here
// that something had got a tag into could otherwise run it. No book needs its scripts, as
// they don't run in the reader.
func scriptType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "javascript") || strings.Contains(ct, "ecmascript") || strings.Contains(ct, "jscript") || strings.Contains(ct, "livescript")
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
	if ct == "" || scriptType(ct) {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	w.Header().Set("Content-Security-Policy", "default-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'none'; object-src 'none'; form-action 'none'; frame-ancestors 'self'; base-uri 'none'; sandbox allow-same-origin")
	_, _ = io.Copy(w, f)
}
