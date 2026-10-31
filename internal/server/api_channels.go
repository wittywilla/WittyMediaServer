package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
)

// TV channels: the library channels' API, the guide shared with tuner
// channels, and tuning in. See docs/CHANNELS.md.

func (s *Server) channelRoutes(m *http.ServeMux) {
	a := s.requireUser
	ad := s.requireAdmin
	if s.Channels == nil && s.DB != nil {
		s.Channels = &channels.Store{DB: s.DB}
	}
	m.Handle("GET /api/guide", a(s.handleGuide))
	m.Handle("GET /api/tv/{key}/tune", a(s.handleTune))
	m.Handle("GET /api/tv/{key}/live/playlist.m3u8", a(s.handleLivePlaylist))
	m.Handle("GET /api/tv/{key}/live/seg/{name}", a(s.handleLiveSegment))
	m.Handle("POST /api/tv/{key}/leave", a(s.handleLiveLeave))
	m.Handle("GET /api/tv/settings", ad(s.handleTVSettings))
	m.Handle("PUT /api/tv/settings", ad(s.handlePutTVSettings))
	m.Handle("PUT /api/libraries/{id}/main", ad(s.handleLibraryShowMain))
	m.Handle("GET /api/channels", a(s.handleChannels))
	m.Handle("POST /api/channels", ad(s.handleCreateChannel))
	m.Handle("POST /api/channels/preview", ad(s.handleChannelPreview))
	m.Handle("GET /api/channels/suggestions", ad(s.handleChannelSuggestions))
	m.Handle("GET /api/channels/options", ad(s.handleChannelOptions))
	m.Handle("POST /api/channels/from-network", ad(s.handleChannelFromNetwork))
	m.Handle("POST /api/channels/networks/refresh", ad(s.handleRefreshNetworks))
	m.Handle("GET /api/channels/networks/refresh", ad(s.handleRefreshNetworksProgress))
	m.Handle("GET /api/channels/{id}", ad(s.handleChannel))
	m.Handle("PUT /api/channels/{id}", ad(s.handleUpdateChannel))
	m.Handle("DELETE /api/channels/{id}", ad(s.handleDeleteChannel))
	m.Handle("POST /api/channels/{id}/rebuild", ad(s.handleRebuildChannel))
	m.Handle("GET /api/channels/{id}/history", ad(s.handleChannelHistory))
	m.Handle("POST /api/channels/{id}/restore", ad(s.handleRestoreChannel))
	m.Handle("GET /api/channels/{id}/logo", a(s.handleChannelLogo))
	m.Handle("PUT /api/channels/{id}/logo", ad(s.handleSetChannelLogo))
	m.Handle("DELETE /api/channels/{id}/logo", ad(s.handleDeleteChannelLogo))
	s.tunerRoutes(m)
}

// ---------------------------------------------------------------------------
// Managing library channels
// ---------------------------------------------------------------------------

// channelView is what an account sees of a channel; only administrators get
// the definition.
type channelView struct {
	ID          int64  `json:"id"`
	Number      string `json:"number"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Logo        string `json:"logo"`
}

// PUT /api/libraries/{id}/main: whether a commercials library shows in the
// menus and on Home. Channels can use it either way.
func (s *Server) handleLibraryShowMain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b struct {
		Show bool `json:"show"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.Store.SetLibraryShowMain(r.Context(), id, b.Show); err != nil {
		writeErr(w, 404, "no such commercials library")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	list, err := s.Channels.List(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	u := userFrom(r)
	if u.IsAdmin {
		writeJSON(w, 200, list)
		return
	}
	a := s.access(r)
	out := []channelView{}
	for _, c := range list {
		if s.Channels.Visible(r.Context(), c, u, a) {
			out = append(out, channelView{c.ID, c.Number, c.Name, c.Description, c.Logo})
		}
	}
	writeJSON(w, 200, out)
}

func channelError(w http.ResponseWriter, err error) {
	if errors.Is(err, channels.ErrNotFound) {
		writeErr(w, 404, "channel not found")
		return
	}
	writeErr(w, 400, err.Error())
}

func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var c channels.Channel
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, "that channel could not be read")
		return
	}
	s.fillWatchedBy(&c, r)
	if err := s.Channels.Create(r.Context(), &c, userFrom(r).ID); err != nil {
		channelError(w, err)
		return
	}
	writeJSON(w, 200, c)
}

// fillWatchedBy: "watched" and "unwatched" rules go by the person who wrote
// them, unless they chose someone else.
func (s *Server) fillWatchedBy(c *channels.Channel, r *http.Request) {
	me := userFrom(r).ID
	fix := func(x *channels.Rules) {
		if x != nil && x.Watched != "" && x.WatchedBy == 0 {
			x.WatchedBy = me
		}
	}
	fix(&c.Definition.Rules)
	fix(c.Definition.Filler.Rules)
	for i := range c.Definition.Blocks {
		fix(c.Definition.Blocks[i].Rules)
	}
}

func (s *Server) handleChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	c, err := s.Channels.Get(r.Context(), id)
	if err != nil {
		channelError(w, err)
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) handleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	c, err := s.Channels.Get(r.Context(), id)
	if err != nil {
		channelError(w, err)
		return
	}
	// The body replaces what it names; nothing else changes.
	if err := readJSON(r, c); err != nil {
		writeErr(w, 400, "that channel could not be read")
		return
	}
	c.ID = id
	s.fillWatchedBy(c, r)
	if err := s.Channels.Update(r.Context(), c); err != nil {
		channelError(w, err)
		return
	}
	c, err = s.Channels.Get(r.Context(), id)
	if err != nil {
		channelError(w, err)
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.Channels.Delete(r.Context(), id); err != nil {
		channelError(w, err)
		return
	}
	s.HLS.StopLive("lib-" + strconv.FormatInt(id, 10))
	removeChannelLogos(s.channelLogoDir(), id)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleRebuildChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.Channels.Rebuild(r.Context(), id, qBool(r, "reshuffle")); err != nil {
		channelError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleChannelHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	h, err := s.Channels.History(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, h)
}

func (s *Server) handleRestoreChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b struct {
		History int64 `json:"history"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c, err := s.Channels.Restore(r.Context(), id, b.History)
	if err != nil {
		channelError(w, err)
		return
	}
	writeJSON(w, 200, c)
}

// handleChannelPreview shows what a draft would do without saving it: how
// much it chooses and its first day.
func (s *Server) handleChannelPreview(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Definition channels.Definition `json:"definition"`
		Day        int                 `json:"day"` // days from today
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "that channel could not be read")
		return
	}
	if err := b.Definition.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Day < 0 || b.Day > 14 {
		writeErr(w, 400, "preview up to two weeks ahead")
		return
	}
	stats, err := s.Channels.Stats(r.Context(), b.Definition.Rules)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	loader := func(rules channels.Rules, minSec int64) ([]channels.Entry, error) {
		return s.Channels.LoadPool(r.Context(), rules, minSec)
	}
	cache := map[string][]channels.Entry{}
	today := channels.DayNumber(time.Now(), time.Local)
	var state channels.State
	var progs []channels.Programme
	for d := 0; d <= b.Day; d++ {
		progs, state, err = channels.GenerateDay(&b.Definition, 1, today+d, time.Local, state, loader, cache)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if len(progs) > 400 {
		progs = progs[:400]
	}
	writeJSON(w, 200, map[string]any{"stats": stats, "programmes": progs})
}

func (s *Server) handleChannelSuggestions(w http.ResponseWriter, r *http.Request) {
	out, err := s.Channels.Suggest(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if out == nil {
		out = []channels.Suggestion{}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleChannelOptions(w http.ResponseWriter, r *http.Request) {
	o, err := s.Channels.EditorOptions(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, o)
}

// handleChannelFromNetwork makes a network's definition in a chosen style,
// to save straight away or open in the editor.
func (s *Server) handleChannelFromNetwork(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Network  string `json:"network"`
		Style    string `json:"style"`
		YearFrom int    `json:"yearFrom"`
		YearTo   int    `json:"yearTo"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(b.Network) == "" || len(b.Network) > 200 {
		writeErr(w, 400, "choose a network")
		return
	}
	def, err := s.Channels.BuildNetwork(r.Context(), b.Network, b.Style, b.YearFrom, b.YearTo)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := def.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	stats, err := s.Channels.Stats(r.Context(), def.Rules)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"definition": def, "stats": stats})
}

func (s *Server) handleRefreshNetworks(w http.ResponseWriter, r *http.Request) {
	s.Scanner.FillNetworkFacts()
	writeJSON(w, 200, library.NetworkFactsProgress())
}

func (s *Server) handleRefreshNetworksProgress(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, library.NetworkFactsProgress())
}

// ---------------------------------------------------------------------------
// Logos: the owner's own pictures. None are bundled.
// ---------------------------------------------------------------------------

func (s *Server) channelLogoDir() string { return filepath.Join(s.Cfg.DataDir, "channel-logos") }

func channelLogoPath(dir string, id int64, format string) string {
	ext := ".png"
	if format == "jpeg" {
		ext = ".jpg"
	}
	return filepath.Join(dir, strconv.FormatInt(id, 10)+ext)
}

func removeChannelLogos(dir string, id int64) {
	for f := range logoTypes {
		os.Remove(channelLogoPath(dir, id, f))
	}
}

func (s *Server) handleChannelLogo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	for format, ctype := range logoTypes {
		b, err := os.ReadFile(channelLogoPath(s.channelLogoDir(), id, format))
		if err != nil {
			continue
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "private, max-age=86400") // the address carries ?v=
		_, _ = w.Write(b)
		return
	}
	writeErr(w, 404, "no logo")
}

func (s *Server) handleSetChannelLogo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if _, err := s.Channels.Get(r.Context(), id); err != nil {
		channelError(w, err)
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxLogoBytes))
	if err != nil {
		writeErr(w, 413, "the logo must be 2 MB or smaller")
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || logoTypes[format] == "" {
		writeErr(w, 400, "use a PNG or JPEG image")
		return
	}
	if cfg.Width < 16 || cfg.Height < 16 || cfg.Width > 4096 || cfg.Height > 4096 {
		writeErr(w, 400, "the logo must be between 16 and 4096 pixels on each side")
		return
	}
	dir := s.channelLogoDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	removeChannelLogos(dir, id)
	dst := channelLogoPath(dir, id, format)
	if err := os.WriteFile(dst+".part", b, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := os.Rename(dst+".part", dst); err != nil {
		os.Remove(dst + ".part")
		writeErr(w, 500, err.Error())
		return
	}
	v := strconv.FormatInt(time.Now().Unix(), 10)
	if err := s.Channels.SetLogo(r.Context(), id, v); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"logo": v})
}

func (s *Server) handleDeleteChannelLogo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	removeChannelLogos(s.channelLogoDir(), id)
	if err := s.Channels.SetLogo(r.Context(), id, ""); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"logo": ""})
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

// tvSettings are stored in the settings table under these names.
var tvSettings = map[string]string{
	"tvLibraryBufferMin": "5",  // minutes kept for pausing a library channel
	"tvTunerBufferMin":   "30", // and a tuner channel
	"tvCopyVideo":        "1",  // pass H.264 through instead of converting it
	"tvMaxHeight":        "0",  // 0 = the server's general limit
	"tvSkin":             "cable",
	"tvLayout":           "classic", // classic | streaming: what a new viewer starts with
	"dvrFolder":          "",        // "" = recordings in the data folder
	"dvrLibrary":         "",        // a library to rescan after recording
}

func (s *Server) tvSetting(ctx context.Context, k string) string {
	return s.DB.GetSetting(ctx, k, tvSettings[k])
}

func (s *Server) tvSettingInt(ctx context.Context, k string) int {
	n, _ := strconv.Atoi(s.tvSetting(ctx, k))
	return n
}

func (s *Server) handleTVSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{}
	for k := range tvSettings {
		out[k] = s.tvSetting(r.Context(), k)
	}
	writeJSON(w, 200, out)
}

func (s *Server) handlePutTVSettings(w http.ResponseWriter, r *http.Request) {
	var b map[string]string
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	for k, v := range b {
		if _, known := tvSettings[k]; !known {
			writeErr(w, 400, "unknown setting: "+k)
			return
		}
		v = strings.TrimSpace(v)
		switch k {
		case "tvLibraryBufferMin", "tvTunerBufferMin":
			if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 240 {
				writeErr(w, 400, "the buffer can be from 1 to 240 minutes")
				return
			}
		case "tvMaxHeight":
			if n, err := strconv.Atoi(v); err != nil || n < 0 || n > 4320 {
				writeErr(w, 400, "bad picture height")
				return
			}
		case "tvSkin":
			if v != "cable" && v != "satellite" && v != "theme" {
				writeErr(w, 400, "unknown guide style")
				return
			}
		case "tvLayout":
			if v != "classic" && v != "streaming" {
				writeErr(w, 400, "unknown guide layout")
				return
			}
		case "tvCopyVideo":
			if v != "0" && v != "1" {
				writeErr(w, 400, "bad value")
				return
			}
		case "dvrFolder":
			if v != "" && !filepath.IsAbs(v) {
				writeErr(w, 400, "the recordings folder must be a full path")
				return
			}
		case "dvrLibrary":
			if _, err := strconv.ParseInt(v, 10, 64); v != "" && err != nil {
				writeErr(w, 400, "bad library")
				return
			}
		}
		if err := s.DB.SetSetting(r.Context(), k, v); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	s.handleTVSettings(w, r)
}

// ---------------------------------------------------------------------------
// The guide
// ---------------------------------------------------------------------------

// guideProgramme is one programme, library or broadcast, in one format.
type guideProgramme struct {
	Start   int64  `json:"start"`
	End     int64  `json:"end"`
	Kind    string `json:"kind"` // show | filler | slate | broadcast
	Title   string `json:"title"`
	Sub     string `json:"sub,omitempty"`
	Desc    string `json:"desc,omitempty"`
	Genre   string `json:"genre,omitempty"`
	Rating  string `json:"rating,omitempty"`
	Year    int    `json:"year,omitempty"`
	ItemID  int64  `json:"itemId,omitempty"`
	ShowID  int64  `json:"showId,omitempty"`
	Season  *int   `json:"season,omitempty"`
	Episode *int   `json:"episode,omitempty"`
}

// guideChannel is one row of the guide.
type guideChannel struct {
	Key       string `json:"key"` // lib-5, tun-12
	Kind      string `json:"kind"`
	ID        int64  `json:"id"`
	Number    string `json:"number"`
	Name      string `json:"name"`
	Group     string `json:"group,omitempty"`
	Logo      string `json:"logo,omitempty"`
	HD        bool   `json:"hd,omitempty"`
	Radio     bool   `json:"radio,omitempty"`
	Encrypted string `json:"encrypted,omitempty"` // why it cannot be tuned
	CanRecord bool   `json:"canRecord,omitempty"`
	NoRecord  string `json:"noRecord,omitempty"` // why not, when the device says
	ATSC3     bool   `json:"atsc3,omitempty"`
	// Security is the same wording for every client. Tuner channels only: a
	// library channel plays the owner's own files.
	Security    *securityInfo       `json:"security,omitempty"`
	Alternative *livetv.Alternative `json:"alternative,omitempty"` // the same station, clear
	Notices     []string            `json:"notices,omitempty"`
	Programmes  []guideProgramme    `json:"programmes"`
}

// securityInfo is whether a channel is encrypted, in words.
type securityInfo struct {
	State   string `json:"state"` // clear | encrypted | unknown
	Label   string `json:"label"`
	Detail  string `json:"detail"`
	Reason  string `json:"reason,omitempty"`
	HelpURL string `json:"helpUrl,omitempty"`
}

const encryptedHelpURL = "/api/tv/encrypted-help"

func fromSchedule(p channels.Programme) guideProgramme {
	g := guideProgramme{Start: p.Start, End: p.End, Kind: p.Kind, Title: p.Title, Sub: p.Sub, Genre: p.Genre, Rating: p.Rating, Year: p.Year,
		ItemID: p.ItemID, ShowID: p.ShowID, Season: p.Season, Episode: p.Episode}
	if p.Kind == channels.KindSlate {
		g.Title = "We'll be right back"
	}
	return g
}

func (s *Server) libraryRows(ctx context.Context, u *http.Request) []guideChannel {
	list, err := s.Channels.List(ctx)
	if err != nil {
		s.Log.WarnContext(ctx, "could not read the guide's channels", "cat", "tv", "err", err)
		return nil
	}
	user, a := userFrom(u), s.access(u)
	out := make([]guideChannel, 0, len(list))
	for _, c := range list {
		if !s.Channels.Visible(ctx, c, user, a) {
			continue
		}
		row := guideChannel{Key: "lib-" + strconv.FormatInt(c.ID, 10), Kind: "library", ID: c.ID, Number: c.Number, Name: c.Name}
		if c.Logo != "" {
			row.Logo = fmt.Sprintf("/api/channels/%d/logo?v=%s", c.ID, c.Logo)
		}
		out = append(out, row)
	}
	return out
}

// GET /api/guide?from=UNIX&hours=3&offset=0&limit=40&kind=library&group=News
func (s *Server) handleGuide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()
	from := now.Truncate(30 * time.Minute)
	if v := qInt(r, "from", 0); v > 0 {
		from = time.Unix(int64(v), 0)
	}
	if from.Before(now.AddDate(0, 0, -1)) || from.After(now.AddDate(0, 0, 15)) {
		writeErr(w, 400, "the guide covers yesterday to two weeks ahead")
		return
	}
	hours := qInt(r, "hours", 3)
	if hours < 1 || hours > 48 {
		writeErr(w, 400, "the guide shows 1 to 48 hours at a time")
		return
	}
	to := from.Add(time.Duration(hours) * time.Hour)
	limit := qInt(r, "limit", 40)
	numbersOnly := qBool(r, "list")
	if limit < 1 || (limit > 200 && !numbersOnly) || limit > 20000 {
		limit = 40
	}
	offset := max(qInt(r, "offset", 0), 0)
	kind := r.URL.Query().Get("kind")
	group := r.URL.Query().Get("group")
	rows := []guideChannel{}
	if kind == "" || kind == "library" {
		rows = append(rows, s.libraryRows(ctx, r)...)
	}
	if kind == "" || kind == "tuner" {
		rows = append(rows, s.tunerRows(ctx, r)...)
	}
	groups := map[string]bool{}
	for _, c := range rows {
		if c.Group != "" {
			groups[c.Group] = true
		}
	}
	if group != "" {
		kept := rows[:0]
		for _, c := range rows {
			if c.Group == group {
				kept = append(kept, c)
			}
		}
		rows = kept
	}
	sort.SliceStable(rows, func(i, j int) bool { return channels.NumberLess(rows[i].Number, rows[j].Number) })
	total := len(rows)
	if offset > total {
		offset = total
	}
	rows = rows[offset:min(offset+limit, total)]
	if numbersOnly {
		// Only the channel list: numbers for dialling, no programmes.
		for i := range rows {
			rows[i].Programmes = []guideProgramme{}
		}
	} else {
		for i := range rows {
			rows[i].Programmes = s.rowProgrammes(ctx, &rows[i], from, to)
		}
	}
	names := make([]string, 0, len(groups))
	for g := range groups {
		names = append(names, g)
	}
	sort.Strings(names)
	writeJSON(w, 200, map[string]any{"now": now.Unix(), "from": from.Unix(), "to": to.Unix(), "total": total, "offset": offset,
		"channels": rows, "groups": names, "skin": s.tvSetting(ctx, "tvSkin"), "layout": s.tvSetting(ctx, "tvLayout")})
}

func (s *Server) rowProgrammes(ctx context.Context, row *guideChannel, from, to time.Time) []guideProgramme {
	out := []guideProgramme{}
	if row.Kind != "library" {
		return s.tunerProgrammes(ctx, row, from, to)
	}
	c, err := s.Channels.Get(ctx, row.ID)
	if err != nil {
		return out
	}
	list, err := s.Channels.Window(ctx, c, from, to)
	if err != nil {
		s.Log.WarnContext(ctx, "could not read a channel's guide", "cat", "tv", "channel", row.Name, "err", err)
		return out
	}
	// A rows' worth of bumpers tells a viewer nothing: they join the show
	// they follow in the guide, and short slates vanish.
	for _, p := range list {
		if p.Kind != channels.KindShow && len(out) > 0 && p.End-p.Start < 900 {
			out[len(out)-1].End = p.End
			continue
		}
		out = append(out, fromSchedule(p))
	}
	return out
}

// ---------------------------------------------------------------------------
// Tuning in
// ---------------------------------------------------------------------------

type tuneInfo struct {
	Key           string          `json:"key"`
	Kind          string          `json:"kind"`
	Number        string          `json:"number"`
	Name          string          `json:"name"`
	Logo          string          `json:"logo,omitempty"`
	URL           string          `json:"url"`
	Now           *guideProgramme `json:"now"`
	Next          *guideProgramme `json:"next"`
	PositionSec   float64         `json:"positionSec"`
	ServerTime    int64           `json:"serverTime"`
	CountProgress bool            `json:"countProgress"`
	CanRecord     bool            `json:"canRecord"`
	BufferSec     float64         `json:"bufferSec"`
	ATSC3         bool            `json:"atsc3,omitempty"`
	Security      *securityInfo   `json:"security,omitempty"`
	Notices       []string        `json:"notices,omitempty"`
	// Alternative: the same station with sound, when this one has none.
	Alternative *livetv.Alternative `json:"alternative,omitempty"`
}

func parseKey(key string) (kind string, id int64, ok bool) {
	k, v, found := strings.Cut(key, "-")
	if !found || (k != "lib" && k != "tun") {
		return "", 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return "", 0, false
	}
	return k, n, true
}

func (s *Server) visibleChannel(r *http.Request, id int64) (*channels.Channel, error) {
	c, err := s.Channels.Get(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if !s.Channels.Visible(r.Context(), c, userFrom(r), s.access(r)) {
		return nil, channels.ErrNotFound
	}
	return c, nil
}

// GET /api/tv/{key}/tune: what is on, and starts the shared stream.
func (s *Server) handleTune(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := parseKey(r.PathValue("key"))
	if !ok {
		writeErr(w, 400, "bad channel")
		return
	}
	if kind == "tun" {
		s.handleTuneTuner(w, r, id)
		return
	}
	c, err := s.visibleChannel(r, id)
	if err != nil {
		writeErr(w, 404, "channel not found")
		return
	}
	now := time.Now()
	cur, next, err := s.Channels.Now(r.Context(), c, now)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// The channel's rules passed; this also catches a title whose rating has
	// changed since the schedule was made.
	if cur.ItemID != 0 {
		if _, err := s.visibleItem(r, cur.ItemID); err != nil && !errors.Is(err, library.ErrNotFound) {
			writeErr(w, 500, err.Error())
			return
		} else if err != nil && s.access(r) != nil {
			writeErr(w, 403, "what is on now is not available to this profile")
			return
		}
	}
	ls, err := s.ensureLibraryLive(r, c)
	if err != nil {
		liveError(w, err)
		return
	}
	ls.Viewer(viewerName(r))
	info := tuneInfo{Key: r.PathValue("key"), Kind: "library", Number: c.Number, Name: c.Name, URL: "/api/tv/" + r.PathValue("key") + "/live/playlist.m3u8",
		PositionSec: float64(now.Unix() - cur.Start), ServerTime: now.Unix(), CountProgress: c.CountProgress && cur.Kind == channels.KindShow}
	if c.Logo != "" {
		info.Logo = fmt.Sprintf("/api/channels/%d/logo?v=%s", c.ID, c.Logo)
	}
	g := fromSchedule(*cur)
	info.Now = &g
	if next != nil {
		n := fromSchedule(*next)
		info.Next = &n
	}
	info.BufferSec = float64(s.tvSettingInt(r.Context(), "tvLibraryBufferMin") * 60)
	writeJSON(w, 200, info)
}

// viewerName is the name a player chose for itself, within reason.
func viewerName(r *http.Request) string {
	v := r.URL.Query().Get("viewer")
	if len(v) > 40 {
		return ""
	}
	return v
}

// POST /api/tv/{key}/leave?viewer=x: this player has gone.
func (s *Server) handleLiveLeave(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if _, _, ok := parseKey(key); !ok {
		writeErr(w, 400, "bad channel")
		return
	}
	if v := viewerName(r); v != "" {
		s.HLS.LeaveLive(key, v)
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func liveError(w http.ResponseWriter, err error) {
	if errors.Is(err, hls.ErrLiveBusy) {
		writeErr(w, 503, err.Error())
		return
	}
	writeErr(w, 500, err.Error())
}

func (s *Server) liveQuality(r *http.Request) (ffmpeg.Quality, int, string) {
	q, h := s.quality(r)
	if v := s.tvSettingInt(r.Context(), "tvMaxHeight"); v > 0 && (h == 0 || v < h) {
		h = v
	}
	preset := ""
	if s.DB.GetSetting(r.Context(), "lowPower", "0") == "1" {
		preset = "superfast"
	}
	return q, h, preset
}

func (s *Server) ensureLibraryLive(r *http.Request, c *channels.Channel) (*hls.LiveSession, error) {
	id := "lib-" + strconv.FormatInt(c.ID, 10)
	if ls := s.HLS.GetLive(id); ls != nil {
		ls.Touch()
		return ls, nil
	}
	q, h, preset := s.liveQuality(r)
	copyOK := s.tvSetting(r.Context(), "tvCopyVideo") == "1"
	minutes := s.tvSettingInt(r.Context(), "tvLibraryBufferMin")
	return s.HLS.EnsureLive(hls.LiveSpec{
		ID: id, Quality: q, MaxHeight: h, Preset: preset, WindowSec: float64(minutes * 60),
		Feed: s.libraryFeed(c.ID, q, h, copyOK),
	})
}

// libraryFeed says what a library channel plays next. Runs are paced to real
// time from a short head start, so a run can finish a little before its
// programme's end: the next programme then starts on schedule rather than
// repeating the end of this one.
func (s *Server) libraryFeed(id int64, q ffmpeg.Quality, maxHeight int, copyOK bool) hls.LiveFeed {
	var lastEnd int64
	return func(ctx context.Context, _ time.Time) (*hls.LiveInput, error) {
		c, err := s.Channels.Get(ctx, id)
		if err != nil {
			return nil, &hls.FatalError{Err: errors.New("this channel no longer exists")}
		}
		now := time.Now()
		at := now
		if lastEnd > now.Unix() {
			if lastEnd-now.Unix() > 30 {
				// The file ended well before the schedule said it would.
				remain := float64(lastEnd - now.Unix())
				return &hls.LiveInput{InputArgs: hls.SlateInput(), Slate: true, Duration: remain, AudioIndex: -1}, nil
			}
			at = time.Unix(lastEnd, 0)
		}
		cur, _, err := s.Channels.Now(ctx, c, at)
		if err != nil {
			return nil, err
		}
		lastEnd = cur.End
		off := max(at.Sub(time.Unix(cur.Start, 0)).Seconds(), 0)
		remain := float64(cur.End) - float64(at.Unix())
		if off < 1 {
			off = 0
		}
		if cur.Kind != channels.KindSlate && cur.ItemID != 0 {
			in, err := s.itemLiveInput(ctx, cur.ItemID, off, remain, q, maxHeight, copyOK)
			if err == nil {
				return in, nil
			}
			s.Log.Warn("channel programme unavailable", "cat", "playback", "channel", c.Name, "item", cur.ItemID, "err", err)
		}
		return &hls.LiveInput{InputArgs: hls.SlateInput(), Slate: true, Duration: remain, AudioIndex: -1}, nil
	}
}

// itemLiveInput prepares a library title for a channel.
func (s *Server) itemLiveInput(ctx context.Context, id int64, seek, remain float64, q ffmpeg.Quality, maxHeight int, copyOK bool) (*hls.LiveInput, error) {
	it, err := s.Store.Item(ctx, id)
	if err != nil {
		return nil, err
	}
	if it.Missing {
		return nil, errors.New("the file is missing")
	}
	if library.IsDiscKind(it.Kind) {
		return s.discLiveInput(ctx, it, seek, remain, q, maxHeight)
	}
	pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	pr, err := s.Tools.ProbeFile(pctx, it.Path)
	if err != nil {
		return nil, err
	}
	in := &hls.LiveInput{InputArgs: ffmpeg.FileInput(it.Path), Seek: seek, Duration: remain, AudioIndex: 0}
	if len(pr.Audio()) == 0 {
		in.AudioIndex = -1
	}
	if v := pr.Video(); v != nil {
		interlaced := v.FieldOrder == "tt" || v.FieldOrder == "bb" || v.FieldOrder == "tb" || v.FieldOrder == "bt"
		if interlaced {
			in.VideoFilter = "bwdif=mode=send_frame:parity=auto:deint=interlaced"
		}
		in.CopyVideo = copyOK && !interlaced && canCopy(v, maxHeight)
	} else {
		return nil, errors.New("no video in the file")
	}
	return in, nil
}

// canCopy: 8-bit H.264 every browser can play as it is. Anything else is
// converted.
func canCopy(v *ffmpeg.Stream, maxHeight int) bool {
	if v.CodecName != "h264" || (maxHeight > 0 && v.Height > maxHeight) {
		return false
	}
	return v.PixFmt == "yuv420p" || v.PixFmt == "yuvj420p"
}

// discLiveInput plays a disc's main title through the disc engines, without
// menus. Encrypted discs are refused by the engines and never reach a pool.
func (s *Server) discLiveInput(ctx context.Context, it *library.Item, seek, remain float64, q ffmpeg.Quality, maxHeight int) (*hls.LiveInput, error) {
	if it.Disc != nil && it.Disc.Scrambled {
		return nil, errors.New("this disc is encrypted")
	}
	ttn := 0
	if it.Disc != nil {
		ttn = it.Disc.MainTitle
	}
	prefs := dvdsession.Prefs{Wide: true, Quality: q, MaxHeight: maxHeight, AudioCodec: "aac",
		MenuLanguage: s.DB.GetSetting(ctx, "menuLanguage", "en"), AudioLanguage: s.DB.GetSetting(ctx, "audioLanguage", ""),
		LowPower: s.DB.GetSetting(ctx, "lowPower", "0") == "1"}
	var p *dvdsession.Presentation
	if strings.HasPrefix(it.Kind, "bluray-") {
		sess, err := s.BD.Open(it.ID, it.Path, prefs)
		if err != nil {
			return nil, err
		}
		defer s.BD.CloseSession(sess.ID)
		p = sess.PlayTitle(ctx, ttn, 0)
	} else {
		sess, err := s.DVD.Open(it.ID, it.Path, prefs)
		if err != nil {
			return nil, err
		}
		defer s.DVD.CloseSession(sess.ID)
		p = sess.PlayTitle(ctx, ttn, 0)
	}
	if p == nil || p.Scrambled || p.Media == nil || p.Media.URL == "" {
		return nil, errors.New("the disc's main title cannot be played on a channel")
	}
	id := strings.TrimSuffix(strings.TrimPrefix(p.Media.URL, "/api/hls/"), "/playlist.m3u8")
	vod := s.HLS.Get(id)
	if vod == nil {
		return nil, errors.New("the disc's stream did not start")
	}
	return &hls.LiveInput{InputArgs: vod.Spec.InputArgs, Seek: seek, Duration: remain, VideoFilter: vod.Spec.VideoFilter, AudioIndex: vod.Spec.AudioIndex}, nil
}

// canWatchLive: every request for a stream checks the channel again.
func (s *Server) canWatchLive(r *http.Request, kind string, id int64) bool {
	if kind == "tun" {
		return s.canWatchTuner(r, id)
	}
	_, err := s.visibleChannel(r, id)
	return err == nil
}

func (s *Server) handleLivePlaylist(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	kind, id, ok := parseKey(key)
	if !ok || !s.canWatchLive(r, kind, id) {
		writeErr(w, 404, "channel not found")
		return
	}
	ls := s.HLS.GetLive(key)
	if ls == nil {
		// Stopped while nobody was looking: start it again where it should be.
		var err error
		if kind == "tun" {
			if ls, err = s.ensureTunerLive(r, id); err != nil {
				tuneError(w, err)
				return
			}
		} else {
			var c *channels.Channel
			if c, err = s.Channels.Get(r.Context(), id); err == nil {
				ls, err = s.ensureLibraryLive(r, c)
			}
			if err != nil {
				liveError(w, err)
				return
			}
		}
	}
	ls.Viewer(viewerName(r))
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	if err := ls.WaitReady(ctx); err != nil {
		if ctx.Err() != nil && ls.Err() == nil {
			writeErr(w, 503, "the channel is still starting")
			return
		}
		tuneError(w, err)
		return
	}
	if err := ls.Err(); err != nil {
		tuneError(w, err)
		return
	}
	// A player that can't set headers (a TV's own player) keeps its token.
	suffix := ""
	for _, k := range []string{"st", "token"} {
		if v := r.URL.Query().Get(k); v != "" {
			suffix = "?" + k + "=" + url.QueryEscape(v)
			break
		}
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(ls.Playlist(func(f string) string { return "seg/" + f + suffix }))
}

func (s *Server) handleLiveSegment(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	kind, id, ok := parseKey(key)
	if !ok || !s.canWatchLive(r, kind, id) {
		writeErr(w, 404, "channel not found")
		return
	}
	ls := s.HLS.GetLive(key)
	if ls == nil {
		writeErr(w, 404, "no such stream")
		return
	}
	// SegmentPath knows only names it was told by FFmpeg, so a request cannot
	// reach anything else on disk.
	p, ok := ls.SegmentPath(r.PathValue("name"))
	if !ok {
		writeErr(w, 404, "that part of the stream has gone")
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, p)
}
