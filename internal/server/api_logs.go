package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/desktop"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Settings → Logs: what is logged, the viewer, crash reports and the support
// bundle. Logs never leave the server unless the owner sends them to someone.

type logsView struct {
	Dir        string             `json:"dir"`
	Level      string             `json:"level"`
	Categories []logging.Category `json:"categories"`
	Levels     map[string]string  `json:"levels"` // each category's level, by id
	On         []string           `json:"on"`
	Files      []logging.FileInfo `json:"files"`
	CanOpen    bool               `json:"canOpen"` // the browser runs on the server: "Open folder" works
	Health     logging.Health     `json:"health"`
	Crashes    []logging.Crash    `json:"crashes"`
	Modules    []LogModule        `json:"modules"` // each with a level of its own and its advanced options
	// RecordingUntil: everything is being recorded until then, to catch a problem.
	RecordingUntil *time.Time `json:"recordingUntil,omitempty"`
}

func (s *Server) logsView(r *http.Request) logsView {
	on := s.Logs.Enabled()
	if on == nil {
		on = []string{}
	}
	files := s.Logs.Files()
	if files == nil {
		files = []logging.FileInfo{}
	}
	crashes := s.Crashes.List()
	if crashes == nil {
		crashes = []logging.Crash{}
	}
	var recording *time.Time
	if t := s.Logs.RecordingUntil(); !t.IsZero() {
		recording = &t
	}
	return logsView{RecordingUntil: recording, Dir: s.Logs.Dir(), Level: logging.LevelName(s.Logs.Level()), Categories: logging.Categories, Levels: s.Logs.Levels(), On: on, Files: files, CanOpen: onThisMachine(r), Health: s.Logs.Health(), Crashes: crashes, Modules: s.logModuleViews(r.Context())}
}

// GET /api/logs (admin)
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.logsView(r))
}

// PUT /api/logs (admin) {levels: {category: level}, modules: {id: {level, topics}},
// modulesSeen, on: [...], level, dir}
// Changes take effect at once, with no restart.
func (s *Server) handlePutLogs(w http.ResponseWriter, r *http.Request) {
	var b struct {
		On     *[]string          `json:"on"`
		Level  *string            `json:"level"`
		Dir    *string            `json:"dir"`
		Levels *map[string]string `json:"levels"`
		// Modules: how each module is logged. ModulesSeen: an admin has seen the new ones.
		Modules     map[string]moduleLogChange `json:"modules"`
		ModulesSeen bool                       `json:"modulesSeen"`
		// RecordMinutes: record everything for this long, up to 4 hours. 0 stops it.
		RecordMinutes *int `json:"recordMinutes"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	if b.Dir != nil {
		dir := strings.TrimSpace(*b.Dir)
		if dir == "" {
			dir = s.Cfg.LogDir // back to the default
		}
		if err := s.Logs.SetDir(dir); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		val := s.Logs.Dir()
		if val == s.Cfg.LogDir {
			val = ""
		}
		s.saveLogSetting(r, "logDir", val)
		s.Crashes.SetLogDir(s.Logs.Dir())
	}
	if b.Level != nil {
		lv, ok := logging.ParseLevel(*b.Level)
		if !ok {
			writeErr(w, 400, "level must be debug, info, warn or error")
			return
		}
		s.Logs.SetLevel(lv)
		s.saveLogLevels(r)
	}
	if b.On != nil {
		s.Logs.SetCategories(*b.On)
		s.saveLogLevels(r)
		s.saveLogSetting(r, "logCategories", "") // replaced by logLevels
	}
	if b.Levels != nil {
		want := map[string]slog.Level{}
		for k, v := range *b.Levels {
			lv, ok := logging.ParseLevel(v)
			if !ok {
				writeErr(w, 400, "level must be debug, info, warn, error or off")
				return
			}
			want[k] = lv
		}
		s.Logs.SetLevels(want)
		s.saveLogLevels(r)
	}
	if b.RecordMinutes != nil {
		if m := *b.RecordMinutes; m < 0 || m > 240 {
			writeErr(w, 400, "recording can run for up to 240 minutes")
			return
		}
		s.Logs.RecordEverything(time.Duration(*b.RecordMinutes) * time.Minute)
		s.Log.InfoContext(ctx, "recording everything", "minutes", *b.RecordMinutes)
		writeJSON(w, 200, s.logsView(r))
		return
	}
	if b.Modules != nil || b.ModulesSeen {
		if why := s.changeLogModules(ctx, b.Modules, b.ModulesSeen); why != "" {
			writeErr(w, 400, why)
			return
		}
	}
	s.Log.InfoContext(ctx, "log settings changed", "dir", s.Logs.Dir(), "levels", fmt.Sprint(s.Logs.Levels()))
	writeJSON(w, 200, s.logsView(r))
}

func (s *Server) saveLogLevels(r *http.Request) {
	b, err := json.Marshal(s.Logs.Levels())
	if err != nil {
		return
	}
	s.saveLogSetting(r, "logLevels", string(b))
}

func (s *Server) saveLogSetting(r *http.Request, k, v string) {
	if err := s.DB.SetSetting(r.Context(), k, v); err != nil {
		s.Log.ErrorContext(r.Context(), "could not save a log setting", "setting", k, "err", err)
	}
}

// GET /api/logs/entries (admin): the viewer. level, cat (a,b), session, module, q, after
// (a seq: only what is newer, for the live tail), before (a time: only what is
// older), limit, history=1 (look in the files too; always for a session).
func (s *Server) handleLogEntries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := logging.Filter{MinLevel: slog.LevelDebug, Session: logging.ValidSession(q.Get("session")), Module: strings.ToLower(logging.ValidSession(q.Get("module"))), Text: q.Get("q"), Limit: qInt(r, "limit", 200), History: qBool(r, "history")}
	if v := q.Get("level"); v != "" {
		lv, ok := logging.ParseLevel(v)
		if !ok {
			writeErr(w, 400, "unknown level")
			return
		}
		f.MinLevel = lv
	}
	if v := q.Get("cat"); v != "" {
		f.Cats = map[string]bool{}
		for _, c := range strings.Split(v, ",") {
			f.Cats[logging.NormalizeCat(c)] = true
		}
	}
	if v := q.Get("after"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			f.After = n
		}
	}
	if v := q.Get("before"); v != "" {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			f.Before = t
		}
	}
	if f.Session != "" {
		f.History = true
	}
	page, err := s.Logs.Query(f)
	if err != nil {
		if errors.Is(err, logging.ErrBusy) {
			writeErr(w, 429, "Another search of the log files is running. Try again in a moment.")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	h := s.Logs.Health()
	page.Health = &h
	writeJSON(w, 200, page)
}

// GET /api/logs/files/{name} (admin): download one log file.
func (s *Server) handleLogFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !logging.IsLogName(name) {
		writeErr(w, 400, "not a log file")
		return
	}
	s.Logs.Flush()
	p := filepath.Join(s.Logs.Dir(), name)
	if _, err := os.Stat(p); err != nil {
		writeErr(w, 404, "no such log file")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, p)
}

// POST /api/logs/open (admin): shows the log folder in the file manager of
// the server's own desktop.
func (s *Server) handleOpenLogs(w http.ResponseWriter, r *http.Request) {
	if !onThisMachine(r) {
		writeErr(w, 501, "the folder can only be opened on the server's own screen")
		return
	}
	if err := desktop.OpenFolder(s.Logs.Dir()); err != nil {
		writeErr(w, 501, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- crash reports

// GET /api/logs/crashes (admin)
func (s *Server) handleCrashes(w http.ResponseWriter, r *http.Request) {
	list := s.Crashes.List()
	if list == nil {
		list = []logging.Crash{}
	}
	writeJSON(w, 200, map[string]any{"crashes": list})
}

// GET /api/logs/crashes/{id} (admin): the report's text.
func (s *Server) handleCrash(w http.ResponseWriter, r *http.Request) {
	b, err := s.Crashes.Read(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "no such crash report")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if qBool(r, "download") {
		w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("id")+`.txt"`)
	}
	_, _ = w.Write(b)
}

// POST /api/logs/crashes/{id}/seen (admin): stop pointing this one out.
func (s *Server) handleCrashSeen(w http.ResponseWriter, r *http.Request) {
	if err := s.Crashes.MarkSeen(r.PathValue("id")); err != nil {
		writeErr(w, 404, "no such crash report")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/logs/crashes/{id} (admin)
func (s *Server) handleCrashDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Crashes.Delete(r.PathValue("id")); err != nil {
		writeErr(w, 404, "no such crash report")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- the support bundle

func (s *Server) bundleOptions(r *http.Request) logging.BundleOptions {
	o := logging.BundleOptions{Days: qInt(r, "days", 3), Redact: qBool(r, "redact")}
	if libs, err := s.Store.Libraries(r.Context()); err == nil {
		for _, l := range libs {
			o.Roots = append(o.Roots, l.Paths...)
		}
	}
	if b, err := json.MarshalIndent(s.SystemSummary(r.Context()), "", "  "); err == nil {
		o.Extra = append(o.Extra, logging.BundleFile{Name: "summary.json", Description: "Version, modules, system, disk space and settings that matter, with no secrets", Data: b})
	}
	if s.BundleExtras != nil {
		o.Extra = append(o.Extra, s.BundleExtras()...)
	}
	return o
}

// GET /api/logs/bundle/preview (admin): what the bundle would hold. days, redact.
func (s *Server) handleBundlePreview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Logs.Preview(s.Crashes, s.bundleOptions(r)))
}

// GET /api/logs/bundle (admin): the zip. It is made on request, and nothing is
// sent anywhere but to whoever asked. days, redact.
func (s *Server) handleBundle(w http.ResponseWriter, r *http.Request) {
	name := "wittymediaserver-support-" + time.Now().Format("2006-01-02-150405") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	o := s.bundleOptions(r)
	s.Log.InfoContext(r.Context(), "support bundle made", "days", o.Days, "redacted", o.Redact)
	if err := s.Logs.WriteBundle(w, s.Crashes, o); err != nil {
		// The zip has started: all that's left is to end it broken, and say why here.
		s.Log.WarnContext(r.Context(), "support bundle stopped", "err", err)
		panic(http.ErrAbortHandler)
	}
}

// ---------------------------------------------------------------- reports from clients

const (
	clientBodyLimit  = 64 << 10
	clientMaxEvents  = 25
	clientMsgChars   = 1000
	clientStackChars = 4000
)

// clientEvent is one thing that went wrong on a client.
type clientEvent struct {
	Level   string `json:"level"`   // error, warn or info
	Kind    string `json:"kind"`    // js, request, player, game, crash...
	Message string `json:"message"`
	Stack   string `json:"stack"`
	Page    string `json:"page"`
	Session string `json:"session"` // the stream, disc or game session it happened in
	Request string `json:"request"` // the X-Request-Id of a failed request
	Module  string `json:"module"`  // the module it happened in, for one with a part in the app
	Count   int    `json:"count"`   // how many times, when the client grouped them
	Time    string `json:"time"`    // when, by the client's clock
}

// POST /api/logs/client (any signed-in user): errors from a web or app client.
// Size- and rate-limited, per account and for the whole server, so a broken
// client cannot fill the disk. Stored unless the owner turned "App errors" off.
func (s *Server) handleClientLog(w http.ResponseWriter, r *http.Request) {
	if !s.Logs.Accepts("clients", slog.LevelWarn) {
		w.WriteHeader(204)
		return
	}
	var b struct {
		clientEvent
		App     string        `json:"app"`
		Version string        `json:"version"`
		Device  string        `json:"device"`
		Events  []clientEvent `json:"events"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, clientBodyLimit)
	if err := readJSON(r, &b); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeErr(w, 413, "report too large")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	events := b.Events
	if len(events) == 0 && b.Message != "" {
		events = []clientEvent{b.clientEvent} // one error, the older form
	}
	u := userFrom(r)
	if len(events) > clientMaxEvents {
		s.clientLogs.note(len(events) - clientMaxEvents)
		events = events[:clientMaxEvents]
	}
	for i := range events {
		if !s.clientLogs.allow(u.ID) {
			s.clientLogs.note(len(events) - i)
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(429)
			return
		}
		e := &events[i]
		lv := slog.LevelWarn // written even if other app logging is later switched off
		switch e.Level {
		case "warn":
			lv = slog.LevelInfo
		case "info", "debug":
			lv = slog.LevelDebug
		}
		ctx := r.Context()
		if id := logging.ValidSession(e.Session); id != "" {
			ctx = logging.WithSession(ctx, id)
		}
		args := []any{"cat", "clients", "user", u.Name, "app", cutRunes(b.App, 60), "version", cutRunes(b.Version, 40), "device", cutRunes(b.Device, 120),
			"kind", cutRunes(e.Kind, 30), "page", cutRunes(e.Page, 200), "agent", cutRunes(r.UserAgent(), 160)}
		if id := logging.ValidSession(e.Request); id != "" {
			args = append(args, "request", id)
		}
		if e.Count > 1 {
			args = append(args, "count", e.Count)
		}
		if id := logging.ValidSession(e.Module); id != "" {
			args = append(args, "module", id)
		}
		if e.Stack != "" {
			args = append(args, "stack", cutRunes(e.Stack, clientStackChars))
		}
		s.Log.Log(ctx, lv, "client: "+cutRunes(e.Message, clientMsgChars), args...)
	}
	if n := s.clientLogs.takeNoted(); n > 0 {
		s.Log.WarnContext(r.Context(), "client error reports were left out: too many", "cat", "clients", "count", n)
	}
	w.WriteHeader(204)
}

func cutRunes(v string, n int) string {
	if len(v) <= n {
		return v
	}
	for n > 0 && v[n]&0xC0 == 0x80 {
		n--
	}
	return v[:n] + "…"
}

// clientLogLimiter allows so many client messages per user and for everyone, per
// minute. It forgets accounts it hasn't heard from, so its size is bounded.
type clientLogLimiter struct {
	mu    sync.Mutex
	m     map[int64]*clientWindow
	all   clientWindow
	noted int
}

type clientWindow struct {
	start time.Time
	n     int
}

const (
	clientPerUser = 60
	clientOverall = 600
)

func (w *clientWindow) take(now time.Time, limit int) bool {
	if now.Sub(w.start) >= time.Minute {
		w.start, w.n = now, 0
	}
	if w.n >= limit {
		return false
	}
	w.n++
	return true
}

func (l *clientLogLimiter) allow(user int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil || len(l.m) > 4096 {
		l.m = map[int64]*clientWindow{}
	}
	now := time.Now()
	w := l.m[user]
	if w == nil {
		w = &clientWindow{}
		l.m[user] = w
	}
	if !w.take(now, clientPerUser) {
		return false
	}
	return l.all.take(now, clientOverall)
}

func (l *clientLogLimiter) note(n int) {
	l.mu.Lock()
	l.noted += n
	l.mu.Unlock()
}

func (l *clientLogLimiter) takeNoted() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.noted
	l.noted = 0
	return n
}
