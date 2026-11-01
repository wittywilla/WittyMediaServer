package server

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/diagnose"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/reset"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Settings, Repair: the same doctor as `wittymediaserver doctor`, run by the server on itself. One thing
// runs at a time, in the background, and this page can be left and come back to. Anything
// that changes data asks for the password again, as a reset does. Paths only ever
// come from what the server already knows, except the new place of a moved drive, which an
// administrator types and which is checked before it's believed.

func (s *Server) repairRoutes(m *http.ServeMux) {
	m.Handle("GET /api/repair", s.requireAdmin(s.handleRepairView))
	m.Handle("POST /api/repair/check", s.requireAdmin(s.handleRepairCheck))
	m.Handle("POST /api/repair/fix", s.requireAdmin(s.handleRepairFix))
	m.Handle("POST /api/repair/remap", s.requireAdmin(s.handleRepairRemap))
	m.Handle("POST /api/repair/restart", s.requireAdmin(s.handleRepairRestart))
}

// repairState is what the page shows, and what the next fix works from.
type repairState struct {
	mu       sync.Mutex
	running  string // "check" or "fix", while one runs
	phase    string
	fraction float64
	cancel   context.CancelFunc
	report   *diagnose.DoctorReport
	results  []diagnose.FixResult
	restart  bool // a fix waits for the next start
	err      string
	task     *tasks.Handle // in the activity panel, while it runs
}

type repairView struct {
	Running    string                 `json:"running,omitempty"`
	Phase      string                 `json:"phase,omitempty"`
	Fraction   float64                `json:"fraction"`
	Report     *diagnose.DoctorReport `json:"report,omitempty"`
	Results    []diagnose.FixResult   `json:"results"`
	Restart    bool                   `json:"restart"`
	CanRestart bool                   `json:"canRestart"`
	TwoFactor  bool                   `json:"twoFactor"`
	Error      string                 `json:"error,omitempty"`
	Copies     []string               `json:"copies"`
	// LastRepair: how the last database to wait for a restart went.
	LastRepair *reset.RepairResult `json:"lastRepair,omitempty"`
}

// repairEnv tells the doctor about this server. Module changes go through the running
// manager, which holds its choices in memory.
func (s *Server) repairEnv() *diagnose.DoctorEnv {
	e := &diagnose.DoctorEnv{Data: s.Cfg.DataDir, Cache: s.Cfg.CacheDir, Images: s.Cfg.ImagesDir, Transcode: s.Cfg.TranscodeDir, LogDir: s.Cfg.LogDir,
		ConfigFile: s.Cfg.ConfigFile, Program: config.ProgramDir(), Portable: s.Cfg.Portable, Listen: s.Cfg.Listen, FFmpeg: s.Cfg.FFmpegPath, FFprobe: s.Cfg.FFprobePath,
		Version: s.Version, InServer: true, Log: s.Log}
	if s.Modules != nil {
		e.Modules = moduleOps{s.Modules}
	}
	return e
}

type moduleOps struct{ m *modmgr.Manager }

func (o moduleOps) Rollback(id string) error   { return o.m.Rollback(id) }
func (o moduleOps) UseBuiltin(id string) error { return o.m.UseBuiltin(id) }
func (o moduleOps) Install(id string) error {
	return o.m.ReinstallNow(context.Background(), id)
}

func (s *Server) repairView(r *http.Request) repairView {
	st := &s.repair
	st.mu.Lock()
	defer st.mu.Unlock()
	v := repairView{Running: st.running, Phase: st.phase, Fraction: st.fraction, Report: st.report, Results: st.results, Restart: st.restart, Error: st.err,
		CanRestart: s.Restart != nil, TwoFactor: userFrom(r).TwoFactor, Copies: diagnose.SafetyCopies(s.Cfg.DataDir), LastRepair: reset.LastRepair(s.Cfg.DataDir)}
	if v.LastRepair != nil && time.Since(v.LastRepair.At) > 7*24*time.Hour {
		v.LastRepair = nil // old news
	}
	if reset.PendingRepair(s.Cfg.DataDir) != nil {
		v.Restart = true // it survives this page being reloaded, and the server restarting
	}
	if v.Results == nil {
		v.Results = []diagnose.FixResult{}
	}
	if v.Copies == nil {
		v.Copies = []string{}
	}
	return v
}

// GET /api/repair (admin): what's running, and the last results.
func (s *Server) handleRepairView(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.repairView(r))
}

// startRepair claims the one slot, and runs work in the background.
func (s *Server) startRepair(kind string, work func(ctx context.Context, st *repairState)) bool {
	st := &s.repair
	st.mu.Lock()
	if st.running != "" {
		st.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	st.running, st.phase, st.fraction, st.cancel, st.err = kind, "Starting", 0, cancel, ""
	st.task = s.Tasks.Start(tasks.Spec{Type: tasks.TypeDiagnose, Title: "Repair: " + kind, Unit: "percent", Cancel: cancel, AdminOnly: true})
	task := st.task
	st.mu.Unlock()
	logging.Go("repair "+kind, func() {
		defer cancel()
		defer func() {
			st.mu.Lock()
			failed := st.err
			st.running, st.phase, st.fraction, st.cancel, st.task = "", "", 0, nil, nil
			st.mu.Unlock()
			if failed != "" {
				task.Finish(errors.New(failed))
			} else {
				task.Finish(ctx.Err())
			}
		}()
		work(ctx, st)
	}, "cat", "server")
	return true
}

func (st *repairState) progress(phase string, f float64) {
	st.mu.Lock()
	st.phase, st.fraction = phase, f
	task := st.task
	st.mu.Unlock()
	task.Phase(phase)
	task.Progress(int64(f*100), 100)
}

// POST /api/repair/check (admin) {only}: looks, and changes nothing.
func (s *Server) handleRepairCheck(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Only []string `json:"only"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	checks, err := diagnose.Selected(b.Only)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.startRepair("check", func(ctx context.Context, st *repairState) {
		rep := diagnose.RunDoctor(ctx, s.repairEnv(), checks, st.progress)
		st.mu.Lock()
		st.report, st.results = rep, nil
		st.mu.Unlock()
	}) {
		writeErr(w, 409, "a check or repair is already running")
		return
	}
	writeJSON(w, 202, s.repairView(r))
}

type repairFixBody struct {
	Keys     []string `json:"keys"`
	AllSafe  bool     `json:"allSafe"`
	Option   string   `json:"option"`
	Password string   `json:"password"`
	Code     string   `json:"code"`
}

// POST /api/repair/fix (admin) {keys, allSafe, option, password, code}: runs fixes from the
// last check. A "safe" fix can't lose anything and needs nothing more; anything else asks
// for the password again.
func (s *Server) handleRepairFix(w http.ResponseWriter, r *http.Request) {
	var b repairFixBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	st := &s.repair
	st.mu.Lock()
	rep := st.report
	st.mu.Unlock()
	if rep == nil {
		writeErr(w, 409, "run the check first")
		return
	}
	var todo []*diagnose.Issue
	needsPassword := b.Option != ""
	if b.AllSafe {
		for i := range rep.Issues {
			if iss := &rep.Issues[i]; iss.CanFix() && iss.Safety == diagnose.SafetySafe {
				todo = append(todo, iss)
			}
		}
	}
	for _, k := range b.Keys {
		iss := rep.Find(k)
		if iss == nil {
			writeErr(w, 404, "that isn't in the last check: run it again")
			return
		}
		if !iss.CanFix() {
			writeErr(w, 400, "that one needs doing by hand")
			return
		}
		if iss.Safety != diagnose.SafetySafe {
			needsPassword = true
		}
		todo = append(todo, iss)
	}
	if len(todo) == 0 {
		writeErr(w, 400, "nothing to fix")
		return
	}
	if needsPassword && !s.resetAllowed(w, r, resetBody{Password: b.Password, Code: b.Code}) {
		return
	}
	who := userFrom(r).Name
	if !s.startRepair("fix", func(ctx context.Context, st *repairState) {
		st.progress("Fixing", 0)
		env := s.repairEnv()
		res := diagnose.ApplyFixes(ctx, env, todo, b.Option)
		s.Log.Warn("repairs were made", "cat", "server", "by", who, "count", len(res))
		st.progress("Checking again", 0.8)
		restart := false
		for _, x := range res {
			restart = restart || x.Restart
		}
		rep := diagnose.RunDoctor(ctx, env, diagnose.AllChecks(), nil)
		st.mu.Lock()
		st.report, st.results = rep, res
		st.restart = st.restart || restart
		st.mu.Unlock()
	}) {
		writeErr(w, 409, "a check or repair is already running")
		return
	}
	writeJSON(w, 202, s.repairView(r))
}

// POST /api/repair/remap (admin) {from, to, apply, password, code}: a folder moved. Without
// apply it only says what would change.
func (s *Server) handleRepairRemap(w http.ResponseWriter, r *http.Request) {
	var b struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Apply    bool   `json:"apply"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(b.From) > 4096 || len(b.To) > 4096 {
		writeErr(w, 400, "that path is too long")
		return
	}
	// Saying what a move would change looks at the new place, before any password.
	if err := s.checkAdminPaths(r, []string{b.To}, nil); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	env := s.repairEnv()
	if !b.Apply {
		plan, err := diagnose.PlanRemap(r.Context(), env, b.From, b.To)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, plan)
		return
	}
	if !s.resetAllowed(w, r, resetBody{Password: b.Password, Code: b.Code}) {
		return
	}
	// A short job, but it can't overlap a fix.
	st := &s.repair
	st.mu.Lock()
	if st.running != "" {
		st.mu.Unlock()
		writeErr(w, 409, "a check or repair is already running")
		return
	}
	st.running, st.phase = "fix", "Moving the folder"
	st.mu.Unlock()
	defer func() {
		st.mu.Lock()
		st.running, st.phase = "", ""
		st.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	plan, err := diagnose.ApplyRemap(ctx, env, b.From, b.To)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	s.Log.WarnContext(r.Context(), "a library folder's paths were moved", "cat", "library", "by", userFrom(r).Name, "from", b.From, "to", b.To, "applied", plan.Applied)
	writeJSON(w, 200, plan)
}

// POST /api/repair/restart (admin): for a fix that waits for the next start.
func (s *Server) handleRepairRestart(w http.ResponseWriter, r *http.Request) {
	st := &s.repair
	st.mu.Lock()
	want := st.restart
	st.mu.Unlock()
	if !want && reset.PendingRepair(s.Cfg.DataDir) == nil {
		writeErr(w, 409, "no repair is waiting for a restart")
		return
	}
	writeJSON(w, 202, map[string]any{"restarting": s.Restart != nil})
	if s.Restart != nil {
		time.AfterFunc(300*time.Millisecond, s.Restart)
	}
}
