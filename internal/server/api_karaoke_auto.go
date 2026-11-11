// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/separator"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Karaoke that needs nobody to set it up. An app asks one thing, "the karaoke of this
// song", and the server does what is right by itself:
//
//   - it uses a separator the owner already installed (internal/separator finds it; the
//     command named in the configuration still wins), and otherwise FFmpeg's quick
//     approximate version;
//
//   - it makes a version the first time someone opens karaoke on a song, the quick one
//     at once and the real one behind it, bounded like every media job (one at a time,
//     sixteen waiting), by a limit per account, and by the size of the machine;
//
//   - it keeps what it made under a size cap, dropping what was sung least recently.
//
//     GET  /api/media/{id}/karaoke           what there is to play now, and what is being made
//     POST /api/media/{id}/karaoke/prepare   the same answer, after starting what is missing
//     GET  /api/karaoke/status               for Settings: what was found on this machine
func init() {
	settingKeys = append(settingKeys, "karaokeAuto", "karaokeCacheMB")
	settingDefaults["karaokeAuto"] = "auto"
	settingDefaults["karaokeCacheMB"] = "2048"
	settingChecks["karaokeAuto"] = oneOf("auto", "admin", "never")
	settingChecks["karaokeCacheMB"] = func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 100 && n <= 1_000_000
	}
}

func (s *Server) karaokeRoutes(m *http.ServeMux) {
	m.Handle("GET /api/media/{id}/karaoke", s.requireUser(s.handleKaraoke))
	m.Handle("POST /api/media/{id}/karaoke/prepare", s.requireUser(s.handleKaraokePrepare))
	m.Handle("GET /api/karaoke/status", s.requireAdmin(s.handleKaraokeStatus))
}

// separatorDiscover looks for an installed separator. Tests replace it.
var separatorDiscover = func(ctx context.Context) *separator.Found {
	return separator.Discover(ctx, separator.Options{})
}

// karaokeBusy says someone is being served work a separation should wait for. Tests replace it.
var karaokeBusy = pace.Busy

const (
	// Real separations one account may start in an hour, and quick versions.
	karaokeRealPerHour   = 6
	karaokeApproxPerHour = 40
	// How long a song whose separation failed isn't tried again by itself.
	karaokeFailedFor = 6 * time.Hour
	// How long "nothing found" is believed before looking again.
	karaokeLookAgain = 15 * time.Minute
	maxKaraokeNotes  = 512
)

// karaokeAuto is what the server remembers between requests. Everything in it is bounded.
type karaokeAuto struct {
	mu       sync.Mutex
	looked   time.Time
	found    *separator.Found
	asks     map[string][]time.Time // "real:<account>" / "approx:<account>" → when
	failed   map[string]karaokeFail // "real:<item>" / "approx:<item>"
	progress map[int64]int          // a separation running now, 0 to 100
}

type karaokeFail struct {
	reason string
	until  time.Time
}

// foundSeparator is the separator found on this machine, looked for at most once in a
// while (once it's found, it stays found until an administrator looks again).
func (s *Server) foundSeparator(ctx context.Context, again bool) *separator.Found {
	k := &s.karaoke
	k.mu.Lock()
	defer k.mu.Unlock()
	if again || k.looked.IsZero() || (k.found == nil && time.Since(k.looked) > karaokeLookAgain) {
		k.found = separatorDiscover(ctx)
		k.looked = time.Now()
		if k.found != nil && s.Log != nil {
			s.Log.Info("a stem separator was found", "cat", "library", "name", k.found.Name, "version", k.found.Version, "path", k.found.Path)
		}
	}
	return k.found
}

// separator is the command that separates songs, and what to call it: the one named in the
// configuration, or else the one found. Nil words when there is none.
func (s *Server) separator(ctx context.Context) (words []string, name, version, source string) {
	if s.Cfg != nil && strings.TrimSpace(s.Cfg.StemSeparator) != "" {
		w := ffmpeg.SplitArgs(s.Cfg.StemSeparator)
		if len(w) == 0 {
			return nil, "", "", ""
		}
		return w, strings.TrimSuffix(filepath.Base(w[0]), filepath.Ext(w[0])), "", "config"
	}
	if f := s.foundSeparator(ctx, false); f != nil {
		return f.Words, f.Name, f.Version, "found"
	}
	return nil, "", "", ""
}

// allow counts one more of kind for an account, unless it has had its share this hour.
func (k *karaokeAuto) allow(kind string, account int64, limit int) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.asks == nil {
		k.asks = map[string][]time.Time{}
	}
	key, now := kind+":"+strconv.FormatInt(account, 10), time.Now()
	recent := k.asks[key][:0]
	for _, t := range k.asks[key] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= limit {
		k.asks[key] = recent
		return false
	}
	if len(k.asks) >= maxKaraokeNotes {
		for other, times := range k.asks {
			if len(times) == 0 || now.Sub(times[len(times)-1]) >= time.Hour {
				delete(k.asks, other)
			}
		}
		if len(k.asks) >= maxKaraokeNotes {
			return false
		}
	}
	k.asks[key] = append(recent, now)
	return true
}

func (k *karaokeAuto) fail(kind string, item int64, reason string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.failed == nil {
		k.failed = map[string]karaokeFail{}
	}
	now := time.Now()
	if len(k.failed) >= maxKaraokeNotes {
		for key, f := range k.failed {
			if now.After(f.until) {
				delete(k.failed, key)
			}
		}
		if len(k.failed) >= maxKaraokeNotes {
			clear(k.failed)
		}
	}
	k.failed[kind+":"+strconv.FormatInt(item, 10)] = karaokeFail{reason, now.Add(karaokeFailedFor)}
}

func (k *karaokeAuto) failure(kind string, item int64) string {
	k.mu.Lock()
	defer k.mu.Unlock()
	f, ok := k.failed[kind+":"+strconv.FormatInt(item, 10)]
	if !ok || time.Now().After(f.until) {
		return ""
	}
	return f.reason
}

func (k *karaokeAuto) setProgress(item int64, pct int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if pct < 0 {
		delete(k.progress, item)
		return
	}
	if k.progress == nil {
		k.progress = map[int64]int{}
	}
	k.progress[item] = min(pct, 100)
}

func (k *karaokeAuto) progressOf(item int64) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	if p, ok := k.progress[item]; ok {
		return p
	}
	return -1
}

// percentWatcher reads the "40%" a separator prints as it goes (Demucs and the others draw
// a progress bar) and passes on the latest.
type percentWatcher struct {
	digits []byte
	last   int
	say    func(pct int)
}

func (p *percentWatcher) Write(b []byte) (int, error) {
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9':
			if len(p.digits) < 3 {
				p.digits = append(p.digits, c)
			}
		case c == '%' && len(p.digits) > 0:
			if n, _ := strconv.Atoi(string(p.digits)); n <= 100 && n != p.last {
				p.last = n
				p.say(n)
			}
			p.digits = p.digits[:0]
		default:
			p.digits = p.digits[:0]
		}
	}
	return len(b), nil
}

// karaokeStem is one thing an app plays, at its own loudness.
type karaokeStem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Role: vocals, music, or full (the whole song, in a blend).
	Role        string  `json:"role"`
	URL         string  `json:"url"`
	MIME        string  `json:"mime"`
	DefaultGain float64 `json:"defaultGain"`
}

// karaokeAnswer is what both routes say. docs/CLIENT-API.md has it for app writers.
type karaokeAnswer struct {
	// State: ready (stems can be played), preparing (they are being made) or unavailable.
	State string `json:"state"`
	// Quality of what Stems holds: stems (really separated) or approximate.
	Quality string `json:"quality"`
	// Mix: parts (each stem at its own loudness) or blend (the whole song against an
	// instrumental: full at m·v, instrumental at m·(1−v) for a voice level v and music level m).
	Mix     string        `json:"mix"`
	Stems   []karaokeStem `json:"stems"`
	Version string        `json:"version"`
	// Improving: a really separated version is being made while this one plays.
	Improving bool `json:"improving"`
	// Progress of what is being made, 0 to 100, or -1 when there's no telling.
	Progress   int    `json:"progress"`
	CanImprove bool   `json:"canImprove"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	// PollSec: ask again after this many seconds; 0 when nothing will change by itself.
	PollSec int `json:"pollSec"`
}

var karaokeLabels = map[string]string{"vocals": "Vocals", "instrumental": "Music", "drums": "Drums", "bass": "Bass", "other": "Other", "full": "Original"}

var karaokeMessages = map[string]string{
	"not-a-song":   "Karaoke versions are for songs.",
	"not-a-file":   "This isn't a file in a library, so there is nothing to make a karaoke version from.",
	"off":          "Making karaoke versions is turned off on this server.",
	"admin-only":   "On this server, an administrator makes karaoke versions. Ask them for this song.",
	"mono":         "This song is mono, so the quick method has nothing to work with. A separator on the server can still do it.",
	"no-ffmpeg":    "This server has neither FFmpeg nor a separator, so it can't make a karaoke version.",
	"busy":         "The server is busy just now. Try again in a little while.",
	"rate-limited": "You've asked for a lot of karaoke versions in the last hour. Try again later.",
	"failed":       "The karaoke version couldn't be made. The server's Tasks page says why.",
	"not-made":     "No karaoke version has been made for this song yet.",
}

// karaokeFull is the whole song for a blend: the copy made beside the approximate
// instrumental when there is one, else the song's own file.
func (s *Server) karaokeFull(ctx context.Context, it *library.Item) (extraFile, string, os.FileInfo, bool) {
	if it.Kind != "audio" {
		return extraFile{}, "", nil, false
	}
	p := filepath.Join(s.extrasRoot(), mediaextras.Key(it.ID, it.Path), mediaextras.ApproxDir, mediaextras.ApproxFull+mediaextras.ApproxExt)
	if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() {
		return extraFile{path: p, source: "approximate"}, mediaextras.StemMIME(p), fi, true
	}
	if !localFileItem(it) || !s.Store.LibraryFile(ctx, it.Path) {
		return extraFile{}, "", nil, false
	}
	fi, err := os.Stat(it.Path)
	if err != nil {
		return extraFile{}, "", nil, false
	}
	return extraFile{path: it.Path, source: "original", library: true}, mediaextras.StemMIME(it.Path), fi, true
}

// karaokeMix chooses what an app plays from the stems a song has.
// While a separation is writing its parts (making), the server's own folder of real
// stems is left out: it is half full.
func (s *Server) karaokeMix(ctx context.Context, it *library.Item, making bool) (quality, mix string, out []karaokeStem) {
	by := map[string]mediaextras.Stem{}
	for _, st := range s.stemsOf(ctx, it) {
		if !st.Multitrack && !(making && st.Uploaded && !st.Approximate) {
			by[st.Name] = st
		}
	}
	h := fnv.New64a()
	stamp := func(name string, size int64, mod time.Time) string {
		fmt.Fprintf(h, "%s|%d|%d;", name, size, mod.UnixNano())
		return strconv.FormatInt(size, 36) + strconv.FormatInt(mod.Unix(), 36)
	}
	add := func(name, role string) {
		st := by[name]
		v := ""
		if fi, err := os.Stat(st.Path); err == nil {
			v = stamp(name, fi.Size(), fi.ModTime())
		}
		out = append(out, karaokeStem{ID: name, Label: karaokeLabels[name], Role: role, MIME: st.MIME, DefaultGain: 1,
			URL: fmt.Sprintf("/api/media/%d/stems/%s?v=%s", it.ID, name, v)})
	}
	has := func(n string) bool { return by[n].Path != "" }
	switch {
	case has("vocals") && (has("instrumental") || has("drums") || has("bass") || has("other")):
		add("vocals", "vocals")
		if all := has("drums") && has("bass") && has("other"); all || !has("instrumental") {
			for _, n := range []string{"drums", "bass", "other"} {
				if has(n) {
					add(n, "music")
				}
			}
		} else {
			add("instrumental", "music")
		}
		return "stems", "parts", out
	case has("instrumental"):
		f, mime, fi, ok := s.karaokeFull(ctx, it)
		if !ok {
			return "", "", nil
		}
		_ = f
		out = append(out, karaokeStem{ID: "full", Label: karaokeLabels["full"], Role: "full", MIME: mime, DefaultGain: 1,
			URL: fmt.Sprintf("/api/media/%d/stems/full?v=%s", it.ID, stamp("full", fi.Size(), fi.ModTime()))})
		add("instrumental", "music")
		if by["instrumental"].Approximate {
			return "approximate", "blend", out
		}
		return "stems", "blend", out
	}
	return "", "", nil
}

func karaokeVersion(stems []karaokeStem) string {
	if len(stems) == 0 {
		return ""
	}
	h := fnv.New64a()
	for _, st := range stems {
		h.Write([]byte(st.URL))
	}
	return strconv.FormatUint(h.Sum64(), 36)
}

// karaokeFor works out the answer for a song, starting what is missing when prepare is set.
func (s *Server) karaokeFor(r *http.Request, it *library.Item, prepare bool) karaokeAnswer {
	ctx := r.Context()
	a := karaokeAnswer{Stems: []karaokeStem{}, Progress: -1}
	unavailable := func(reason string) karaokeAnswer {
		a.State, a.Reason, a.Message = "unavailable", reason, karaokeMessages[reason]
		return a
	}
	if it.Kind != "audio" {
		return unavailable("not-a-song")
	}
	who := userFrom(r)
	mode := s.DB.GetSetting(ctx, "karaokeAuto", settingDefaults["karaokeAuto"])
	may := mode == "auto" || (mode == "admin" && who.IsAdmin)
	isFile := localFileItem(it) && s.Store.LibraryFile(ctx, it.Path)
	hasFFmpeg := s.Tools != nil && s.Tools.FFmpeg != ""
	words, _, _, _ := s.separator(ctx)
	tiny := pace.Machine() == pace.Tiny
	approxKey, realKey := fmt.Sprintf("stems-approx:%d", it.ID), fmt.Sprintf("stems-separate:%d", it.ID)

	quality, mix, stems := s.karaokeMix(ctx, it, s.mediaJobTask(realKey) != "")
	wantReal := isFile && quality != "stems" && words != nil && !tiny && s.karaoke.failure("real", it.ID) == ""
	why := ""
	if prepare && may && isFile {
		// The quick version first, so there is something to sing to while the real one is made.
		if quality == "" && hasFFmpeg && s.karaoke.failure("approx", it.ID) == "" && s.mediaJobTask(approxKey) == "" {
			if !s.karaoke.allow("approx", who.ID, karaokeApproxPerHour) {
				why = "rate-limited"
			} else if err := s.startKaraokeApprox(r, it); err != nil && !errors.Is(err, errJobQueued) {
				why = "busy"
			}
		}
		if wantReal && s.mediaJobTask(realKey) == "" {
			switch {
			case pace.Machine() == pace.Small && karaokeBusy():
				why = "busy"
			case !who.IsAdmin && !s.karaoke.allow("real", who.ID, karaokeRealPerHour):
				why = "rate-limited"
			default:
				if err := s.startKaraokeReal(r, it, words); err != nil && !errors.Is(err, errJobQueued) {
					why = "busy"
				}
			}
		}
	}
	realTask, approxTask := s.mediaJobTask(realKey), s.mediaJobTask(approxKey)
	a.CanImprove = wantReal && may && realTask == ""
	if len(stems) > 0 {
		a.State, a.Quality, a.Mix, a.Stems, a.Version = "ready", quality, mix, stems, karaokeVersion(stems)
		mediaextras.TouchKaraoke(filepath.Join(s.extrasRoot(), mediaextras.Key(it.ID, it.Path)))
		if realTask != "" && quality != "stems" {
			a.Improving, a.Progress, a.PollSec = true, s.karaoke.progressOf(it.ID), 5
		}
		return a
	}
	if realTask != "" || approxTask != "" {
		a.State, a.PollSec = "preparing", 2
		if approxTask == "" {
			a.Progress, a.PollSec = s.karaoke.progressOf(it.ID), 4
		}
		return a
	}
	switch {
	case !isFile:
		return unavailable("not-a-file")
	case mode == "never":
		return unavailable("off")
	case !may:
		return unavailable("admin-only")
	case why != "":
		return unavailable(why)
	case s.karaoke.failure("approx", it.ID) != "" && (words == nil || tiny || s.karaoke.failure("real", it.ID) != ""):
		return unavailable(s.karaoke.failure("approx", it.ID))
	case s.karaoke.failure("real", it.ID) != "" && !hasFFmpeg:
		return unavailable("failed")
	case !hasFFmpeg && (words == nil || tiny):
		return unavailable("no-ffmpeg")
	}
	a.CanImprove = may
	return unavailable("not-made")
}

// karaokeCap is the most the server keeps of what it made, in bytes.
func (s *Server) karaokeCap(ctx context.Context) int64 {
	n, err := strconv.ParseInt(s.DB.GetSetting(ctx, "karaokeCacheMB", settingDefaults["karaokeCacheMB"]), 10, 64)
	if err != nil || n < 100 {
		n = 2048
	}
	return n << 20
}

// karaokeTidy keeps what the server made under its cap, after something new was made.
func (s *Server) karaokeTidy(ctx context.Context, it *library.Item) {
	n, freed := mediaextras.EvictKaraoke(s.extrasRoot(), s.karaokeCap(context.WithoutCancel(ctx)), mediaextras.Key(it.ID, it.Path))
	if n > 0 {
		s.Log.Info("old karaoke versions were cleared to make room", "cat", "library", "songs", n, "bytes", freed)
	}
}

func (s *Server) startKaraokeApprox(r *http.Request, it *library.Item) error {
	dir := filepath.Join(s.extrasRoot(), mediaextras.Key(it.ID, it.Path), mediaextras.ApproxDir)
	item := *it
	_, err := s.startMediaJob(fmt.Sprintf("stems-approx:%d", it.ID), userFrom(r),
		tasks.Spec{Type: tasks.TypeOther, Title: "Making a quick karaoke version", Private: true},
		func(ctx context.Context, task *tasks.Handle) error {
			task.Item(item.Title, "")
			err := s.makeApproxInstrumental(ctx, &item, dir)
			switch {
			case errors.Is(err, errNotStereo):
				s.karaoke.fail("approx", item.ID, "mono")
			case err != nil && ctx.Err() == nil:
				s.karaoke.fail("approx", item.ID, "failed")
			}
			return err
		})
	return err
}

func (s *Server) startKaraokeReal(r *http.Request, it *library.Item, words []string) error {
	if _, _, err := mediaextras.SeparatorCommand(words, "in", "out"); err != nil {
		return err
	}
	item := *it
	_, err := s.startMediaJob(fmt.Sprintf("stems-separate:%d", it.ID), userFrom(r),
		tasks.Spec{Type: tasks.TypeOther, Title: "Separating a song into stems", Phase: "Waiting", AdminOnly: true},
		func(ctx context.Context, task *tasks.Handle) error {
			task.Item(item.Title, "")
			err := s.separateStems(ctx, &item, words, task)
			if err != nil && ctx.Err() == nil {
				s.karaoke.fail("real", item.ID, "failed")
			}
			return err
		})
	return err
}

// compactStems turns the WAV and FLAC files a separator wrote under dir into AAC, when
// FFmpeg is there to do it, before they are kept. A file that can't be converted stays as
// it is.
func (s *Server) compactStems(ctx context.Context, dir string, depth int) {
	if s.Tools == nil || s.Tools.FFmpeg == "" || depth > 4 {
		return
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) > 200 {
		entries = entries[:200]
	}
	for _, e := range entries {
		src := filepath.Join(dir, e.Name())
		if e.IsDir() {
			s.compactStems(ctx, src, depth+1)
			continue
		}
		ext := strings.ToLower(filepath.Ext(src))
		if (ext != ".wav" && ext != ".flac") || !e.Type().IsRegular() {
			continue
		}
		if pace.Yield(ctx) != nil {
			return
		}
		out := strings.TrimSuffix(src, filepath.Ext(src)) + ".m4a"
		args := append([]string{"-v", "error", "-nostdin", "-y", "-threads", "1"}, ffmpeg.FileInput(src)...)
		args = append(args, mediaextras.CompactArgs(out)...)
		cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		_, _, err := ffmpeg.Run(cctx, s.Tools.FFmpeg, args, "for", "compacting a stem")
		cancel()
		if fi, serr := os.Stat(out); err != nil || serr != nil || fi.Size() == 0 {
			os.Remove(out)
			continue
		}
		os.Remove(src)
	}
}

func (s *Server) karaokeSong(w http.ResponseWriter, r *http.Request) *library.Item {
	return s.extrasItem(w, r)
}

// GET /api/media/{id}/karaoke
func (s *Server) handleKaraoke(w http.ResponseWriter, r *http.Request) {
	if it := s.karaokeSong(w, r); it != nil {
		writeJSON(w, 200, s.karaokeFor(r, it, false))
	}
}

// POST /api/media/{id}/karaoke/prepare: start making what is missing, and answer as GET does.
func (s *Server) handleKaraokePrepare(w http.ResponseWriter, r *http.Request) {
	if it := s.karaokeSong(w, r); it != nil {
		writeJSON(w, 200, s.karaokeFor(r, it, true))
	}
}

// GET /api/karaoke/status: what Settings shows. look=1 looks for a separator again.
func (s *Server) handleKaraokeStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.URL.Query().Get("look") == "1" {
		s.foundSeparator(ctx, true)
	}
	words, name, version, source := s.separator(ctx)
	used, songs := mediaextras.KaraokeUsage(s.extrasRoot())
	machine := pace.Machine()
	writeJSON(w, 200, map[string]any{
		"separator":   map[string]any{"available": words != nil, "name": name, "version": version, "source": source, "usable": words != nil && machine != pace.Tiny},
		"approximate": s.Tools != nil && s.Tools.FFmpeg != "",
		"machine":     machine.String(),
		"auto":        s.DB.GetSetting(ctx, "karaokeAuto", settingDefaults["karaokeAuto"]),
		"cacheMB":     s.karaokeCap(ctx) >> 20,
		"usedBytes":   used,
		"songs":       songs,
	})
}
