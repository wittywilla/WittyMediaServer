package server

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/desktop"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Settings → Logs: what is logged, where the files are, opening the folder
// and downloading a file to send along with a bug report. Logs never leave
// the server unless the owner sends them to someone.

type logsView struct {
	Dir        string             `json:"dir"`
	Level      string             `json:"level"`
	Categories []logging.Category `json:"categories"`
	On         []string           `json:"on"`
	Files      []logging.FileInfo `json:"files"`
	CanOpen    bool               `json:"canOpen"` // the browser runs on the server: "Open folder" works
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
	return logsView{Dir: s.Logs.Dir(), Level: strings.ToLower(s.Logs.Level().String()), Categories: logging.Categories, On: on, Files: files, CanOpen: onThisMachine(r)}
}

// GET /api/logs (admin)
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.logsView(r))
}

// PUT /api/logs (admin) {on: [...], level, dir}
func (s *Server) handlePutLogs(w http.ResponseWriter, r *http.Request) {
	var b struct {
		On    *[]string `json:"on"`
		Level *string   `json:"level"`
		Dir   *string   `json:"dir"`
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
		_ = s.DB.SetSetting(ctx, "logDir", val)
	}
	if b.Level != nil {
		lv, ok := parseLevel(*b.Level)
		if !ok {
			writeErr(w, 400, "level must be debug, info, warn or error")
			return
		}
		s.Logs.SetLevel(lv)
		_ = s.DB.SetSetting(ctx, "logLevel", *b.Level)
	}
	if b.On != nil {
		s.Logs.SetCategories(*b.On)
		_ = s.DB.SetSetting(ctx, "logCategories", strings.Join(s.Logs.Enabled(), ","))
	}
	s.Log.Info("log settings changed", "dir", s.Logs.Dir(), "level", s.Logs.Level(), "on", strings.Join(s.Logs.Enabled(), ","))
	writeJSON(w, 200, s.logsView(r))
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

// POST /api/logs/client (any signed-in user): an error from a web or app
// client. Only accepted when the owner turned on "App errors"; size- and
// rate-limited so a broken client cannot fill the disk.
func (s *Server) handleClientLog(w http.ResponseWriter, r *http.Request) {
	if !s.Logs.On("client") {
		w.WriteHeader(204)
		return
	}
	u := userFrom(r)
	if !s.clientLogs.allow(u.ID) {
		w.WriteHeader(429)
		return
	}
	var b struct {
		Level   string `json:"level"`
		Message string `json:"message"`
		Stack   string `json:"stack"`
		Page    string `json:"page"`
		App     string `json:"app"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cut := func(v string, n int) string {
		v = tokenRe.ReplaceAllString(v, "token=REDACTED") // a media URL in an error message
		v = streamTokenRe.ReplaceAllString(v, "st=REDACTED")
		if len(v) > n {
			return v[:n] + "…"
		}
		return v
	}
	lv := slog.LevelInfo
	if b.Level == "error" {
		lv = slog.LevelWarn // written even if other app logging is later switched off
	}
	s.Log.Log(r.Context(), lv, "client: "+cut(b.Message, 500), "cat", "client", "user", u.Name, "app", cut(b.App, 60),
		"page", cut(b.Page, 200), "agent", cut(r.UserAgent(), 160), "stack", cut(b.Stack, 2000))
	w.WriteHeader(204)
}

func parseLevel(v string) (slog.Level, bool) {
	switch strings.ToLower(v) {
	case "debug":
		return slog.LevelDebug, true
	case "info", "":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return 0, false
}

// clientLogLimiter allows 30 client messages per user per minute.
type clientLogLimiter struct {
	mu sync.Mutex
	m  map[int64][]time.Time
}

func (l *clientLogLimiter) allow(user int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[int64][]time.Time{}
	}
	now := time.Now()
	recent := l.m[user][:0]
	for _, t := range l.m[user] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 30 {
		l.m[user] = recent
		return false
	}
	l.m[user] = append(recent, now)
	return true
}

var tokenRe = regexp.MustCompile(`token=[^&\s"']+`)
var streamTokenRe = regexp.MustCompile(`\bst=[^&\s"']+`)
