// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

// .wittyinfo files (docs/formats/wittyinfo.md): what people add by hand to a film, an
// episode, a season or a show. One is read from beside the media, or from the copy the
// server keeps when edits can't be written there. The kept copy wins: it is what someone
// changed here last, and Export takes it to the disk by hand.
//
//	Film or episode: <name>.wittyinfo (or <name.ext>.wittyinfo) beside the video
//	Season:          <show>/season01.wittyinfo, or <season folder>/season.wittyinfo
//	Show:            <show>/show.wittyinfo

// Wittyinfo targets.
const (
	InfoItem   = "item"
	InfoShow   = "show"
	InfoSeason = "season"
)

// InfoTarget says what a .wittyinfo is about: an item, a show, or a show's season.
type InfoTarget struct {
	Kind   string // item | show | season
	ID     int64  // the item's or the show's id
	Season int    // season only
}

func (t InfoTarget) key() (string, int64, int) {
	if t.Kind == InfoItem {
		return "item", t.ID, -1
	}
	if t.Kind == InfoShow {
		return "show", t.ID, -1
	}
	return "show", t.ID, t.Season
}

// Scope is the scope a file for this target has.
func (t InfoTarget) Scope() string {
	switch t.Kind {
	case InfoShow:
		return wittyinfo.ScopeShow
	case InfoSeason:
		return wittyinfo.ScopeSeason
	}
	return wittyinfo.ScopeItem
}

// LoadedInfo is a .wittyinfo as the server found it.
type LoadedInfo struct {
	Info *wittyinfo.Info
	// Origin: "stored" (the server's copy), "file" (beside the media), or "" for none.
	Origin string
	// Path: the file beside the media, found or where it would go ("" when there is no
	// place for one, such as a stream).
	Path string
	// FileErr: the file beside the media can't be used, and why. A kept copy may still be.
	FileErr error
	// FileFound: there is a file beside the media (used or not).
	FileFound bool
	// Force: someone said to use it although it was made for another cut.
	Force   bool
	Updated int64
}

// ------------------------------------------------------------------ the kept copy

// StoredInfo reads the server's copy for a target: its bytes ("" when there is none) and
// whether it is to be used on another cut.
func (s *Store) StoredInfo(ctx context.Context, t InfoTarget) (body []byte, force bool, updated int64) {
	target, id, season := t.key()
	var f int
	err := s.DB.QueryRowContext(ctx, `SELECT body, force, updated_at FROM wittyinfo WHERE target = ? AND target_id = ? AND season = ?`,
		target, id, season).Scan(&body, &f, &updated)
	if err != nil {
		return nil, false, 0
	}
	return body, f == 1, updated
}

// SetStoredInfo keeps the server's copy for a target; nil forgets it (the file beside the
// media is used again). Whether to use it on another cut is kept as it was.
func (s *Store) SetStoredInfo(ctx context.Context, t InfoTarget, body []byte) error {
	target, id, season := t.key()
	if body == nil {
		body = []byte{}
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO wittyinfo(target, target_id, season, body, updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(target, target_id, season) DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at`,
		target, id, season, body, db.Now())
	if err == nil {
		s.dropEmptyInfo(ctx)
	}
	return err
}

// SetInfoForce says whether to use a target's file on a cut it wasn't made for.
func (s *Store) SetInfoForce(ctx context.Context, t InfoTarget, force bool) error {
	target, id, season := t.key()
	f := 0
	if force {
		f = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO wittyinfo(target, target_id, season, force, updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(target, target_id, season) DO UPDATE SET force = excluded.force`,
		target, id, season, f, db.Now())
	if err == nil {
		s.dropEmptyInfo(ctx)
	}
	return err
}

func (s *Store) dropEmptyInfo(ctx context.Context) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM wittyinfo WHERE length(body) = 0 AND force = 0`)
}

// ------------------------------------------------------------------ files beside the media

// InfoPaths are where an item's .wittyinfo can be, the one to write first.
func InfoPaths(it *Item) []string {
	if it == nil || it.Path == "" || strings.Contains(it.Path, "://") {
		return nil
	}
	p := filepath.Clean(it.Path)
	trimmed := strings.TrimSuffix(p, filepath.Ext(p))
	if trimmed == p || it.Kind == "dvd-folder" || it.Kind == "bluray-folder" {
		return []string{p + wittyinfo.Ext}
	}
	return []string{trimmed + wittyinfo.Ext, p + wittyinfo.Ext}
}

// ShowInfoPaths are where a show's or a season's .wittyinfo can be. seasonDir is the
// folder of the season's episodes, if they have one of their own.
func ShowInfoPaths(showPath string, season int, seasonDir string) []string {
	if showPath == "" {
		return nil
	}
	if season < 0 {
		return []string{filepath.Join(showPath, "show"+wittyinfo.Ext)}
	}
	out := []string{filepath.Join(showPath, fmt.Sprintf("season%02d%s", season, wittyinfo.Ext))}
	if seasonDir != "" && !strings.EqualFold(filepath.Clean(seasonDir), filepath.Clean(showPath)) {
		out = append(out, filepath.Join(seasonDir, "season"+wittyinfo.Ext))
	}
	return out
}

type infoCacheEntry struct {
	mod  time.Time
	size int64
	info *wittyinfo.Info
	err  error
}

// infoCache keeps what each file read as, until it changes on disk: a player asks for the
// markers often, and an archive with pictures is worth reading once.
var infoCache = struct {
	sync.Mutex
	m map[string]infoCacheEntry
}{m: map[string]infoCacheEntry{}}

// ReadInfoFile reads a .wittyinfo beside the media, through the library's checks. found
// says whether there is one; err why it can't be used.
func (s *Store) ReadInfoFile(ctx context.Context, path string) (info *wittyinfo.Info, found bool, err error) {
	if !s.LibraryFile(ctx, path) {
		return nil, false, nil
	}
	f, st, err := s.OpenLibraryFile(ctx, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, true, err
	}
	defer f.Close()
	if st.IsDir() {
		return nil, false, nil
	}
	infoCache.Lock()
	c, ok := infoCache.m[path]
	infoCache.Unlock()
	if ok && c.mod.Equal(st.ModTime()) && c.size == st.Size() {
		return c.info, true, c.err
	}
	if st.Size() > wittyinfo.MaxFile {
		err = fmt.Errorf("not a usable .wittyinfo: larger than %d MB", wittyinfo.MaxFile>>20)
	} else {
		var b []byte
		if b, err = io.ReadAll(io.LimitReader(f, wittyinfo.MaxFile+1)); err == nil {
			info, err = wittyinfo.Parse(b)
		}
	}
	infoCache.Lock()
	if len(infoCache.m) > 2000 {
		clear(infoCache.m)
	}
	infoCache.m[path] = infoCacheEntry{mod: st.ModTime(), size: st.Size(), info: info, err: err}
	infoCache.Unlock()
	return info, true, err
}

// ForgetInfoFile drops what a file read as, after it was written.
func ForgetInfoFile(path string) {
	infoCache.Lock()
	delete(infoCache.m, path)
	infoCache.Unlock()
}

// LoadInfo finds a target's .wittyinfo: the kept copy, else the file beside the media.
func (s *Store) LoadInfo(ctx context.Context, t InfoTarget) LoadedInfo {
	var out LoadedInfo
	var paths []string
	switch t.Kind {
	case InfoItem:
		it, err := s.Item(ctx, t.ID)
		if err != nil {
			return out
		}
		paths = InfoPaths(it)
	default:
		sh, err := s.Show(ctx, t.ID, 0)
		if err != nil {
			return out
		}
		season := -1
		seasonDir := ""
		if t.Kind == InfoSeason {
			season = t.Season
			seasonDir = s.seasonDir(ctx, t.ID, t.Season)
		}
		paths = ShowInfoPaths(sh.Path, season, seasonDir)
	}
	if len(paths) > 0 {
		out.Path = paths[0]
	}
	for _, p := range paths {
		info, found, err := s.ReadInfoFile(ctx, p)
		if !found {
			continue
		}
		out.Path, out.FileFound, out.FileErr = p, true, err
		if err == nil {
			out.Info, out.Origin = info, "file"
		}
		break
	}
	body, force, updated := s.StoredInfo(ctx, t)
	out.Force, out.Updated = force, updated
	if len(body) > 0 {
		if info, err := wittyinfo.Parse(body); err == nil {
			out.Info, out.Origin = info, "stored"
		}
	}
	return out
}

// seasonDir is the folder a season's episodes are in, when they share one.
func (s *Store) seasonDir(ctx context.Context, showID int64, season int) string {
	var p string
	if s.DB.QueryRowContext(ctx, `SELECT path FROM items WHERE show_id = ? AND season = ? LIMIT 1`, showID, season).Scan(&p) != nil {
		return ""
	}
	return filepath.Dir(p)
}

// ItemInfos are the files that apply to a video: its own, its season's and its show's.
type ItemInfos struct {
	Item, Season, Show LoadedInfo
	// Match: whether the video's own file fits this cut.
	Match wittyinfo.Match
}

// UsableItem is the video's own file if it can be used on this cut.
func (x *ItemInfos) UsableItem() *wittyinfo.Info {
	if x.Item.Info == nil || !(x.Match.Usable || x.Item.Force) {
		return nil
	}
	return x.Item.Info
}

// InfosFor finds every .wittyinfo that applies to an item, and checks its own against
// the cut. The fingerprint is only worked out when the length alone can't settle it.
func (s *Store) InfosFor(ctx context.Context, it *Item) ItemInfos {
	var out ItemInfos
	out.Item = s.LoadInfo(ctx, InfoTarget{Kind: InfoItem, ID: it.ID})
	if out.Item.Info != nil {
		out.Match = out.Item.Info.MatchVideo(it.RuntimeSec, nil)
		if !out.Match.Usable && out.Item.Info.Identity != nil && out.Item.Info.Identity.Fingerprint != nil {
			if fp := s.fingerprintOf(ctx, it); fp != nil {
				out.Match = out.Item.Info.MatchVideo(it.RuntimeSec, fp)
			}
		}
		if !out.Match.Usable && out.Item.Force {
			out.Match.Result = wittyinfo.MatchForced
			out.Match.Usable = true
			out.Match.Message += " Used anyway, as asked."
		} else if !out.Match.Usable {
			out.Match.Message += " Its times aren't used."
		}
	}
	if it.ShowID != nil {
		if it.Season != nil {
			out.Season = s.LoadInfo(ctx, InfoTarget{Kind: InfoSeason, ID: *it.ShowID, Season: *it.Season})
		}
		out.Show = s.LoadInfo(ctx, InfoTarget{Kind: InfoShow, ID: *it.ShowID})
	}
	return out
}

// fingerprintOf works out a video's fingerprint, if it is a file in a library.
func (s *Store) fingerprintOf(ctx context.Context, it *Item) *wittyinfo.Fingerprint {
	if it.Kind != "video" || !s.LibraryFile(ctx, it.Path) {
		return nil
	}
	f, st, err := s.OpenLibraryFile(ctx, it.Path)
	if err != nil {
		return nil
	}
	defer f.Close()
	fp, err := wittyinfo.FingerprintReader(f, st.Size())
	if err != nil {
		return nil
	}
	return fp
}

// IdentityOf says which video a new file for this item is made for.
func (s *Store) IdentityOf(ctx context.Context, it *Item) *wittyinfo.Identity {
	if it.RuntimeSec <= 0 {
		return nil
	}
	id := &wittyinfo.Identity{DurationSec: wittyinfo.Seconds(it.RuntimeSec), FileName: filepath.Base(it.Path)}
	if st, err := os.Stat(it.Path); err == nil && !st.IsDir() {
		id.FileSize = st.Size()
	}
	id.Fingerprint = s.fingerprintOf(ctx, it)
	return id
}

// WriteInfoFile writes a .wittyinfo beside the media in one step. Only inside a library.
func (s *Store) WriteInfoFile(ctx context.Context, path string, b []byte) error {
	if !strings.EqualFold(filepath.Ext(path), wittyinfo.Ext) {
		return errors.New("not a .wittyinfo")
	}
	err := writeBeside(path, b, s.Roots(ctx))
	ForgetInfoFile(path)
	if err == nil {
		s.savedBeside(ctx, path)
	}
	return err
}

// RetireInfoFile moves a .wittyinfo beside the media out of the way, to <name>.wittyinfo.bak,
// replacing an older .bak.
func (s *Store) RetireInfoFile(ctx context.Context, path string) error {
	if !strings.EqualFold(filepath.Ext(path), wittyinfo.Ext) || !s.LibraryFile(ctx, path) {
		return ErrNotInLibrary
	}
	b, err := readSmall(path, wittyinfo.MaxFile)
	if err != nil {
		return err
	}
	if err := writeBeside(path+".bak", b, s.Roots(ctx)); err != nil {
		return err
	}
	ForgetInfoFile(path)
	return os.Remove(path)
}

// ------------------------------------------------------------------ what a scan takes

// applyInfo puts what a video's own .wittyinfo says about it in place of what was found:
// its metadata and its pictures. Times are used when they are asked for (see the markers),
// so nothing about them is kept here. A file for another cut changes nothing.
func (s *Scanner) applyInfo(ctx context.Context, it *Item) {
	if it.ID == 0 {
		// A new item: only the file beside it can apply yet.
		for _, p := range InfoPaths(it) {
			info, found, err := s.Store.ReadInfoFile(ctx, p)
			if found {
				if err == nil && info.MatchVideo(it.RuntimeSec, nil).Usable {
					s.takeInfo(it, info.Localize(s.Store.serverLanguage(ctx)))
				}
				return
			}
		}
		return
	}
	infos := s.Store.InfosFor(ctx, it)
	if x := infos.UsableItem(); x != nil {
		s.takeInfo(it, x.Localize(s.Store.serverLanguage(ctx)))
	}
}

// ApplyInfoNow takes an item's .wittyinfo into the library at once, after an edit.
func (s *Scanner) ApplyInfoNow(ctx context.Context, id int64) error {
	it, err := s.Store.Item(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	before := *it
	s.applyInfo(ctx, it)
	if it.Title == before.Title && it.SortTitle == before.SortTitle && it.Overview == before.Overview && it.Tagline == before.Tagline &&
		it.ContentRating == before.ContentRating && it.Poster == before.Poster && it.Backdrop == before.Backdrop &&
		strings.Join(it.Genres, "\x00") == strings.Join(before.Genres, "\x00") && intEq(it.Year, before.Year) {
		return nil
	}
	_, err = s.Store.DB.ExecContext(ctx, `UPDATE items SET title = ?, sort_title = ?, year = ?, overview = ?, tagline = ?, content_rating = ?, poster = ?, backdrop = ?, genres = ? WHERE id = ?`,
		it.Title, it.SortTitle, it.Year, it.Overview, it.Tagline, it.ContentRating, it.Poster, it.Backdrop, joinList(it.Genres), it.ID)
	return err
}

func intEq(a, b *int) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func (s *Scanner) takeInfo(it *Item, x *wittyinfo.Info) {
	if m := x.Metadata; m != nil {
		if m.Title != "" {
			it.Title = m.Title
			it.SortTitle = SortTitle(m.Title)
		}
		if m.SortTitle != "" {
			it.SortTitle = m.SortTitle
		}
		if m.Year != nil {
			y := *m.Year
			it.Year = &y
		}
		if m.Overview != "" {
			it.Overview = m.Overview
		}
		if m.Tagline != "" {
			it.Tagline = m.Tagline
		}
		if len(m.Genres) > 0 {
			it.Genres = append([]string(nil), m.Genres...)
		}
		if m.ContentRating != "" {
			it.ContentRating = m.ContentRating
		}
	}
	if p := s.infoPicture(x, "poster"); p != "" {
		it.Poster = p
	} else if p := s.infoPicture(x, "thumb"); p != "" && it.ShowID != nil {
		it.Poster = p
	}
	if p := s.infoPicture(x, "backdrop"); p != "" {
		it.Backdrop = p
	}
}

// infoPicture copies a picture out of a .wittyinfo into the artwork cache, named for its
// content, and gives its "cache:" reference.
func (s *Scanner) infoPicture(x *wittyinfo.Info, role string) string {
	name := x.Artwork[role]
	b := x.Files[name]
	if name == "" || len(b) == 0 || s.ImagesDir == "" {
		return ""
	}
	h := sha1.Sum(b)
	out := "wi-" + hex.EncodeToString(h[:12]) + strings.ToLower(filepath.Ext(name))
	dst := filepath.Join(s.ImagesDir, out)
	if st, err := os.Stat(dst); err != nil || st.Size() != int64(len(b)) {
		if os.MkdirAll(s.ImagesDir, 0o755) != nil || os.WriteFile(dst, b, 0o644) != nil {
			return ""
		}
	}
	return "cache:" + out
}

// ------------------------------------------------------------------ languages and shows

// serverLanguage is the language the library's details are kept in: the app's language,
// else the one disc menus follow. "" leaves a file's own text.
func (s *Store) serverLanguage(ctx context.Context) string {
	if v := s.DB.GetSetting(ctx, "uiLanguage", ""); v != "" {
		return v
	}
	return s.DB.GetSetting(ctx, "menuLanguage", "")
}

// applyShowInfo puts what a show's own show.wittyinfo says about it in place of what was
// found: its details and pictures, in the server's language.
func (s *Scanner) applyShowInfo(ctx context.Context, sh *Show) {
	if sh.Path == "" {
		return
	}
	var x *wittyinfo.Info
	if sh.ID != 0 {
		x = s.Store.LoadInfo(ctx, InfoTarget{Kind: InfoShow, ID: sh.ID}).Info
	} else if info, found, err := s.Store.ReadInfoFile(ctx, ShowInfoPaths(sh.Path, -1, "")[0]); found && err == nil {
		x = info
	}
	if x == nil {
		return
	}
	x = x.Localize(s.Store.serverLanguage(ctx))
	if m := x.Metadata; m != nil {
		if m.Title != "" {
			sh.Title, sh.SortTitle = m.Title, SortTitle(m.Title)
		}
		if m.SortTitle != "" {
			sh.SortTitle = m.SortTitle
		}
		if m.Year != nil {
			y := *m.Year
			sh.Year = &y
		}
		if m.Overview != "" {
			sh.Overview = m.Overview
		}
		if len(m.Genres) > 0 {
			sh.Genres = append([]string(nil), m.Genres...)
		}
		if m.ContentRating != "" {
			sh.ContentRating = m.ContentRating
		}
	}
	if p := s.infoPicture(x, "poster"); p != "" {
		sh.Poster = p
	}
	if p := s.infoPicture(x, "backdrop"); p != "" {
		sh.Backdrop = p
	}
}

// InfoSubtitle is a subtitle file kept in a .wittyinfo, copied out where players can read it.
type InfoSubtitle struct {
	Path     string // in the server's own cache
	Format   string // srt, vtt, ass or ssa
	Language string
	Title    string
	Forced   bool
	SDH      bool
}

// InfoSubtitles copies the subtitles in a video's .wittyinfo (if it fits this cut) into
// cacheDir, once each, and lists them.
func (s *Store) InfoSubtitles(ctx context.Context, it *Item, cacheDir string) []InfoSubtitle {
	infos := s.InfosFor(ctx, it)
	x := infos.UsableItem()
	if x == nil || len(x.Subtitles) == 0 {
		return nil
	}
	dir := filepath.Join(cacheDir, "wittyinfo-subtitles")
	var out []InfoSubtitle
	for _, sub := range x.Subtitles {
		b := x.Files[sub.File]
		if len(b) == 0 {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(sub.File), "."))
		h := sha1.Sum(b)
		p := filepath.Join(dir, hex.EncodeToString(h[:12])+"."+ext)
		if st, err := os.Stat(p); err != nil || st.Size() != int64(len(b)) {
			if os.MkdirAll(dir, 0o755) != nil || os.WriteFile(p, b, 0o644) != nil {
				continue
			}
		}
		out = append(out, InfoSubtitle{Path: p, Format: ext, Language: sub.Language, Title: sub.Title, Forced: sub.Forced, SDH: sub.SDH})
	}
	return out
}

// Localize puts each file's text in the language closest to lang (see wittyinfo.Localize).
func (x *ItemInfos) Localize(lang string) {
	if lang == "" {
		return
	}
	for _, l := range []*LoadedInfo{&x.Item, &x.Season, &x.Show} {
		if l.Info != nil {
			l.Info = l.Info.Localize(lang)
		}
	}
}
