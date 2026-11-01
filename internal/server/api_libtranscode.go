package server

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/libtranscode"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Library transcoding: converting a library's files ahead of time. Not to be confused
// with live playback conversion, which leaves files alone. docs/LIBRARY-TRANSCODING.md
// explains it. Everything here is for admins.

func (s *Server) libTranscodeRoutes(m *http.ServeMux) {
	s.wireLibTranscode()
	ad := s.requireAdmin
	m.Handle("GET /api/libtranscode/options", ad(s.handleLTCOptions))
	m.Handle("GET /api/libtranscode/settings", ad(s.handleLTCGetSettings))
	m.Handle("PUT /api/libtranscode/settings", ad(s.handleLTCPutSettings))
	m.Handle("GET /api/libtranscode/libraries", ad(s.handleLTCLibraries))
	m.Handle("PUT /api/libtranscode/libraries/{id}", ad(s.handleLTCPutLibrary))
	m.Handle("GET /api/libtranscode/libraries/{id}/items", ad(s.handleLTCItems))
	m.Handle("POST /api/libtranscode/libraries/{id}/preview", ad(s.handleLTCPreview))
	m.Handle("POST /api/libtranscode/libraries/{id}/sample", ad(s.handleLTCSample))
	m.Handle("POST /api/libtranscode/libraries/{id}/runs", ad(s.handleLTCStart))
	m.Handle("GET /api/libtranscode/runs", ad(s.handleLTCRuns))
	m.Handle("GET /api/libtranscode/runs/{id}", ad(s.handleLTCRun))
	m.Handle("GET /api/libtranscode/runs/{id}/files", ad(s.handleLTCRunFiles))
	m.Handle("GET /api/libtranscode/leftovers", ad(s.handleLTCLeftovers))
	m.Handle("POST /api/libtranscode/files/{id}/undo", ad(s.handleLTCUndo))
	m.Handle("POST /api/libtranscode/files/{id}/use-copy", ad(s.handleLTCUseCopy))
	m.Handle("DELETE /api/libtranscode/files/{id}/leftover", ad(s.handleLTCDeleteLeftover))
	m.Handle("GET /api/libtranscode/tdarr", ad(s.handleTdarrGet))
	m.Handle("PUT /api/libtranscode/tdarr", ad(s.handleTdarrPut))
	m.Handle("POST /api/libtranscode/tdarr/test", ad(s.handleTdarrTest))
	m.Handle("POST /api/libtranscode/tdarr/send", ad(s.handleTdarrSend))
}

// wireLibTranscode makes the engine and tidies up after anything a stopped server left.
func (s *Server) wireLibTranscode() {
	if s.Tools == nil || s.Store == nil || s.Scanner == nil {
		return
	}
	s.LibConvert = &libtranscode.Engine{DB: s.DB, Store: s.Store, Scanner: s.Scanner, Tools: s.Tools, Tasks: s.Tasks,
		Log: s.Log.With("cat", "transcode"),
		Busy: func() int {
			streams, _, games := s.streams.counts()
			return streams + games
		},
		Playing: s.streams.playing,
		SaveMeta: func() bool {
			return s.DB.GetSetting(context.Background(), "saveMetadata", settingDefaults["saveMetadata"]) == "1"
		},
		HardwareAllowed: func() []string {
			v := s.DB.GetSetting(context.Background(), "hwEncoders", "")
			if v == "" {
				return nil
			}
			return strings.Split(v, ",")
		},
	}
	ctx := context.Background()
	s.LibConvert.Recover(ctx)
	s.LibConvert.WatchTdarr()
	s.Tasks.OnRetry(libtranscode.TaskType, func(old tasks.Task, by tasks.By) error {
		id, err := strconv.ParseInt(old.RetryKey, 10, 64)
		if err != nil {
			return tasks.ErrCant
		}
		_, err = s.LibConvert.Retry(ctx, id, by)
		return err
	})
}

// playing says whether someone is playing an item right now.
func (t *streamTracker) playing(item int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, st := range t.live {
		if st.view.ItemID == item {
			return true
		}
	}
	return false
}

func (s *Server) ltcReady(w http.ResponseWriter) bool {
	if s.LibConvert == nil {
		writeErr(w, 503, "FFmpeg isn't available here, so nothing can be converted")
		return false
	}
	return true
}

// ltcCapabilities says what this machine's FFmpeg can do, in words.
type ltcCapabilities struct {
	H264     bool     `json:"h264"`
	HEVC     bool     `json:"hevc"`
	AV1      bool     `json:"av1"`
	Opus     bool     `json:"opus"`
	ToneMap  bool     `json:"toneMap"`
	BurnText bool     `json:"burnText"`
	Hardware []string `json:"hardware"`
	Cores    int      `json:"cores"`
}

func (s *Server) handleLTCOptions(w http.ResponseWriter, r *http.Request) {
	if !s.ltcReady(w) {
		return
	}
	env := s.LibConvert.Env(r.Context())
	has := func(names ...string) bool {
		return slices.ContainsFunc(names, func(n string) bool { return s.Tools.Encoders[n] })
	}
	hw := []string{}
	for _, h := range env.Hardware {
		hw = append(hw, libtranscode.HardwareFamily(h))
	}
	slices.Sort(hw)
	hw = slices.Compact(hw)
	writeJSON(w, 200, map[string]any{
		"presets":  libtranscode.Presets(),
		"outputs":  []string{libtranscode.OutputKeep, libtranscode.OutputReplace, libtranscode.OutputBeside},
		"saveMeta": env.Sidecars,
		"capabilities": ltcCapabilities{
			H264: has("libx264") || slices.ContainsFunc(env.Hardware, func(h string) bool { return strings.HasPrefix(h, "h264_") }),
			HEVC: has("libx265") || slices.ContainsFunc(env.Hardware, func(h string) bool { return strings.HasPrefix(h, "hevc_") }),
			AV1:  has("libsvtav1") || slices.ContainsFunc(env.Hardware, func(h string) bool { return strings.HasPrefix(h, "av1_") }),
			Opus: has("libopus"), ToneMap: env.ToneMap != "", BurnText: s.Tools.HasFilter("subtitles"), Hardware: hw, Cores: runtime.NumCPU(),
		},
	})
}

func (s *Server) handleLTCGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, libtranscode.LoadSettings(r.Context(), s.DB))
}

func (s *Server) handleLTCPutSettings(w http.ResponseWriter, r *http.Request) {
	in := libtranscode.LoadSettings(r.Context(), s.DB)
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := libtranscode.SaveSettings(r.Context(), s.DB, in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, in)
}

// ltcLibrary is a library as this page shows it.
type ltcLibrary struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Items    int               `json:"items"`
	ReadOnly bool              `json:"readOnly"`
	Rule     libtranscode.Rule `json:"rule"`
	Blocked  string            `json:"blocked,omitempty"`
}

func (s *Server) handleLTCLibraries(w http.ResponseWriter, r *http.Request) {
	if !s.ltcReady(w) {
		return
	}
	libs, err := s.Store.Libraries(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []ltcLibrary{}
	for i := range libs {
		l := &libs[i]
		if !libtranscode.Eligible(l.Type) {
			continue
		}
		rule := libtranscode.LoadRule(r.Context(), s.DB, l.ID)
		out = append(out, ltcLibrary{ID: l.ID, Name: l.Name, Type: l.Type, Items: l.ItemCount, ReadOnly: l.ReadOnly, Rule: rule, Blocked: s.LibConvert.Blocked(r.Context(), l, rule)})
	}
	writeJSON(w, 200, out)
}

func (s *Server) libraryFromPath(w http.ResponseWriter, r *http.Request) *library.Library {
	if !s.ltcReady(w) {
		return nil
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "that's not a library")
		return nil
	}
	lib, err := s.Store.Library(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "that library no longer exists")
		return nil
	}
	if !libtranscode.Eligible(lib.Type) {
		writeErr(w, 400, "this kind of library doesn't hold video files to convert")
		return nil
	}
	return lib
}

func (s *Server) handleLTCPutLibrary(w http.ResponseWriter, r *http.Request) {
	lib := s.libraryFromPath(w, r)
	if lib == nil {
		return
	}
	var in struct {
		Policy      libtranscode.Policy `json:"policy"`
		Output      string              `json:"output"`
		AllowWrites bool                `json:"allowWrites"`
		ReadOnly    bool                `json:"readOnly"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := in.Policy.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !libtranscode.ValidOutput(in.Output) {
		writeErr(w, 400, "that's not one of the ways of writing the new file")
		return
	}
	if err := libtranscode.SaveRule(r.Context(), s.DB, libtranscode.Rule{LibraryID: lib.ID, Policy: in.Policy, Output: in.Output, AllowWrites: in.AllowWrites}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.Store.SetLibraryReadOnly(r.Context(), lib.ID, in.ReadOnly); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	lib.ReadOnly = in.ReadOnly
	rule := libtranscode.LoadRule(r.Context(), s.DB, lib.ID)
	writeJSON(w, 200, ltcLibrary{ID: lib.ID, Name: lib.Name, Type: lib.Type, Items: lib.ItemCount, ReadOnly: lib.ReadOnly, Rule: rule, Blocked: s.LibConvert.Blocked(r.Context(), lib, rule)})
}

// handleLTCItems lists a library's files to choose one from.
func (s *Server) handleLTCItems(w http.ResponseWriter, r *http.Request) {
	lib := s.libraryFromPath(w, r)
	if lib == nil {
		return
	}
	items, err := s.Store.ConversionItems(r.Context(), lib.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	type row struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
		Kind  string `json:"kind"`
	}
	out := []row{}
	for _, it := range items {
		if q != "" && !strings.Contains(strings.ToLower(it.Title), q) {
			continue
		}
		out = append(out, row{it.ID, it.Title, it.Kind})
		if len(out) == 100 {
			break
		}
	}
	writeJSON(w, 200, out)
}

type ltcOneItem struct {
	ItemID int64                `json:"itemId"`
	Policy *libtranscode.Policy `json:"policy"`
}

// ltcPolicy reads an item and a recipe, falling back to what's saved.
func (s *Server) ltcPolicy(w http.ResponseWriter, r *http.Request, lib *library.Library) (int64, libtranscode.Policy, bool) {
	var in ltcOneItem
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return 0, libtranscode.Policy{}, false
	}
	p := libtranscode.LoadRule(r.Context(), s.DB, lib.ID).Policy
	if in.Policy != nil {
		p = *in.Policy
		if err := p.Validate(); err != nil {
			writeErr(w, 400, err.Error())
			return 0, libtranscode.Policy{}, false
		}
	}
	return in.ItemID, p, true
}

func (s *Server) handleLTCPreview(w http.ResponseWriter, r *http.Request) {
	lib := s.libraryFromPath(w, r)
	if lib == nil {
		return
	}
	id, p, ok := s.ltcPolicy(w, r, lib)
	if !ok {
		return
	}
	pv, err := s.LibConvert.PreviewItem(r.Context(), lib, id, p)
	if errors.Is(err, library.ErrNotFound) {
		writeErr(w, 404, "that file isn't in this library")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, pv)
}

func (s *Server) handleLTCSample(w http.ResponseWriter, r *http.Request) {
	lib := s.libraryFromPath(w, r)
	if lib == nil {
		return
	}
	id, p, ok := s.ltcPolicy(w, r, lib)
	if !ok {
		return
	}
	res, err := s.LibConvert.Sample(r.Context(), lib, id, p, s.Cfg.CacheDir)
	if errors.Is(err, library.ErrNotFound) {
		writeErr(w, 404, "that file isn't in this library")
		return
	}
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleLTCStart(w http.ResponseWriter, r *http.Request) {
	lib := s.libraryFromPath(w, r)
	if lib == nil {
		return
	}
	var in struct {
		Dry    bool                 `json:"dry"`
		Items  []int64              `json:"items"`
		Policy *libtranscode.Policy `json:"policy"`
		Output string               `json:"output"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(in.Items) > 10000 {
		writeErr(w, 400, "too many files at once")
		return
	}
	rule := libtranscode.LoadRule(r.Context(), s.DB, lib.ID)
	q := libtranscode.RunRequest{Library: lib.ID, Policy: rule.Policy, Output: rule.Output, Dry: in.Dry, Items: in.Items, By: byUser(userFrom(r))}
	// A dry run can try out a recipe that hasn't been saved. A real run does what's saved,
	// so what happened can always be found from what the page says is set.
	if in.Dry && in.Policy != nil {
		q.Policy = *in.Policy
		if err := q.Policy.Validate(); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if in.Dry && in.Output != "" {
		q.Output = in.Output
	}
	if !in.Dry && !rule.Saved {
		writeErr(w, 409, "save a recipe for this library first")
		return
	}
	id, err := s.LibConvert.Start(r.Context(), q)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, map[string]any{"run": id})
}

func (s *Server) handleLTCRuns(w http.ResponseWriter, r *http.Request) {
	if !s.ltcReady(w) {
		return
	}
	library, _ := strconv.ParseInt(r.URL.Query().Get("library"), 10, 64)
	list, err := s.LibConvert.ListRuns(r.Context(), library, 30)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleLTCRun(w http.ResponseWriter, r *http.Request) {
	if !s.ltcReady(w) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "that's not a run")
		return
	}
	run, err := s.LibConvert.GetRun(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, run)
}

func (s *Server) handleLTCRunFiles(w http.ResponseWriter, r *http.Request) {
	if !s.ltcReady(w) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "that's not a run")
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	files, total, err := s.LibConvert.ListFiles(r.Context(), id, q.Get("state"), limit, max(offset, 0))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"files": files, "total": total})
}

func (s *Server) handleLTCLeftovers(w http.ResponseWriter, r *http.Request) {
	if !s.ltcReady(w) {
		return
	}
	library, _ := strconv.ParseInt(r.URL.Query().Get("library"), 10, 64)
	files, total, err := s.LibConvert.Leftovers(r.Context(), library, 200)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"files": files, "totalBytes": total})
}

// ltcFileAction runs an action on a file of a run.
func (s *Server) ltcFileAction(w http.ResponseWriter, r *http.Request, act func(context.Context, int64) error) {
	if !s.ltcReady(w) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "that's not a file")
		return
	}
	switch err := act(r.Context(), id); {
	case err == nil:
		w.WriteHeader(204)
	case errors.Is(err, libtranscode.ErrNoRun):
		writeErr(w, 404, err.Error())
	default:
		writeErr(w, 409, err.Error())
	}
}

func (s *Server) handleLTCUndo(w http.ResponseWriter, r *http.Request) {
	s.ltcFileAction(w, r, func(ctx context.Context, id int64) error { return s.LibConvert.Undo(ctx, id) })
}

func (s *Server) handleLTCUseCopy(w http.ResponseWriter, r *http.Request) {
	s.ltcFileAction(w, r, func(ctx context.Context, id int64) error { return s.LibConvert.UseCopy(ctx, id) })
}

func (s *Server) handleLTCDeleteLeftover(w http.ResponseWriter, r *http.Request) {
	s.ltcFileAction(w, r, func(ctx context.Context, id int64) error { return s.LibConvert.DeleteLeftover(ctx, id) })
}

// ------------------------------------------------------------------ Tdarr

func (s *Server) handleTdarrGet(w http.ResponseWriter, r *http.Request) {
	if !s.ltcReady(w) {
		return
	}
	writeJSON(w, 200, s.LibConvert.LoadTdarr(r.Context()))
}

func (s *Server) handleTdarrPut(w http.ResponseWriter, r *http.Request) {
	if !s.ltcReady(w) {
		return
	}
	var in struct {
		libtranscode.TdarrSettings
		APIKey   string `json:"apiKey"`
		ClearKey bool   `json:"clearKey"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.LibConvert.SaveTdarr(r.Context(), in.TdarrSettings, in.APIKey, in.ClearKey); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, s.LibConvert.LoadTdarr(r.Context()))
}

// handleTdarrTest looks at a Tdarr server, the one typed in or the one saved.
func (s *Server) handleTdarrTest(w http.ResponseWriter, r *http.Request) {
	if !s.ltcReady(w) {
		return
	}
	var in struct {
		URL    string `json:"url"`
		APIKey string `json:"apiKey"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	writeJSON(w, 200, s.LibConvert.TdarrLook(ctx, in.URL, in.APIKey))
}

func (s *Server) handleTdarrSend(w http.ResponseWriter, r *http.Request) {
	if !s.ltcReady(w) {
		return
	}
	var in struct {
		LibraryID int64   `json:"libraryId"`
		Items     []int64 `json:"items"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(in.Items) > 10000 {
		writeErr(w, 400, "too many files at once")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	msg, err := s.LibConvert.SendToTdarr(ctx, in.LibraryID, in.Items)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"message": msg})
}
