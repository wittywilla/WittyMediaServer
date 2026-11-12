// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// What a theme's zip holds is read once and kept, keyed by the file's size and time, so that
// resolving a theme for every page drawn (internal/server, SDUI scopes) costs a stat per
// theme rather than opening and parsing zips: a small machine draws many pages. A file
// changed or replaced is read again; a theme removed drops out with its file.

type memoEntry struct {
	size   int64
	mod    time.Time
	meta   *Meta
	tokens *Tokens
}

// themeMemo is shared by every Manager: it is keyed by the zip's path.
var themeMemo sync.Map // path → *memoEntry

// maxMemo bounds it: past this many themes, it starts again.
const maxMemo = 256

var memoCount struct {
	sync.Mutex
	n int
}

func memoFor(path string) (*memoEntry, os.FileInfo) {
	st, err := os.Stat(path)
	if err != nil {
		themeMemo.Delete(path)
		return nil, nil
	}
	if v, ok := themeMemo.Load(path); ok {
		e := v.(*memoEntry)
		if e.size == st.Size() && e.mod.Equal(st.ModTime()) {
			return e, st
		}
	}
	return nil, st
}

func memoStore(path string, st os.FileInfo, fill func(e *memoEntry)) {
	if st == nil {
		return
	}
	var e memoEntry
	if v, ok := themeMemo.Load(path); ok {
		if old := v.(*memoEntry); old.size == st.Size() && old.mod.Equal(st.ModTime()) {
			e = *old
		}
	}
	e.size, e.mod = st.Size(), st.ModTime()
	fill(&e)
	memoCount.Lock()
	if _, had := themeMemo.Load(path); !had {
		memoCount.n++
		if memoCount.n > maxMemo {
			themeMemo.Range(func(k, _ any) bool { themeMemo.Delete(k); return true })
			memoCount.n = 1
		}
	}
	memoCount.Unlock()
	themeMemo.Store(path, &e)
}

// copyOf deep-copies what is kept, so a caller can't change it for the next one.
func copyOf[T any](v *T) *T {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out T
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return &out
}
