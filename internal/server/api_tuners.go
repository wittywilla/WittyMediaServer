package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittytuner"
)

// Tuners, their guide data and recordings. See docs/CHANNELS.md.

func (s *Server) tunerRoutes(m *http.ServeMux) {
	a := s.requireUser
	ad := s.requireAdmin
	if s.TV == nil && s.DB != nil {
		s.TV = livetv.New(s.DB, s.Tools, s.Log)
		s.TV.Folder = func() string {
			if f := s.tvSetting(context.Background(), "dvrFolder"); f != "" {
				return f
			}
			return filepath.Join(s.Cfg.DataDir, "recordings")
		}
		s.TV.AfterRecording = func(string) {
			if id, err := strconv.ParseInt(s.tvSetting(context.Background(), "dvrLibrary"), 10, 64); err == nil && id > 0 && s.Scanner != nil {
				s.Scanner.Start(id)
			}
		}
		s.TV.Start()
	}
	m.Handle("GET /api/tuners/drivers", ad(s.handleTunerDrivers))
	m.Handle("GET /api/tuners/discover", ad(s.handleTunerDiscover))
	m.Handle("GET /api/tuners", ad(s.handleTuners))
	m.Handle("POST /api/tuners", ad(s.handleCreateTuner))
	m.Handle("PUT /api/tuners/{id}", ad(s.handleUpdateTuner))
	m.Handle("DELETE /api/tuners/{id}", ad(s.handleDeleteTuner))
	m.Handle("POST /api/tuners/{id}/refresh", ad(s.handleRefreshTuner))
	m.Handle("POST /api/tuners/{id}/scan", ad(s.handleScanTuner))
	m.Handle("GET /api/tuners/{id}/scan", ad(s.handleScanProgress2))
	m.Handle("GET /api/tuners/{id}/channels", ad(s.handleTunerChannels))
	m.Handle("POST /api/tuners/{id}/channels/enable", ad(s.handleEnableAllChannels))
	m.Handle("PUT /api/tuner-channels/{id}", ad(s.handleEditTunerChannel))
	m.Handle("GET /api/epg/sources", ad(s.handleEPGSources))
	m.Handle("POST /api/epg/sources", ad(s.handleAddEPGSource))
	m.Handle("DELETE /api/epg/sources/{id}", ad(s.handleDeleteEPGSource))
	m.Handle("POST /api/epg/sources/{id}/refresh", ad(s.handleRefreshEPGSource))
	m.Handle("GET /api/epg/ids", ad(s.handleEPGIDs))
	m.Handle("GET /api/recordings", a(s.handleRecordings))
	m.Handle("POST /api/recordings", a(s.handleSchedule))
	m.Handle("DELETE /api/recordings/{id}", a(s.handleCancelRecording))
	m.Handle("GET /api/recording-rules", a(s.handleRecordingRules))
	m.Handle("POST /api/recording-rules", a(s.handleAddRecordingRule))
	m.Handle("DELETE /api/recording-rules/{id}", a(s.handleDeleteRecordingRule))
}

func tunerError(w http.ResponseWriter, err error) {
	var enc *wittytuner.EncryptedError
	switch {
	case errors.Is(err, livetv.ErrNotFound):
		writeErr(w, 404, "not found")
	case errors.As(err, &enc):
		writeErr(w, 403, err.Error())
	case errors.Is(err, livetv.ErrBusy):
		writeErr(w, 503, err.Error())
	default:
		writeErr(w, 400, err.Error())
	}
}

func (s *Server) handleTunerDrivers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, wittytuner.Drivers(s.TV.Env()))
}

func (s *Server) handleTunerDiscover(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	writeJSON(w, 200, s.TV.Discover(ctx))
}

func (s *Server) handleTuners(w http.ResponseWriter, r *http.Request) {
	list, err := s.TV.Tuners(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, t := range list {
		t.Options = livetv.PublicOptions(t)
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleCreateTuner(w http.ResponseWriter, r *http.Request) {
	var t livetv.Tuner
	if err := readJSON(r, &t); err != nil {
		writeErr(w, 400, "that tuner could not be read")
		return
	}
	got, err := s.TV.Create(r.Context(), &t)
	if err != nil {
		tunerError(w, err)
		return
	}
	got.Options = livetv.PublicOptions(got)
	writeJSON(w, 200, got)
}

func (s *Server) handleUpdateTuner(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b struct {
		Name       string            `json:"name"`
		Options    map[string]string `json:"options"`
		Enabled    *bool             `json:"enabled"`
		MaxStreams int               `json:"maxStreams"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cur, err := s.TV.Tuner(r.Context(), id)
	if err != nil {
		tunerError(w, err)
		return
	}
	en := cur.Enabled
	if b.Enabled != nil {
		en = *b.Enabled
	}
	got, err := s.TV.Update(r.Context(), id, firstNonEmpty(b.Name, cur.Name), b.Options, en, b.MaxStreams)
	if err != nil {
		tunerError(w, err)
		return
	}
	got.Options = livetv.PublicOptions(got)
	writeJSON(w, 200, got)
}

func (s *Server) handleDeleteTuner(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.TV.Delete(r.Context(), id); err != nil {
		tunerError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleRefreshTuner(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.TV.Refresh(r.Context(), id); err != nil {
		tunerError(w, err)
		return
	}
	t, err := s.TV.Tuner(r.Context(), id)
	if err != nil {
		tunerError(w, err)
		return
	}
	t.Options = livetv.PublicOptions(t)
	writeJSON(w, 200, t)
}

func (s *Server) handleScanTuner(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b struct {
		Source string `json:"source"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.TV.StartScan(id, b.Source); err != nil {
		tunerError(w, err)
		return
	}
	writeJSON(w, 200, s.TV.ScanProgress(id))
}

func (s *Server) handleScanProgress2(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	writeJSON(w, 200, s.TV.ScanProgress(id))
}

func (s *Server) handleTunerChannels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	list, err := s.TV.Channels(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleEnableAllChannels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.TV.SetAllEnabled(r.Context(), id, b.Enabled); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleEditTunerChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var e livetv.ChannelEdit
	if err := readJSON(r, &e); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c, err := s.TV.EditChannel(r.Context(), id, e)
	if err != nil {
		tunerError(w, err)
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) handleEPGSources(w http.ResponseWriter, r *http.Request) {
	list, err := s.TV.Sources(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleAddEPGSource(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	src, err := s.TV.AddSource(r.Context(), b.Name, b.URL)
	if err != nil {
		tunerError(w, err)
		return
	}
	writeJSON(w, 200, src)
}

func (s *Server) handleDeleteEPGSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.TV.DeleteSource(r.Context(), id); err != nil {
		tunerError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleRefreshEPGSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	go func() {
		if err := s.TV.FetchSource(context.WithoutCancel(r.Context()), id); err != nil {
			s.Log.Info("guide source", "cat", "tv", "err", err)
		}
	}()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleEPGIDs(w http.ResponseWriter, r *http.Request) {
	list, err := s.TV.GuideIDs(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

// ---------------------------------------------------------------------------
// The guide's tuner rows
// ---------------------------------------------------------------------------

func (s *Server) tunerRows(ctx context.Context, r *http.Request) []guideChannel {
	if !s.access(r).CanWatchChannels("tuner") {
		return nil
	}
	list, err := s.TV.GuideChannels(ctx)
	if err != nil {
		s.Log.Warn("guide", "err", err)
		return nil
	}
	out := make([]guideChannel, 0, len(list))
	for _, c := range list {
		row := guideChannel{Key: "tun-" + strconv.FormatInt(c.ID, 10), Kind: "tuner", ID: c.ID, Number: c.Number, Name: c.Name, Group: c.Group, Logo: c.Logo, HD: c.HD, Radio: c.Radio,
			Encrypted: c.Encrypted, CanRecord: c.Encrypted == ""}
		out = append(out, row)
	}
	return out
}

func broadcast(p livetv.Programme) guideProgramme {
	return guideProgramme{Start: p.Start, End: p.End, Kind: "broadcast", Title: p.Title, Sub: p.Sub, Desc: p.Desc, Genre: p.Category, Rating: p.Rating, Season: p.Season, Episode: p.Episode}
}

func (s *Server) tunerProgrammes(ctx context.Context, row *guideChannel, from, to time.Time) []guideProgramme {
	out := []guideProgramme{}
	c, err := s.TV.Channel(ctx, row.ID)
	if err != nil {
		return out
	}
	list, err := s.TV.Guide(ctx, c, from, to)
	if err != nil {
		s.Log.Warn("guide", "channel", row.Name, "err", err)
		return out
	}
	for _, p := range list {
		out = append(out, broadcast(p))
	}
	return out
}

// ---------------------------------------------------------------------------
// Tuning in
// ---------------------------------------------------------------------------

func (s *Server) canWatchTuner(r *http.Request, id int64) bool {
	if !s.access(r).CanWatchChannels("tuner") {
		return false
	}
	c, err := s.TV.Channel(r.Context(), id)
	return err == nil && c.Enabled
}

func (s *Server) handleTuneTuner(w http.ResponseWriter, r *http.Request, id int64) {
	if !s.canWatchTuner(r, id) {
		writeErr(w, 404, "channel not found")
		return
	}
	c, err := s.TV.Channel(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "channel not found")
		return
	}
	if c.Encrypted != "" {
		writeErr(w, 403, (&wittytuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}).Error())
		return
	}
	if _, err := s.ensureTunerLive(r, id); err != nil {
		tuneError(w, err)
		return
	}
	now := time.Now()
	info := tuneInfo{Key: r.PathValue("key"), Kind: "tuner", Number: c.Number, Name: c.Name, Logo: c.Logo, URL: "/api/tv/" + r.PathValue("key") + "/live/playlist.m3u8",
		ServerTime: now.Unix(), CanRecord: true, BufferSec: float64(s.tvSettingInt(r.Context(), "tvTunerBufferMin") * 60)}
	if list, err := s.TV.Guide(r.Context(), c, now, now.Add(12*time.Hour)); err == nil {
		if len(list) > 0 && list[0].Start <= now.Unix() {
			g := broadcast(list[0])
			info.Now = &g
			info.PositionSec = float64(now.Unix() - list[0].Start)
			list = list[1:]
		}
		if len(list) > 0 {
			g := broadcast(list[0])
			info.Next = &g
		}
	}
	writeJSON(w, 200, info)
}

func tuneError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hls.ErrLiveBusy), errors.Is(err, livetv.ErrBusy):
		writeErr(w, 503, err.Error())
	case wittytuner.IsEncrypted(err):
		writeErr(w, 403, err.Error())
	default:
		writeErr(w, 502, err.Error())
	}
}

// ensureTunerLive starts the shared stream of a tuner channel. FFmpeg and the
// tuner run only while someone keeps asking for the playlist.
func (s *Server) ensureTunerLive(r *http.Request, id int64) (*hls.LiveSession, error) {
	key := "tun-" + strconv.FormatInt(id, 10)
	if ls := s.HLS.GetLive(key); ls != nil {
		ls.Touch()
		return ls, nil
	}
	q, h, preset := s.liveQuality(r)
	copyOK := s.tvSetting(r.Context(), "tvCopyVideo") == "1"
	minutes := s.tvSettingInt(r.Context(), "tvTunerBufferMin")
	// A stream that fails to open right away gives the viewer a clear answer,
	// rather than a player waiting for segments that will never come.
	octx, ocancel := context.WithCancel(context.Background())
	opened, err := s.TV.Open(octx, id)
	if err != nil {
		ocancel()
		return nil, err
	}
	first := opened
	spec := hls.LiveSpec{
		ID: key, Quality: q, MaxHeight: h, Preset: preset, WindowSec: float64(minutes * 60),
		Feed: func(ctx context.Context, _ time.Time) (*hls.LiveInput, error) {
			o := first
			first = nil
			if o == nil {
				var err error
				if o, err = s.TV.Open(ctx, id); err != nil {
					return nil, err
				}
			}
			return s.tunerInput(ctx, o, copyOK), nil
		},
		OnStop: func() {
			ocancel()
			s.TV.ForgetViewer(id)
		},
	}
	ls, err := s.HLS.EnsureLive(spec)
	if err != nil {
		opened.Release()
		ocancel()
		return nil, err
	}
	s.TV.RegisterViewer(id, func() { s.HLS.StopLive(key) })
	return ls, nil
}

// refusalReader turns a scrambled stream into words a person can read.
type refusalReader struct {
	io.ReadCloser
	name  string
	fatal func() error
}

// Fatal gives the reason a guard stopped the stream.
func (r *refusalReader) Fatal() error {
	if r.fatal == nil {
		return nil
	}
	if err := r.fatal(); err != nil {
		return wittytuner.RefusalMessage(r.name, err)
	}
	return nil
}

// tunerInput turns an open stream into what FFmpeg should read.
func (s *Server) tunerInput(ctx context.Context, o *livetv.Opened, copyOK bool) *hls.LiveInput {
	in := &hls.LiveInput{AudioIndex: 0, Realtime: true, Release: o.Release,
		VideoFilter: "bwdif=mode=send_frame:parity=auto:deint=interlaced"}
	if o.Stream.Reader != nil {
		rr := &refusalReader{ReadCloser: o.Stream.Reader, name: o.Channel.Name}
		if o.Stream.Guard != nil {
			rr.fatal = o.Stream.Guard.Fatal
		}
		in.Stdin = rr
		in.InputArgs = []string{"-fflags", "+genpts+discardcorrupt", "-analyzeduration", "3M", "-probesize", "5M", "-f", "mpegts", "-i", "pipe:0"}
		return in
	}
	in.InputArgs = o.Stream.Input
	// Addresses can be probed first: H.264 that needs no work passes through.
	if copyOK && !hasCaptureInput(o.Stream.Input) {
		pctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		if pr, err := s.Tools.ProbeInput(pctx, o.Stream.Input); err == nil {
			if v := pr.Video(); v != nil && v.FieldOrder != "tt" && v.FieldOrder != "bb" && v.FieldOrder != "tb" && v.FieldOrder != "bt" && canCopy(v, s.liveHeight(ctx)) {
				in.CopyVideo = true
			}
			if len(pr.Audio()) == 0 {
				in.AudioIndex = -1
			}
		}
	}
	return in
}

func (s *Server) liveHeight(ctx context.Context) int { return s.tvSettingInt(ctx, "tvMaxHeight") }

func hasCaptureInput(input []string) bool {
	for i, a := range input {
		if a == "-f" && i+1 < len(input) {
			switch input[i+1] {
			case "dshow", "v4l2", "avfoundation", "alsa":
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Recordings
// ---------------------------------------------------------------------------

// canRecord: recordings are of tuner channels, for accounts that may use them.
func (s *Server) canRecord(r *http.Request) bool {
	u := userFrom(r)
	return u != nil && (u.IsAdmin || s.access(r).CanWatchChannels("tuner"))
}

func (s *Server) handleRecordings(w http.ResponseWriter, r *http.Request) {
	if !s.canRecord(r) {
		writeErr(w, 403, "this profile cannot use recordings")
		return
	}
	list, err := s.TV.Recordings(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !userFrom(r).IsAdmin {
		for _, x := range list {
			x.Path = ""
		}
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if !s.canRecord(r) {
		writeErr(w, 403, "this profile cannot use recordings")
		return
	}
	var b struct {
		Channel string `json:"channel"`
		Title   string `json:"title"`
		Sub     string `json:"sub"`
		Start   int64  `json:"start"`
		Stop    int64  `json:"stop"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id, ok := recordableChannel(w, b.Channel)
	if !ok {
		return
	}
	rec, err := s.TV.Schedule(r.Context(), id, userFrom(r).ID, b.Title, b.Sub, b.Start, b.Stop)
	if err != nil {
		tunerError(w, err)
		return
	}
	writeJSON(w, 200, rec)
}

// recordableChannel: library channels are not recorded. Their programmes are
// already the owner's files.
func recordableChannel(w http.ResponseWriter, key string) (int64, bool) {
	kind, id, ok := parseKey(key)
	if !ok {
		writeErr(w, 400, "bad channel")
		return 0, false
	}
	if kind != "tun" {
		writeErr(w, 400, "library channels can't be recorded: what they play is already in your library")
		return 0, false
	}
	return id, true
}

func (s *Server) handleCancelRecording(w http.ResponseWriter, r *http.Request) {
	if !s.canRecord(r) {
		writeErr(w, 403, "this profile cannot use recordings")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	del := qBool(r, "deleteFile") && userFrom(r).IsAdmin
	if err := s.TV.CancelRecording(r.Context(), id, del); err != nil {
		tunerError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleRecordingRules(w http.ResponseWriter, r *http.Request) {
	if !s.canRecord(r) {
		writeErr(w, 403, "this profile cannot use recordings")
		return
	}
	list, err := s.TV.Rules(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleAddRecordingRule(w http.ResponseWriter, r *http.Request) {
	if !s.canRecord(r) {
		writeErr(w, 403, "this profile cannot use recordings")
		return
	}
	var b struct {
		Channel string `json:"channel"`
		Title   string `json:"title"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id, ok := recordableChannel(w, b.Channel)
	if !ok {
		return
	}
	rule, err := s.TV.AddRule(r.Context(), id, userFrom(r).ID, b.Title)
	if err != nil {
		tunerError(w, err)
		return
	}
	writeJSON(w, 200, rule)
}

func (s *Server) handleDeleteRecordingRule(w http.ResponseWriter, r *http.Request) {
	if !s.canRecord(r) {
		writeErr(w, 403, "this profile cannot use recordings")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.TV.DeleteRule(r.Context(), id); err != nil {
		tunerError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
