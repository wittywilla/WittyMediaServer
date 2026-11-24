// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

// Converting a library's sidecars to .wittyinfo files and back (first-run setup, and
// Settings, Metadata). Gathering puts what lies beside each title in its .wittyinfo: the
// NFO's details, the pictures, a .skip.json's or .edl's skips and, if asked, its text
// subtitle files; a show's tvshow.nfo and pictures go in show.wittyinfo and its seasons'
// posters in seasonNN.wittyinfo. Spreading writes them back out: NFO files and pictures in
// the usual layout (nfo_write.go), a .skip.json and the subtitles. What a .wittyinfo already
// says is never replaced by what lies beside it.
//
// The old files stay, unless Retire is set: then each one taken in (or the .wittyinfo, when
// spreading) is renamed to <name>.bak beside it, which a scan doesn't read and a person can
// rename back.

// ConvertOptions says how to convert.
type ConvertOptions struct {
	// Subtitles: also take in (or write out) text subtitle files.
	Subtitles bool
	// Retire: rename what was converted from to .bak.
	Retire bool
	// Beside: .wittyinfo files may be written beside the media; otherwise the server keeps
	// them. Spreading always writes beside the media, so it needs it.
	Beside bool
	// Author and Tool go in the files' authorship.
	Author, Tool, ToolVersion string
	// Format: "wittyinfo" (films and shows) or "wittyaudio" (songs and albums); "" both.
	Format string
}

func (o ConvertOptions) does(format string) bool { return o.Format == "" || o.Format == format }

// ConvertReport says what a conversion did.
type ConvertReport struct {
	Titles    int `json:"titles"`    // films and episodes looked at
	Shows     int `json:"shows"`     // shows (and their seasons) looked at
	Converted int `json:"converted"` // titles, shows and songs that gained something
	// Each kind on its own, so a result can say what it counted: films and episodes, shows,
	// songs (their lyrics) and albums (cover and album.nfo).
	TitlesConverted int      `json:"titlesConverted"`
	ShowsConverted  int      `json:"showsConverted"`
	Songs           int      `json:"songs"`
	SongsConverted  int      `json:"songsConverted"`
	Albums          int      `json:"albums"`
	AlbumsConverted int      `json:"albumsConverted"`
	Unchanged       int      `json:"unchanged"` // had nothing to convert
	Failed          int      `json:"failed"`
	Retired         int      `json:"retired"` // files renamed to .bak
	Errors          []string `json:"errors,omitempty"`
}

func (r *ConvertReport) fail(what string, err error) {
	r.Failed++
	if len(r.Errors) < 20 {
		r.Errors = append(r.Errors, fmt.Sprintf("%s: %v", what, err))
	}
}

// ConvertCounts says how much a conversion would find, for setup to offer it.
type ConvertCounts struct {
	Titles          int `json:"titles"`
	WithNFO         int `json:"withNfo"`
	WithArt         int `json:"withArt"`
	WithSkips       int `json:"withSkips"`
	WithInfo        int `json:"withInfo"`
	Shows           int `json:"shows"`
	ShowsInfo       int `json:"showsWithInfo"`
	ShowsNFO        int `json:"showsWithNfo"`
	Songs           int `json:"songs"`
	SongsWithLyrics int `json:"songsWithLyrics"`
	SongsWithInfo   int `json:"songsWithInfo"`
	Albums          int `json:"albums"`
	AlbumsWithArt   int `json:"albumsWithArt"`
	AlbumsWithInfo  int `json:"albumsWithInfo"`
	Libraries       int `json:"libraries"`
	ReadOnly        int `json:"readOnlyLibraries"`
	TookMillis      int `json:"tookMs"`
}

// convertKinds are the items a .wittyinfo is for.
const convertKinds = `('video', 'dvd-folder', 'dvd-iso', 'bluray-folder', 'bluray-iso')`

func (s *Store) convertItemIDs(ctx context.Context, libs []int64) ([]int64, error) {
	return s.idsIn(ctx, `SELECT id FROM items WHERE kind IN `+convertKinds+` AND library_id = ? ORDER BY id`, libs)
}

func (s *Store) convertShowIDs(ctx context.Context, libs []int64) ([]int64, error) {
	return s.idsIn(ctx, `SELECT id FROM shows WHERE library_id = ? ORDER BY id`, libs)
}

func (s *Store) idsIn(ctx context.Context, q string, libs []int64) ([]int64, error) {
	var out []int64
	for _, lib := range libs {
		rows, err := s.DB.QueryContext(ctx, q, lib)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				out = append(out, id)
			}
		}
		rows.Close()
	}
	return out, nil
}

// ------------------------------------------------------------------ what lies beside

// besideFiles is what lies beside one title that a .wittyinfo can hold.
type besideFiles struct {
	nfo       string
	art       map[string]string   // role: path
	more      map[string][]string // role: further pictures (Film-poster-2.jpg, extrafanart/...)
	skipJSON  string
	edl       string
	subtitles []SubtitleFile
}

func (b *besideFiles) any() bool {
	return b.nfo != "" || len(b.art) > 0 || len(b.more) > 0 || b.skipJSON != "" || b.edl != "" || len(b.subtitles) > 0
}

// besideItem finds what lies beside a film or an episode.
func (s *Store) besideItem(ctx context.Context, it *Item, subtitles bool) besideFiles {
	out := besideFiles{art: map[string]string{}}
	if it.Path == "" || strings.Contains(it.Path, "://") {
		return out
	}
	dir, base := filepath.Dir(it.Path), strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path))
	if it.Kind == "dvd-folder" || it.Kind == "bluray-folder" {
		dir, base = it.Path, filepath.Base(it.Path)
	}
	lib, _ := s.Library(ctx, it.LibraryID)
	if it.ShowID != nil {
		if p := filepath.Join(dir, base+".nfo"); nfoUsable(lib, p) && fileThere(p) {
			out.nfo = p
		}
		if p := findArt(dir, base+"-thumb", base); p != "" {
			out.art["thumb"] = p
		}
	} else {
		for _, p := range usableNFOs(lib, filmNFOs(it.Path, dir, base)) {
			if fileThere(p) {
				out.nfo = p
				break
			}
		}
		poster, backdrop := localArtwork(dir, base)
		if poster != "" {
			out.art["poster"] = poster
		}
		if backdrop != "" {
			out.art["backdrop"] = backdrop
		}
		alone := soleMedia(dir, it.Path)
		for role, names := range map[string][]string{
			"logo": {base + "-clearlogo", base + "-logo"}, "clearart": {base + "-clearart"}, "banner": {base + "-banner"}, "disc": {base + "-disc", base + "-discart"},
		} {
			if alone {
				names = append(names, strings.TrimPrefix(names[0], base+"-"), role)
			}
			if p := findArt(dir, names...); p != "" {
				out.art[role] = p
			}
		}
		names := map[string][]string{}
		for _, role := range []string{"poster", "backdrop", "logo", "clearart", "banner", "disc"} {
			names[role] = []string{base + "-" + extraArtName(role)}
			if alone {
				names[role] = append(names[role], extraArtName(role))
			}
		}
		out.more = morePictures(dir, names, alone, out.art)
	}
	for _, p := range sidecarPaths(it.Path, ".skip.json") {
		if fileThere(p) {
			out.skipJSON = p
			break
		}
	}
	for _, p := range sidecarPaths(it.Path, ".edl") {
		if fileThere(p) {
			out.edl = p
			break
		}
	}
	if subtitles && it.Kind == "video" {
		for _, f := range FindSubtitles(it.Path) {
			ext := strings.ToLower(filepath.Ext(f.Path))
			if wittyinfo.SubtitleExts[ext] {
				out.subtitles = append(out.subtitles, f)
			}
		}
	}
	return out
}

func sidecarPaths(mediaPath, ext string) []string {
	trimmed := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	if trimmed == mediaPath {
		return []string{mediaPath + ext}
	}
	return []string{trimmed + ext, mediaPath + ext}
}

func fileThere(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// metadataFromNFO reads an NFO's details as a .wittyinfo's metadata.
func metadataFromNFO(path string, episode bool) *wittyinfo.Metadata {
	var scratch Item
	ok := false
	if episode {
		ok = applyEpisodeNFO(&scratch, path)
	} else {
		ok = applyNFO(&scratch, path)
	}
	if !ok {
		return nil
	}
	m := &wittyinfo.Metadata{Title: scratch.Title, Overview: scratch.Overview, Tagline: scratch.Tagline, ContentRating: scratch.ContentRating,
		Genres: append([]string(nil), scratch.Genres...)}
	if scratch.SortTitle != "" && scratch.SortTitle != SortTitle(scratch.Title) {
		m.SortTitle = scratch.SortTitle
	}
	if scratch.Year != nil {
		y := *scratch.Year
		m.Year = &y
	}
	if scratch.Studio != "" {
		m.Studios = []string{scratch.Studio}
	}
	if scratch.Provider != "" && scratch.ProviderID != "" {
		m.ExternalIDs = map[string]string{scratch.Provider: scratch.ProviderID}
	}
	if len(m.Genres) > wittyinfo.MaxListItems {
		m.Genres = m.Genres[:wittyinfo.MaxListItems]
	}
	return m
}

// fillMetadata takes into have what it doesn't say yet.
func fillMetadata(have, from *wittyinfo.Metadata) *wittyinfo.Metadata {
	if from == nil {
		return have
	}
	if have == nil {
		return from
	}
	set := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}
	set(&have.Title, from.Title)
	set(&have.SortTitle, from.SortTitle)
	set(&have.Overview, from.Overview)
	set(&have.Tagline, from.Tagline)
	set(&have.ContentRating, from.ContentRating)
	if have.Year == nil {
		have.Year = from.Year
	}
	if len(have.Genres) == 0 {
		have.Genres = from.Genres
	}
	if len(have.Studios) == 0 {
		have.Studios = from.Studios
	}
	if len(have.ExternalIDs) == 0 {
		have.ExternalIDs = from.ExternalIDs
	}
	return have
}

// segmentsFrom are a .skip.json's or an .edl's skips as segments.
func segmentsFrom(m mediaextras.Markers) []wittyinfo.Segment {
	var out []wittyinfo.Segment
	add := func(typ string, r *mediaextras.Range) {
		if r == nil {
			return
		}
		seg := wittyinfo.Segment{Type: typ, Start: wittyinfo.Seconds(r.StartSec), Label: r.Label}
		if r.EndSec > r.StartSec {
			seg.End = wittyinfo.Sec(r.EndSec)
		}
		out = append(out, seg)
	}
	add(wittyinfo.SegRecap, m.Recap)
	add(wittyinfo.SegIntro, m.Intro)
	add(wittyinfo.SegCredits, m.Credits)
	for i := range m.Commercials {
		add(wittyinfo.SegCommercial, &m.Commercials[i])
	}
	return out
}

// ------------------------------------------------------------------ gathering

// takePicture puts a picture file in a .wittyinfo for a role, unless it has one.
func takePicture(x *wittyinfo.Info, role, path string) (bool, error) {
	if x.Artwork.First(role) != "" {
		return false, nil
	}
	b, err := readSmall(path, wittyinfo.MaxImage)
	if err != nil {
		return false, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".jpeg" {
		ext = ".jpg"
	}
	if _, err := x.SetImage(role, ext, b); err != nil {
		return false, err
	}
	return true, nil
}

// gatherInto takes what lies beside into x. It says what it took, by file.
func (s *Store) gatherInto(x *wittyinfo.Info, b besideFiles, episode bool, duration float64) (used []string, err error) {
	var errs []error
	if b.nfo != "" {
		if m := metadataFromNFO(b.nfo, episode); m != nil {
			x.Metadata = fillMetadata(x.Metadata, m)
			used = append(used, b.nfo)
		}
	}
	for _, role := range sortedRoles(b.art) {
		p := b.art[role]
		ok, err := takePicture(x, role, p)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(p), err))
			continue
		}
		if ok {
			used = append(used, p)
		}
	}
	for _, role := range wittyinfo.ArtworkRoles {
		if n, err := takeMorePictures(x, role, b.more[role]); err != nil {
			errs = append(errs, err)
		} else if n > 0 {
			used = append(used, b.more[role][:n]...)
		}
	}
	if len(x.Segments) == 0 {
		var m mediaextras.Markers
		var from string
		if b.skipJSON != "" {
			if raw, err := readSmall(b.skipJSON, mediaextras.MaxSidecar); err == nil {
				if mm, err := mediaextras.ParseSkipJSON(raw, duration); err == nil {
					m, from = mm, b.skipJSON
				}
			}
		}
		if from == "" && b.edl != "" {
			if raw, err := readSmall(b.edl, mediaextras.MaxSidecar); err == nil {
				m, from = mediaextras.ParseEDL(raw), b.edl
			}
		}
		if segs := segmentsFrom(m); len(segs) > 0 {
			x.Segments = segs
			used = append(used, from)
		}
	}
	have := map[string]bool{}
	for _, sub := range x.Subtitles {
		have[strings.ToLower(sub.Language)+"|"+fmt.Sprint(sub.Forced, sub.SDH)] = true
	}
	for i, f := range b.subtitles {
		key := strings.ToLower(f.Language) + "|" + fmt.Sprint(f.Forced, f.SDH)
		if have[key] || len(x.Subtitles) >= 100 {
			continue
		}
		raw, err := readSmall(f.Path, wittyinfo.MaxSubtitle)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(f.Path), err))
			continue
		}
		name := fmt.Sprintf("subtitles/%02d", i+1)
		if f.Language != "" {
			name += "." + strings.ToLower(f.Language)
		}
		if f.Forced {
			name += ".forced"
		}
		if f.SDH {
			name += ".sdh"
		}
		name += strings.ToLower(filepath.Ext(f.Path))
		if !wittyinfo.ValidMemberName(name) {
			continue
		}
		if x.Files == nil {
			x.Files = map[string][]byte{}
		}
		x.Files[name] = raw
		x.Subtitles = append(x.Subtitles, wittyinfo.Subtitle{File: name, Language: f.Language, Title: f.Title, Forced: f.Forced, SDH: f.SDH})
		have[key] = true
		used = append(used, f.Path)
	}
	return used, errors.Join(errs...)
}

func sortedRoles(m map[string]string) []string {
	var out []string
	for _, r := range wittyinfo.ArtworkRoles {
		if m[r] != "" {
			out = append(out, r)
		}
	}
	return out
}

// saveConverted keeps a converted file: beside the media when allowed, else in the server.
func (s *Store) saveConverted(ctx context.Context, t InfoTarget, l LoadedInfo, x *wittyinfo.Info, o ConvertOptions) error {
	x.Stamp(o.Author, o.Tool, o.ToolVersion, time.Now())
	b, err := x.Encode()
	if err != nil {
		return err
	}
	if o.Beside && l.Path != "" {
		if l.FileFound && l.FileErr != nil {
			_ = s.RetireInfoFile(ctx, l.Path)
		}
		if err := s.WriteInfoFile(ctx, l.Path, b); err == nil {
			return s.SetStoredInfo(ctx, t, nil)
		}
	}
	return s.SetStoredInfo(ctx, t, b)
}

// retire renames files to <name>.bak, inside the libraries only.
func (s *Store) retire(ctx context.Context, paths []string) int {
	n := 0
	roots := s.Roots(ctx)
	for _, p := range paths {
		if p == "" || !inRoots(p, roots) || !fileThere(p) {
			continue
		}
		if err := os.Rename(p, p+".bak"); err == nil {
			n++
		}
	}
	return n
}

// GatherInfo takes the sidecars of the given libraries into .wittyinfo files. progress is
// told how far it has got.
func (s *Scanner) GatherInfo(ctx context.Context, libs []int64, o ConvertOptions, progress func(done, total int, what string)) (ConvertReport, error) {
	var rep ConvertReport
	var items, shows, songs []int64
	var err error
	if o.does(wittyinfo.Format) {
		if items, err = s.Store.convertItemIDs(ctx, libs); err != nil {
			return rep, err
		}
		if shows, err = s.Store.convertShowIDs(ctx, libs); err != nil {
			return rep, err
		}
	}
	if o.does(wittyinfo.FormatAudio) {
		if songs, err = s.Store.songIDs(ctx, libs); err != nil {
			return rep, err
		}
	}
	total := len(items) + len(shows) + len(songs)
	done := 0

	for _, id := range items {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		done++
		it, err := s.Store.Item(ctx, id)
		if err != nil || s.Store.libraryReadOnly(ctx, it.LibraryID) {
			continue
		}
		rep.Titles++
		if progress != nil {
			progress(done, total, it.Title)
		}
		b := s.Store.besideItem(ctx, it, o.Subtitles)
		if !b.any() {
			rep.Unchanged++
			continue
		}
		t := InfoTarget{Kind: InfoItem, ID: it.ID}
		l := s.Store.LoadInfo(ctx, t)
		x := wittyinfo.New(wittyinfo.ScopeItem)
		if l.Info != nil {
			x = l.Info.Clone()
		}
		if x.Identity == nil {
			x.Identity = s.Store.IdentityOf(ctx, it)
		}
		used, gerr := s.Store.gatherInto(x, b, it.ShowID != nil, it.RuntimeSec)
		if gerr != nil {
			rep.fail(it.Title, gerr)
		}
		if len(used) == 0 {
			rep.Unchanged++
			continue
		}
		if err := s.Store.saveConverted(ctx, t, l, x, o); err != nil {
			rep.fail(it.Title, err)
			continue
		}
		rep.Converted++
		rep.TitlesConverted++
		_ = s.ApplyInfoNow(ctx, it.ID)
		if o.Retire {
			rep.Retired += s.Store.retire(ctx, used)
		}
	}
	for _, id := range shows {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		done++
		sh, err := s.Store.Show(ctx, id, 0)
		if err != nil || sh.Path == "" || s.Store.libraryReadOnly(ctx, sh.LibraryID) {
			continue
		}
		rep.Shows++
		if progress != nil {
			progress(done, total, sh.Title)
		}
		changed := false
		// The show: tvshow.nfo and the show folder's pictures.
		t := InfoTarget{Kind: InfoShow, ID: sh.ID}
		l := s.Store.LoadInfo(ctx, t)
		x := wittyinfo.New(wittyinfo.ScopeShow)
		if l.Info != nil {
			x = l.Info.Clone()
		}
		b := besideFiles{art: map[string]string{}}
		lib, _ := s.Store.Library(ctx, sh.LibraryID)
		if p := filepath.Join(sh.Path, "tvshow.nfo"); nfoUsable(lib, p) && fileThere(p) {
			b.nfo = p
		}
		for role, names := range map[string][]string{"poster": {"poster", "folder", "cover"}, "backdrop": {"fanart", "backdrop", "background"},
			"logo": {"clearlogo", "logo"}, "banner": {"banner"}, "clearart": {"clearart"}} {
			if p := findArt(sh.Path, names...); p != "" {
				b.art[role] = p
			}
		}
		more := map[string][]string{}
		for _, role := range []string{"poster", "backdrop", "logo", "banner", "clearart"} {
			more[role] = []string{extraArtName(role)}
		}
		b.more = morePictures(sh.Path, more, true, b.art)
		var used []string
		if b.nfo != "" {
			var scratch Show
			if applyShowNFO(&scratch, b.nfo) {
				m := &wittyinfo.Metadata{Title: scratch.Title, Overview: scratch.Overview, ContentRating: scratch.ContentRating, Genres: scratch.Genres, Year: scratch.Year}
				x.Metadata = fillMetadata(x.Metadata, m)
				used = append(used, b.nfo)
			}
		}
		for _, role := range sortedRoles(b.art) {
			if ok, err := takePicture(x, role, b.art[role]); err != nil {
				rep.fail(sh.Title, err)
			} else if ok {
				used = append(used, b.art[role])
			}
		}
		for _, role := range wittyinfo.ArtworkRoles {
			if n, err := takeMorePictures(x, role, b.more[role]); err != nil {
				rep.fail(sh.Title, err)
			} else if n > 0 {
				used = append(used, b.more[role][:n]...)
			}
		}
		if len(used) > 0 {
			if err := s.Store.saveConverted(ctx, t, l, x, o); err != nil {
				rep.fail(sh.Title, err)
			} else {
				changed = true
				if o.Retire {
					rep.Retired += s.Store.retire(ctx, used)
				}
			}
		}
		// Each season's poster, in its own seasonNN.wittyinfo.
		for _, n := range s.Store.seasonsOf(ctx, sh.ID) {
			name := fmt.Sprintf("season%02d", n)
			if n == 0 {
				name = "season-specials"
			}
			p := findArt(sh.Path, name+"-poster", name)
			if p == "" {
				continue
			}
			st := InfoTarget{Kind: InfoSeason, ID: sh.ID, Season: n}
			sl := s.Store.LoadInfo(ctx, st)
			sx := wittyinfo.New(wittyinfo.ScopeSeason)
			if sl.Info != nil {
				sx = sl.Info.Clone()
			}
			sx.Season = &n
			ok, err := takePicture(sx, "poster", p)
			if err != nil {
				rep.fail(fmt.Sprintf("%s, season %d", sh.Title, n), err)
				continue
			}
			if !ok {
				continue
			}
			if err := s.Store.saveConverted(ctx, st, sl, sx, o); err != nil {
				rep.fail(fmt.Sprintf("%s, season %d", sh.Title, n), err)
				continue
			}
			changed = true
			if o.Retire {
				rep.Retired += s.Store.retire(ctx, []string{p})
			}
		}
		if changed {
			rep.Converted++
			rep.ShowsConverted++
		} else {
			rep.Unchanged++
		}
	}
	if o.does(wittyinfo.FormatAudio) {
		err = s.gatherSongs(ctx, libs, o, &rep, func(what string) {
			done++
			if progress != nil {
				progress(done, total, what)
			}
		})
	}
	return rep, err
}

func (s *Store) seasonsOf(ctx context.Context, showID int64) []int {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT season FROM items WHERE show_id = ? AND season IS NOT NULL ORDER BY season`, showID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var n int
		if rows.Scan(&n) == nil {
			out = append(out, n)
		}
	}
	return out
}

// ------------------------------------------------------------------ spreading

// SpreadInfo writes what the given libraries' .wittyinfo files hold back out as NFO files,
// pictures, a .skip.json and subtitle files, in the language the server is in.
func (s *Scanner) SpreadInfo(ctx context.Context, libs []int64, o ConvertOptions, progress func(done, total int, what string)) (ConvertReport, error) {
	var rep ConvertReport
	var items, shows, songs []int64
	var err error
	if o.does(wittyinfo.Format) {
		if items, err = s.Store.convertItemIDs(ctx, libs); err != nil {
			return rep, err
		}
		if shows, err = s.Store.convertShowIDs(ctx, libs); err != nil {
			return rep, err
		}
	}
	if o.does(wittyinfo.FormatAudio) {
		if songs, err = s.Store.songIDs(ctx, libs); err != nil {
			return rep, err
		}
	}
	lang := s.Store.serverLanguage(ctx)
	roots := s.Store.Roots(ctx)
	total, done := len(items)+len(shows)+len(songs), 0
	for _, id := range items {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		done++
		it, err := s.Store.Item(ctx, id)
		if err != nil || s.Store.libraryReadOnly(ctx, it.LibraryID) {
			continue
		}
		rep.Titles++
		if progress != nil {
			progress(done, total, it.Title)
		}
		t := InfoTarget{Kind: InfoItem, ID: it.ID}
		l := s.Store.LoadInfo(ctx, t)
		infos := s.Store.InfosFor(ctx, it)
		x := infos.UsableItem()
		if x == nil {
			rep.Unchanged++
			continue
		}
		x = x.Localize(lang)
		// The details and pictures: the library already has them from the file.
		if err := s.ApplyInfoNow(ctx, it.ID); err != nil {
			rep.fail(it.Title, err)
			continue
		}
		fresh, err := s.Store.Item(ctx, it.ID)
		if err != nil {
			rep.fail(it.Title, err)
			continue
		}
		var errs []error
		if err := s.SaveItemMetadata(ctx, fresh); err != nil {
			errs = append(errs, err)
		}
		// The further pictures, as Film-poster-2.jpg and so on (the first is the one above).
		if err := writeMorePictures(x, filepath.Dir(it.Path), strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path))+"-", roots); err != nil {
			errs = append(errs, err)
		}
		// The skips, as a .skip.json any version of the server reads.
		m := mediaextras.FromWittyinfo(x)
		m.FillEnds(it.RuntimeSec)
		if !m.Empty() && (m.Intro != nil || m.Recap != nil || m.Credits != nil || len(m.Commercials) > 0) {
			body, _ := json.MarshalIndent(m.ToSkipFile(), "", "  ")
			if err := writeBeside(sidecarPaths(it.Path, ".skip.json")[0], append(body, '\n'), roots); err != nil {
				errs = append(errs, err)
			}
		}
		// The subtitles, named as players read them: Film.de.forced.srt.
		base := strings.TrimSuffix(it.Path, filepath.Ext(it.Path))
		for i, sub := range x.Subtitles {
			b := x.Files[sub.File]
			if len(b) == 0 || !o.Subtitles {
				continue
			}
			name := base
			if sub.Language != "" {
				name += "." + sub.Language
			} else {
				name += fmt.Sprintf(".%d", i+1)
			}
			if sub.Forced {
				name += ".forced"
			}
			if sub.SDH {
				name += ".sdh"
			}
			name += strings.ToLower(filepath.Ext(sub.File))
			if fileThere(name) {
				continue
			}
			if err := writeBeside(name, b, roots); err != nil {
				errs = append(errs, err)
			}
		}
		if err := errors.Join(errs...); err != nil {
			rep.fail(it.Title, err)
			continue
		}
		rep.Converted++
		rep.TitlesConverted++
		if o.Retire {
			if l.Origin == "file" {
				rep.Retired += s.Store.retire(ctx, []string{l.Path})
			}
			_ = s.Store.SetStoredInfo(ctx, t, nil)
		}
	}
	for _, id := range shows {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		done++
		sh, err := s.Store.Show(ctx, id, 0)
		if err != nil || s.Store.libraryReadOnly(ctx, sh.LibraryID) {
			continue
		}
		rep.Shows++
		if progress != nil {
			progress(done, total, sh.Title)
		}
		t := InfoTarget{Kind: InfoShow, ID: sh.ID}
		l := s.Store.LoadInfo(ctx, t)
		if l.Info == nil {
			rep.Unchanged++
			continue
		}
		s.applyShowInfo(ctx, sh)
		if err := s.Store.UpsertShow(ctx, sh); err != nil {
			rep.fail(sh.Title, err)
			continue
		}
		if err := s.SaveShowMetadata(ctx, sh); err != nil {
			rep.fail(sh.Title, err)
			continue
		}
		if err := writeMorePictures(l.Info.Localize(lang), sh.Path, "", roots); err != nil {
			rep.fail(sh.Title, err)
			continue
		}
		rep.Converted++
		rep.ShowsConverted++
		if o.Retire {
			if l.Origin == "file" {
				rep.Retired += s.Store.retire(ctx, []string{l.Path})
			}
			_ = s.Store.SetStoredInfo(ctx, t, nil)
		}
	}
	if o.does(wittyinfo.FormatAudio) {
		err = s.spreadSongs(ctx, libs, o, &rep, func(what string) {
			done++
			if progress != nil {
				progress(done, total, what)
			}
		})
	}
	return rep, err
}

// ------------------------------------------------------------------ counting

// CountConvertible says how much the given libraries have to convert either way. It only
// looks at file names: nothing is read.
func (s *Store) CountConvertible(ctx context.Context, libs []int64) (ConvertCounts, error) {
	began := time.Now()
	var c ConvertCounts
	c.Libraries = len(libs)
	for _, lib := range libs {
		if s.libraryReadOnly(ctx, lib) {
			c.ReadOnly++
		}
	}
	items, err := s.convertItemIDs(ctx, libs)
	if err != nil {
		return c, err
	}
	for _, id := range items {
		if ctx.Err() != nil {
			break
		}
		it, err := s.Item(ctx, id)
		if err != nil {
			continue
		}
		c.Titles++
		b := s.besideItem(ctx, it, false)
		if b.nfo != "" {
			c.WithNFO++
		}
		if len(b.art) > 0 || len(b.more) > 0 {
			c.WithArt++
		}
		if b.skipJSON != "" || b.edl != "" {
			c.WithSkips++
		}
		for _, p := range InfoPaths(it) {
			if fileThere(p) {
				c.WithInfo++
				break
			}
		}
	}
	shows, err := s.convertShowIDs(ctx, libs)
	if err != nil {
		return c, err
	}
	for _, id := range shows {
		sh, err := s.Show(ctx, id, 0)
		if err != nil || sh.Path == "" {
			continue
		}
		c.Shows++
		if fileThere(filepath.Join(sh.Path, "tvshow.nfo")) {
			c.ShowsNFO++
		}
		if fileThere(filepath.Join(sh.Path, "show"+wittyinfo.Ext)) {
			c.ShowsInfo++
		}
	}
	if err := s.countSongs(ctx, libs, &c); err != nil {
		return c, err
	}
	c.TookMillis = int(time.Since(began).Milliseconds())
	return c, nil
}

// ------------------------------------------------------------------ further pictures

// extraArtName is a role as picture files beside media are usually named.
func extraArtName(role string) string {
	switch role {
	case "backdrop":
		return "fanart"
	case "logo":
		return "clearlogo"
	case "disc":
		return "discart"
	}
	return role
}

// morePictures finds further pictures for each role in one listing of dir: <name>-2.jpg,
// <name>2.jpg and so on for the names given, and, where the folder is the title's own,
// what an extrafanart folder holds as further backdrops. What first already names is left
// out, and each role holds wittyinfo.MaxPicturesPerRole at most.
func morePictures(dir string, names map[string][]string, own bool, first map[string]string) map[string][]string {
	out := map[string][]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	type found struct {
		n    int
		path string
	}
	byRole := map[string][]found{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !mediatypes.IsArtwork("x" + ext) {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		for role, list := range names {
			for _, want := range list {
				rest, ok := strings.CutPrefix(stem, strings.ToLower(want))
				if !ok {
					continue
				}
				rest = strings.TrimPrefix(rest, "-")
				n, err := strconv.Atoi(rest)
				if err != nil || n < 1 || n > 99 || strconv.Itoa(n) != rest {
					continue
				}
				p := filepath.Join(dir, name)
				if p != first[role] {
					byRole[role] = append(byRole[role], found{n, p})
				}
			}
		}
	}
	if own {
		if extra, err := os.ReadDir(filepath.Join(dir, "extrafanart")); err == nil {
			for i, e := range extra {
				if !e.IsDir() && mediatypes.IsArtwork("x"+strings.ToLower(filepath.Ext(e.Name()))) {
					byRole["backdrop"] = append(byRole["backdrop"], found{1000 + i, filepath.Join(dir, "extrafanart", e.Name())})
				}
			}
		}
	}
	for role, list := range byRole {
		sort.SliceStable(list, func(i, j int) bool { return list[i].n < list[j].n })
		for _, f := range list {
			if len(out[role]) < wittyinfo.MaxPicturesPerRole-1 {
				out[role] = append(out[role], f.path)
			}
		}
	}
	return out
}

// takeMorePictures adds further pictures for a role to a file that has none yet beyond the
// one shown, so that gathering again adds nothing twice. It says how many it took.
func takeMorePictures(x *wittyinfo.Info, role string, paths []string) (int, error) {
	if len(paths) == 0 || len(x.Artwork[role]) > 1 {
		return 0, nil
	}
	n := 0
	var errs []error
	for _, p := range paths {
		b, err := readSmall(p, wittyinfo.MaxImage)
		if err == nil {
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".jpeg" {
				ext = ".jpg"
			}
			_, err = x.AddImage(role, ext, b)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(p), err))
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}

// writeMorePictures writes each role's further pictures beside the media as
// <prefix><name>-2.jpg and on, leaving any file already there alone.
func writeMorePictures(x *wittyinfo.Info, dir, prefix string, roots []string) error {
	var errs []error
	for _, role := range wittyinfo.ArtworkRoles {
		p := x.Artwork[role]
		for i := 1; i < len(p); i++ {
			b := x.Files[p[i]]
			if len(b) == 0 {
				continue
			}
			dst := filepath.Join(dir, prefix+extraArtName(role)+"-"+strconv.Itoa(i+1)+strings.ToLower(filepath.Ext(p[i])))
			if fileThere(dst) {
				continue
			}
			if err := writeBeside(dst, b, roots); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
