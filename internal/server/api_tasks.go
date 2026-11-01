package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// The activity API: what the server is doing in the background, for the button in the
// top bar, WittyStats and the native apps. docs/TASKS.md explains it.

func init() {
	settingKeys = append(settingKeys, "activityEnabled", "activityViewers", "activityKeepCount", "activityKeepDays")
	// activityEnabled: the owner's switch for the whole feature.
	settingDefaults["activityEnabled"] = "1"
	// activityViewers: who besides admins gets the read-only view.
	settingDefaults["activityViewers"] = "admins"
	settingDefaults["activityKeepCount"] = "500"
	settingDefaults["activityKeepDays"] = "30"
	settingChecks["activityEnabled"] = oneOf("0", "1")
	settingChecks["activityViewers"] = oneOf("admins", "adults", "everyone")
	settingChecks["activityKeepCount"] = intBetween(10, 5000)
	settingChecks["activityKeepDays"] = intBetween(1, 365)
}

// taskRoutes registers the API, and makes the registry if nobody has yet.
func (s *Server) taskRoutes(m *http.ServeMux) {
	s.wireTasks()
	a, ad := s.requireUser, s.requireAdmin
	m.Handle("GET /api/system/tasks", a(s.handleTasks))
	m.Handle("GET /api/system/tasks/stream", a(s.handleTaskStream))
	m.Handle("GET /api/system/tasks/history", a(s.handleTaskHistory))
	m.Handle("DELETE /api/system/tasks/history", ad(s.handleTaskHistoryClear))
	m.Handle("DELETE /api/system/tasks/history/{id}", ad(s.handleTaskHistoryDelete))
	m.Handle("POST /api/system/tasks/{id}/pause", ad(s.handleTaskPause))
	m.Handle("POST /api/system/tasks/{id}/resume", ad(s.handleTaskResume))
	m.Handle("POST /api/system/tasks/{id}/cancel", ad(s.handleTaskCancel))
	m.Handle("POST /api/system/tasks/{id}/retry", ad(s.handleTaskRetry))
}

// wireTasks gives every background job the same registry. It comes before anything
// that could start one.
func (s *Server) wireTasks() {
	store := &tasks.SQLStore{DB: s.DB, Failed: func(err error) {
		s.Log.Warn("could not save the activity history", "cat", "server", "err", err)
	}, Keep: func() (int, int) {
		ctx := context.Background()
		count, _ := strconv.Atoi(s.DB.GetSetting(ctx, "activityKeepCount", settingDefaults["activityKeepCount"]))
		days, _ := strconv.Atoi(s.DB.GetSetting(ctx, "activityKeepDays", settingDefaults["activityKeepDays"]))
		return count, days
	}}
	store.MarkInterrupted(context.Background())
	s.taskStore = store
	s.Tasks = tasks.New(store)
	if s.Scanner != nil {
		s.Scanner.Tasks = s.Tasks
	}
	modmgr.ReportTasksTo(s.Tasks)
	s.Tasks.OnRetry(tasks.TypeScan, func(old tasks.Task, by tasks.By) error {
		id, err := strconv.ParseInt(old.RetryKey, 10, 64)
		if err != nil {
			return tasks.ErrCant
		}
		if _, err := s.Store.Library(context.Background(), id); err != nil {
			return errors.New("that library no longer exists")
		}
		if !s.Scanner.StartBy(id, by) {
			return errors.New("that library is already being scanned")
		}
		return nil
	})
}

func byUser(u *auth.User) tasks.By {
	if u == nil {
		return tasks.By{}
	}
	return tasks.By{ID: u.ID, Name: u.Name}
}

// activityAccess says what this account gets. The server's rule, not the client's.
type activityAccess struct {
	Enabled  bool `json:"enabled"`  // the owner's switch
	Visible  bool `json:"visible"`  // this account may see it
	ReadOnly bool `json:"readOnly"` // no controls, no file names, nobody else's details
}

func (s *Server) activityAccessFor(ctx context.Context, u *auth.User) activityAccess {
	acc := activityAccess{Enabled: s.DB.GetSetting(ctx, "activityEnabled", "1") != "0", ReadOnly: !u.IsAdmin}
	if !acc.Enabled {
		return acc
	}
	if u.IsAdmin {
		acc.Visible = true
		return acc
	}
	switch s.DB.GetSetting(ctx, "activityViewers", "admins") {
	case "everyone":
		acc.Visible = true
	case "adults":
		acc.Visible = u.Kind == auth.KindAdult
	}
	return acc
}

func (s *Server) taskViewer(r *http.Request) tasks.Viewer {
	u := userFrom(r)
	if u.IsAdmin {
		return tasks.Viewer{Admin: true, UserID: u.ID}
	}
	acc := s.access(r)
	return tasks.Viewer{UserID: u.ID, Library: acc.AllowsLibrary}
}

// TaskList is what's going on, as this account may see it.
type TaskList struct {
	Access  activityAccess `json:"access"`
	Tasks   []tasks.Task   `json:"tasks"`
	Recent  []tasks.Task   `json:"recent"`
	Summary tasks.Summary  `json:"summary"`
}

func visible(v tasks.Viewer, list []tasks.Task) []tasks.Task {
	out := make([]tasks.Task, 0, len(list))
	for _, t := range list {
		if vt, ok := v.For(t); ok {
			out = append(out, vt)
		}
	}
	return out
}

func (s *Server) taskList(v tasks.Viewer, acc activityAccess) TaskList {
	out := TaskList{Access: acc, Tasks: []tasks.Task{}, Recent: []tasks.Task{}, Summary: tasks.Summary{Percent: -1}}
	if !acc.Visible {
		return out
	}
	out.Tasks = visible(v, s.Tasks.Live())
	out.Recent = visible(v, s.Tasks.Recent())
	out.Summary = tasks.Summarize(out.Tasks)
	return out
}

// GET /api/system/tasks: for an account that may not see it, an empty list that says so.
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.taskList(s.taskViewer(r), s.activityAccessFor(r.Context(), userFrom(r))))
}

// How many streams can be open, so a misbehaving client can't hold every connection.
const (
	maxTaskStreams        = 256
	maxTaskStreamsPerUser = 16
	taskStreamBeat        = 25 * time.Second
	taskStreamWrite       = 20 * time.Second
)

type taskStreamCounts struct {
	sync.Mutex
	all    int
	byUser map[int64]int
}

func (c *taskStreamCounts) take(user int64) bool {
	c.Lock()
	defer c.Unlock()
	if c.byUser == nil {
		c.byUser = map[int64]int{}
	}
	if c.all >= maxTaskStreams || c.byUser[user] >= maxTaskStreamsPerUser {
		return false
	}
	c.all++
	c.byUser[user]++
	return true
}

func (c *taskStreamCounts) give(user int64) {
	c.Lock()
	defer c.Unlock()
	c.all--
	if c.byUser[user]--; c.byUser[user] <= 0 {
		delete(c.byUser, user)
	}
}

// taskUpdate is a change: Tasks holds whatever changed, whole, finished ones included.
type taskUpdate struct {
	Tasks   []tasks.Task  `json:"tasks"`
	Gone    []string      `json:"gone,omitempty"`
	Summary tasks.Summary `json:"summary"`
}

// GET /api/system/tasks/stream: server-sent events. "snapshot" first, then "update" as things
// change, at most twice a second, and nothing at all when nothing does. The account is
// looked at again with each keep-alive, so signing out or losing access ends it.
func (s *Server) handleTaskStream(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	acc := s.activityAccessFor(r.Context(), u)
	if !acc.Visible {
		writeErr(w, 403, "activity isn't available to this account")
		return
	}
	if !s.taskStreams.take(u.ID) {
		writeErr(w, 429, "too many activity streams are open")
		return
	}
	defer s.taskStreams.give(u.ID)
	tok := tokenFrom(r)
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // a proxy mustn't hold it back
	w.WriteHeader(200)

	send := func(event string, v any) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(taskStreamWrite))
		b, err := json.Marshal(v)
		if err != nil {
			return false
		}
		if _, err := w.Write(append(append(append(append([]byte("event: "), event...), "\ndata: "...), b...), "\n\n"...)); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	sub := s.Tasks.Subscribe()
	defer sub.Close()
	viewer := s.taskViewer(r)
	if !send("snapshot", s.taskList(viewer, acc)) {
		return
	}
	beat := time.NewTicker(taskStreamBeat)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-beat.C:
			if !s.taskStreamStillAllowed(r, tok, u) {
				return
			}
			_ = rc.SetWriteDeadline(time.Now().Add(taskStreamWrite))
			if _, err := w.Write([]byte(": still here\n\n")); err != nil || rc.Flush() != nil {
				return
			}
		case <-sub.C():
			c := sub.Take()
			up := taskUpdate{Tasks: []tasks.Task{}, Gone: c.Gone}
			for _, t := range c.Tasks {
				if vt, ok := viewer.For(t); ok {
					up.Tasks = append(up.Tasks, vt)
				}
			}
			if len(up.Tasks) == 0 && len(up.Gone) == 0 {
				continue // nothing this account may see
			}
			up.Summary = tasks.Summarize(visible(viewer, s.Tasks.Live()))
			if !send("update", up) {
				return
			}
		}
	}
}

// taskStreamStillAllowed: the session is good, it's the same kind of account, and the
// owner hasn't turned this off for them.
func (s *Server) taskStreamStillAllowed(r *http.Request, tok string, was *auth.User) bool {
	if tok == "" {
		return true // signed in some other way, for which there's nothing to look up
	}
	var now *auth.User
	var err error
	if auth.IsStreamToken(tok) {
		return false // a stream token is for media; it has no business here
	}
	if now, err = s.Auth.Resolve(r.Context(), tok); err != nil || now == nil {
		return false
	}
	if now.IsAdmin != was.IsAdmin || now.Kind != was.Kind {
		return false // the client reconnects, and gets what they get now
	}
	return s.activityAccessFor(r.Context(), now).Visible
}

// GET /api/system/tasks/history: finished tasks, newest first. limit, before (a time, from the
// last row of the page before), type, state, library.
func (s *Server) handleTaskHistory(w http.ResponseWriter, r *http.Request) {
	if !s.activityAccessFor(r.Context(), userFrom(r)).Visible {
		writeErr(w, 403, "activity isn't available to this account")
		return
	}
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	lib, _ := strconv.ParseInt(q.Get("library"), 10, 64)
	list, err := s.taskStore.List(r.Context(), tasks.Query{Limit: qInt(r, "limit", 50), Before: before, Type: q.Get("type"), State: q.Get("state"), Library: lib})
	if err != nil {
		writeErr(w, 500, "the history could not be read")
		return
	}
	for i := range list {
		list[i].CanRetry = s.Tasks.CanRetry(list[i])
	}
	writeJSON(w, 200, map[string]any{"tasks": visible(s.taskViewer(r), list)})
}

// DELETE /api/system/tasks/history: forgets the history. What's still going stays.
func (s *Server) handleTaskHistoryClear(w http.ResponseWriter, r *http.Request) {
	if err := s.taskStore.Clear(r.Context(), ""); err != nil {
		writeErr(w, 500, "the history could not be cleared")
		return
	}
	s.Tasks.ForgetRecent("")
	s.Log.InfoContext(r.Context(), "activity history cleared", "cat", "server", "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/system/tasks/history/{id}: forgets one.
func (s *Server) handleTaskHistoryDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.taskStore.Clear(r.Context(), id); err != nil {
		writeErr(w, 500, "that could not be removed")
		return
	}
	s.Tasks.ForgetRecent(id)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) taskControl(w http.ResponseWriter, r *http.Request, action string, f func(id string) error) {
	id := r.PathValue("id")
	if err := f(id); err != nil {
		switch {
		case errors.Is(err, tasks.ErrNotFound):
			writeErr(w, 404, err.Error())
		case errors.Is(err, tasks.ErrCant):
			writeErr(w, 409, err.Error())
		default:
			writeErr(w, 409, err.Error())
		}
		return
	}
	s.Log.InfoContext(logging.WithSession(r.Context(), logging.ValidSession(id)), "activity control", "cat", "server", "action", action, "task", id, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/system/tasks/{id}/pause
func (s *Server) handleTaskPause(w http.ResponseWriter, r *http.Request) {
	s.taskControl(w, r, "pause", s.Tasks.Pause)
}

// POST /api/system/tasks/{id}/resume
func (s *Server) handleTaskResume(w http.ResponseWriter, r *http.Request) {
	s.taskControl(w, r, "resume", s.Tasks.Resume)
}

// POST /api/system/tasks/{id}/cancel
func (s *Server) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	s.taskControl(w, r, "cancel", s.Tasks.Cancel)
}

// POST /api/system/tasks/{id}/retry: starts a failed or cancelled task again.
func (s *Server) handleTaskRetry(w http.ResponseWriter, r *http.Request) {
	s.taskControl(w, r, "retry", func(id string) error {
		old, ok := s.Tasks.Find(id)
		if !ok {
			if old, ok = s.taskStore.Get(r.Context(), id); !ok {
				return tasks.ErrNotFound
			}
		}
		return s.Tasks.Retry(old, byUser(userFrom(r)))
	})
}
