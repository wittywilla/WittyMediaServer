package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// What surrounds the guide: searching the schedule, an account's favourite
// channels and reminders, the bumper library, and the discs scenes come from.

func (s *Server) tvExtraRoutes(m *http.ServeMux) {
	a := s.requireUser
	ad := s.requireAdmin
	m.Handle("GET /api/guide/search", a(s.handleGuideSearch))
	m.Handle("GET /api/tv/favourites", a(s.handleTVFavourites))
	m.Handle("PUT /api/tv/favourites/{key}", a(s.handleSetTVFavourite))
	m.Handle("DELETE /api/tv/favourites/{key}", a(s.handleSetTVFavourite))
	m.Handle("GET /api/tv/reminders", a(s.handleTVReminders))
	m.Handle("POST /api/tv/reminders", a(s.handleAddTVReminder))
	m.Handle("DELETE /api/tv/reminders/{id}", a(s.handleDeleteTVReminder))
	m.Handle("GET /api/channels/bumpers", ad(s.handleBumpers))
	m.Handle("POST /api/channels/bumpers", ad(s.handleAddBumper))
	m.Handle("POST /api/channels/bumpers/upload", ad(s.handleUploadBumper))
	m.Handle("DELETE /api/channels/bumpers/item/{id}", ad(s.handleDeleteBumper))
	m.Handle("GET /api/channels/discs", ad(s.handleChannelDiscs))
	m.Handle("POST /api/channels/clip-preview", ad(s.handleClipPreview))
}

// ---------------------------------------------------------------------------
// Searching the schedule
// ---------------------------------------------------------------------------

type guideHit struct {
	Channel string `json:"channel"` // its key
	Number  string `json:"number"`
	Name    string `json:"name"`
	guideProgramme
}

const (
	maxSearchDays = 14
	maxSearchHits = 100
)

// GET /api/guide/search?q=word&days=7: what is coming up that matches, soonest
// first, on the channels this account can see.
// Each search works out every channel's schedule for up to two weeks, so only a few run at once.
var guideSearchSlots = make(chan struct{}, 4)

func (s *Server) handleGuideSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 || len(q) > 100 {
		writeErr(w, 400, "search for 2 to 100 letters")
		return
	}
	days := qInt(r, "days", 7)
	if days < 1 || days > maxSearchDays {
		writeErr(w, 400, "search up to two weeks ahead")
		return
	}
	limit := qInt(r, "limit", 50)
	if limit < 1 || limit > maxSearchHits {
		limit = 50
	}
	select {
	case guideSearchSlots <- struct{}{}:
		defer func() { <-guideSearchSlots }()
	default:
		w.Header().Set("Retry-After", "2")
		writeErr(w, 429, "the guide is busy: try again in a moment")
		return
	}
	ctx := r.Context()
	now := time.Now()
	to := now.AddDate(0, 0, days)
	hits := []guideHit{}
	for _, row := range s.libraryRows(ctx, r) {
		c, err := s.Channels.Get(ctx, row.ID)
		if err != nil {
			continue
		}
		found, err := s.Channels.Find(ctx, c, now, to, q, limit)
		if err != nil {
			s.Log.WarnContext(ctx, "could not search a channel's schedule", "cat", "tv", "channel", row.Name, "err", err)
			continue
		}
		for _, p := range found {
			hits = append(hits, guideHit{row.Key, row.Number, row.Name, fromSchedule(p)})
		}
	}
	if tuners := s.tunerRows(ctx, r); len(tuners) > 0 {
		hits = append(hits, s.findBroadcasts(ctx, tuners, now, to, q, limit)...)
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Start < hits[j].Start })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	writeJSON(w, 200, hits)
}

// findBroadcasts looks through guide data. A playlist can have thousands of
// channels, so the search goes through the programmes, not channel by channel.
func (s *Server) findBroadcasts(ctx context.Context, rows []guideChannel, from, to time.Time, q string, limit int) []guideHit {
	byGuide := map[string]guideChannel{}
	for _, row := range rows {
		c, err := s.TV.Channel(ctx, row.ID)
		if err != nil || c.Encrypted != "" {
			continue
		}
		if k := c.GuideKey(); k != "" {
			byGuide[k] = row
		}
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	res, err := s.DB.QueryContext(ctx, `SELECT channel, start, stop, title, sub, descr, category, rating FROM epg_programmes
		WHERE stop > ? AND start < ? AND (title LIKE ? ESCAPE '\' OR sub LIKE ? ESCAPE '\') ORDER BY start LIMIT ?`, from.Unix(), to.Unix(), like, like, maxSearchHits*4)
	if err != nil {
		s.Log.WarnContext(ctx, "could not search the guide data", "cat", "tv", "err", err)
		return nil
	}
	defer res.Close()
	out := []guideHit{}
	for res.Next() && len(out) < limit {
		var key string
		p := guideProgramme{Kind: "broadcast"}
		if err := res.Scan(&key, &p.Start, &p.End, &p.Title, &p.Sub, &p.Desc, &p.Genre, &p.Rating); err != nil {
			break
		}
		if row, ok := byGuide[key]; ok {
			out = append(out, guideHit{row.Key, row.Number, row.Name, p})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Favourites and reminders: each account's own
// ---------------------------------------------------------------------------

func (s *Server) handleTVFavourites(w http.ResponseWriter, r *http.Request) {
	fav, err := s.Channels.Favourites(r.Context(), userFrom(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]string, 0, len(fav))
	for k := range fav {
		out = append(out, k)
	}
	sort.Strings(out)
	writeJSON(w, 200, out)
}

// PUT and DELETE /api/tv/favourites/{key}: the method says which.
func (s *Server) handleSetTVFavourite(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !s.canSeeChannelKey(r, key) {
		writeErr(w, 404, "channel not found")
		return
	}
	if err := s.Channels.SetFavourite(r.Context(), userFrom(r).ID, key, r.Method == http.MethodPut); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) canSeeChannelKey(r *http.Request, key string) bool {
	kind, id, ok := parseKey(key)
	return ok && s.canWatchLive(r, kind, id)
}

func (s *Server) handleTVReminders(w http.ResponseWriter, r *http.Request) {
	list, err := s.Channels.Reminders(r.Context(), userFrom(r).ID, time.Now().Unix())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// A channel this account can't see any more says nothing about what is on it.
	seen := map[string]bool{}
	out := make([]channels.Reminder, 0, len(list))
	for _, e := range list {
		ok, known := seen[e.Channel]
		if !known {
			ok = s.canSeeChannelKey(r, e.Channel)
			seen[e.Channel] = ok
		}
		if ok {
			out = append(out, e)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleAddTVReminder(w http.ResponseWriter, r *http.Request) {
	var b channels.Reminder
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "that reminder could not be read")
		return
	}
	now := time.Now().Unix()
	b.Title, b.Sub = strings.TrimSpace(b.Title), strings.TrimSpace(b.Sub)
	if b.Title == "" || len(b.Title) > 300 || len(b.Sub) > 300 || b.Start < now-60 || b.End <= b.Start || b.Start > now+maxSearchDays*2*86400 {
		writeErr(w, 400, "that reminder isn't for a programme still to come")
		return
	}
	if !s.canSeeChannelKey(r, b.Channel) {
		writeErr(w, 404, "channel not found")
		return
	}
	if err := s.Channels.AddReminder(r.Context(), userFrom(r).ID, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, b)
}

func (s *Server) handleDeleteTVReminder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.Channels.RemoveReminder(r.Context(), userFrom(r).ID, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// The bumper library
// ---------------------------------------------------------------------------

const (
	maxBumperBytes  = 2 << 30
	maxBumperLength = 30 * 60
	bumperUploadFor = 30 * time.Minute
	bumperLibrary   = "tvBumperLibrary" // the setting that remembers its library
)

func (s *Server) bumperDir() string { return filepath.Join(s.Cfg.DataDir, "bumpers") }

// followBumperFolder keeps uploads playing when the data folder has moved, as after
// restoring a backup somewhere else: the library and its files still point at the
// old place.
func (s *Server) followBumperFolder(ctx context.Context) {
	id, err := strconv.ParseInt(s.DB.GetSetting(ctx, bumperLibrary, ""), 10, 64)
	if err != nil {
		return
	}
	moved, err := s.Channels.MoveBumperFolder(ctx, id, s.bumperDir())
	if err != nil {
		s.Log.WarnContext(ctx, "could not follow the bumpers to a moved data folder", "cat", "tv", "err", err)
	} else if moved > 0 {
		s.Log.InfoContext(ctx, "the data folder has moved: uploaded bumpers now point at the new place", "cat", "tv", "videos", moved)
	}
}

func (s *Server) handleBumpers(w http.ResponseWriter, r *http.Request) {
	list, err := s.Channels.Bumpers(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

// POST /api/channels/bumpers {"itemId": 12}: any video in any library can be one.
func (s *Server) handleAddBumper(w http.ResponseWriter, r *http.Request) {
	var b struct {
		ItemID int64 `json:"itemId"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	it, err := s.Store.Item(r.Context(), b.ItemID)
	if err != nil || it.Missing || it.Kind != "video" {
		writeErr(w, 404, "that video could not be found")
		return
	}
	if err := s.Channels.AddBumper(r.Context(), it.ID, false); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// bumperLib is the commercials library uploads go into, made the first time one
// is needed. It is an ordinary library in the data folder, shown in Settings.
func (s *Server) bumperLib(ctx context.Context) (*library.Library, error) {
	if id, err := strconv.ParseInt(s.DB.GetSetting(ctx, bumperLibrary, ""), 10, 64); err == nil {
		if lib, err := s.Store.Library(ctx, id); err == nil && lib.Type == library.CommercialsType {
			return lib, nil
		}
	}
	if err := os.MkdirAll(s.bumperDir(), 0o755); err != nil {
		return nil, err
	}
	lib, err := s.Store.CreateLibrary(ctx, "WittyTV bumpers", library.CommercialsType, []string{s.bumperDir()})
	if err != nil {
		return nil, err
	}
	if err := s.DB.SetSetting(ctx, bumperLibrary, strconv.FormatInt(lib.ID, 10)); err != nil {
		return nil, err
	}
	return lib, nil
}

// POST /api/channels/bumpers/upload?name=sting.mp4: the body is the file. It is
// named safely, checked by its contents as well as its name, and kept only if
// FFmpeg finds a short video in it.
func (s *Server) handleUploadBumper(w http.ResponseWriter, r *http.Request) {
	name := library.SafeFileName(r.URL.Query().Get("name"))
	if name == "" || !library.IsVideoFile(name) {
		writeErr(w, 400, "choose a video file")
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(bumperUploadFor))
	body := http.MaxBytesReader(w, r.Body, maxBumperBytes)
	head := make([]byte, 512)
	n, err := io.ReadFull(body, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		writeErr(w, 400, "the upload did not arrive")
		return
	}
	if !library.SniffMedia(head[:n]) {
		writeErr(w, 400, "the file's contents are not a video")
		return
	}
	ctx := r.Context()
	lib, err := s.bumperLib(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if free := sysinfo.FreeBytes(s.bumperDir()); free >= 0 && free-uploadFreeFloor <= max(r.ContentLength, 0) {
		writeErr(w, 507, "The disk this server keeps its data on is nearly full.")
		return
	}
	f, path, err := library.CreateInLibrary(s.bumperDir(), s.bumperDir(), name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, err = f.Write(head[:n])
	if err == nil {
		_, err = io.Copy(f, body)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeErr(w, 413, "a bumper can be at most 2 GB")
			return
		}
		writeErr(w, 500, "the upload could not be saved")
		return
	}
	it, err := s.Scanner.IndexUploaded(ctx, lib, path)
	if err != nil || it == nil || it.RuntimeSec <= 0 {
		os.Remove(path)
		s.removeItem(ctx, it)
		writeErr(w, 400, "the file could not be read as a video")
		return
	}
	if it.RuntimeSec > maxBumperLength {
		os.Remove(path)
		s.removeItem(ctx, it)
		writeErr(w, 400, fmt.Sprintf("a bumper can run for at most %d minutes: add longer videos to a library instead", maxBumperLength/60))
		return
	}
	if err := s.Channels.AddBumper(ctx, it.ID, true); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"itemId": it.ID, "title": it.Title, "durationSec": it.RuntimeSec})
}

func (s *Server) removeItem(ctx context.Context, it *library.Item) {
	if it == nil {
		return
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, it.ID); err != nil {
		s.Log.WarnContext(ctx, "could not remove an uploaded bumper from the library", "cat", "tv", "item", it.ID, "err", err)
	}
}

// DELETE /api/channels/bumpers/item/{id}: an upload goes altogether, its file too. A
// video chosen from a library only comes off this list.
func (s *Server) handleDeleteBumper(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	ctx := r.Context()
	it, err := s.Store.Item(ctx, id)
	if err != nil {
		writeErr(w, 404, "that bumper could not be found")
		return
	}
	uploaded, err := s.Channels.RemoveBumper(ctx, id)
	if err != nil {
		if errors.Is(err, channels.ErrBundledBumper) {
			writeErr(w, 400, err.Error())
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	if uploaded {
		// Only ever a file in the bumpers folder, whatever the item's path says.
		if rel, err := filepath.Rel(s.bumperDir(), it.Path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			if err := os.Remove(it.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				s.Log.WarnContext(ctx, "could not delete an uploaded bumper's file", "cat", "tv", "item", id, "err", err)
			}
		}
		s.removeItem(ctx, it)
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// Discs for the scene picker
// ---------------------------------------------------------------------------

// GET /api/channels/discs: every disc, and the words for those that are refused.
func (s *Server) handleChannelDiscs(w http.ResponseWriter, r *http.Request) {
	list, err := s.Channels.Discs(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"discs": list, "refusal": dvdsession.EncryptedRefusal})
}

// POST /api/channels/clip-preview {"item":3,"title":1}: a stream of a disc's
// title, to try a scene in the picker. It stops by itself when nobody watches.
func (s *Server) handleClipPreview(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Item  int64 `json:"item"`
		Title int   `json:"title"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	it, err := s.Store.Item(ctx, b.Item)
	if err != nil || it.Missing || !library.IsDiscKind(it.Kind) {
		writeErr(w, 404, "disc not found")
		return
	}
	q, h, _ := s.liveQuality(r)
	in, err := s.discLiveInput(ctx, it, b.Title, 0, 0, q, h)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id := fmt.Sprintf("clip-%d-%d", it.ID, b.Title)
	s.HLS.Ensure(hls.Spec{ID: id, ItemID: it.ID, InputArgs: in.InputArgs, Duration: in.Total, VideoFilter: in.VideoFilter, AudioIndex: in.AudioIndex, SubtitleIndex: -1,
		SeekMargin: 1.5, Quality: q, MaxHeight: h, AudioCodec: "aac"})
	writeJSON(w, 200, map[string]any{"url": "/api/hls/" + id + "/playlist.m3u8", "durationSec": in.Total})
}
