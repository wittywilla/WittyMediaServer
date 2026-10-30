package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The disc page: the feature first ("Feature Presentation") with named
// chapters, then bonus features, then shortcuts into the disc's menus.

type discChapter struct {
	N     int     `json:"n"`
	Start float64 `json:"start"`
	Label string  `json:"label"` // user label, "" = default
}

type discTitle struct {
	TTN         int           `json:"ttn"`
	Label       string        `json:"label"` // user label, "" = default
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
	"audio": "Audio & languages", "subpicture": "Subtitles", "angle": "Angles",
}

// GET /api/items/{id}/disc
func (s *Server) handleDisc(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathInt(r, "id"); ok {
		if it, err := s.visibleItem(r, id); err == nil && isBluray(it) {
			s.handleDiscBluray(w, r, it)
			return
		}
	}
	it, o, ok := s.openDisc(w, r)
	if !ok {
		return
	}
	defer o.Close()
	labels := s.Store.DiscLabels(r.Context(), it.ID)
	d := o.Disc
	view := func(ts *wittydvd.TitleSummary) discTitle {
		t := discTitle{TTN: ts.Number, Label: labels.Titles[strconv.Itoa(ts.Number)], DurationSec: ts.Duration, Angles: ts.Angles, Wide: ts.Video.Wide,
			Chapters: []discChapter{}, Audio: []string{}, Subtitles: []string{}}
		names := labels.Chapters[strconv.Itoa(ts.Number)]
		for i, c := range ts.Chapters {
			t.Chapters = append(t.Chapters, discChapter{N: i + 1, Start: c, Label: names[strconv.Itoa(i+1)]})
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
	out := map[string]any{"main": nil, "extras": []discTitle{}, "menus": []discMenu{}}
	main := d.MainTitle()
	seen := map[[2]int]bool{}
	if main != nil {
		out["main"] = view(main)
		seen[[2]int{main.VTS, main.VTSTitle}] = true
	}
	var extras []discTitle
	for _, ts := range d.Titles() {
		k := [2]int{ts.VTS, ts.VTSTitle}
		if seen[k] || ts.Duration < 10 {
			continue // duplicates (several title numbers for one program) and tiny stubs
		}
		seen[k] = true
		extras = append(extras, view(&ts))
	}
	sort.SliceStable(extras, func(i, j int) bool { return extras[i].DurationSec > extras[j].DurationSec })
	if extras != nil {
		out["extras"] = extras
	}
	// Menus a viewer can jump straight into.
	var menus []discMenu
	has := func(lus []wittydvd.LangUnit, typ int) bool {
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
				if has(v.Menus, wittydvd.MenuTypes[kind]) {
					menus = append(menus, discMenu{Kind: kind, VTS: main.VTS, Label: menuLabels[kind]})
				}
			}
		}
	}
	if has(d.VMG.Menus, wittydvd.MenuTypes["title"]) {
		menus = append(menus, discMenu{Kind: "title", Label: menuLabels["title"]})
	}
	if menus != nil {
		out["menus"] = menus
	}
	writeJSON(w, 200, out)
}

func (s *Server) openDisc(w http.ResponseWriter, r *http.Request) (*library.Item, *wittydvd.Opened, bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil, nil, false
	}
	it, err := s.visibleItem(r, id)
	if err != nil || (it.Kind != "dvd-iso" && it.Kind != "dvd-folder") {
		writeErr(w, 404, "disc not found")
		return nil, nil, false
	}
	o, err := wittydvd.Open(it.Path)
	if err != nil {
		writeErr(w, 500, err.Error())
		return nil, nil, false
	}
	return it, o, true
}

// PUT /api/items/{id}/disc/labels  {"titles":{"3":"Making of"},"chapters":{"1":{"4":"The dragon"}}}
func (s *Server) handleDiscLabels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b library.DiscLabels
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.Store.SetDiscLabels(r.Context(), id, b.Clean()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, s.Store.DiscLabels(r.Context(), id))
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
		if err != nil {
			writeErr(w, 500, err.Error())
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
	o, err := wittydvd.Open(it.Path)
	if err != nil {
		return err
	}
	defer o.Close()
	ts := o.Disc.Title(ttn)
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
	sp := wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: ts.VTS}
	ext, err := o.CellExtents(sp, cell.Cell)
	if err != nil || len(ext) == 0 {
		return fmt.Errorf("cannot read the chapter")
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o755)
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
