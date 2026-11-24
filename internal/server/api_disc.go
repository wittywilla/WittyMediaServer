// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The disc page: the feature first ("Feature Presentation") with its chapters, then bonus
// features, then shortcuts into the disc's menus. WittyDVD and WittyBluray show it (their routes
// and SDUI sources, with the names given to titles and chapters, which are theirs); the server
// works it out from the disc engine's checked answers and hands it over (host_discs.go).

type discChapter struct {
	N     int     `json:"n"`
	Start float64 `json:"start"`
}

type discTitle struct {
	TTN         int           `json:"ttn"`
	DurationSec float64       `json:"durationSec"`
	Chapters    []discChapter `json:"chapters"`
	Audio       []string      `json:"audio"`
	Subtitles   []string      `json:"subtitles"`
	Angles      int           `json:"angles"`
	Wide        bool          `json:"wide"`
}

type discMenu struct {
	Kind  string `json:"kind"` // title | root | chapter | audio | subpicture | angle
	VTS   int    `json:"vts"`
	Label string `json:"label"`
}

var menuLabels = map[string]string{
	"title": "Title menu", "root": "Main menu", "chapter": "Scene selection",
	"audio": i18n.T("Audio & languages"), "subpicture": "Subtitles", "angle": "Angles",
}

// discPageView is a disc's page: the film, the other titles worth watching, and the disc's
// own menus to jump into, as the host API gives it to the disc modules.
type discPageView struct {
	Main   *discTitle
	Extras []discTitle
	Menus  []discMenu
	// Hidden counts titles the engine found to be decoys or didn't list; -1 without a ranking.
	Hidden int
}

// errDiscEncrypted: the disc's video is encrypted, so nothing of it is read to play.
var errDiscEncrypted = errors.New("This disc is encrypted, and encrypted discs aren't supported.")

// discPages keeps the last few disc pages worked out, for a short while: one page of the
// app asks for a disc's feature, scenes, extras and menus as separate sections, and each
// would otherwise start the disc's helper again. A change of its film forgets it
// (discPageForget).
var discPages = struct {
	sync.Mutex
	m map[int64]discPageCached
}{m: map[int64]discPageCached{}}

type discPageCached struct {
	v   *discPageView
	key string
	at  time.Time
}

const (
	discPageLife = 30 * time.Second
	discPageMax  = 16
)

func discPageForget(id int64) {
	discPages.Lock()
	delete(discPages.m, id)
	discPages.Unlock()
}

// discPage works a disc's page out, or takes it from the last few.
func (s *Server) discPage(ctx context.Context, it *library.Item) (*discPageView, error) {
	key := fmt.Sprintf("%s|%d|%v", it.Path, it.Disc.MainTitle, it.Disc.Rank != nil)
	discPages.Lock()
	if c, ok := discPages.m[it.ID]; ok && c.key == key && time.Since(c.at) < discPageLife {
		discPages.Unlock()
		return c.v, nil
	}
	discPages.Unlock()
	var v *discPageView
	var err error
	if isBluray(it) {
		v, err = s.discPageBluray(ctx, it)
	} else {
		v, err = s.discPageDVD(ctx, it)
	}
	if err != nil {
		return nil, err
	}
	discPages.Lock()
	if len(discPages.m) >= discPageMax {
		clear(discPages.m)
	}
	discPages.m[it.ID] = discPageCached{v: v, key: key, at: time.Now()}
	discPages.Unlock()
	return v, nil
}

func (s *Server) discPageDVD(ctx context.Context, it *library.Item) (*discPageView, error) {
	if it.Disc != nil && it.Disc.Scrambled {
		return nil, errDiscEncrypted
	}
	o, err := dvdsession.Open(it.Path)
	if err != nil {
		return nil, err
	}
	defer o.Close()
	d := o.Disc
	view := func(ts *dvd.TitleSummary) discTitle {
		t := discTitle{TTN: ts.Number, DurationSec: ts.Duration, Angles: ts.Angles, Wide: ts.Video.Wide,
			Chapters: []discChapter{}, Audio: []string{}, Subtitles: []string{}}
		for i, c := range ts.Chapters {
			t.Chapters = append(t.Chapters, discChapter{N: i + 1, Start: c})
		}
		for _, a := range ts.Audio {
			label := strings.TrimSpace(fmt.Sprintf("%s %s", a.Language, a.Purpose()))
			t.Audio = append(t.Audio, label)
		}
		for _, sp := range ts.Subp {
			t.Subtitles = append(t.Subtitles, strings.TrimSpace(fmt.Sprintf("%s %s", sp.Language, sp.Purpose())))
		}
		return t
	}
	out := &discPageView{Hidden: -1}
	// The film is the title an admin chose, or the engine's ranked pick laid out as the
	// engine plays it, or with neither the older rule's guess.
	main := library.DiscMainTitle(d, it.Disc)
	seen := map[[2]int]bool{}
	if main != nil {
		v := view(main)
		out.Main = &v
		seen[[2]int{main.VTS, main.VTSTitle}] = true
	}
	// With a ranking, the extras are what the engine counts as something to watch: not
	// its decoys, the copies of other titles, or the parts of the film itself.
	var listed map[int]bool
	if rk := rankOf(it); rk != nil {
		listed = map[int]bool{}
		for _, c := range rk.Candidates {
			if c.Class != "duplicate" && c.Class != "stub" && c.Class != "part" {
				listed[c.TTN] = true
			}
		}
		// Another cut of the film is under the page's other versions, not a bonus feature.
		// (The film itself, chosen or ranked, is the page's main title, never an extra.)
		for _, v := range rk.Versions {
			if v.Kind != "angle" {
				delete(listed, v.TTN)
			}
		}
		if main != nil {
			if f := it.Disc.FeaturePlays(main.Number); f != nil {
				for _, n := range f.Titles {
					delete(listed, n)
				}
			}
		}
		out.Hidden = rk.Hidden + rk.Decoys
	}
	for _, ts := range d.Titles() {
		k := [2]int{ts.VTS, ts.VTSTitle}
		if seen[k] || ts.Duration < 10 || (listed != nil && !listed[ts.Number]) {
			continue // duplicates (several title numbers for one program) and tiny stubs
		}
		seen[k] = true
		out.Extras = append(out.Extras, view(&ts))
	}
	sort.SliceStable(out.Extras, func(i, j int) bool { return out.Extras[i].DurationSec > out.Extras[j].DurationSec })
	// Menus a viewer can jump straight into.
	has := func(lus []dvd.LangUnit, typ int) bool {
		for _, lu := range lus {
			for _, e := range lu.PGCs {
				if e.Entry && e.MenuType == typ && e.PGC != nil && len(e.PGC.Cells)+len(e.PGC.Pre) > 0 {
					return true
				}
			}
		}
		return false
	}
	if main != nil {
		if v := d.VTS[main.VTS]; v != nil {
			for _, kind := range []string{"root", "chapter", "audio", "subpicture", "angle"} {
				if has(v.Menus, dvd.MenuTypes[kind]) {
					out.Menus = append(out.Menus, discMenu{Kind: kind, VTS: main.VTS, Label: menuLabels[kind]})
				}
			}
		}
	}
	if has(d.VMG.Menus, dvd.MenuTypes["title"]) {
		out.Menus = append(out.Menus, discMenu{Kind: "title", Label: menuLabels["title"]})
	}
	return out, nil
}

// Thumbnails are produced by FFmpeg on first request; at most two at a time
// so a page full of chapters doesn't swamp a small server.
var thumbSlots = make(chan struct{}, 2)

// GET /api/items/{id}/disc/thumb/{ttn}/{n}   (n = chapter, 0 = the title)
func (s *Server) handleDiscThumb(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	ttn, _ := strconv.Atoi(r.PathValue("ttn"))
	n, _ := strconv.Atoi(strings.TrimSuffix(r.PathValue("n"), ".jpg"))
	it, err := s.visibleItem(r, id)
	if err != nil || !isDiscKind(it) {
		writeErr(w, 404, "disc not found")
		return
	}
	if it.Disc != nil && it.Disc.Scrambled {
		s.Log.DebugContext(r.Context(), "no thumbnail for an encrypted disc", "cat", "discs", "item", it.ID)
		writeErr(w, 404, errNoPicture.Error()) // protected discs are never read
		return
	}
	h := sha1.Sum([]byte(it.Path))
	out := filepath.Join(s.Cfg.ImagesDir, fmt.Sprintf("dvdthumb-%s-%d-%d.jpg", hex.EncodeToString(h[:8]), ttn, n))
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		select {
		case thumbSlots <- struct{}{}:
		case <-r.Context().Done():
			return
		}
		err := s.makeDiscThumb(r.Context(), it, ttn, n, out)
		<-thumbSlots
		if err == errNoPicture || err == errNoTitle {
			writeErr(w, 404, err.Error())
			return
		}
		if r.Context().Err() != nil {
			// The pointer moved on along the timeline before the picture was made. FFmpeg was
			// stopped for it, and how a stopped FFmpeg exits (status 1 on Windows) says nothing.
			_ = os.Remove(out) // whatever it had begun to write
			return
		}
		if err != nil {
			writeErr(w, 500, s.viewerError(r, err))
			return
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeFile(w, r, out)
}

func (s *Server) makeDiscThumb(ctx context.Context, it *library.Item, ttn, n int, out string) error {
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return nil // made by a parallel request
	}
	if isBluray(it) {
		return s.makeBlurayThumb(ctx, it, ttn, n, out)
	}
	o, err := dvdsession.Open(it.Path)
	if err != nil {
		return err
	}
	defer o.Close()
	ts := o.Disc.Title(ttn)
	if f := it.Disc.FeaturePlays(ttn); f != nil {
		// The film's chapters are the engine's, along every title it runs through.
		if ft, _, ok := o.Disc.FeatureTitle(f.Engine(), 1); ok {
			ts = ft
		}
	}
	if ts == nil || len(ts.Cells) == 0 {
		return errNoPicture
	}
	// Only chapters that exist get a file: the address is the viewer's to choose.
	if n < 0 || n > len(ts.Chapters) {
		return errNoPicture
	}
	// A few seconds into the chapter skips fades from black.
	at := ts.Duration * 0.1
	if at > 90 {
		at = 90
	}
	if n >= 1 && n <= len(ts.Chapters) {
		end := ts.Duration
		if n < len(ts.Chapters) {
			end = ts.Chapters[n]
		}
		at = ts.Chapters[n-1] + min(6, (end-ts.Chapters[n-1])/3)
	}
	cell := ts.Cells[0]
	for _, c := range ts.Cells {
		if c.Start <= at {
			cell = c
		}
	}
	sp := dvd.Space{Domain: dvd.DomainVTS, VTS: ts.VTS}
	ext, err := o.CellExtents(sp, cell.Cell)
	if err != nil || len(ext) == 0 {
		return fmt.Errorf("cannot read the chapter")
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o755) // a failure shows when FFmpeg can't write the picture
	list, err := ffmpeg.WriteConcat(filepath.Join(s.Cfg.CacheDir, "lists"), strings.TrimSuffix(filepath.Base(out), ".jpg"), []ffmpeg.Segment{{Extents: ext, Duration: cell.Cell.Duration}})
	if err != nil {
		return err
	}
	tctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	off := at - cell.Start
	if off > cell.Cell.Duration-1 {
		off = cell.Cell.Duration / 2
	}
	if err := s.Tools.Still(tctx, ffmpeg.ConcatInputArgs(list), off, 360, out); err != nil {
		return err
	}
	// Cells that hold a single still frame end before the chosen moment and
	// ffmpeg then writes nothing: take the first frame instead.
	if st, err := os.Stat(out); (err != nil || st.Size() == 0) && off > 0 {
		if err := s.Tools.Still(tctx, ffmpeg.ConcatInputArgs(list), 0, 360, out); err != nil {
			return err
		}
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return errNoPicture
	}
	return nil
}

var (
	errNoPicture = errors.New("no picture in this title")
	errNoTitle   = errors.New("no such title or chapter")
)

// isVersion says a title is a cut of the film in the kept ranking: the one that plays is the
// page's main title, the others its versions, and none of them an extra.
func isVersion(rk *library.DiscRank, ttn int) bool {
	for _, v := range rk.Versions {
		if v.TTN == ttn && v.Kind != "angle" {
			return true
		}
	}
	return false
}

// rankOf is the kept ranking of an item's disc, or nil.
func rankOf(it *library.Item) *library.DiscRank {
	if it == nil || it.Disc == nil {
		return nil
	}
	return it.Disc.Rank
}
