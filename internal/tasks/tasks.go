// Package tasks is the one place the server keeps track of its background work:
// library scans, metadata, module installs, transcodes, backups and anything else
// that takes longer than a request. The activity button, the WittyStats module and
// the native apps all read from it.
//
// Using it from a job takes a few lines (docs/TASKS.md has the whole story):
//
//	t := reg.Start(tasks.Spec{Type: tasks.TypeBackup, Title: "Backing up", Cancel: cancel, Pausable: true})
//	defer func() { t.Finish(err) }()
//	for i, f := range files {
//		if err = t.Wait(ctx); err != nil { return }  // holds here while paused; ctx's error once cancelled
//		t.Progress(int64(i), int64(len(files)))
//		t.Item(filepath.Base(f), f)
//	}
//
// A nil *Registry and a nil *Handle are both fine to use and do nothing, so a job
// never has to ask whether anyone is keeping track.
//
// It is built to cost next to nothing. Reporting takes a lock and sets a few fields.
// With nobody watching nothing else happens: there is no timer, no goroutine and
// no copying. With someone watching, one goroutine sends what changed at most
// twice a second, however often jobs report.
package tasks

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
)

// State is where a task is in its life.
type State string

const (
	Queued    State = "queued"
	Running   State = "running"
	Paused    State = "paused"
	Failed    State = "failed"
	Done      State = "done"
	Cancelled State = "cancelled"
)

// Finished reports whether the task will not change again.
func (s State) Finished() bool { return s == Failed || s == Done || s == Cancelled }

// The kinds of work the server knows. A module or a new job may use any short name
// of its own: the app shows an unknown kind with a plain icon and its title.
const (
	TypeScan      = "scan"
	TypeAnalysis  = "analysis"
	TypeMetadata  = "metadata"
	TypeThumbnail = "thumbnails"
	TypeIntro     = "intros"
	TypeGames     = "games"
	TypeModule    = "module"
	TypeBackup    = "backup"
	TypeTranscode = "transcode"
	TypeUpdate    = "update"
	TypeGuide     = "guide"
	TypeRecording = "recording"
	TypeDiagnose  = "diagnose"
	TypeOther     = "other"
)

// By says who asked for a task. The zero value is the server itself.
type By struct {
	ID   int64
	Name string
}

// Problem is something that went wrong with one thing inside a task that carried
// on anyway: one file out of thousands.
type Problem struct {
	Item    string `json:"item,omitempty"`
	Message string `json:"message"`
	Count   int    `json:"count,omitempty"` // this many times, if more than once
	At      int64  `json:"at"`
}

// Task is a task as anyone outside this package sees it: a copy, safe to keep.
type Task struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Title        string    `json:"title"`
	State        State     `json:"state"`
	Phase        string    `json:"phase,omitempty"`
	Steps        []string  `json:"steps,omitempty"` // the phases it goes through, in order
	Step         int       `json:"step,omitempty"`  // the one it's on, from 1 (0: none), and then Percent is across them all
	LibraryID    int64     `json:"libraryId,omitempty"`
	LibraryName  string    `json:"libraryName,omitempty"`
	Item         string    `json:"item,omitempty"` // the thing being worked on, for admins
	Path         string    `json:"path,omitempty"` // where it is, for admins
	Done         int64     `json:"done"`
	Total        int64     `json:"total"` // 0: not known (yet)
	Unit         string    `json:"unit,omitempty"`
	Percent      float64   `json:"percent"`        // 0 to 100, or -1 when there's no telling
	Rate         float64   `json:"rate,omitempty"` // units per second, smoothed
	ETASec       int64     `json:"etaSec,omitempty"`
	StartedAt    int64     `json:"startedAt"`
	UpdatedAt    int64     `json:"updatedAt"`
	FinishedAt   int64     `json:"finishedAt,omitempty"`
	ByID         int64     `json:"byId,omitempty"`
	By           string    `json:"by,omitempty"`
	Source       string    `json:"source,omitempty"`       // the module reporting it; empty for the server
	Key          string    `json:"key,omitempty"`          // a module's task: "<module id>/<its key>"
	Result       string    `json:"result,omitempty"`       // what came of it, in a sentence
	Error        string    `json:"error,omitempty"`        // what went wrong, for a person
	Fix          string    `json:"fix,omitempty"`          // what they can do about it
	Detail       string    `json:"detail,omitempty"`       // the error as it came, for admins and reports
	ProblemCount int       `json:"problemCount,omitempty"` // every problem, including those not kept
	Problems     []Problem `json:"problems,omitempty"`

	CanPause  bool `json:"canPause"`
	CanResume bool `json:"canResume"`
	CanCancel bool `json:"canCancel"`
	CanRetry  bool `json:"canRetry"`

	// Private tasks are about one person's activity: only they and admins see them.
	Private bool `json:"private,omitempty"`
	// RetryKey is what a retry handler needs to start this again.
	RetryKey string `json:"-"`
	// AdminOnly: never shown to anyone else, whatever the settings say.
	AdminOnly bool `json:"-"`
}

// Spec describes a task when it starts.
type Spec struct {
	Type        string
	Title       string // no file paths: non-admins may read it
	Phase       string
	LibraryID   int64
	LibraryName string
	Unit        string // what Done and Total count: "files", "titles", "bytes"
	By          By
	Source      string // a module's id
	Key         string // a module's task: "<module id>/<its key>"

	// Cancel, when set, makes it cancellable. It should stop the job's context; the
	// job then ends with Finish(ctx.Err()).
	Cancel func()
	// Pausable: the job calls Wait between pieces of work.
	Pausable bool
	// Private: only the person who started it, and admins, can see it.
	Private bool
	// AdminOnly: only admins can see it (a module's report, a server update).
	AdminOnly bool
	// NoHistory leaves it out of the history when it ends. For things that come and
	// go constantly, such as someone's transcode.
	NoHistory bool
	// Quiet drops it from the history when it ends with nothing done and nothing
	// wrong: a background service that woke up and found nothing to do.
	Quiet bool
	// RetryKey lets a handler given to OnRetry start this again, even after a restart.
	RetryKey string
}

// RetryFunc starts a task again from what's left of it.
type RetryFunc func(old Task, by By) error

// Store keeps tasks across restarts. Begin is called when one starts, so a task
// that a crash cut short can be told about afterwards, and End when it finishes.
type Store interface {
	Begin(t Task)
	End(t Task)
}

const (
	maxProblems    = 50 // kept per task; the count goes on
	maxText        = 400
	maxLive        = 200
	recentFinished = 40 // kept in memory for the panel and for retries
	flushGap       = 500 * time.Millisecond
	rateEvery      = 2 * time.Second
)

// Registry holds every task.
type Registry struct {
	store Store
	now   func() time.Time
	boot  string

	mu       sync.Mutex
	seq      uint64
	live     map[string]*Handle
	order    []string // live ids, oldest first
	recent   []Task   // finished, newest last
	retries  map[string]RetryFunc
	subs     map[*Subscription]struct{}
	dirty    map[string]struct{}
	gone     map[string]struct{}
	flushing bool
	wake     chan struct{}
	writes   []func() // for the store, in order
	writing  bool
}

// New makes a registry. store may be nil.
func New(store Store) *Registry {
	return &Registry{
		store: store, now: time.Now, live: map[string]*Handle{}, retries: map[string]RetryFunc{},
		subs: map[*Subscription]struct{}{}, dirty: map[string]struct{}{}, gone: map[string]struct{}{},
		wake: make(chan struct{}, 1),
		boot: strconv.FormatInt(time.Now().UnixMilli()%1_000_000_000, 36),
	}
}

// OnRetry says how to start a kind of task again. Only tasks with a RetryKey can be.
func (r *Registry) OnRetry(taskType string, f RetryFunc) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retries[taskType] = f
}

func clip(s string) string {
	if len(s) <= maxText {
		return s
	}
	cut := maxText
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// Start makes a running task.
func (r *Registry) Start(s Spec) *Handle { return r.add(s, Running) }

// Queue makes a task that is waiting its turn. Begin moves it to running.
func (r *Registry) Queue(s Spec) *Handle { return r.add(s, Queued) }

func (r *Registry) add(s Spec, st State) *Handle {
	if r == nil {
		return nil
	}
	now := r.now()
	typ := s.Type
	if typ == "" {
		typ = TypeOther
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.live) >= maxLive {
		return nil // a runaway reporter costs itself its reports, not the server its memory
	}
	r.seq++
	id := "t" + r.boot + "-" + strconv.FormatUint(r.seq, 36)
	h := &Handle{
		reg: r, spec: s, started: now, resume: closedChan,
		t: Task{ID: id, Type: typ, Title: clip(s.Title), State: st, Phase: clip(s.Phase), LibraryID: s.LibraryID, LibraryName: clip(s.LibraryName), Unit: s.Unit,
			StartedAt: now.Unix(), UpdatedAt: now.Unix(), ByID: s.By.ID, By: clip(s.By.Name), Source: s.Source, Key: s.Key, Private: s.Private, AdminOnly: s.AdminOnly, RetryKey: s.RetryKey},
	}
	r.live[id] = h
	r.order = append(r.order, id)
	r.touch(id)
	if r.store != nil && !s.NoHistory && !s.Quiet {
		view := h.view()
		r.toStore(func() { r.store.Begin(view) })
	}
	return h
}

// toStore hands a write to the store. A job may be holding the database itself when it
// reports, and must never wait on it here, so one goroutine does the writes for as long
// as there are any. reg.mu is held.
func (r *Registry) toStore(f func()) {
	r.writes = append(r.writes, f)
	if r.writing {
		return
	}
	r.writing = true
	go func() {
		for {
			r.mu.Lock()
			if len(r.writes) == 0 {
				r.writing = false
				r.mu.Unlock()
				return
			}
			next := r.writes[0]
			r.writes = r.writes[1:]
			r.mu.Unlock()
			next()
		}
	}()
}

// Settle waits for what's on its way to the store. For tests, and an orderly shutdown.
func (r *Registry) Settle() {
	if r == nil {
		return
	}
	for {
		r.mu.Lock()
		idle := !r.writing
		r.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

var closedChan = func() chan struct{} { c := make(chan struct{}); close(c); return c }()

// Handle is a job's hold on its entry in the registry. Every method may be called
// from any goroutine, and on a nil Handle.
type Handle struct {
	reg     *Registry
	spec    Spec
	started time.Time
	t       Task // guarded by reg.mu
	resume  chan struct{}
	pausing bool
	ended   bool

	rateAt   time.Time
	rateDone int64
	rate     float64

	shares []float64 // each step's part of the whole, guarded by reg.mu
}

// ID is the task's id, or "" on a nil handle.
func (h *Handle) ID() string {
	if h == nil {
		return ""
	}
	return h.t.ID
}

// update runs f on the task and tells anyone watching, unless it had already ended.
func (h *Handle) update(f func(t *Task)) {
	if h == nil {
		return
	}
	h.reg.mu.Lock()
	defer h.reg.mu.Unlock()
	if h.ended {
		return
	}
	f(&h.t)
	h.t.UpdatedAt = h.reg.now().Unix()
	h.reg.touch(h.t.ID)
}

// Begin moves a queued task to running.
func (h *Handle) Begin() {
	h.update(func(t *Task) {
		if t.State == Queued {
			t.State = Running
		}
	})
}

// Title changes what it's called. No file paths: non-admins may read it.
func (h *Handle) Title(s string) { h.update(func(t *Task) { t.Title = clip(s) }) }

// Phase says which stage it's at: "Finding files", "Reading details". A phase named in
// Steps moves the task on to it, and counts its own Progress from nothing.
func (h *Handle) Phase(s string) {
	h.update(func(t *Task) {
		t.Phase = clip(s)
		if i := slices.Index(t.Steps, t.Phase); i >= 0 && i+1 != t.Step {
			t.Step = i + 1
			t.Done, t.Total = 0, 0
			h.rateAt, h.rate = time.Time{}, 0
		}
	})
}

// Step is one of the phases a job names with Steps, and its rough part of the work, on any
// scale. 0 counts as 1.
type Step struct {
	Name  string
	Share float64
}

// Steps names the phases a job goes through, in order, so the app shows one bar across them
// with each phase named, rather than a bar that starts again at each. A phase that isn't
// among them, such as waiting for a turn, leaves the bar where it was.
func (h *Handle) Steps(steps ...Step) {
	if h == nil {
		return
	}
	names, shares := make([]string, len(steps)), make([]float64, len(steps))
	for i, st := range steps {
		names[i], shares[i] = clip(st.Name), st.Share
		if shares[i] <= 0 {
			shares[i] = 1
		}
	}
	h.update(func(t *Task) {
		t.Steps, h.shares = names, shares
		t.Step = slices.Index(names, t.Phase) + 1
	})
}

// Library says which library this is about.
func (h *Handle) Library(id int64, name string) {
	h.update(func(t *Task) { t.LibraryID, t.LibraryName = id, clip(name) })
}

// Item says what it's working on now: a name, and optionally where that is. Only
// admins see these.
func (h *Handle) Item(name, path string) {
	h.update(func(t *Task) { t.Item, t.Path = clip(name), clip(path) })
}

// Progress says how far along it is. total 0 means not known.
func (h *Handle) Progress(done, total int64) {
	if h == nil {
		return
	}
	h.update(func(t *Task) {
		t.Done, t.Total = done, total
		h.noteRate(done)
	})
}

// Add counts more done, for work done in parallel.
func (h *Handle) Add(n int64) {
	if h == nil {
		return
	}
	h.update(func(t *Task) {
		t.Done += n
		h.noteRate(t.Done)
	})
}

// Unit says what Progress counts.
func (h *Handle) Unit(s string) { h.update(func(t *Task) { t.Unit = s }) }

// Result says in a sentence what came of it, for the history.
func (h *Handle) Result(s string) { h.update(func(t *Task) { t.Result = clip(s) }) }

// noteRate keeps a smoothed speed, looking at the clock at most every couple of seconds.
func (h *Handle) noteRate(done int64) {
	now := h.reg.now()
	if h.rateAt.IsZero() {
		h.rateAt, h.rateDone = now, done
		return
	}
	dt := now.Sub(h.rateAt)
	if dt < rateEvery {
		return
	}
	got := float64(done-h.rateDone) / dt.Seconds()
	if got < 0 {
		got = 0
	}
	if h.rate == 0 {
		h.rate = got
	} else {
		h.rate = h.rate*0.7 + got*0.3
	}
	h.rateAt, h.rateDone = now, done
}

// Problem notes something that went wrong with one thing, while the task carries on.
// The message is for a person, and should say what to do where that isn't obvious.
func (h *Handle) Problem(item, message string) { h.ProblemN(item, message, 1) }

// ProblemN is Problem for something that happened n times.
func (h *Handle) ProblemN(item, message string, n int) {
	h.update(func(t *Task) {
		t.ProblemCount += n
		if len(t.Problems) >= maxProblems {
			return
		}
		p := Problem{Item: clip(item), Message: clip(message), At: h.reg.now().Unix()}
		if n > 1 {
			p.Count = n
		}
		t.Problems = append(t.Problems, p)
	})
}

// Wait holds a job here while its task is paused. It returns ctx's error once that
// is done, so it's also the check for having been cancelled.
func (h *Handle) Wait(ctx context.Context) error {
	if h == nil {
		return ctx.Err()
	}
	h.reg.mu.Lock()
	if !h.pausing {
		h.reg.mu.Unlock()
		return ctx.Err()
	}
	if h.t.State == Running || h.t.State == Queued {
		h.t.State = Paused
		h.t.UpdatedAt = h.reg.now().Unix()
		h.reg.touch(h.t.ID)
	}
	resume := h.resume
	h.reg.mu.Unlock()
	select {
	case <-resume:
	case <-ctx.Done():
	}
	return ctx.Err()
}

// PauseWanted reports whether someone asked for this to pause, for a job that wants
// to let go of something (its turn at the disk) before calling Wait.
func (h *Handle) PauseWanted() bool {
	if h == nil {
		return false
	}
	h.reg.mu.Lock()
	defer h.reg.mu.Unlock()
	return h.pausing
}

// Finish ends the task according to how the job ended: nil is done, a context's
// cancellation is cancelled, and anything else failed, explained with Explain.
// Calling it again changes nothing.
func (h *Handle) Finish(err error) {
	switch {
	case err == nil:
		h.end(Done, "", "", "")
	case errors.Is(err, context.Canceled):
		h.end(Cancelled, "", "", "")
	default:
		msg, fix := Explain(err)
		h.end(Failed, msg, fix, err.Error())
	}
}

// Fail ends the task as failed, with words of the job's own.
func (h *Handle) Fail(message, fix string, cause error) {
	detail := ""
	if cause != nil {
		detail = cause.Error()
	}
	h.end(Failed, message, fix, detail)
}

func (h *Handle) end(st State, msg, fix, detail string) {
	if h == nil {
		return
	}
	r := h.reg
	r.mu.Lock()
	defer r.mu.Unlock()
	if h.ended {
		return
	}
	h.ended = true
	now := r.now()
	t := &h.t
	t.State, t.Error, t.Fix, t.Detail = st, clip(msg), clip(fix), clip(detail)
	t.UpdatedAt, t.FinishedAt = now.Unix(), now.Unix()
	if st == Done && t.Total > 0 {
		t.Done = t.Total
	}
	t.Item, t.Path, t.Phase = "", "", ""
	if h.pausing {
		h.pausing = false
		close(h.resume)
	}
	delete(r.live, t.ID)
	for i, id := range r.order {
		if id == t.ID {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	quiet := h.spec.Quiet && st == Done && t.Done == 0 && t.ProblemCount == 0 && t.Result == ""
	if !quiet && !h.spec.NoHistory {
		r.recent = append(r.recent, h.view())
		if len(r.recent) > recentFinished {
			r.recent = append(r.recent[:0], r.recent[len(r.recent)-recentFinished:]...)
		}
		if r.store != nil {
			view := h.view()
			r.toStore(func() { r.store.End(view) })
		}
	}
	if len(r.subs) > 0 && (quiet || h.spec.NoHistory) {
		r.gone[t.ID] = struct{}{}
	}
	r.touch(t.ID)
}

// view is the public copy of the task. reg.mu is held.
func (h *Handle) view() Task {
	t := h.t
	t.Problems = append([]Problem(nil), h.t.Problems...)
	t.Percent = -1
	if t.Total > 0 {
		t.Percent = min(100, float64(t.Done)*100/float64(t.Total))
	}
	if t.Step > 0 && t.Step <= len(h.shares) {
		t.Percent = h.across(t)
	}
	if t.State == Done {
		t.Percent = 100 // even when its last stage counted nothing
	}
	if t.State.Finished() {
		return t
	}
	t.Rate = h.rate
	if t.Total > t.Done && h.rate > 0 {
		t.ETASec = int64(float64(t.Total-t.Done) / h.rate)
	}
	t.CanCancel = h.spec.Cancel != nil
	t.CanPause = h.spec.Pausable && !h.pausing
	t.CanResume = h.spec.Pausable && h.pausing
	return t
}

// across is how far along the whole job is, from the steps behind it and the one it's on.
func (h *Handle) across(t Task) float64 {
	var before, all float64
	for i, w := range h.shares {
		if i < t.Step-1 {
			before += w
		}
		all += w
	}
	if t.Total > 0 {
		before += h.shares[t.Step-1] * min(1, float64(t.Done)/float64(t.Total))
	}
	return min(100, before*100/all)
}

// ---------------------------------------------------------------- controls

// Errors from the controls below.
var (
	ErrNotFound = errors.New("that task isn't known; it may have finished")
	ErrCant     = errors.New("that can't be done to this task")
)

// Pause asks a task to stop at its next step. It stays "running" until the job gets
// there, which is rarely long.
func (r *Registry) Pause(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.live[id]
	if h == nil {
		return ErrNotFound
	}
	if !h.spec.Pausable {
		return ErrCant
	}
	if !h.pausing {
		h.pausing = true
		h.resume = make(chan struct{})
		r.touch(id)
	}
	return nil
}

// Resume lets a paused task carry on.
func (r *Registry) Resume(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.live[id]
	if h == nil {
		return ErrNotFound
	}
	if !h.spec.Pausable {
		return ErrCant
	}
	if h.pausing {
		h.pausing = false
		close(h.resume)
		if h.t.State == Paused {
			h.t.State = Running
		}
		h.t.UpdatedAt = r.now().Unix()
		r.touch(id)
	}
	return nil
}

// Cancel asks a task to stop. It ends when the job notices, moments later.
func (r *Registry) Cancel(id string) error {
	r.mu.Lock()
	h := r.live[id]
	if h == nil {
		r.mu.Unlock()
		return ErrNotFound
	}
	cancel := h.spec.Cancel
	if cancel == nil {
		r.mu.Unlock()
		return ErrCant
	}
	h.t.Phase = "Stopping"
	r.touch(id)
	r.mu.Unlock()
	cancel() // outside the lock: it's the job's own code
	return nil
}

// Retry starts a finished task again, by the handler for its type.
func (r *Registry) Retry(old Task, by By) error {
	r.mu.Lock()
	f := r.retries[old.Type]
	r.mu.Unlock()
	if f == nil || old.RetryKey == "" || !old.State.Finished() || old.State == Done {
		return ErrCant
	}
	return f(old, by)
}

// CanRetry reports whether a finished task could be started again.
func (r *Registry) CanRetry(t Task) bool {
	if r == nil || t.RetryKey == "" || (t.State != Failed && t.State != Cancelled) {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.retries[t.Type] != nil
}

// ---------------------------------------------------------------- reading

// Live lists what's going on, oldest first.
func (r *Registry) Live() []Task {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Task, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.live[id].view())
	}
	return out
}

// Recent lists what finished lately, newest first. The rest is in the store.
func (r *Registry) Recent() []Task {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Task, 0, len(r.recent))
	for i := len(r.recent) - 1; i >= 0; i-- {
		t := r.recent[i]
		t.CanRetry = t.RetryKey != "" && (t.State == Failed || t.State == Cancelled) && r.retries[t.Type] != nil
		out = append(out, t)
	}
	return out
}

// Find returns a task that's live or finished lately.
func (r *Registry) Find(id string) (Task, bool) {
	if r == nil {
		return Task{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if h := r.live[id]; h != nil {
		return h.view(), true
	}
	for i := len(r.recent) - 1; i >= 0; i-- {
		if r.recent[i].ID == id {
			return r.recent[i], true
		}
	}
	return Task{}, false
}

// ForgetRecent drops what's held in memory about finished tasks: the history was cleared.
func (r *Registry) ForgetRecent(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.recent[:0]
	for _, t := range r.recent {
		if id != "" && t.ID != id {
			kept = append(kept, t)
		}
	}
	r.recent = kept
}

// ---------------------------------------------------------------- watching

// Update is what changed. Tasks holds changed tasks whole, including finished ones;
// Gone lists tasks to forget.
type Update struct {
	Tasks []Task
	Gone  []string
}

// Subscription is someone watching. Next gives each change once, coalesced: a slow
// watcher gets the latest state rather than a backlog.
type Subscription struct {
	reg   *Registry
	wake  chan struct{}
	tasks map[string]Task // guarded by reg.mu
	gone  map[string]struct{}
}

// Subscribe starts watching. Close it when done.
func (r *Registry) Subscribe() *Subscription {
	s := &Subscription{reg: r, wake: make(chan struct{}, 1), tasks: map[string]Task{}, gone: map[string]struct{}{}}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs[s] = struct{}{}
	if !r.flushing {
		r.flushing = true
		go r.flusher()
	}
	return s
}

// Watchers says how many subscriptions are open.
func (r *Registry) Watchers() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.subs)
}

// Close stops watching. When the last one goes, the flusher stops too.
func (s *Subscription) Close() {
	r := s.reg
	r.mu.Lock()
	delete(r.subs, s)
	r.mu.Unlock()
	r.poke()
}

// C says something may be waiting for Take.
func (s *Subscription) C() <-chan struct{} { return s.wake }

// Take returns what changed since the last Take.
func (s *Subscription) Take() Update {
	r := s.reg
	r.mu.Lock()
	defer r.mu.Unlock()
	u := Update{}
	for _, t := range s.tasks {
		u.Tasks = append(u.Tasks, t)
	}
	for id := range s.gone {
		u.Gone = append(u.Gone, id)
	}
	clear(s.tasks)
	clear(s.gone)
	return u
}

// touch notes a change. reg.mu is held. Without watchers it's nothing.
func (r *Registry) touch(id string) {
	if len(r.subs) == 0 {
		return
	}
	r.dirty[id] = struct{}{}
	r.poke()
}

func (r *Registry) poke() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// flusher hands changes to the watchers, at most twice a second, for as long as there
// are any. It starts with the first and ends with the last.
func (r *Registry) flusher() {
	for {
		<-r.wake
		r.mu.Lock()
		if len(r.subs) == 0 {
			clear(r.dirty)
			clear(r.gone)
			r.flushing = false
			r.mu.Unlock()
			return
		}
		r.deliver()
		r.mu.Unlock()
		time.Sleep(flushGap)
	}
}

// deliver hands out what's dirty. reg.mu is held.
func (r *Registry) deliver() {
	if len(r.dirty) == 0 && len(r.gone) == 0 {
		return
	}
	finished := map[string]Task{}
	for _, t := range r.recent {
		finished[t.ID] = t
	}
	for id := range r.dirty {
		var t Task
		if h := r.live[id]; h != nil {
			t = h.view()
		} else if f, ok := finished[id]; ok {
			t = f
			t.CanRetry = t.RetryKey != "" && (t.State == Failed || t.State == Cancelled) && r.retries[t.Type] != nil
		} else {
			r.gone[id] = struct{}{}
			continue
		}
		for s := range r.subs {
			s.tasks[id] = t
		}
	}
	for id := range r.gone {
		for s := range r.subs {
			delete(s.tasks, id)
			s.gone[id] = struct{}{}
		}
	}
	clear(r.dirty)
	clear(r.gone)
	for s := range r.subs {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// ---------------------------------------------------------------- who sees what

// Viewer is who's asking.
type Viewer struct {
	Admin  bool
	UserID int64
	// Library says whether they may see a library. Nil means none.
	Library func(id int64) bool
}

// For returns a task as this viewer may see it, and whether they may see it at all.
// A non-admin never gets a file name or path, anyone else's name, the words of an
// error (they often hold paths), or any control.
func (v Viewer) For(t Task) (Task, bool) {
	if v.Admin {
		return t, true
	}
	if t.AdminOnly || (t.Private && t.ByID != v.UserID) {
		return Task{}, false
	}
	if t.LibraryID != 0 && (v.Library == nil || !v.Library(t.LibraryID)) {
		return Task{}, false
	}
	t.Item, t.Path, t.Detail, t.Problems, t.Fix = "", "", "", nil, ""
	if t.ByID != v.UserID {
		t.ByID, t.By = 0, ""
	}
	if t.Error != "" {
		t.Error = "This didn't finish. An administrator can see why."
	}
	t.Result = ""
	t.CanPause, t.CanResume, t.CanCancel, t.CanRetry = false, false, false, false
	return t, true
}

// Summary is the state of things in a few numbers, for the button.
type Summary struct {
	Active  int     `json:"active"` // running or paused
	Queued  int     `json:"queued"`
	Paused  int     `json:"paused"`
	Percent float64 `json:"percent"` // across what can be measured, or -1
}

// Summarize counts up a list of tasks. Each task weighs the same: a scan of ten files
// and one of ten thousand each move the ring by their share of the way.
func Summarize(list []Task) Summary {
	s := Summary{Percent: -1}
	sum, n := 0.0, 0
	for _, t := range list {
		switch t.State {
		case Running:
			s.Active++
		case Paused:
			s.Active++
			s.Paused++
		case Queued:
			s.Queued++
		default:
			continue
		}
		if t.Percent >= 0 {
			sum += t.Percent
			n++
		}
	}
	if n > 0 {
		s.Percent = sum / float64(n)
	}
	return s
}
