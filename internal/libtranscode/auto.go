package libtranscode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Automatic runs: a library that's asked for it has its new files converted by itself, after
// a scan finds them. What's in the library from before it was turned on is left to a person.
//
// There's no timer of its own and no goroutine with nothing to do. A scan ending, or the server
// starting, looks for new files. Only if there are any does something stay around, watching
// them until they've stopped changing and work is allowed, handing them to an ordinary run
// with its ordinary limits, and then going away.
//
// What's been dealt with is remembered against the file's size and time, in ltc_memo, whatever
// the outcome. So a file that fails is never tried again until it changes, or the recipe does,
// or someone asks. A run a person cancels counts as an answer too.

// noRoomDetail marks a failure that says nothing against the file, so it may be tried again.
const noRoomDetail = "no-room"

// How long a new file is watched before giving up on it for now. The next scan looks again.
const autoGiveUp = 24 * time.Hour

// autoWatch is one library's watcher.
type autoWatch struct {
	cancel context.CancelFunc
	// again: a scan has ended since this last looked.
	again chan struct{}
}

// ScanFinished is told when a scan of a library has ended.
func (e *Engine) ScanFinished(libraryID int64) {
	e.init()
	ctx := context.Background()
	if r := LoadRule(ctx, e.DB, libraryID); !r.Auto || !r.Saved {
		return
	}
	e.kickAuto(libraryID)
}

// KickAllAuto looks for new files in every library that wants them converted, for what was
// waiting when the server stopped.
func (e *Engine) KickAllAuto(ctx context.Context) {
	e.init()
	rows, err := e.DB.QueryContext(ctx, `SELECT library_id FROM ltc_rules WHERE auto <> 0`)
	if err != nil {
		e.Log.WarnContext(ctx, "could not list libraries with automatic conversion", "err", err)
		return
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		e.kickAuto(id)
	}
}

// StopAuto ends every watcher, for shutting down. What they were waiting for is looked at
// again next time.
func (e *Engine) StopAuto() {
	e.init()
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, w := range e.auto {
		w.cancel()
	}
}

func (e *Engine) kickAuto(libraryID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if w := e.auto[libraryID]; w != nil {
		select {
		case w.again <- struct{}{}:
		default:
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &autoWatch{cancel: cancel, again: make(chan struct{}, 1)}
	e.auto[libraryID] = w
	logging.Go("automatic conversion", func() {
		defer func() {
			e.mu.Lock()
			delete(e.auto, libraryID)
			e.mu.Unlock()
			cancel()
		}()
		e.watchLibrary(ctx, libraryID, w)
	}, "cat", "transcode")
}

// fileSig says what a file looks like right now.
type fileSig struct {
	size  int64
	mtime int64
}

// itemSig is the sizes and times of all an item's files, which are several for a split film.
func itemSig(path string) ([]fileSig, error) {
	var out []fileSig
	for _, p := range library.StackedParts(path) {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		out = append(out, fileSig{st.Size(), st.ModTime().UnixNano()})
	}
	return out, nil
}

func sameSig(a, b []fileSig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// autoCandidates lists what's new since this was turned on, and hasn't been dealt with.
func (e *Engine) autoCandidates(ctx context.Context, libraryID int64, r Rule) ([]library.ConversionItem, error) {
	items, err := e.Store.ConversionItemsSince(ctx, libraryID, r.AutoSince)
	if err != nil {
		return nil, err
	}
	out := items[:0]
	for _, it := range items {
		st, err := os.Stat(it.Path)
		if err != nil {
			continue // the next scan will say what became of it
		}
		if e.recall(ctx, it.Path, r.Policy, st.Size(), st.ModTime().Unix()) == "" {
			out = append(out, it)
		}
	}
	return out, nil
}

func (e *Engine) settle(s Settings) time.Duration {
	if e.AutoSettle > 0 {
		return e.AutoSettle
	}
	return time.Duration(s.SettleSeconds) * time.Second
}

// autoTick is how often a watcher looks: often enough for what it's waiting for, and only
// while it has something to wait for.
func autoTick(settle time.Duration) time.Duration {
	return min(max(settle/4, 20*time.Millisecond), 15*time.Second)
}

type observed struct {
	sig   []fileSig
	since time.Time
	first time.Time
}

// watchLibrary watches a library's new files until there are none left to wait for.
func (e *Engine) watchLibrary(ctx context.Context, libraryID int64, w *autoWatch) {
	seen := map[int64]*observed{}
	// tried holds what a run has been given. Whatever it left undone (someone was playing
	// it, or the drive was full) waits for the next scan, so this can't go round in circles.
	tried := map[int64]bool{}
	var t *tasks.Handle
	endTask := func(err error) {
		if t != nil {
			t.Finish(err)
			t = nil
		}
	}
	defer func() { endTask(nil) }()
	cancelled := false
	for {
		select {
		case <-w.again:
			clear(tried)
		default:
		}
		s := LoadSettings(ctx, e.DB)
		settle := e.settle(s)
		wait := func() bool {
			select {
			case <-time.After(autoTick(settle)):
				return true
			case <-w.again:
				clear(tried)
				return true
			case <-ctx.Done():
				return false
			}
		}
		// A run of this library is going: what it finds is its own.
		if e.libraryBusy(libraryID) {
			endTask(nil)
			if !wait() {
				return
			}
			continue
		}
		r := LoadRule(ctx, e.DB, libraryID)
		lib, err := e.Store.Library(ctx, libraryID)
		if err != nil || !r.Auto || !r.Saved || e.Blocked(ctx, lib, r) != "" {
			return // the page says why, if it's something someone needs to know
		}
		list, err := e.autoCandidates(ctx, libraryID, r)
		if err != nil {
			if ctx.Err() == nil {
				e.Log.WarnContext(ctx, "could not look for new files to convert", "library", lib.Name, "err", err)
			}
			return
		}
		now := e.now()
		var ready []int64
		var changing []library.ConversionItem
		for _, it := range list {
			if tried[it.ID] {
				continue
			}
			sig, err := itemSig(it.Path)
			if err != nil {
				delete(seen, it.ID)
				continue
			}
			o := seen[it.ID]
			switch {
			case o == nil:
				seen[it.ID] = &observed{sig: sig, since: now, first: now}
				changing = append(changing, it)
			case !sameSig(o.sig, sig):
				o.sig, o.since = sig, now
				changing = append(changing, it)
			case now.Sub(o.since) >= settle:
				ready = append(ready, it.ID)
			case now.Sub(o.first) >= autoGiveUp:
				tried[it.ID] = true
			default:
				changing = append(changing, it)
			}
		}
		if len(ready) == 0 && len(changing) == 0 {
			return
		}
		hold := e.holdWords(s)
		if t == nil {
			watchCtx := w.cancel
			t = e.Tasks.Start(tasks.Spec{Type: TaskType, Title: "New files in " + lib.Name, LibraryID: lib.ID, LibraryName: lib.Name, Unit: "files",
				Cancel: func() { cancelled = true; watchCtx() }, Quiet: true})
		}
		switch {
		case hold != "":
			t.Phase(hold)
		case len(changing) > 0 && len(ready) == 0:
			t.Phase(fmt.Sprintf("Waiting for %s to stop changing", countFiles(len(changing))))
		}
		if len(changing) > 0 {
			t.Item(filepath.Base(changing[0].Path), changing[0].Path)
		}
		if len(ready) > 0 && hold == "" {
			endTask(nil)
			q := RunRequest{Library: libraryID, Policy: r.Policy, Output: r.Output, Items: ready, Auto: true}
			if _, err := e.Start(ctx, q); err != nil {
				if !errors.Is(err, errBusy) {
					e.Log.WarnContext(ctx, "could not start converting new files", "library", lib.Name, "err", err)
					for _, id := range ready {
						tried[id] = true
					}
				}
			} else {
				for _, id := range ready {
					tried[id] = true
					delete(seen, id)
				}
			}
		}
		if !wait() {
			if cancelled {
				// Someone said no to what was waiting. That stands until a file changes.
				bg := context.WithoutCancel(ctx)
				if list, err := e.autoCandidates(bg, libraryID, r); err == nil {
					for _, it := range list {
						if st, err := os.Stat(it.Path); err == nil {
							e.remember(bg, it.Path, r.Policy, st.Size(), st.ModTime().Unix(), "cancelled")
						}
					}
				}
				endTask(context.Canceled)
			}
			return
		}
	}
}

func countFiles(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

// memoAuto writes down what an automatic run came to, item by item, against what each item's
// file is now. What didn't get an answer isn't written, and a later scan looks at it again.
func (e *Engine) memoAuto(ctx context.Context, run int64, p Policy, state string) {
	rows, err := e.DB.QueryContext(ctx, `SELECT item_id, state, detail FROM ltc_files WHERE run_id = ? ORDER BY item_id, part`, run)
	if err != nil {
		e.Log.WarnContext(ctx, "could not read what a conversion came to", "run", run, "err", err)
		return
	}
	type outcome struct {
		item  int64
		state string
		open  bool
	}
	var list []*outcome
	for rows.Next() {
		var item int64
		var st, detail string
		if rows.Scan(&item, &st, &detail) != nil {
			continue
		}
		if len(list) == 0 || list[len(list)-1].item != item {
			list = append(list, &outcome{item: item, state: st})
		}
		o := list[len(list)-1]
		switch st {
		case FilePending, FileRunning, FilePostponed, FileChanged:
			o.open = true
		case FileFailed:
			o.state = FileFailed
			if detail == noRoomDetail {
				o.open = true
			}
		case FileDone:
			if o.state != FileFailed {
				o.state = FileDone
			}
		}
	}
	rows.Close()
	for _, o := range list {
		outcome := o.state
		if o.open {
			if state != "cancelled" {
				continue
			}
			outcome = "cancelled"
		}
		it, err := e.Store.Item(ctx, o.item)
		if err != nil {
			continue
		}
		st, err := os.Stat(it.Path)
		if err != nil {
			continue
		}
		e.remember(ctx, it.Path, p, st.Size(), st.ModTime().Unix(), outcome)
	}
}
