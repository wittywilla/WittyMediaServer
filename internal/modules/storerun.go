// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymodule"
)

// A run is a list of modules to change, one after another: everything Update All does, and an
// install that has to bring what it's built on. One failure doesn't stop the rest, and the
// summary stays until the next run, including across a restart.

// What a module in a run is doing.
const (
	RunQueued  = "queued"
	RunWorking = "working"
	RunDone    = "done"
	RunFailed  = "failed"
	RunSkipped = "skipped"
	RunUndone  = "undone"
)

// Kinds of run.
const (
	RunUpdateAll = "update-all"
	RunInstall   = "install"
)

const (
	runFile     = "last-run.json"
	runPoll     = 300 * time.Millisecond
	runItemWait = 31 * time.Minute // longer than an install's own limit
)

// RunItem is one module in a run.
type RunItem struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	From    string   `json:"from,omitempty"`
	To      string   `json:"to,omitempty"`
	State   string   `json:"state"`
	Why     string   `json:"why,omitempty"` // why it was skipped, or what it's here for
	Phase   string   `json:"phase,omitempty"`
	Done    int64    `json:"done,omitempty"`
	Total   int64    `json:"total,omitempty"`
	Failure *Failure `json:"failure,omitempty"`
	// OlderServer: To was made for an older server (MadeFor), because nothing made for this one
	// is known. Warning says so; the run installs it only when the request said confirmOlder.
	OlderServer bool   `json:"olderServer,omitempty"`
	MadeFor     string `json:"madeFor,omitempty"`
	Warning     string `json:"warning,omitempty"`
}

// Run is a whole run, as the page shows it.
type Run struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	By       string    `json:"by,omitempty"`
	Started  int64     `json:"started"`
	Finished int64     `json:"finished,omitempty"`
	Items    []RunItem `json:"items"`
	Updated  int       `json:"updated"`
	Failed   int       `json:"failed"`
	Skipped  int       `json:"skipped"`
	CanUndo  bool      `json:"canUndo,omitempty"`
	Undone   bool      `json:"undone,omitempty"`

	confirmOlder bool // the request confirmed the versions made for an older server in it
}

func (r *Run) copy() *Run {
	if r == nil {
		return nil
	}
	c := *r
	c.Items = append([]RunItem(nil), r.Items...)
	return &c
}

func (s *Stores) runPath() string { return filepath.Join(s.cfg.Dir, runFile) }

func (s *Stores) loadRun() {
	b, err := os.ReadFile(s.runPath())
	if err != nil {
		return
	}
	var r Run
	if json.Unmarshal(b, &r) == nil && r.ID != "" {
		for i := range r.Items { // a run the server stopped in the middle of says so
			if st := r.Items[i].State; st == RunQueued || st == RunWorking {
				r.Items[i].State, r.Items[i].Why = RunSkipped, "The server stopped before this was done."
			}
		}
		if r.Finished == 0 {
			r.Finished = time.Now().UnixMilli()
		}
		s.last = &r
	}
}

// CurrentRun is the run going on now, or the last one to finish.
func (s *Stores) CurrentRun() *Run {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.current != nil {
		return s.current.copy()
	}
	return s.last.copy()
}

// DismissRun forgets the last run's summary.
func (s *Stores) DismissRun() {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.last = nil
	_ = os.Remove(s.runPath())
}

// orderByNeeds puts what a module is built on before the module, and otherwise keeps names in
// order. A loop, which a catalogue could claim, is only broken: it can't hang anything.
func orderByNeeds(ids []string, requires func(id string) []string) []string {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	seen := map[string]int{} // 1 while visiting, 2 once placed
	out := make([]string, 0, len(ids))
	var visit func(id string)
	visit = func(id string) {
		if seen[id] != 0 {
			return
		}
		seen[id] = 1
		deps := append([]string(nil), requires(id)...)
		sort.Strings(deps)
		for _, d := range deps {
			if want[d] {
				visit(d)
			}
		}
		seen[id] = 2
		out = append(out, id)
	}
	for _, id := range sorted {
		visit(id)
	}
	return out
}

// UpdatePlan says what Update All would do now.
func (s *Stores) updatePlan() []RunItem {
	mods := s.m.Report().Modules
	byID := make(map[string]Status, len(mods))
	var ids []string
	for _, st := range mods {
		byID[st.ID] = st
		if st.Installed && st.Latest != nil {
			ids = append(ids, st.ID)
		}
	}
	var items []RunItem
	for _, id := range orderByNeeds(ids, func(id string) []string { return byID[id].Requires }) {
		st := byID[id]
		it := RunItem{ID: id, Name: st.Name, From: st.Version, To: st.Latest.Version, State: RunQueued}
		switch {
		case st.Pin != "":
			it.State, it.Why = RunSkipped, "It's pinned to this version on purpose."
		case st.Source != nil:
			it.State, it.Why = RunSkipped, "It's from another publisher, so you choose each of its versions on its own page."
		case st.Latest.Blocked != "":
			it.State, it.Why = RunSkipped, "This version was taken out of use before ("+st.Latest.Blocked+"), so it won't come back unless you choose it."
		case st.Phase == PhaseDownloading || st.Phase == PhaseVerifying || st.Phase == PhaseInstalling:
			it.State, it.Why = RunSkipped, "It's already being changed."
		}
		items = append(items, it)
	}
	return items
}

// UpdateCount is how many modules Update All would change now.
func (s *Stores) UpdateCount() int {
	n := 0
	for _, it := range s.updatePlan() {
		if it.State == RunQueued {
			n++
		}
	}
	return n
}

// ErrRunBusy: one run at a time.
var ErrRunBusy = errors.New("modules are already being changed; wait for that to finish")

// UpdateAll starts updating every module with an update, in order of what's built on what.
func (s *Stores) UpdateAll(by string) (*Run, error) {
	items := s.updatePlan()
	any := false
	for _, it := range items {
		any = any || it.State == RunQueued
	}
	if !any {
		return nil, errors.New("there's nothing to update right now")
	}
	return s.start(RunUpdateAll, by, items, false)
}

// AutoUpdate is what the owner's "Install updates automatically" runs: everything Update All would
// do, in the same order, except what has been told to wait for an admin.
func (s *Stores) AutoUpdate() {
	auto := map[string]bool{}
	for _, st := range s.m.Report().Modules {
		auto[st.ID] = st.AutoUpdate
	}
	var items []RunItem
	for _, it := range s.updatePlan() {
		if it.State == RunQueued && auto[it.ID] {
			items = append(items, it)
		}
	}
	if len(items) == 0 {
		return
	}
	if _, err := s.start(RunUpdateAll, "automatic updates", items, false); err != nil {
		s.log.Info("modules: automatic updates wait for the next check", "err", err)
	}
}

// InstallPlan says what installing a module would bring with it: everything it's built on that
// isn't here yet, first, then the module. A module with no version that fits is named with why,
// so nothing starts that can't finish.
func (s *Stores) InstallPlan(id string) ([]RunItem, error) {
	mods := s.m.Report().Modules
	byID := make(map[string]Status, len(mods))
	for _, st := range mods {
		byID[st.ID] = st
	}
	if _, ok := byID[id]; !ok {
		return nil, ErrUnknown
	}
	// What it needs, however far down that goes.
	need := map[string]bool{}
	var walk func(x string, depth int) error
	walk = func(x string, depth int) error {
		if need[x] || depth > 32 {
			return nil
		}
		st, ok := byID[x]
		if !ok {
			return fmt.Errorf("%s is built on %s, and this server doesn't know of that module", byID[id].Name, x)
		}
		if st.Installed && x != id {
			return nil
		}
		// A library has nothing to install: it reaches this server inside what is built on it.
		// With no release listed, one can't be told from a module with none yet, and the module
		// built on it is what will say so if that matters.
		if x != id && st.Latest == nil && st.OlderOffer == nil && st.Updatable {
			return nil
		}
		if x != id && st.Delivery == wittymodule.DeliveryLibrary {
			return nil
		}
		need[x] = true
		for _, d := range st.Requires {
			if err := walk(d, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(id, 0); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(need))
	for x := range need {
		ids = append(ids, x)
	}
	var items []RunItem
	for _, x := range orderByNeeds(ids, func(i string) []string { return byID[i].Requires }) {
		st := byID[x]
		it := RunItem{ID: x, Name: st.Name, State: RunQueued, From: st.Version}
		if !st.Installed {
			it.From = ""
		}
		if st.Latest != nil {
			it.To = st.Latest.Version
		} else if o := st.OlderOffer; o != nil && !st.Installed {
			// Only a version made for an older server: in the plan with its warning, and
			// installed only if the request confirms it.
			it.To, it.OlderServer, it.MadeFor, it.Warning = o.Version, true, o.MadeFor, o.Reason
		}
		if x != id {
			it.Why = byID[id].Name + " is built on it."
		}
		items = append(items, it)
	}
	return items, nil
}

// InstallWithNeeds starts installing a module and what it's built on. A plan with a version made
// for an older server in it is refused (*ErrOlderServer); InstallWithNeedsOlder can confirm it.
func (s *Stores) InstallWithNeeds(id, by string) (*Run, error) {
	return s.InstallWithNeedsOlder(id, by, false)
}

// OlderInPlan lists the versions in a plan that were made for an older server, as refusals that
// name each one.
func (s *Stores) OlderInPlan(plan []RunItem) []*ErrOlderServer {
	var out []*ErrOlderServer
	for _, it := range plan {
		if it.OlderServer {
			out = append(out, newErrOlderServer(it.ID, it.Name, it.To, it.MadeFor, s.m.cfg.Server))
		}
	}
	return out
}

// InstallWithNeedsOlder is InstallWithNeeds that installs versions made for an older server in
// the plan when confirmOlder says someone was shown each of them and said go ahead.
func (s *Stores) InstallWithNeedsOlder(id, by string, confirmOlder bool) (*Run, error) {
	items, err := s.InstallPlan(id)
	if err != nil {
		return nil, err
	}
	if older := s.OlderInPlan(items); len(older) > 0 && !confirmOlder {
		return nil, older[0]
	}
	return s.startWith(RunInstall, by, items, true, confirmOlder)
}

// start begins a run in the background. stopOnFail: a module whose dependency failed is skipped,
// which an install wants; Update All goes on regardless.
func (s *Stores) start(kind, by string, items []RunItem, stopOnFail bool) (*Run, error) {
	return s.startWith(kind, by, items, stopOnFail, false)
}

func (s *Stores) startWith(kind, by string, items []RunItem, stopOnFail, confirmOlder bool) (*Run, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.current != nil {
		return nil, ErrRunBusy
	}
	r := &Run{ID: logging.NewID("run"), Kind: kind, By: by, Started: time.Now().UnixMilli(), Items: items, confirmOlder: confirmOlder}
	s.current = r
	s.log.Info("modules: a run started", "run", r.ID, "kind", kind, "modules", len(items), "by", by)
	logging.Go("modules run", func() { s.work(r, stopOnFail) }, "cat", "modules", "session", r.ID)
	return r.copy(), nil
}

// setItem changes one item under the lock, and tells whoever is watching.
func (s *Stores) setItem(r *Run, i int, change func(*RunItem)) {
	s.runMu.Lock()
	change(&r.Items[i])
	c := r.copy()
	s.runMu.Unlock()
	s.notify(c)
}

// notify tells the activity panel, if there is one, how a run stands. It must not block.
func (s *Stores) notify(r *Run) {
	if s.Watch != nil {
		s.Watch(r)
	}
}

func (s *Stores) work(r *Run, stopOnFail bool) {
	failed := map[string]bool{}
	for i := range r.Items {
		s.runMu.Lock()
		it := r.Items[i]
		s.runMu.Unlock()
		if it.State != RunQueued {
			continue
		}
		if stopOnFail && len(failed) > 0 {
			s.setItem(r, i, func(x *RunItem) { x.State, x.Why = RunSkipped, "Something it's built on didn't install." })
			continue
		}
		s.setItem(r, i, func(x *RunItem) { x.State = RunWorking })
		failure := s.runOne(r, i, it)
		s.setItem(r, i, func(x *RunItem) {
			x.Phase, x.Done, x.Total = "", 0, 0
			if failure != nil {
				x.State, x.Failure = RunFailed, failure
			} else {
				x.State = RunDone
			}
		})
		if failure != nil {
			failed[it.ID] = true
			s.log.Warn("modules: a run's module failed", "run", r.ID, "module", it.ID, "kind", failure.Kind, "detail", failure.Detail)
		}
	}
	s.finish(r)
}

// runOne does one item and waits for it, following its progress.
func (s *Stores) runOne(r *Run, i int, it RunItem) *Failure {
	var err error
	st, ok := s.statusOf(it.ID)
	switch {
	case !ok:
		err = ErrUnknown
	case it.To != "":
		// A version made for an older server goes ahead only when this run's request confirmed it.
		err = s.m.InstallVersionOlder(it.ID, it.To, false, it.OlderServer && r.confirmOlder)
	case !st.Installed:
		err = s.m.AddModule(it.ID)
	default:
		err = errors.New("no version of it that fits this server is known")
	}
	if err != nil {
		var need *ErrNeedsConfirm
		if errors.As(err, &need) {
			s.setItem(r, i, func(x *RunItem) { x.Why = need.Warning })
			return &Failure{Kind: FailOther, Title: "This one needs your say", Message: need.Warning, Detail: need.Warning,
				Try: []string{"Open the module and choose this version yourself."}}
		}
		return explainErr(it.Name, err)
	}
	deadline := time.Now().Add(runItemWait)
	for time.Now().Before(deadline) {
		st, ok := s.statusOf(it.ID)
		if !ok {
			return explainErr(it.Name, ErrUnknown)
		}
		switch st.Phase {
		case PhaseDownloading, PhaseVerifying, PhaseInstalling:
			s.setItem(r, i, func(x *RunItem) { x.Phase, x.Done, x.Total = st.Phase, st.Done, st.Total })
		case PhaseFailed:
			return explainErr(it.Name, errors.New(st.Error))
		default:
			s.setItem(r, i, func(x *RunItem) { x.To = st.Version })
			return nil
		}
		time.Sleep(runPoll)
	}
	return explainErr(it.Name, context.DeadlineExceeded)
}

func (s *Stores) statusOf(id string) (Status, bool) {
	for _, st := range s.m.Report().Modules {
		if st.ID == id {
			return st, true
		}
	}
	return Status{}, false
}

func (s *Stores) finish(r *Run) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	r.Finished = time.Now().UnixMilli()
	r.Updated, r.Failed, r.Skipped = 0, 0, 0
	for i := range r.Items {
		switch r.Items[i].State {
		case RunDone:
			r.Updated++
		case RunFailed:
			r.Failed++
		case RunSkipped:
			r.Skipped++
		}
	}
	r.CanUndo = r.Updated > 0 && r.Kind == RunUpdateAll
	s.current, s.last = nil, r
	s.saveRun(r)
	s.log.Info("modules: a run finished", "run", r.ID, "updated", r.Updated, "failed", r.Failed, "skipped", r.Skipped)
	defer s.notify(r.copy())
}

// saveRun keeps the summary. Call with s.runMu held.
func (s *Stores) saveRun(r *Run) {
	if b, err := json.Marshal(r); err == nil {
		if err := writeAtomic(s.runPath(), b); err != nil {
			s.log.Warn("modules: keeping the summary of a run", "err", err)
		}
	}
}

// UndoRun takes the last Update All back: each module it moved goes back to what it was using,
// and the version it moved to isn't installed by itself again.
func (s *Stores) UndoRun(by string) (*Run, error) {
	s.runMu.Lock()
	r := s.last
	busy := s.current != nil
	can := r != nil && r.CanUndo
	if can && !busy {
		r.CanUndo = false // pressed twice, it only runs once
	}
	items := []RunItem(nil)
	if r != nil {
		items = append(items, r.Items...)
	}
	s.runMu.Unlock()
	switch {
	case busy:
		return nil, ErrRunBusy
	case !can:
		return nil, errors.New("there's nothing to take back")
	}
	// Newest dependents first: the reverse of how they went in.
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		if it.State != RunDone {
			continue
		}
		err := s.m.Rollback(it.ID)
		s.runMu.Lock()
		if err != nil {
			r.Items[i].Failure = explainErr(it.Name, err)
		} else {
			r.Items[i].State = RunUndone
		}
		s.runMu.Unlock()
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	r.Undone = true
	s.saveRun(r)
	s.log.Info("modules: a run was taken back", "run", r.ID, "by", by)
	return r.copy(), nil
}
