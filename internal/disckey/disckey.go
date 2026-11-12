// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package disckey remembers what was worked out about a disc for as long as the disc on
// disk is the same one: a small cache keyed by where the disc is, how large it is and when
// it last changed. A disc's analysis is asked for again and again (the scan, its page, each
// viewer's session), and costs a helper process each time it isn't remembered.
package disckey

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// indexFiles are what change when a disc folder's contents do. A folder's own time moves
// only when entries come and go, so the files a disc is read from are looked at as well.
var indexFiles = []string{"VIDEO_TS/VIDEO_TS.IFO", "VIDEO_TS.IFO", "BDMV/index.bdmv", "index.bdmv", "BDMV/MovieObject.bdmv", "BDMV/PLAYLIST", "PLAYLIST"}

// Of names the disc at path as it is now: its place, size and time, and for a folder those
// of the files its structure is read from. extra tells apart answers that differ for the
// same disc, such as the version of the module that gave them. ok is false when the disc
// can't be looked at, and nothing should be remembered for it.
func Of(path, extra string) (key string, ok bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%d|%d|%s", abs, st.Size(), st.ModTime().UnixNano(), extra)
	if st.IsDir() {
		for _, name := range indexFiles {
			if fst, err := os.Stat(filepath.Join(abs, filepath.FromSlash(name))); err == nil {
				fmt.Fprintf(&b, "|%s:%d:%d", name, fst.Size(), fst.ModTime().UnixNano())
			}
		}
		// A folder whose index is spelled in small letters, on a file system that minds.
		if entries, err := os.ReadDir(abs); err == nil && len(entries) <= 64 {
			for _, e := range entries {
				if info, err := e.Info(); err == nil {
					fmt.Fprintf(&b, "|%s:%d", e.Name(), info.ModTime().UnixNano())
				}
			}
		}
	}
	return b.String(), true
}

// Cache holds the latest answers, at most Max of them. The oldest goes first: a library's
// discs are looked at in runs, and one that is being watched is asked about by the same few
// requests.
type Cache[T any] struct {
	Max int

	mu    sync.Mutex
	items map[string]T
	order []string
}

// Get returns what is remembered under key.
func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.items[key]
	return v, ok
}

// Put remembers v under key, forgetting the oldest when there are too many.
func (c *Cache[T]) Put(key string, v T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[string]T{}
	}
	if _, ok := c.items[key]; !ok {
		c.order = append(c.order, key)
	}
	c.items[key] = v
	most := c.Max
	if most <= 0 {
		most = 16
	}
	for len(c.order) > most {
		delete(c.items, c.order[0])
		c.order = c.order[1:]
	}
}

// Clear forgets everything.
func (c *Cache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items, c.order = nil, nil
}

// Len is how many answers are remembered.
func (c *Cache[T]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
