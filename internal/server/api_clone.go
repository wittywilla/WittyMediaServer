// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Cloning a station (docs/CHANNELS.md, "Clone a station"): reading a guide from where the owner
// points, matching it to the library for the editor to preview, the stand-ins one slot could
// have, and reading the guide again each week for clones that follow their station.

// maxChannelBody: a channel with a cloned week in it is longer than most requests.
const maxChannelBody = 4 << 20

// maxGuideBytes bounds a guide read from a link or an upload, compressed or not.
const maxGuideBytes = 200 << 20

// tvmazeBase is TVmaze's public API; tests point it elsewhere.
var tvmazeBase = "https://api.tvmaze.com"

func (s *Server) cloneRoutes(m *http.ServeMux) {
	ad := s.requireAdmin
	m.Handle("POST /api/channels/clone/upload", ad(s.handleCloneUpload))
	m.Handle("POST /api/channels/clone/stations", ad(s.handleCloneStations))
	m.Handle("POST /api/channels/clone/import", ad(s.handleCloneImport))
	m.Handle("POST /api/channels/clone/candidates", ad(s.handleCloneCandidates))
	if s.DB != nil && s.Cfg != nil {
		s.startCloneRefresh()
	}
}

// cloneRequest is what the editor sends: where the guide is, which station, which days.
type cloneRequest struct {
	Source  channels.CloneSource  `json:"source"`
	Station string                `json:"station"`
	From    string                `json:"from"`
	Days    int                   `json:"days"`
	Options channels.CloneOptions `json:"options"`
	// Slots: match these again (after the options changed) rather than reading the guide.
	Slots []channels.CloneSlot `json:"slots,omitempty"`
}

func (c *cloneRequest) window() (channels.Window, error) {
	from, err := time.Parse("2006-01-02", c.From)
	if err != nil {
		return channels.Window{}, errors.New("choose the first day to clone")
	}
	if c.Days < 1 || c.Days > 7 {
		return channels.Window{}, errors.New("clone from 1 to 7 days")
	}
	return channels.Window{From: from, Days: c.Days}, nil
}

// ---------------------------------------------------------------------------
// Uploads
// ---------------------------------------------------------------------------

func (s *Server) cloneGuideDir() string { return filepath.Join(s.Cfg.DataDir, "clone-guides") }

var uploadID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// handleCloneUpload keeps an XMLTV file (gzipped or not) for a week, for the editor to read.
func (s *Server) handleCloneUpload(w http.ResponseWriter, r *http.Request) {
	dir := s.cloneGuideDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	pruneDir(dir, 7*24*time.Hour)
	var b [16]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	f, err := os.CreateTemp(dir, "up-*")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	n, err := io.Copy(f, io.LimitReader(r.Body, maxGuideBytes+1))
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil || n > maxGuideBytes || n == 0 {
		os.Remove(f.Name())
		msg := "that guide could not be read"
		if n > maxGuideBytes {
			msg = "that guide is larger than 200 MB"
		}
		writeErr(w, 400, msg)
		return
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, "upload-"+id+".xml")); err != nil {
		os.Remove(f.Name())
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"url": "upload:" + id, "bytes": n})
}

// pruneDir removes files older than age.
func pruneDir(dir string, age time.Duration) {
	list, _ := os.ReadDir(dir)
	for _, e := range list {
		if fi, err := e.Info(); err == nil && !e.IsDir() && time.Since(fi.ModTime()) > age {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// ---------------------------------------------------------------------------
// Reading guides
// ---------------------------------------------------------------------------

// openXMLTV opens an XMLTV guide: an upload, or a link read once and kept for half an hour, so
// that choosing a station and then importing it reads the link only once. "{key}" in a link is
// the owner's guide key from Advanced settings, which is never kept in the channel.
func (s *Server) openXMLTV(ctx context.Context, src string) (io.ReadCloser, error) {
	dir := s.cloneGuideDir()
	if id, ok := strings.CutPrefix(src, "upload:"); ok {
		if !uploadID.MatchString(id) {
			return nil, errors.New("that upload isn't there")
		}
		f, err := os.Open(filepath.Join(dir, "upload-"+id+".xml"))
		if err != nil {
			return nil, errors.New("that upload has gone: add the file again")
		}
		return f, nil
	}
	link := strings.TrimSpace(src)
	keyed, keyHost, key := strings.Contains(link, "{key}"), "", ""
	if keyed {
		var err error
		if keyHost, err = s.guideKeyHost(ctx, link); err != nil {
			return nil, err
		}
		key = s.DB.GetSetting(ctx, "guideKey", "")
		if key == "" {
			return nil, errors.New("this link needs your guide key: add it under Advanced")
		}
		logging.AddSecret(key)
		logging.AddSecret(url.QueryEscape(key))
		link = strings.ReplaceAll(link, "{key}", url.QueryEscape(key))
	}
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("a guide link starts with http:// or https://")
	}
	sum := sha256.Sum256([]byte(link))
	cached := filepath.Join(dir, "link-"+hex.EncodeToString(sum[:12])+".xml")
	if fi, err := os.Stat(cached); err == nil && time.Since(fi.ModTime()) < 30*time.Minute {
		return os.Open(cached)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	pruneDir(dir, 7*24*time.Hour)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
	if err != nil {
		return nil, errors.New("that guide link can't be read")
	}
	req.Header.Set("User-Agent", "WittyMediaServer guide import")
	// Links on the internet only: a guide on the home network is added in WittyTuner, or
	// uploaded, so that a link can't be used to reach other devices.
	client := library.SafeClient
	if keyed {
		client = keyedGuideClient(keyHost)
	}
	resp, err := client.Do(req)
	if err != nil {
		msg := err.Error()
		if keyed {
			// The address in the error holds the key: never echo it, whatever the log would hide.
			msg = strings.ReplaceAll(strings.ReplaceAll(msg, url.QueryEscape(key), "…"), key, "…")
		}
		return nil, fmt.Errorf("the guide couldn't be read from that link (a guide on your own network: upload the file, or add it to WittyTuner): %s", logging.Redact(msg))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("the guide's site answered %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(dir, "dl-*")
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxGuideBytes+1))
	tmp.Close()
	if err != nil || n > maxGuideBytes {
		os.Remove(tmp.Name())
		if err == nil {
			err = errors.New("that guide is larger than 200 MB")
		}
		return nil, err
	}
	if err := os.Rename(tmp.Name(), cached); err != nil {
		os.Remove(tmp.Name())
		return nil, err
	}
	return os.Open(cached)
}

// tvmazeCache keeps days of TVmaze's schedule for an hour; it asks that clients be gentle.
var tvmazeCache = struct {
	sync.Mutex
	m map[string]tvmazeDay
}{m: map[string]tvmazeDay{}}

type tvmazeDay struct {
	at   time.Time
	body []byte
}

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

// tvmazeSchedule reads one day of one country's schedule.
func tvmazeSchedule(ctx context.Context, country string, day time.Time) ([]byte, error) {
	if !countryCode.MatchString(country) {
		return nil, errors.New("choose a country")
	}
	key := country + day.Format("2006-01-02")
	tvmazeCache.Lock()
	if d, ok := tvmazeCache.m[key]; ok && time.Since(d.at) < time.Hour {
		tvmazeCache.Unlock()
		return d.body, nil
	}
	tvmazeCache.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	q := url.Values{"country": {country}, "date": {day.Format("2006-01-02")}}
	req, err := http.NewRequestWithContext(ctx, "GET", tvmazeBase+"/schedule?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "WittyMediaServer guide import")
	resp, err := library.SafeClient.Do(req)
	if err != nil {
		return nil, errors.New("TVmaze couldn't be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return nil, errors.New("TVmaze asks to wait a moment: try again shortly")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TVmaze answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	tvmazeCache.Lock()
	if len(tvmazeCache.m) > 64 {
		clear(tvmazeCache.m)
	}
	tvmazeCache.m[key] = tvmazeDay{time.Now(), body}
	tvmazeCache.Unlock()
	return body, nil
}

func (s *Server) tvmazeDays(ctx context.Context, country string, w channels.Window) ([][]byte, error) {
	var out [][]byte
	for i := range w.Days {
		b, err := tvmazeSchedule(ctx, country, w.From.AddDate(0, 0, i))
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// cloneStations lists the stations a source has, and whose data it is.
func (s *Server) cloneStations(ctx context.Context, src channels.CloneSource, w channels.Window) ([]channels.Station, string, error) {
	switch src.Kind {
	case "xmltv":
		f, err := s.openXMLTV(ctx, src.URL)
		if err != nil {
			return nil, "", err
		}
		defer f.Close()
		st, err := channels.XMLTVStations(f, w)
		if err != nil && len(st) == 0 {
			return nil, "", errors.New("that isn't an XMLTV guide")
		}
		return st, "", nil
	case "guide":
		from := time.Date(w.From.Year(), w.From.Month(), w.From.Day(), 0, 0, 0, 0, s.Channels.Location())
		st, err := s.Channels.GuideStations(ctx, from, from.AddDate(0, 0, w.Days))
		return st, "", err
	case "tvmaze":
		days, err := s.tvmazeDays(ctx, src.Country, w)
		if err != nil {
			return nil, "", err
		}
		entries, err := parseTVmazeDays(days)
		if err != nil {
			return nil, "", err
		}
		all := channels.TVmazeStations(entries...)
		return all, channels.TVmazeCredit, nil
	}
	return nil, "", errors.New("choose where the guide comes from")
}

func parseTVmazeDays(days [][]byte) ([][]channels.TVmazeEntry, error) {
	var out [][]channels.TVmazeEntry
	for _, b := range days {
		e, err := channels.ParseTVmaze(b)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// cloneSlots reads one station's programmes.
func (s *Server) cloneSlots(ctx context.Context, src channels.CloneSource, w channels.Window) ([]channels.CloneSlot, string, error) {
	if src.Channel == "" {
		return nil, "", errors.New("choose a station")
	}
	switch src.Kind {
	case "xmltv":
		f, err := s.openXMLTV(ctx, src.URL)
		if err != nil {
			return nil, "", err
		}
		defer f.Close()
		slots, err := channels.XMLTVSlots(f, src.Channel, w)
		if err != nil && len(slots) == 0 {
			return nil, "", errors.New("that isn't an XMLTV guide")
		}
		return slots, "", nil
	case "guide":
		from := time.Date(w.From.Year(), w.From.Month(), w.From.Day(), 0, 0, 0, 0, s.Channels.Location())
		slots, err := s.Channels.GuideSlots(ctx, src.Channel, from, from.AddDate(0, 0, w.Days))
		return slots, "", err
	case "tvmaze":
		days, err := s.tvmazeDays(ctx, src.Country, w)
		if err != nil {
			return nil, "", err
		}
		entries, err := parseTVmazeDays(days)
		if err != nil {
			return nil, "", err
		}
		return channels.TVmazeSlots(src.Channel, entries...), channels.TVmazeCredit, nil
	}
	return nil, "", errors.New("choose where the guide comes from")
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (s *Server) handleCloneStations(w http.ResponseWriter, r *http.Request) {
	var b cloneRequest
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	win, err := b.window()
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	st, credit, err := s.cloneStations(r.Context(), b.Source, win)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if st == nil {
		st = []channels.Station{}
	}
	writeJSON(w, 200, map[string]any{"stations": st, "credit": credit})
}

// buildClone reads a station's guide (or takes the slots given) and matches it to the library.
func (s *Server) buildClone(ctx context.Context, b *cloneRequest) (*channels.Clone, error) {
	win, err := b.window()
	if err != nil {
		return nil, err
	}
	c := &channels.Clone{Source: b.Source, Station: strings.TrimSpace(b.Station), From: b.From, Days: b.Days, Options: b.Options, Slots: b.Slots}
	if c.Slots == nil {
		slots, credit, err := s.cloneSlots(ctx, b.Source, win)
		if err != nil {
			return nil, err
		}
		if len(slots) == 0 {
			return nil, errors.New("the guide has nothing for that station on those days")
		}
		c.Slots, c.Credit, c.ImportedAt = slots, credit, db.Now()
	}
	if len(c.Slots) > 1500 {
		c.Slots = c.Slots[:1500]
	}
	lib, err := s.Channels.CloneLibrary(ctx, c.Options.Libraries)
	if err != nil {
		return nil, err
	}
	lib.Match(c.Slots, c.Options)
	return c, nil
}

func (s *Server) handleCloneImport(w http.ResponseWriter, r *http.Request) {
	var b cloneRequest
	if err := readJSONUpTo(r, &b, maxChannelBody); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c, err := s.buildClone(r.Context(), &b)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ads, err := s.Channels.CommercialLibraries(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	counts := map[string]int{}
	for _, sl := range c.Slots {
		counts[sl.Rule]++
	}
	if ads == nil {
		ads = []int64{}
	}
	writeJSON(w, 200, map[string]any{"clone": c, "commercials": ads, "counts": counts})
}

func (s *Server) handleCloneCandidates(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Slot    channels.CloneSlot    `json:"slot"`
		Options channels.CloneOptions `json:"options"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	lib, err := s.Channels.CloneLibrary(r.Context(), b.Options.Libraries)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"candidates": lib.Candidates(b.Slot, b.Options, 12)})
}

// ---------------------------------------------------------------------------
// Following the station: a weekly re-import
// ---------------------------------------------------------------------------

var cloneRefreshOnce sync.Once

func (s *Server) startCloneRefresh() {
	cloneRefreshOnce.Do(func() {
		s.background("clone refresh", func(ctx context.Context) {
			t := time.NewTicker(6 * time.Hour)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Minute):
				}
				s.refreshClones(ctx, time.Now())
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}, "cat", "tv")
	})
}

// refreshClones reads again, for the coming week, the guide of every clone that follows its
// station and was last read a week or more ago. What was chosen by hand stays chosen where the
// same programme is in the same slot.
func (s *Server) refreshClones(ctx context.Context, now time.Time) {
	list, err := s.Channels.List(ctx)
	if err != nil {
		return
	}
	for _, ch := range list {
		c := ch.Definition.Clone
		if c == nil || !c.Options.Refresh || strings.HasPrefix(c.Source.URL, "upload:") || now.Unix()-c.ImportedAt < 7*24*3600 {
			continue
		}
		if err := s.refreshClone(ctx, ch, now); err != nil {
			slog.Warn("a cloned station's guide could not be read again", "channel", ch.ID, "err", err, "cat", "tv")
		}
	}
}

func (s *Server) refreshClone(ctx context.Context, ch *channels.Channel, now time.Time) error {
	c := ch.Definition.Clone
	from, err := time.Parse("2006-01-02", c.From)
	if err != nil {
		return err
	}
	// The same weekdays, from the coming one on.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for from.Before(today) {
		from = from.AddDate(0, 0, 7)
	}
	b := cloneRequest{Source: c.Source, Station: c.Station, From: from.Format("2006-01-02"), Days: c.Days, Options: c.Options}
	fresh, err := s.buildClone(ctx, &b)
	if err != nil {
		return err
	}
	channels.CarryPins(c.Slots, fresh.Slots)
	lib, err := s.Channels.CloneLibrary(ctx, c.Options.Libraries)
	if err != nil {
		return err
	}
	lib.Match(fresh.Slots, fresh.Options)
	ch.Definition.Clone = fresh
	return s.Channels.Update(ctx, ch)
}
