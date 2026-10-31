package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// E-reader tools: annotations, search in a book, and word definitions.

// GET /api/items/{id}/annotations: the account's own bookmarks/highlights.
func (s *Server) handleAnnotations(w http.ResponseWriter, r *http.Request) {
	it, ok := s.bookItem(w, r)
	if !ok {
		return
	}
	as, err := s.Store.Annotations(r.Context(), userFrom(r).ID, it.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, as)
}

// POST /api/items/{id}/annotations
func (s *Server) handleAddAnnotation(w http.ResponseWriter, r *http.Request) {
	it, ok := s.bookItem(w, r)
	if !ok {
		return
	}
	var a library.Annotation
	if err := readJSON(r, &a); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.ID, a.ItemID = 0, it.ID
	if err := s.Store.SaveAnnotation(r.Context(), userFrom(r).ID, &a); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, a)
}

// PUT /api/annotations/{aid} {color, note}
func (s *Server) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "aid")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var a library.Annotation
	if err := readJSON(r, &a); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.ID = id
	a.Kind = "highlight" // only colour and note can change (validated as a highlight)
	if err := s.Store.SaveAnnotation(r.Context(), userFrom(r).ID, &a); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, a)
}

// DELETE /api/annotations/{aid}
func (s *Server) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "aid")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.Store.DeleteAnnotation(r.Context(), userFrom(r).ID, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/items/{id}/search?q=
func (s *Server) handleBookSearch(w http.ResponseWriter, r *http.Request) {
	it, ok := s.bookItem(w, r)
	if !ok || it.Container != "epub" {
		if ok {
			writeJSON(w, 200, []any{})
		}
		return
	}
	zr, done, err := library.OpenArchive(it.Path)
	if err != nil {
		s.Log.WarnContext(r.Context(), "could not open a book", "cat", "library", "item", it.ID, "file", it.Path, "err", err)
		writeErr(w, 500, "cannot open this book")
		return
	}
	defer done()
	info, err := library.ReadEPUB(zr)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, library.SearchBook(zr, info.Spine, r.URL.Query().Get("q")))
}

// Definitions are cached: readers look up the same words again and again.
var dictCache = struct {
	sync.Mutex
	m map[string]json.RawMessage
}{m: map[string]json.RawMessage{}}

// GET /api/dictionary?word=: definitions from the free dictionary API
// (Wiktionary data), fetched by the server so devices stay private.
func (s *Server) handleDictionary(w http.ResponseWriter, r *http.Request) {
	word := strings.ToLower(strings.TrimFunc(r.URL.Query().Get("word"), func(c rune) bool { return !unicode.IsLetter(c) && c != '\'' && c != '-' }))
	if word == "" || len([]rune(word)) > 40 {
		writeErr(w, 400, "choose one word")
		return
	}
	dictCache.Lock()
	cached, ok := dictCache.m[word]
	dictCache.Unlock()
	if ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cached)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.dictionaryapi.dev/api/v2/entries/en/"+url.PathEscape(word), nil)
	res, err := library.SafeClient.Do(req)
	if err != nil {
		if r.Context().Err() == nil {
			s.Log.WarnContext(r.Context(), "could not reach the dictionary", "err", err)
		}
		writeErr(w, 502, "the dictionary did not answer")
		return
	}
	defer res.Body.Close()
	var entries []struct {
		Word     string `json:"word"`
		Phonetic string `json:"phonetic"`
		Meanings []struct {
			PartOfSpeech string `json:"partOfSpeech"`
			Definitions  []struct {
				Definition string `json:"definition"`
				Example    string `json:"example"`
			} `json:"definitions"`
		} `json:"meanings"`
	}
	if res.StatusCode == 404 {
		writeJSON(w, 200, []any{}) // no definition: an empty list
		return
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&entries) != nil {
		writeErr(w, 502, "the dictionary did not answer")
		return
	}
	// Keep it short: up to 3 senses per part of speech.
	for i := range entries {
		for j := range entries[i].Meanings {
			if d := entries[i].Meanings[j].Definitions; len(d) > 3 {
				entries[i].Meanings[j].Definitions = d[:3]
			}
		}
	}
	b, _ := json.Marshal(entries)
	dictCache.Lock()
	if len(dictCache.m) > 5000 {
		dictCache.m = map[string]json.RawMessage{}
	}
	dictCache.m[word] = b
	dictCache.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}
