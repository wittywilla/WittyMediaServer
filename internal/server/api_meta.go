package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Editing a title and fixing its match (docs/METADATA.md#editing-and-fixing-matches).
// Everything here is for administrators; {target} is item or show.

// titleFromPath reads the title a route names.
func (s *Server) titleFromPath(w http.ResponseWriter, r *http.Request) (*library.Title, bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil, false
	}
	t, err := s.Store.LoadTitle(r.Context(), r.PathValue("target"), id)
	if err != nil {
		writeErr(w, 404, "not found")
		return nil, false
	}
	return t, true
}

func userName(r *http.Request) string {
	if u := userFrom(r); u != nil {
		return u.Name
	}
	return ""
}

// metaField is one field as the edit dialog shows it.
type metaField struct {
	Value       string `json:"value"`
	Source      string `json:"source"`
	SourceLabel string `json:"sourceLabel"`
	Locked      bool   `json:"locked"`
	CanRevert   bool   `json:"canRevert"`
	RevertTo    string `json:"revertTo,omitempty"`
	RevertFrom  string `json:"revertFrom,omitempty"`
}

// titleEditor is everything the edit dialog needs.
type titleEditor struct {
	Target     string               `json:"target"`
	ID         int64                `json:"id"`
	LibraryID  int64                `json:"libraryId"`
	Name       string               `json:"name"`
	File       string               `json:"file"`
	Kind       string               `json:"kind"` // movie | show | episode
	UpdatedAt  int64                `json:"updatedAt"`
	Order      []string             `json:"order"`
	Fields     map[string]metaField `json:"fields"`
	Match      titleMatch           `json:"match"`
	Link       *linkInfo            `json:"link,omitempty"`
	Extras     []attachedTitle      `json:"extras"`
	Versions   []attachedTitle      `json:"versions"`
	Move       library.MoveInfo     `json:"move"`
	Brands     []library.Logo       `json:"brands"`
	IntroAuto  bool                 `json:"introAuto"`
	Runtime    float64              `json:"runtimeSec"`
	CanDelete  bool                 `json:"canDelete"`
	Disc       bool                 `json:"disc"`
	ExtraTypes []string             `json:"extraTypes"`
}

type titleMatch struct {
	Source     string   `json:"source"`
	SourceName string   `json:"sourceLabel"`
	ID         string   `json:"id"`
	URL        string   `json:"url,omitempty"`
	State      string   `json:"state"`
	Confidence int      `json:"confidence"`
	Reasons    []string `json:"reasons"`
}

type linkInfo struct {
	library.ItemLink
	ParentName string `json:"parentName"`
}

type attachedTitle struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	ExtraType string `json:"extraType,omitempty"`
	Label     string `json:"label,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	Runtime   float64 `json:"runtimeSec,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}

func (s *Server) buildEditor(ctx context.Context, t *library.Title) *titleEditor {
	libType := ""
	if lib, err := s.Store.Library(ctx, t.LibraryID()); err == nil {
		libType = lib.Type
	}
	e := &titleEditor{Target: t.Target, ID: t.ID, LibraryID: t.LibraryID(), Name: t.Name(), File: filepath.Base(t.Path()), Order: t.FieldNames(), Fields: map[string]metaField{}, ExtraTypes: library.ExtraTypes, CanDelete: true}
	for _, f := range t.FieldNames() {
		mf := metaField{Value: t.Field(f), Source: t.SourceOf(f), Locked: t.Meta.Locks[f]}
		mf.SourceLabel = library.SourceLabel(mf.Source)
		if b, ok := t.Meta.Baseline[f]; ok {
			mf.CanRevert, mf.RevertTo, mf.RevertFrom = true, b.Value, library.SourceLabel(b.Source)
		}
		e.Fields[f] = mf
	}
	provider, id := t.Provider()
	score, why := library.ConfidenceFor(t, libType)
	e.Match = titleMatch{Source: provider, SourceName: library.SourceLabel(provider), ID: id, URL: library.SourceURL(provider, id), State: t.Meta.MatchState, Confidence: score, Reasons: why}
	if t.Item != nil {
		e.Kind, e.UpdatedAt, e.Runtime = "movie", t.Item.UpdatedAt, t.Item.RuntimeSec
		if t.Item.ShowID != nil {
			e.Kind = "episode"
		}
		if l := s.Store.LinkOf(ctx, t.ID); l != nil {
			e.Link = &linkInfo{ItemLink: *l}
			if l.ParentItem != nil {
				if p, err := s.Store.Item(ctx, *l.ParentItem); err == nil {
					e.Link.ParentName = p.Title
				}
			} else if l.ParentShow != nil {
				if p, err := s.Store.ShowFacts(ctx, *l.ParentShow); err == nil {
					e.Link.ParentName = p.Title
				}
			}
		}
		e.IntroAuto = t.Meta.Sources["introEnd"] != library.SourceYou && t.Item.IntroEnd != nil
		e.Disc = library.IsDiscKind(t.Item.Kind)
	} else {
		e.Kind, e.UpdatedAt = "show", t.Show.UpdatedAt
	}
	e.Extras, e.Versions = []attachedTitle{}, []attachedTitle{}
	for _, c := range []struct {
		link string
		to   *[]attachedTitle
	}{{library.LinkExtra, &e.Extras}, {library.LinkVersion, &e.Versions}} {
		items, links, err := s.Store.AttachedTo(ctx, t.Target, t.ID, c.link)
		if err != nil {
			continue
		}
		for i, it := range items {
			*c.to = append(*c.to, attachedTitle{ID: it.ID, Title: it.Title, ExtraType: links[i].ExtraType, Label: links[i].Label, Width: it.Width, Height: it.Height, SizeBytes: it.SizeBytes, Runtime: it.RuntimeSec, UpdatedAt: it.UpdatedAt})
		}
	}
	e.Move = s.Store.MoveOptions(ctx, t)
	e.Brands = []library.Logo{}
	refs := []library.LogoRef{}
	for _, n := range t.Studios {
		refs = append(refs, library.LogoRef{Kind: library.LogoStudio, Name: n})
	}
	if t.Show != nil && t.Show.Network != "" {
		refs = append(refs, library.LogoRef{Kind: library.LogoNetwork, Name: t.Show.Network})
	}
	if out, err := s.logos().LookupLogos(ctx, refs, true); err == nil {
		e.Brands = out
	}
	return e
}

// lockEdited locks fields set through the older edit routes, as the edit dialog does.
func (s *Server) lockEdited(ctx context.Context, target string, id int64, set map[string]bool) error {
	m := s.Store.TitleMeta(ctx, target, id)
	any := false
	for f, on := range set {
		if on {
			m.Locks[f] = true
			m.Sources[f] = library.SourceYou
			any = true
		}
	}
	if !any {
		return nil
	}
	return s.Store.SaveMeta(ctx, target, id, m)
}

// GET /api/meta/{target}/{id}
func (s *Server) handleMetaGet(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, s.buildEditor(r.Context(), t))
}

// validArt: pictures are only ever web addresses, files in the image cache, or files in a library.
func (s *Server) validArt(ctx context.Context, v string) bool {
	if v == "" || isWeb(v) {
		return true
	}
	if strings.HasPrefix(v, "cache:") {
		name := strings.TrimPrefix(v, "cache:")
		return name != "" && name == filepath.Base(name) && !strings.ContainsAny(name, `/\:`)
	}
	return s.Store.LibraryFile(ctx, v)
}

// PUT /api/meta/{target}/{id} {fields: {name: value}, lock: [..], unlock: [..], lockAll, unlockAll, keepUnlocked}
func (s *Server) handleMetaPut(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	var b struct {
		Fields       map[string]string `json:"fields"`
		Lock         []string          `json:"lock"`
		Unlock       []string          `json:"unlock"`
		LockAll      bool              `json:"lockAll"`
		UnlockAll    bool              `json:"unlockAll"`
		KeepUnlocked bool              `json:"keepUnlocked"` // a change that shouldn't lock what it changes
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	for _, f := range []string{"poster", "backdrop", "banner", "logo"} {
		if v, on := b.Fields[f]; on && !s.validArt(ctx, v) {
			writeErr(w, 400, "that picture isn't a web address or a file in one of your libraries")
			return
		}
	}
	batch := library.NewBatch()
	if err := s.Store.Remember(ctx, batch, userName(r), "Edited", t, t.Snapshot()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := t.EditFields(b.Fields, !b.KeepUnlocked); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	for _, f := range b.Lock {
		t.Meta.Locks[f] = true
	}
	for _, f := range b.Unlock {
		delete(t.Meta.Locks, f)
	}
	if b.LockAll {
		for _, f := range t.FieldNames() {
			t.Meta.Locks[f] = true
		}
	}
	if b.UnlockAll {
		t.Meta.Locks = map[string]bool{}
	}
	if len(b.Fields) > 0 {
		s.Scanner.Persist(ctx, t)
	}
	if err := s.Store.SaveTitle(ctx, t); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"editor": s.buildEditor(ctx, t), "batch": batch})
}

// POST /api/meta/{target}/{id}/revert {field}
func (s *Server) handleMetaRevert(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	var b struct {
		Field string `json:"field"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	batch := library.NewBatch()
	if err := s.Store.Remember(ctx, batch, userName(r), "Reverted", t, t.Snapshot()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := t.RevertField(b.Field); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.Scanner.Persist(ctx, t)
	if err := s.Store.SaveTitle(ctx, t); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"editor": s.buildEditor(ctx, t), "batch": batch})
}

// POST /api/meta/{target}/{id}/search {query, year}
func (s *Server) handleMetaSearch(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	var b struct {
		Query string `json:"query"`
		Year  *int   `json:"year"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(b.Query) > 500 {
		writeErr(w, 400, "that's too long to search for")
		return
	}
	res, err := s.Scanner.SearchMatches(r.Context(), t, b.Query, b.Year)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// POST /api/meta/{target}/{id}/preview {source, id}: what choosing this would change.
// With no source, what refreshing would change.
func (s *Server) handleMetaPreview(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	var b struct {
		Source string `json:"source"`
		ID     string `json:"id"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var out *library.MatchPreview
	var err error
	if b.Source == "" {
		out, err = s.Scanner.PreviewRefresh(r.Context(), t)
	} else {
		out, err = s.Scanner.PreviewMatch(r.Context(), t, b.Source, b.ID)
	}
	if err != nil {
		writeErr(w, sourceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func sourceErrorStatus(err error) int {
	if errors.Is(err, library.ErrNotFound) {
		return 404
	}
	return 502
}

func fieldSet(list []string, given bool) map[string]bool {
	if !given {
		return nil
	}
	out := map[string]bool{}
	for _, f := range list {
		out[f] = true
	}
	return out
}

// POST /api/meta/{target}/{id}/match {source, id, fields?}: choose a result. With fields, only those change.
func (s *Server) handleMetaMatch(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	var b struct {
		Source string    `json:"source"`
		ID     string    `json:"id"`
		Fields *[]string `json:"fields"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var only map[string]bool
	if b.Fields != nil {
		only = fieldSet(*b.Fields, true)
	}
	batch, err := s.Scanner.ApplyMatch(r.Context(), t, b.Source, b.ID, only, userName(r))
	if err != nil {
		writeErr(w, sourceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"editor": s.buildEditor(r.Context(), t), "batch": batch})
}

// DELETE /api/meta/{target}/{id}/match: wrong film or show. Keep only what was typed.
func (s *Server) handleMetaUnmatch(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	batch, err := s.Scanner.RemoveMatch(r.Context(), t, userName(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"editor": s.buildEditor(r.Context(), t), "batch": batch})
}

// POST /api/meta/{target}/{id}/refresh {fields?}
func (s *Server) handleMetaRefresh(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	var b struct {
		Fields *[]string `json:"fields"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var only map[string]bool
	if b.Fields != nil {
		only = fieldSet(*b.Fields, true)
	}
	batch, err := s.Scanner.RefreshTitle(r.Context(), t, only, userName(r))
	if err != nil {
		writeErr(w, sourceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"editor": s.buildEditor(r.Context(), t), "batch": batch})
}

const maxArtUpload = 20 << 20

// PUT /api/meta/{target}/{id}/art/{kind}, with the picture as the body. It's checked to
// be a picture and kept in the image cache, under a name that's ours alone.
func (s *Server) handleMetaArtUpload(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	field := r.PathValue("kind")
	switch field {
	case "poster", "backdrop", "banner", "logo":
	default:
		writeErr(w, 400, "that isn't a kind of artwork")
		return
	}
	if !t.HasField(field) {
		writeErr(w, 400, "this title has no "+field)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxArtUpload))
	if err != nil {
		writeErr(w, 413, "the picture is too big: the limit is 20 MB")
		return
	}
	ext := library.PictureExt(body)
	if ext == "" {
		writeErr(w, 400, "use a JPEG, PNG, WebP or GIF picture")
		return
	}
	if err := os.MkdirAll(s.Cfg.ImagesDir, 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h := sha1.Sum(body)
	name := "up-" + hex.EncodeToString(h[:10]) + ext
	if err := os.WriteFile(filepath.Join(s.Cfg.ImagesDir, name), body, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ctx := r.Context()
	batch := library.NewBatch()
	if err := s.Store.Remember(ctx, batch, userName(r), "New artwork", t, t.Snapshot()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := t.EditFields(map[string]string{field: "cache:" + name}, true); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.Scanner.Persist(ctx, t)
	if err := s.Store.SaveTitle(ctx, t); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"editor": s.buildEditor(ctx, t), "batch": batch})
}

// GET /api/meta/{target}/{id}/artwork?kind=: every picture every source has for it, once.
func (s *Server) handleMetaArtwork(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	kind := r.URL.Query().Get("kind")
	switch kind {
	case "poster", "backdrop", "banner", "logo":
	default:
		writeErr(w, 400, "that isn't a kind of artwork")
		return
	}
	type cand struct {
		URL    string `json:"url"`
		Source string `json:"source"`
		Label  string `json:"sourceLabel"`
		Now    bool   `json:"selected"`
	}
	out := []cand{}
	seen := map[string]bool{}
	cur := t.Field(kind)
	add := func(u, src string) {
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		out = append(out, cand{u, src, library.SourceLabel(src), u == cur})
	}
	add(cur, t.SourceOf(kind))
	dir := filepath.Dir(t.Path())
	if t.Show != nil {
		dir = t.Show.Path
	} else if strings.HasSuffix(t.Item.Kind, "-folder") {
		dir = t.Item.Path
	}
	if t.Item == nil || !library.SharesFolder(dir, t.Item.Path) {
		for _, c := range localImages(dir) {
			add(c.URL, library.SourceLocal)
		}
	} else {
		for _, c := range gameImages(dir, t.Item.Path) {
			add(c.URL, library.SourceLocal)
		}
	}
	warnings := []string{}
	all, warn := s.Scanner.ArtworkFor(ctx, t, kind)
	warnings = append(warnings, warn...)
	for _, c := range all {
		add(c.URL, c.Source)
	}
	writeJSON(w, 200, map[string]any{"candidates": out, "warnings": warnings})
}

// POST /api/meta/{target}/{id}/link {link: extra|version, extraType, label, parentItem, parentShow}
func (s *Server) handleMetaLink(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	if t.Item == nil {
		writeErr(w, 400, "a show can't be attached to something")
		return
	}
	var b struct {
		Link       string `json:"link"`
		ExtraType  string `json:"extraType"`
		Label      string `json:"label"`
		ParentItem *int64 `json:"parentItem"`
		ParentShow *int64 `json:"parentShow"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.Store.AttachItem(r.Context(), t.ID, b.Link, b.ExtraType, b.Label, b.ParentItem, b.ParentShow); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, s.buildEditor(r.Context(), t))
}

// DELETE /api/meta/{target}/{id}/link: a title of its own again.
func (s *Server) handleMetaUnlink(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	if err := s.Store.DetachItem(r.Context(), t.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, s.buildEditor(r.Context(), t))
}

// POST /api/meta/{target}/{id}/move {library}
func (s *Server) handleMetaMove(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	var b struct {
		Library int64 `json:"library"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.Store.MoveTitle(r.Context(), t, b.Library); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	t, err := s.Store.LoadTitle(r.Context(), t.Target, t.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, s.buildEditor(r.Context(), t))
}

// DELETE /api/meta/{target}/{id}: takes a title out of its library. Its file stays.
func (s *Server) handleMetaDelete(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	if err := s.Store.RemoveTitle(r.Context(), t); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "path": t.Path(), "libraryId": t.LibraryID()})
}

// GET /api/meta/removed?library=
func (s *Server) handleMetaRemoved(w http.ResponseWriter, r *http.Request) {
	out, err := s.Store.RemovedTitles(r.Context(), int64(qInt(r, "library", 0)))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"removed": out})
}

// POST /api/meta/removed/restore {path}
func (s *Server) handleMetaRestore(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.Store.RestoreRemoved(r.Context(), b.Path); err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeErr(w, 404, "that wasn't removed")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/meta/undo {batch}
func (s *Server) handleMetaUndo(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Batch string `json:"batch"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	n, err := s.Store.Undo(r.Context(), b.Batch)
	switch {
	case errors.Is(err, library.ErrNotFound):
		writeErr(w, 404, "there's nothing to undo")
	case errors.Is(err, library.ErrAlreadyUndone):
		writeErr(w, 409, err.Error())
	case err != nil:
		writeErr(w, 500, err.Error())
	default:
		writeJSON(w, 200, map[string]any{"undone": n})
	}
}

// GET /api/meta/history?limit=
func (s *Server) handleMetaHistory(w http.ResponseWriter, r *http.Request) {
	out, err := s.Store.History(r.Context(), qInt(r, "limit", 50))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"history": out})
}

const maxBulk = 500

type bulkRef struct {
	Target string `json:"target"`
	ID     int64  `json:"id"`
}

// POST /api/meta/bulk {titles: [{target, id}], set: {field: value}, addGenres, removeGenres, addTags, removeTags, lock, unlock}
func (s *Server) handleMetaBulk(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Titles       []bulkRef         `json:"titles"`
		Set          map[string]string `json:"set"`
		AddGenres    []string          `json:"addGenres"`
		RemoveGenres []string          `json:"removeGenres"`
		AddTags      []string          `json:"addTags"`
		RemoveTags   []string          `json:"removeTags"`
		Lock         []string          `json:"lock"`
		Unlock       []string          `json:"unlock"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(b.Titles) == 0 || len(b.Titles) > maxBulk {
		writeErr(w, 400, "choose from 1 to 500 titles")
		return
	}
	ctx := r.Context()
	for _, f := range []string{"poster", "backdrop", "banner", "logo"} {
		if v, on := b.Set[f]; on && !s.validArt(ctx, v) {
			writeErr(w, 400, "that picture isn't a web address or a file in one of your libraries")
			return
		}
	}
	batch := library.NewBatch()
	done := 0
	failed := []string{}
	for _, ref := range b.Titles {
		t, err := s.Store.LoadTitle(ctx, ref.Target, ref.ID)
		if err != nil {
			continue
		}
		if err := s.Store.Remember(ctx, batch, userName(r), "Edited together", t, t.Snapshot()); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		changes := map[string]string{}
		for f, v := range b.Set {
			if t.HasField(f) {
				changes[f] = v
			}
		}
		if t.HasField("genres") && (len(b.AddGenres) > 0 || len(b.RemoveGenres) > 0) {
			changes["genres"] = library.ListChange(t.Field("genres"), b.AddGenres, b.RemoveGenres)
		}
		if t.HasField("tags") && (len(b.AddTags) > 0 || len(b.RemoveTags) > 0) {
			changes["tags"] = library.ListChange(t.Field("tags"), b.AddTags, b.RemoveTags)
		}
		if err := t.EditFields(changes, true); err != nil {
			failed = append(failed, t.Name()+": "+err.Error())
			continue
		}
		for _, f := range b.Lock {
			if t.HasField(f) {
				t.Meta.Locks[f] = true
			}
		}
		for _, f := range b.Unlock {
			delete(t.Meta.Locks, f)
		}
		if len(changes) > 0 {
			s.Scanner.Persist(ctx, t)
		}
		if err := s.Store.SaveTitle(ctx, t); err != nil {
			failed = append(failed, t.Name()+": "+err.Error())
			continue
		}
		done++
	}
	writeJSON(w, 200, map[string]any{"done": done, "failed": failed, "batch": batch})
}

// GET /api/match/queue?library=&state=&search=&limit=&offset=
func (s *Server) handleMatchQueue(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := s.Scanner.MatchQueue(r.Context(), int64(qInt(r, "library", 0)), q.Get("state"), q.Get("search"), qInt(r, "limit", 100), qInt(r, "offset", 0))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// POST /api/meta/{target}/{id}/suggest
func (s *Server) handleMetaSuggest(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	out, err := s.Scanner.Suggest(r.Context(), t)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	if out == nil {
		out = []library.MatchCandidate{}
	}
	writeJSON(w, 200, map[string]any{"suggestions": out})
}

// POST /api/match/suggest-all {library}; GET /api/match/suggest-all; DELETE stops it.
func (s *Server) handleSuggestAll(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Library int64 `json:"library"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	started := s.Scanner.StartSuggesting(b.Library)
	writeJSON(w, 202, map[string]any{"started": started, "job": library.SuggestProgress()})
}

func (s *Server) handleSuggestProgress(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, library.SuggestProgress())
}

func (s *Server) handleSuggestStop(w http.ResponseWriter, r *http.Request) {
	library.StopSuggesting()
	writeJSON(w, 200, library.SuggestProgress())
}

// POST /api/match/accept {choices: [{target, id, source, sourceId}]}: several at once, undone together.
func (s *Server) handleMatchAccept(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Choices []struct {
			Target   string `json:"target"`
			ID       int64  `json:"id"`
			Source   string `json:"source"`
			SourceID string `json:"sourceId"`
		} `json:"choices"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(b.Choices) == 0 || len(b.Choices) > 100 {
		writeErr(w, 400, "choose from 1 to 100 titles at a time")
		return
	}
	ctx := r.Context()
	done, batches, failed := 0, []string{}, []string{}
	for _, c := range b.Choices {
		t, err := s.Store.LoadTitle(ctx, c.Target, c.ID)
		if err != nil {
			continue
		}
		batch, err := s.Scanner.ApplyMatch(ctx, t, c.Source, c.SourceID, nil, userName(r))
		if err != nil {
			failed = append(failed, t.Name()+": "+err.Error())
			continue
		}
		batches = append(batches, batch)
		done++
	}
	writeJSON(w, 200, map[string]any{"done": done, "failed": failed, "batches": batches})
}

// POST /api/meta/{target}/{id}/leave: it has no match, and needs no more looking at.
func (s *Server) handleMetaLeave(w http.ResponseWriter, r *http.Request) {
	t, ok := s.titleFromPath(w, r)
	if !ok {
		return
	}
	var b struct {
		Keep bool `json:"keep"` // keep what it has, and only stop asking about it
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	batch := library.NewBatch()
	if err := s.Store.Remember(ctx, batch, userName(r), "Left alone", t, t.Snapshot()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if b.Keep {
		t.Meta.MatchState = library.MatchManual
	} else {
		t.Meta.MatchState = library.MatchNone
	}
	if err := s.Store.SaveTitle(ctx, t); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"batch": batch})
}

// GET /api/match/rules?library=; PUT /api/match/rules {library, rules}; POST /api/match/rules/try {library, rules}
func (s *Server) handleMatchRules(w http.ResponseWriter, r *http.Request) {
	out, err := s.Store.MatchRules(r.Context(), int64(qInt(r, "library", 0)))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"rules": out})
}

type rulesBody struct {
	Library int64               `json:"library"`
	Rules   []library.MatchRule `json:"rules"`
}

func (s *Server) handleMatchRulesPut(w http.ResponseWriter, r *http.Request) {
	var b rulesBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := s.Store.Library(r.Context(), b.Library); err != nil {
		writeErr(w, 404, "library not found")
		return
	}
	out, err := s.Store.SetMatchRules(r.Context(), b.Library, b.Rules)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"rules": out})
}

func (s *Server) handleMatchRulesTry(w http.ResponseWriter, r *http.Request) {
	var b rulesBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(b.Rules) > 100 {
		writeErr(w, 400, "too many rules")
		return
	}
	out, err := s.Store.TryRules(r.Context(), b.Library, b.Rules)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"matches": out})
}

// PUT /api/meta/show/{id}/episodes {episodes: [{id, season, episode, episodeEnd}]}: renumbering.
func (s *Server) handleEpisodesRemap(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b struct {
		Episodes []library.EpisodeNumber `json:"episodes"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	batch, err := s.Scanner.RemapEpisodes(r.Context(), id, b.Episodes, userName(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"batch": batch})
}

// GET /api/meta/show/{id}/episodes: the show's files beside what its source lists.
func (s *Server) handleEpisodesList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	out, err := s.Scanner.EpisodeMap(r.Context(), id)
	if err != nil {
		writeErr(w, sourceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, out)
}
