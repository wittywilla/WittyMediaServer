// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
	m.Handle("POST /api/channels/{id}/reset", ad(s.handleResetChannel))
	m.Handle("GET /api/channels/{id}/history", ad(s.handleChannelHistory))
	m.Handle("POST /api/channels/{id}/restore", ad(s.handleRestoreChannel))
	m.Handle("GET /api/channels/{id}/logo", a(s.handleChannelLogo))
	m.Handle("PUT /api/channels/{id}/logo", ad(s.handleSetChannelLogo))
	m.Handle("DELETE /api/channels/{id}/logo", ad(s.handleDeleteChannelLogo))
	s.tunerRoutes(m)
	s.tvExtraRoutes(m)
	if s.DB != nil && s.Cfg != nil {
		s.followBumperFolder(context.Background())
	}
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
	CanRecord   bool   `json:"canRecord"` // always false: see recordable.go
}

// libraryChannel is a library channel as an administrator edits it, with the
// same canRecord as everywhere else: always false.
type libraryChannel struct {
	*channels.Channel
	CanRecord bool `json:"canRecord"`
}

func (s *Server) libraryChannelOut(r *http.Request, c *channels.Channel) libraryChannel {
	return libraryChannel{c, s.canRecord(r, nil)}
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
		all := make([]libraryChannel, 0, len(list))
		for _, c := range list {
			all = append(all, s.libraryChannelOut(r, c))
		}
		writeJSON(w, 200, all)
		return
	}
	a := s.access(r)
	out := []channelView{}
	for _, c := range list {
		if s.Channels.Visible(r.Context(), c, u, a) {
			out = append(out, channelView{c.ID, c.Number, c.Name, c.Description, c.Logo, s.canRecord(r, nil)})
		}
	}
	writeJSON(w, 200, out)
}

func channelError(w http.ResponseWriter, err error) {
	if errors.Is(err, channels.ErrNotFound) {
		writeErr(w, 404, "channel not found")
		return
	}
	if errors.Is(err, channels.ErrEncryptedClip) {
		writeErr(w, 400, dvdsession.EncryptedRefusal)
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
	s.noteChannelLogo(r.Context(), &c)
	writeJSON(w, 200, s.libraryChannelOut(r, &c))
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
	for i := range c.Definition.Bumpers.Rules {
		fix(&c.Definition.Bumpers.Rules[i].Source)
		fix(c.Definition.Bumpers.Rules[i].Only)
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
	writeJSON(w, 200, s.libraryChannelOut(r, c))
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
	s.noteChannelLogo(r.Context(), c)
	c, err = s.Channels.Get(r.Context(), id)
	if err != nil {
		channelError(w, err)
		return
	}
	writeJSON(w, 200, s.libraryChannelOut(r, c))
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

// POST /api/channels/{id}/reset: the schedule made again from the channel's
// settings and the titles that match them now, what is on now included.
// Anyone watching moves to the new schedule; reminders follow their programme
// or go, and the answer says how many.
func (s *Server) handleResetChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b struct {
		StartOver bool `json:"startOver"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	res, err := s.Channels.Reset(r.Context(), id, b.StartOver)
	if err != nil {
		channelError(w, err)
		return
	}
	watching := s.HLS != nil && s.HLS.RestartLive("lib-"+strconv.FormatInt(id, 10))
	s.Log.InfoContext(r.Context(), "channel reset", "cat", "tv", "channel", id, "startOver", b.StartOver, "watching", watching,
		"remindersKept", res.Kept, "remindersMoved", res.Moved, "remindersDropped", res.Dropped)
	writeJSON(w, 200, map[string]any{"ok": true, "watching": watching, "reminders": res})
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
	writeJSON(w, 200, s.libraryChannelOut(r, c))
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
	if len(progs) > 3000 {
		progs = progs[:3000]
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
	if _, err := s.visibleChannel(r, id); err != nil {
		writeErr(w, 404, "not found")
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
	// tvTheme is the owner's WittyTV look for anyone whose own choice and UI suggest none: a theme
	// name, or "" for the guide's own default.
	"tvTheme": "",
	// tvBackgroundPlay: whether a device that hasn't chosen keeps playing a channel while its tab
	// or app is in the background. Each device can choose for itself: see backgroundPlay.
	"tvBackgroundPlay": "1",
	"dvrFolder":        "", // "" = recordings in the data folder
	"dvrLibrary":       "", // a library to rescan after recording
}

func (s *Server) tvSetting(ctx context.Context, k string) string {
	return s.DB.GetSetting(ctx, k, tvSettings[k])
}

// backgroundPlay is what a device starts with for "Keep playing when this tab is in the
// background": on, unless the owner set new devices to stop. A device's own choice is kept on
// the device and wins. Either way the server never ends a channel because a player is in the
// background: a player that stays keeps asking for the playlist (docs/CLIENT-API.md).
func (s *Server) backgroundPlay(ctx context.Context) bool {
	return s.tvSetting(ctx, "tvBackgroundPlay") != "0"
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

// tvThemes are the WittyTV looks an owner can start new viewers on. Custom is each person's own.
var tvThemes = map[string]bool{"modern": true, "classic": true, "satellite": true, "teletext": true, "listings": true,
	"bigpicture": true, "outline": true, "compact": true}

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
		case "tvTheme":
			if v != "" && !tvThemes[v] {
				writeErr(w, 400, "unknown WittyTV theme")
				return
			}
		case "tvCopyVideo", "tvBackgroundPlay":
			if v != "0" && v != "1" {
				writeErr(w, 400, "bad value")
				return
			}
		case "dvrFolder":
			if err := s.checkAdminPaths(r, []string{v}, []string{s.tvSetting(r.Context(), "dvrFolder")}); err != nil {
				writeErr(w, 400, err.Error())
				return
			}
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
	// Role and Trimmed say what a stretch of bumpers is doing, for what is
	// playing now; the guide itself joins them to their programme.
	Role    string `json:"role,omitempty"`
	Trimmed int64  `json:"trimmed,omitempty"`
	// Clip: a scene from the disc ItemID, not the whole of it.
	Clip *channels.ClipRef `json:"clip,omitempty"`
}

// guideChannel is one row of the guide.
type guideChannel struct {
	Key    string `json:"key"` // lib-5, tun-12
	Kind   string `json:"kind"`
	ID     int64  `json:"id"`
	Number string `json:"number"`
	Name   string `json:"name"`
	Group  string `json:"group,omitempty"`
	Logo   string `json:"logo,omitempty"`
	// Description, Categories and Accent are a library channel's own; a tuner
	// channel has its playlist's Group.
	Description string   `json:"description,omitempty"`
	Categories  []string `json:"categories,omitempty"`
	Accent      string   `json:"accent,omitempty"`
	Favourite   bool     `json:"favourite,omitempty"`
	// Brand names its logo in the logo service, for a channel with none of its own.
	Brand     *channelBrand `json:"brand,omitempty"`
	HD        bool          `json:"hd,omitempty"`
	Radio     bool          `json:"radio,omitempty"`
	Encrypted string        `json:"encrypted,omitempty"` // why it cannot be tuned
	// CanRecord is the only thing that says whether to offer Record: see recordable.go.
	// Always sent; false for every library channel.
	CanRecord bool   `json:"canRecord"`
	NoRecord  string `json:"noRecord,omitempty"` // why not, when the device says
	ATSC3     bool   `json:"atsc3,omitempty"`
	// Security is the same wording for every client. Tuner channels only: a
	// library channel plays the owner's own files.
	Security    *securityInfo       `json:"security,omitempty"`
	Alternative *livetv.Alternative `json:"alternative,omitempty"` // the same station, clear
	Notices     []string            `json:"notices,omitempty"`
	Programmes  []guideProgramme    `json:"programmes"`
	// CatchupDays: how far back the source keeps what it showed, to watch from the start.
	CatchupDays int `json:"catchupDays,omitempty"`
	// Health is what the last check or tune of a tuner channel found: ok, failing, or "".
	Health string `json:"health,omitempty"`

	tc *livetv.Channel // a tuner row's channel, for decorateTunerRows
}

// channelBrand is a name to look up in the logo service.
type channelBrand struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func brandOf(c *channels.Channel) *channelBrand {
	if kind, name := c.Brand(); name != "" {
		return &channelBrand{kind, name}
	}
	return nil
}

// noteChannelLogo tells the logo service about a channel's name, so that it's
// looked for once the owner has agreed to downloads. Nothing is fetched until then.
func (s *Server) noteChannelLogo(ctx context.Context, c *channels.Channel) {
	kind, name := c.Brand()
	if name == "" {
		return
	}
	if _, err := s.Store.EnsureLogo(ctx, kind, name); err != nil {
		s.Log.WarnContext(ctx, "could not note a channel's logo", "cat", "tv", "channel", c.Name, "err", err)
		return
	}
	s.logos().Kick()
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
		ItemID: p.ItemID, ShowID: p.ShowID, Season: p.Season, Episode: p.Episode, Role: p.Role, Trimmed: p.Trimmed, Clip: p.Clip}
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
		row := guideChannel{Key: "lib-" + strconv.FormatInt(c.ID, 10), Kind: "library", ID: c.ID, Number: c.Number, Name: c.Name,
			Description: c.Description, Categories: c.GuideCategories(), Accent: c.Look.Accent, Brand: brandOf(c), CanRecord: s.canRecord(u, nil)}
		if c.Logo != "" {
			row.Logo = fmt.Sprintf("/api/channels/%d/logo?v=%s", c.ID, c.Logo)
		}
		out = append(out, row)
	}
	return out
}

// GET /api/guide?from=UNIX&hours=3&offset=0&limit=40&kind=library&group=News&favourites=1&channel=lib-5
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
	q := r.URL.Query()
	ix := s.guideIndex(ctx, r)
	pick, err := s.guidePick(ctx, r, ix, q.Get("set"))
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	// The older ways of narrowing the guide, which a set can stand for too.
	kind, group := q.Get("kind"), q.Get("group")
	one, wantFavs := q.Get("channel"), qBool(r, "favourites")
	find := strings.ToLower(strings.TrimSpace(q.Get("q")))
	rows := make([]guideChannel, 0, len(pick))
	for _, c := range pick {
		if (kind == "" || c.Kind == kind) && (group == "" || c.Group == group || slices.Contains(c.Categories, group)) &&
			(!wantFavs || c.Favourite) && (one == "" || c.Key == one) && (find == "" || channelMatches(&c, find)) {
			rows = append(rows, c)
		}
	}
	total := len(rows)
	located := -2
	if want := strings.TrimSpace(q.Get("locate")); want != "" {
		located = locateChannel(rows, want)
		if located >= 0 {
			offset = located - located%limit
		}
	}
	if offset > total {
		offset = total
	}
	rows = rows[offset:min(offset+limit, total)]
	out := map[string]any{"now": now.Unix(), "from": from.Unix(), "to": to.Unix(), "total": total, "offset": offset,
		"groups": ix.groupNames(), "set": cmp.Or(q.Get("set"), "all"), "theme": s.tvSetting(ctx, "tvTheme"), "backgroundPlay": s.backgroundPlay(ctx)}
	if located != -2 {
		out["located"] = located
	}
	if numbersOnly && qBool(r, "lite") {
		lite := make([]liteChannel, len(rows))
		for i, c := range rows {
			lite[i] = liteChannel{c.Key, c.Kind, c.ID, c.Number, c.Name, c.Group, c.Logo, c.Brand, c.Favourite, c.Encrypted != ""}
		}
		out["channels"] = lite
		writeJSON(w, 200, out)
		return
	}
	s.decorateTunerRows(r, rows, ix.tunerList)
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
	out["channels"] = rows
	writeJSON(w, 200, out)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
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
	// A little either side, so that what runs across the edge of the window
	// still joins up with its programme.
	list, err := s.Channels.Window(ctx, c, from.Add(-12*time.Hour), to.Add(time.Hour))
	if err != nil {
		s.Log.WarnContext(ctx, "could not read a channel's guide", "cat", "tv", "channel", row.Name, "err", err)
		return out
	}
	// A row's worth of bumpers tells a viewer nothing: they join their programme.
	for _, p := range channels.GuideCells(list) {
		if p.End > from.Unix() && p.Start < to.Unix() {
			out = append(out, fromSchedule(p))
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tuning in
// ---------------------------------------------------------------------------

type tuneInfo struct {
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	Number string `json:"number"`
	Name   string `json:"name"`
	Logo   string `json:"logo,omitempty"`
	URL    string `json:"url"`
	// For a player that can't set headers: asked for with ?streamToken=1.
	StreamURL   string          `json:"streamUrl,omitempty"`
	StreamToken string          `json:"streamToken,omitempty"`
	Now         *guideProgramme `json:"now"`
	Next        *guideProgramme `json:"next"`
	// Playing is what is on the screen when that isn't Now itself: a bumper
	// or a commercial inside it.
	Playing     *guideProgramme `json:"playing,omitempty"`
	Description string          `json:"description,omitempty"`
	Accent      string          `json:"accent,omitempty"`
	Brand       *channelBrand   `json:"brand,omitempty"`
	// PositionSec is how far into the title it is at ServerTime.
	PositionSec float64 `json:"positionSec"`
	// PartEnd, on a channel that counts watch progress, is when what is on the
	// screen now ends: this stretch of Now, or the bumper or commercial in it.
	// Progress counts only up to the end of a stretch, so a break doesn't count
	// as the programme; ask again then.
	PartEnd       int64 `json:"partEnd,omitempty"`
	ServerTime    int64 `json:"serverTime"`
	CountProgress bool  `json:"countProgress"`
	CanRecord     bool  `json:"canRecord"`
	// BackgroundPlay: whether a device that hasn't chosen keeps playing while it's in the
	// background (see backgroundPlay). The same as the guide's.
	BackgroundPlay bool          `json:"backgroundPlay"`
	BufferSec      float64       `json:"bufferSec"`
	ATSC3          bool          `json:"atsc3,omitempty"`
	Security       *securityInfo `json:"security,omitempty"`
	Notices        []string      `json:"notices,omitempty"`
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
		Description: c.Description, Accent: c.Look.Accent, Brand: brandOf(c),
		PositionSec: float64(now.Unix() - cur.Start + cur.Offset), ServerTime: now.Unix(), CountProgress: c.CountProgress && cur.Kind == channels.KindShow,
		CanRecord: s.canRecord(r, nil)}
	if c.CountProgress {
		info.PartEnd = cur.End
	}
	if c.Logo != "" {
		info.Logo = fmt.Sprintf("/api/channels/%d/logo?v=%s", c.ID, c.Logo)
	}
	// The banner speaks of programmes; the schedule knows each stretch of them.
	shown, after, err := s.Channels.GuideNow(r.Context(), c, now)
	if err != nil || shown == nil {
		shown, after = cur, next
	}
	g := fromSchedule(*shown)
	info.Now = &g
	if after != nil {
		n := fromSchedule(*after)
		info.Next = &n
	}
	if cur.Kind == channels.KindFiller {
		p := fromSchedule(*cur)
		info.Playing = &p
	}
	info.BufferSec = float64(s.tvSettingInt(r.Context(), "tvLibraryBufferMin") * 60)
	info.BackgroundPlay = s.backgroundPlay(r.Context())
	if err := s.addLiveTicket(r, &info); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
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
		// A catch-up stream is left by naming the programme's start as well.
		if start, err := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64); err == nil && start > 0 && strings.HasPrefix(key, "tun-") {
			id, _ := strconv.ParseInt(strings.TrimPrefix(key, "tun-"), 10, 64)
			s.HLS.LeaveLive(catchupID(id, start), v)
		} else {
			s.HLS.LeaveLive(key, v)
		}
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

// feedHold is how far past its head start a channel's stream may get before
// its feed waits for the clock, and the longest it waits at once: a player
// keeps about three segments in hand, and a longer pause would empty them.
const feedHold = 4 * time.Second

// feedPlan says when a library channel's next run starts and from what moment
// of the schedule. Runs are paced to real time from a short head start, so a
// run can finish a little before its programme's end: the next programme then
// starts on schedule rather than repeating the end of this one. Every file also
// ends a moment before its slot, which is whole seconds, and a long break of
// commercials would carry the stream ever further ahead of the guide; so once it
// is more than a little past its head start, the feed waits, a few seconds a
// run, until it is back to it. A file that ran out
// long before its time gets a card for the rest (card, in seconds).
func feedPlan(now time.Time, lastEnd int64) (at time.Time, wait time.Duration, card float64) {
	end := time.Unix(lastEnd, 0)
	ahead := end.Sub(now)
	switch {
	case ahead <= 0:
		return now, 0, 0
	case ahead > 30*time.Second:
		return now, 0, ahead.Seconds()
	case ahead > hls.LiveHeadStart+feedHold:
		wait = feedHold
	}
	return end, wait, 0
}

// libraryFeed says what a library channel plays next: see feedPlan.
func (s *Server) libraryFeed(id int64, q ffmpeg.Quality, maxHeight int, copyOK bool) hls.LiveFeed {
	var lastEnd int64
	var lastRev int
	return func(ctx context.Context, _ time.Time) (*hls.LiveInput, error) {
		c, err := s.Channels.Get(ctx, id)
		if err != nil {
			return nil, &hls.FatalError{Err: errors.New("this channel no longer exists")}
		}
		now := time.Now()
		if lastRev != 0 && c.Rev != lastRev && lastEnd > now.Unix() {
			// The schedule was made again. An edit keeps what is on now, so it
			// still ends when it did; after a reset it doesn't, and the channel
			// goes straight to what the new schedule has on now.
			if cur, _, err := s.Channels.Now(ctx, c, now); err == nil && cur.End != lastEnd {
				lastEnd = 0
			}
		}
		lastRev = c.Rev
		at, wait, card := feedPlan(now, lastEnd)
		if card > 0 {
			return &hls.LiveInput{InputArgs: hls.SlateInput(), Slate: true, Duration: card, AudioIndex: -1}, nil
		}
		if wait > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		cur, _, err := s.Channels.Now(ctx, c, at)
		if err != nil {
			return nil, err
		}
		lastEnd = cur.End
		off, remain := runSpan(cur, at)
		if cur.Kind != channels.KindSlate && cur.ItemID != 0 {
			in, err := s.itemLiveInput(ctx, cur, off, remain, q, maxHeight, copyOK)
			if err == nil {
				return in, nil
			}
			s.Log.Warn("channel programme unavailable", "cat", "playback", "channel", c.Name, "item", cur.ItemID, "err", err)
		}
		return &hls.LiveInput{InputArgs: hls.SlateInput(), Slate: true, Duration: remain, AudioIndex: -1}, nil
	}
}

// runSpan is how far into a programme a run starts at, and how long it plays: to
// the programme's end, to the fraction of a second, so that the next programme
// isn't joined late. Less than a second in is the start.
func runSpan(p *channels.Programme, at time.Time) (off, remain float64) {
	off = max(at.Sub(time.Unix(p.Start, 0)).Seconds(), 0)
	if off < 1 {
		off = 0
	}
	return off, time.Unix(p.End, 0).Sub(at).Seconds()
}

// itemLiveInput prepares what a programme plays, from off seconds into it.
func (s *Server) itemLiveInput(ctx context.Context, p *channels.Programme, off, remain float64, q ffmpeg.Quality, maxHeight int, copyOK bool) (*hls.LiveInput, error) {
	it, err := s.Store.Item(ctx, p.ItemID)
	if err != nil {
		return nil, err
	}
	if it.Missing {
		return nil, errors.New("the file is missing")
	}
	seek := off + float64(p.Offset)
	if library.IsDiscKind(it.Kind) {
		title := 0
		if p.Clip != nil {
			title = p.Clip.Title
			seek += p.Clip.Start
		}
		return s.discLiveInput(ctx, it, title, seek, remain, q, maxHeight)
	}
	if !s.Store.LibraryFile(ctx, it.Path) {
		return nil, errors.New("the file is missing")
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
		in.Height = min(v.Width, v.Height)
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

// discLiveInput plays a title of a disc (0 = its main feature) through the disc
// engines, without menus. Encrypted discs are refused by the engines and never
// reach a pool.
func (s *Server) discLiveInput(ctx context.Context, it *library.Item, ttn int, seek, remain float64, q ffmpeg.Quality, maxHeight int) (*hls.LiveInput, error) {
	if it.Disc != nil && it.Disc.Scrambled {
		return nil, errors.New(dvdsession.EncryptedRefusal)
	}
	if ttn == 0 && it.Disc != nil {
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
	if p != nil && p.Scrambled {
		return nil, errors.New(dvdsession.EncryptedRefusal)
	}
	if p == nil || p.Media == nil || p.Media.URL == "" {
		return nil, errors.New("this part of the disc cannot be played on a channel")
	}
	id := strings.TrimSuffix(strings.TrimPrefix(p.Media.URL, "/api/hls/"), "/playlist.m3u8")
	vod := s.HLS.Get(id)
	if vod == nil {
		return nil, errors.New("the disc's stream did not start")
	}
	return &hls.LiveInput{InputArgs: vod.Spec.InputArgs, Seek: seek, Duration: remain, VideoFilter: vod.Spec.VideoFilter, AudioIndex: vod.Spec.AudioIndex, Total: vod.Spec.Duration}, nil
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
				s.tuneError(w, r, err)
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
		s.tuneError(w, r, err)
		return
	}
	if err := ls.Err(); err != nil {
		s.tuneError(w, r, err)
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
