// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
)

// SourceOf says where a field's value came from.
func (t *Title) SourceOf(f string) string { return t.sourceOf(f) }

// SaveMeta stores a title's row by itself.
func (s *Store) SaveMeta(ctx context.Context, target string, id int64, m *TitleMeta) error {
	return s.saveTitleMeta(ctx, target, id, m)
}

// HasField reports whether a title has this field.
func (t *Title) HasField(f string) bool { return t.hasField(f) }

// Snapshot is a title's state now, for Remember.
func (t *Title) Snapshot() titleSnapshot { return t.snapshot() }

// Persist writes a title's metadata beside its files, if the owner wants that.
func (s *Scanner) Persist(ctx context.Context, t *Title) { s.persist(ctx, t) }

// PictureExt names what kind of picture some bytes are, or "".
func PictureExt(b []byte) string {
	if !isPicture(b) {
		return ""
	}
	return map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}[http.DetectContentType(b)]
}

// maxArtPixels is the largest picture an upload may be: big enough for any poster or
// backdrop, small enough to decode on a modest machine.
const maxArtPixels = 25_000_000

// CleanPicture reads an uploaded picture and writes it again from its pixels, so nothing
// but the picture survives: no location or camera details, no trailing data. A picture
// with see-through parts (a logo) becomes a PNG, which keeps them; anything else a JPEG,
// which keeps a large photo small.
func CleanPicture(b []byte) (out []byte, ext string, err error) {
	defer func() {
		if p := recover(); p != nil { // decoders see whatever was sent
			out, ext, err = nil, "", fmt.Errorf("this picture couldn't be read")
		}
	}()
	if PictureExt(b) == "" {
		return nil, "", errors.New("use a JPEG, PNG, WebP or GIF picture")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, "", errors.New("use a JPEG, PNG, WebP or GIF picture")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxArtPixels {
		return nil, "", errors.New("the picture is too big: at most 25 megapixels")
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, "", errors.New("this picture couldn't be read")
	}
	var buf bytes.Buffer
	if format == "jpeg" || opaque(img) {
		err, ext = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}), ".jpg"
	} else {
		err, ext = png.Encode(&buf, img), ".png"
	}
	if err != nil {
		return nil, "", err
	}
	return buf.Bytes(), ext, nil
}

// opaque: nothing in the picture can be seen through.
func opaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return false
}

// Frame is a picture from about a fifth of the way into a video or a disc's main
// feature, kept in the image cache: the frame a title's poster is made from when nothing
// else has one, large enough to be a backdrop.
func (s *Scanner) Frame(ctx context.Context, it *Item) (string, error) {
	if it.Disc != nil && it.Disc.Scrambled {
		return "", errors.New("this disc is encrypted, so no picture can be taken from it")
	}
	switch it.Kind {
	case "video", "dvd-iso", "dvd-folder", "bluray-iso", "bluray-folder":
	default:
		return "", errors.New("only a video or a disc has frames to take")
	}
	return s.frameAt(ctx, it, "frame-", 1280)
}

// SharesFolder: other media are beside this file, so the folder's pictures aren't all its own.
func SharesFolder(dir, path string) bool { return !soleMedia(dir, path) }

// ListChange adds to and takes from a list held as text.
func ListChange(cur string, add, remove []string) string {
	gone := map[string]bool{}
	for _, r := range remove {
		gone[strings.ToLower(strings.TrimSpace(r))] = true
	}
	var out []string
	for _, v := range splitList(cur) {
		if !gone[strings.ToLower(v)] {
			out = append(out, v)
		}
	}
	return joinList(append(out, add...))
}

// ArtCandidate is a picture a source has.
type ArtCandidate struct {
	URL    string
	Source string
}

// ArtworkFor asks the title's match, then whatever else is on, for pictures of it. The
// answer can't be limited to one service: the point is to see all of them.
func (s *Scanner) ArtworkFor(ctx context.Context, t *Title, kind string) ([]ArtCandidate, []string) {
	var out []ArtCandidate
	var warnings []string
	pick := func(d *MatchDetails) []string {
		switch kind {
		case "poster":
			return append([]string{d.Poster}, d.Posters...)
		case "backdrop":
			return append([]string{d.Backdrop}, d.Backdrops...)
		case "banner":
			return append([]string{d.Banner}, d.Banners...)
		case "logo":
			return append([]string{d.Logo}, d.Logos...)
		}
		return nil
	}
	seen := map[string]bool{}
	ask := func(source, id string) {
		if seen[source] {
			return
		}
		seen[source] = true
		d, err := s.detailsOf(ctx, source, id)
		if err != nil {
			warnings = append(warnings, SourceLabel(source)+" couldn't answer just now.")
			return
		}
		for _, u := range pick(d) {
			if u != "" && len(out) < 120 {
				out = append(out, ArtCandidate{u, source})
			}
		}
	}
	provider, id := t.Provider()
	if provider != "" && id != "" && s.matchSourceNamed(provider) != nil {
		ask(provider, id)
	}
	if t.Meta.MatchState == MatchNone {
		return out, warnings
	}
	// The best search result on each other source that has pictures.
	res, err := s.SearchMatches(ctx, t, "", nil)
	if err != nil {
		return out, warnings
	}
	for _, c := range res.Results {
		if c.Score >= autoMatchScore {
			ask(c.Source, c.ID)
		}
	}
	if len(out) == 0 && kind == "poster" {
		key := ""
		if s.TMDBKey != nil {
			key = s.TMDBKey()
		}
		if key == "" {
			warnings = append(warnings, "Add a free TMDB key under Metadata agents for film artwork. Without one, films have only what's in their folder.")
		}
	}
	return out, warnings
}

// EpisodeNumber is a new number for a file.
type EpisodeNumber struct {
	ID         int64 `json:"id"`
	Season     *int  `json:"season"`
	Episode    *int  `json:"episode"`
	EpisodeEnd *int  `json:"episodeEnd"`
}

// RemapEpisodes renumbers a show's files, locks the numbers so a scan keeps them, and fills each
// from what its source says that number is. Only files of this show can be touched.
func (s *Scanner) RemapEpisodes(ctx context.Context, showID int64, list []EpisodeNumber, user string) (string, error) {
	if len(list) == 0 || len(list) > 5000 {
		return "", errors.New("nothing to renumber")
	}
	sh, err := s.Store.Show(ctx, showID, 0)
	if err != nil {
		return "", err
	}
	var eps map[[2]int]EpisodeMeta
	if lister, ok := s.matchSourceNamed(sh.Provider).(SeriesLister); ok && sh.ProviderID != "" {
		if eps, err = lister.Episodes(ctx, sh.ProviderID); err != nil {
			s.Log.WarnContext(ctx, "could not read a show's episodes", "show", sh.Title, "err", err)
		}
	}
	batch := NewBatch()
	for _, e := range list {
		t, err := s.Store.LoadTitle(ctx, TargetItem, e.ID)
		if err != nil || t.Item.ShowID == nil || *t.Item.ShowID != showID {
			return batch, fmt.Errorf("that file (%d) isn't an episode of this show", e.ID)
		}
		if intStr(t.Item.Season) == intStr(e.Season) && intStr(t.Item.Episode) == intStr(e.Episode) && intStr(t.Item.EpisodeEnd) == intStr(e.EpisodeEnd) {
			continue
		}
		if err := s.Store.Remember(ctx, batch, user, "Renumbered episodes", t, t.snapshot()); err != nil {
			return batch, err
		}
		if err := t.EditFields(map[string]string{"season": intStr(e.Season), "episode": intStr(e.Episode), "episodeEnd": intStr(e.EpisodeEnd)}, true); err != nil {
			return batch, err
		}
		if eps != nil && t.Item.Season != nil && t.Item.Episode != nil {
			prev := *t.Item
			applyEpisodeMeta(t.Item, sh.Provider, eps, false)
			t.Item.SortTitle = SortTitle(t.Item.Title)
			s.Store.keepLockedItem(ctx, t.Item, &prev, t.Meta)
		}
		if err := s.Store.SaveTitle(ctx, t); err != nil {
			return batch, err
		}
	}
	return batch, nil
}

// EpisodeRow is a file beside what the source says its number is.
type EpisodeRow struct {
	ID         int64  `json:"id"`
	File       string `json:"file"`
	Title      string `json:"title"`
	Season     *int   `json:"season"`
	Episode    *int   `json:"episode"`
	EpisodeEnd *int   `json:"episodeEnd,omitempty"`
	Locked     bool   `json:"locked"`
	Known      string `json:"known,omitempty"` // what the source calls this number
}

// EpisodeListing is one number on the source.
type EpisodeListing struct {
	Season  int    `json:"season"`
	Episode int    `json:"episode"`
	Title   string `json:"title"`
	Aired   string `json:"aired,omitempty"`
}

// EpisodeMapResult is everything renumbering needs.
type EpisodeMapResult struct {
	Rows   []EpisodeRow     `json:"rows"`
	Source string           `json:"source"`
	Label  string           `json:"sourceLabel"`
	Listed []EpisodeListing `json:"listed"`
	Note   string           `json:"note,omitempty"`
}

// EpisodeMap lists a show's files with what its source calls each number.
func (s *Scanner) EpisodeMap(ctx context.Context, showID int64) (*EpisodeMapResult, error) {
	sh, err := s.Store.Show(ctx, showID, 0)
	if err != nil {
		return nil, err
	}
	out := &EpisodeMapResult{Rows: []EpisodeRow{}, Listed: []EpisodeListing{}, Source: sh.Provider, Label: SourceLabel(sh.Provider)}
	var eps map[[2]int]EpisodeMeta
	if lister, ok := s.matchSourceNamed(sh.Provider).(SeriesLister); ok && sh.ProviderID != "" {
		if eps, err = lister.Episodes(ctx, sh.ProviderID); err != nil {
			out.Note = SourceLabel(sh.Provider) + " couldn't answer just now, so its episode names aren't shown."
		}
	} else {
		out.Note = "This show has no match, so there are no episode names to compare with."
	}
	items, _, err := s.Store.Items(ctx, ItemQuery{ShowID: showID, Sort: "episode", NoTotal: true, Brief: true, IncludeAttached: true})
	if err != nil {
		return nil, err
	}
	metas, err := s.Store.allTitleMeta(ctx)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		r := EpisodeRow{ID: it.ID, File: baseOf(it.Path), Title: it.Title, Season: it.Season, Episode: it.Episode, EpisodeEnd: it.EpisodeEnd, Locked: metas.get(TargetItem, it.ID).Locks["episode"]}
		if it.Season != nil && it.Episode != nil {
			if e, ok := eps[[2]int{*it.Season, *it.Episode}]; ok {
				r.Known = cleanText(e.Title, 200, false)
			}
		}
		out.Rows = append(out.Rows, r)
	}
	for k, e := range eps {
		if len(out.Listed) >= 5000 {
			break
		}
		out.Listed = append(out.Listed, EpisodeListing{Season: k[0], Episode: k[1], Title: cleanText(e.Title, 200, false), Aired: cleanText(e.Aired, 20, false)})
	}
	sortListed(out.Listed)
	return out, nil
}

func baseOf(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func sortListed(l []EpisodeListing) {
	// A short sort of a short list: seasons, then numbers.
	for i := 1; i < len(l); i++ {
		for j := i; j > 0 && (l[j-1].Season > l[j].Season || (l[j-1].Season == l[j].Season && l[j-1].Episode > l[j].Episode)); j-- {
			l[j-1], l[j] = l[j], l[j-1]
		}
	}
}
