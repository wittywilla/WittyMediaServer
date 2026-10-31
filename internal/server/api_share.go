package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Links that share one item or album with someone who has no account.
//
// The rules: the link holds a long random secret and only its hash is stored;
// it opens exactly what it names and nothing else; it checks on every request
// that the person who made it may still see that; viewers get pictures re-encoded
// by the server, so EXIF and locations stay home, unless "allow download" was
// ticked; and it can be ended at any time.

// POST /api/shares {kind: item|album, id, days, allowDownload}
func (s *Server) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !canPost(u) {
		writeErr(w, 403, "this profile can't share links")
		return
	}
	if s.DB.GetSetting(r.Context(), "shareLinks", "1") != "1" {
		writeErr(w, 403, "links are turned off on this server")
		return
	}
	var b struct {
		Kind          string `json:"kind"`
		ID            int64  `json:"id"`
		Days          int    `json:"days"` // 0 = no end date
		AllowDownload bool   `json:"allowDownload"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Days < 0 || b.Days > 3650 {
		writeErr(w, 400, "days must be between 0 and 3650")
		return
	}
	l := library.ShareLink{UserID: u.ID, Kind: b.Kind, TargetID: b.ID, AllowDownload: b.AllowDownload}
	switch b.Kind {
	case "item":
		it, err := s.visibleItem(r, b.ID)
		if err != nil || (it.Kind != "photo" && it.Kind != "video") {
			writeErr(w, 404, "only pictures and videos can be shared by link")
			return
		}
		l.Title = it.Title
	case "album":
		a, err := s.Store.PhotoAlbumByID(r.Context(), b.ID, u.ID, u.IsAdmin)
		if err != nil {
			writeErr(w, 404, "not found")
			return
		}
		l.Title = a.Name
	default:
		writeErr(w, 400, "kind must be item or album")
		return
	}
	if b.Days > 0 {
		exp := db.Now() + int64(b.Days)*86400
		l.ExpiresAt = &exp
	}
	out, err := s.Store.CreateShare(r.Context(), l)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 201, out)
}

// GET /api/shares?all=1
func (s *Server) handleShares(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	s.Store.PurgeExpiredShares(r.Context())
	l, err := s.Store.Shares(r.Context(), u.ID, u.IsAdmin && qBool(r, "all"))
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, l)
}

// DELETE /api/shares/{id}
func (s *Server) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	u := userFrom(r)
	owner, err := s.Store.ShareOwner(r.Context(), id)
	if err != nil || (owner != u.ID && !u.IsAdmin) {
		writeErr(w, 404, "not found")
		return
	}
	if err := s.Store.DeleteShare(r.Context(), id); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// The public side: no account, only the link's secret
// ---------------------------------------------------------------------------

// shareContext is an open link and the account it speaks for.
type shareContext struct {
	link  *library.ShareLink
	owner *auth.User
	acc   *library.Access
}

func (s *Server) openShare(r *http.Request) (*shareContext, error) {
	if s.DB.GetSetting(r.Context(), "shareLinks", "1") != "1" {
		return nil, library.ErrNotFound
	}
	l, err := s.Store.ShareByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		return nil, err
	}
	u, err := s.Auth.User(r.Context(), l.UserID)
	if err != nil {
		return nil, library.ErrNotFound
	}
	return &shareContext{link: l, owner: u, acc: accessFor(u)}, nil
}

// sharedItem loads an item the link names and its owner may still see.
func (s *Server) sharedItem(ctx context.Context, c *shareContext, id int64) (*library.Item, error) {
	it, err := s.Store.Item(ctx, id)
	if err != nil || it.Missing || (it.Kind != "photo" && it.Kind != "video") {
		return nil, library.ErrNotFound
	}
	switch c.link.Kind {
	case "item":
		if id != c.link.TargetID {
			return nil, library.ErrNotFound
		}
	case "album":
		var one int
		if err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM photo_album_items WHERE album_id = ? AND item_id = ?`, c.link.TargetID, id).Scan(&one); err != nil {
			return nil, library.ErrNotFound
		}
	default:
		return nil, library.ErrNotFound
	}
	if !c.acc.AllowsItem(it, nil) {
		return nil, library.ErrNotFound
	}
	return it, nil
}

type sharedPhoto struct {
	ID          int64   `json:"id"`
	Kind        string  `json:"kind"`
	Title       string  `json:"title"`
	TakenAt     int64   `json:"takenAt,omitempty"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	DurationSec float64 `json:"durationSec,omitempty"`
}

// GET /api/share/{token}
func (s *Server) handleShareOpen(w http.ResponseWriter, r *http.Request) {
	c, err := s.openShare(r)
	if err != nil {
		writeErr(w, 404, "this link has ended or never existed")
		return
	}
	photos := []sharedPhoto{}
	addItem := func(it *library.Item, taken int64) {
		photos = append(photos, sharedPhoto{ID: it.ID, Kind: it.Kind, Title: it.Title, TakenAt: taken, Width: it.Width, Height: it.Height, DurationSec: it.RuntimeSec})
	}
	switch c.link.Kind {
	case "item":
		it, err := s.sharedItem(r.Context(), c, c.link.TargetID)
		if err != nil {
			writeErr(w, 404, "this link has ended or never existed")
			return
		}
		addItem(it, 0)
	case "album":
		a, err := s.Store.PhotoAlbumByID(r.Context(), c.link.TargetID, c.owner.ID, c.owner.IsAdmin)
		if err != nil {
			writeErr(w, 404, "this link has ended or never existed")
			return
		}
		list, _, err := s.Store.Photos(r.Context(), library.PhotoQuery{LibraryID: a.LibraryID, UserID: 0, Access: c.acc, Album: a.ID, Limit: 500})
		if err != nil {
			writeLibErr(w, err)
			return
		}
		for _, p := range list {
			photos = append(photos, sharedPhoto{ID: p.ID, Kind: p.Kind, Title: p.Title, TakenAt: p.TakenAt, Width: p.Width, Height: p.Height, DurationSec: p.DurationSec})
		}
	}
	s.Store.CountShareView(r.Context(), c.link.ID)
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	writeJSON(w, 200, map[string]any{"kind": c.link.Kind, "title": c.link.Title, "owner": c.owner.Name, "allowDownload": c.link.AllowDownload, "expiresAt": c.link.ExpiresAt, "items": photos})
}

// GET /api/share/{token}/image/{id}?w=: always re-encoded.
func (s *Server) handleShareImage(w http.ResponseWriter, r *http.Request) {
	c, err := s.openShare(r)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.sharedItem(r.Context(), c, id)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	width := qInt(r, "w", 1920)
	if width <= 0 || width > 1920 {
		width = 1920
	}
	s.serveThumb(w, r, it, width)
}

// GET /api/share/{token}/original/{id}: only when the link allows it.
func (s *Server) handleShareOriginal(w http.ResponseWriter, r *http.Request) {
	c, err := s.openShare(r)
	if err != nil || !c.link.AllowDownload {
		writeErr(w, 404, "not found")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.sharedItem(r.Context(), c, id)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	s.serveOriginal(w, r, it, true)
}

// GET /api/share/{token}/play/{id}: how to play a shared clip.
func (s *Server) handleSharePlay(w http.ResponseWriter, r *http.Request) {
	c, err := s.openShare(r)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.sharedItem(r.Context(), c, id)
	if err != nil || it.Kind != "video" {
		writeErr(w, 404, "not found")
		return
	}
	token := r.PathValue("token")
	if s.browserSafeVideo(r.Context(), it) {
		writeJSON(w, 200, map[string]any{"type": "direct", "url": fmt.Sprintf("/api/share/%s/file/%d", token, id), "durationSec": it.RuntimeSec})
		return
	}
	if err := s.ensureShareStream(r, it); err != nil {
		s.Log.WarnContext(r.Context(), "could not start a shared video", "cat", "playback", "item", id, "err", err)
		writeErr(w, 500, "this video can't be played")
		return
	}
	writeJSON(w, 200, map[string]any{"type": "hls", "url": fmt.Sprintf("/api/share/%s/hls/%d/playlist.m3u8", token, id), "durationSec": it.RuntimeSec})
}

// browserSafeVideo: the scan says the file plays as it is. A clip whose file says
// where it was shot goes through the re-encoded stream instead, which carries
// nothing of the sort.
func (s *Server) browserSafeVideo(ctx context.Context, it *library.Item) bool {
	ext := strings.ToLower(filepath.Ext(it.Path))
	return (ext == ".mp4" || ext == ".m4v" || ext == ".webm") && (it.VideoCodec == "h264" || it.VideoCodec == "vp9" || it.VideoCodec == "av1") && !s.Store.PhotoLocated(ctx, it.ID)
}

func shareStreamID(it *library.Item) string { return "share-" + hls.Key(it.Path) }

func (s *Server) ensureShareStream(r *http.Request, it *library.Item) error {
	id := shareStreamID(it)
	pctx, cancel := context.WithTimeout(logging.WithSession(r.Context(), id), 30*time.Second)
	defer cancel()
	pr, err := s.Tools.ProbeFile(pctx, it.Path)
	if err != nil {
		return err
	}
	audio := 0
	if len(pr.Audio()) == 0 {
		audio = -1
	}
	q, maxHeight := s.quality(r)
	s.HLS.Ensure(hls.Spec{ID: shareStreamID(it), InputArgs: ffmpeg.FileInput(it.Path), Duration: pr.Duration(), SeekMargin: pr.SeekMargin(), AudioIndex: audio, AudioChannels: 2, SubtitleIndex: -1, Quality: q, MaxHeight: maxHeight, AudioCodec: "aac"})
	s.Log.InfoContext(r.Context(), "playback started", "cat", "playback", "session", shareStreamID(it), "item", it.ID, "title", it.Title, "mode", "shared")
	return nil
}

// GET /api/share/{token}/file/{id}: a clip that plays as it is.
func (s *Server) handleShareFile(w http.ResponseWriter, r *http.Request) {
	c, err := s.openShare(r)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.sharedItem(r.Context(), c, id)
	if err != nil || it.Kind != "video" || !s.browserSafeVideo(r.Context(), it) {
		writeErr(w, 404, "not found")
		return
	}
	s.serveOriginal(w, r, it, false)
}

// GET /api/share/{token}/hls/{id}/playlist.m3u8
func (s *Server) handleShareHLSPlaylist(w http.ResponseWriter, r *http.Request) {
	it, ok := s.sharedStream(w, r)
	if !ok {
		return
	}
	sess := s.HLS.Get(shareStreamID(it))
	if sess == nil {
		if err := s.ensureShareStream(r, it); err != nil {
			writeErr(w, 404, "not found")
			return
		}
		if sess = s.HLS.Get(shareStreamID(it)); sess == nil {
			writeErr(w, 404, "not found")
			return
		}
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(sess.Playlist(func(n int) string { return fmt.Sprintf("seg/%d.ts", n) }))
}

// GET /api/share/{token}/hls/{id}/seg/{n}
func (s *Server) handleShareHLSSegment(w http.ResponseWriter, r *http.Request) {
	it, ok := s.sharedStream(w, r)
	if !ok {
		return
	}
	sess := s.HLS.Get(shareStreamID(it))
	if sess == nil {
		writeErr(w, 404, "no such stream")
		return
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimSuffix(r.PathValue("n"), ".ts"), "%d", &n); err != nil || n < 0 {
		writeErr(w, 400, "bad segment")
		return
	}
	p, err := sess.Segment(r.Context(), n)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			// The visitor is told nothing about the server; the owner should be.
			s.Log.WarnContext(r.Context(), "could not serve a shared segment", "cat", "transcode", "session", sess.LogSession(), "segment", n, "err", err)
		}
		writeErr(w, 500, "segment failed")
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, p)
}

func (s *Server) sharedStream(w http.ResponseWriter, r *http.Request) (*library.Item, bool) {
	c, err := s.openShare(r)
	if err != nil {
		writeErr(w, 404, "not found")
		return nil, false
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil, false
	}
	it, err := s.sharedItem(r.Context(), c, id)
	if err != nil || it.Kind != "video" {
		writeErr(w, 404, "not found")
		return nil, false
	}
	return it, true
}

var shareTokenRe = regexp.MustCompile(`(/api/share/|/s/)[A-Za-z0-9_-]+`)

// redactPath hides the secret of a shared link from request logs, which people
// attach to bug reports.
func redactPath(p string) string {
	if !strings.Contains(p, "/s/") && !strings.Contains(p, "/share/") {
		return p
	}
	return shareTokenRe.ReplaceAllString(p, "${1}REDACTED")
}

// logWatched adds the time since a player's last report to the account's daily
// record. Jumps don't count as watching: the gap is capped at a generous report
// interval.
func (s *Server) logWatched(r *http.Request, id int64, p library.Progress) {
	var last float64
	u := userFrom(r)
	if err := s.DB.QueryRowContext(r.Context(), `SELECT position_sec FROM progress WHERE user_id = ? AND item_id = ?`, u.ID, id).Scan(&last); err != nil {
		last = 0
	}
	if d := p.PositionSec - last; d > 0 && d <= 90 {
		if err := s.Store.LogPlay(r.Context(), u.ID, id, d); err != nil && r.Context().Err() == nil {
			s.Log.WarnContext(r.Context(), "could not record watching time", "cat", "playback", "item", id, "err", err)
		}
	}
}
