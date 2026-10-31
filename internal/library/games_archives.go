package library

import (
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
)

// What archives hold, remembered so a scan looks inside a file once and never again
// until it changes. Only headers are read, and that happens in a worker process.

// archiveInfo is what a scan needs from one archive.
type archiveInfo struct {
	size, mtime int64
	exts        []string
	problem     string
}

// archiveIndex lives for one scan of one library. Nil store: remembers nothing past it.
type archiveIndex struct {
	store     *Store
	libraryID int64
	once      sync.Once
	known     map[string]archiveInfo
	seen      map[string]archiveInfo
	fresh     map[string]bool
	lister    *gamearchive.Lister
}

func newArchiveIndex(store *Store, libraryID int64) *archiveIndex {
	return &archiveIndex{store: store, libraryID: libraryID, known: map[string]archiveInfo{}, seen: map[string]archiveInfo{}, fresh: map[string]bool{}}
}

func (ix *archiveIndex) close() {
	if ix.lister != nil {
		ix.lister.Close()
		ix.lister = nil
	}
}

func (ix *archiveIndex) load() {
	if ix.store == nil {
		return
	}
	rows, err := ix.store.DB.QueryContext(context.Background(), `SELECT path, size, mtime, exts, problem FROM game_archives WHERE library_id = ?`, ix.libraryID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var path, exts string
		var a archiveInfo
		if rows.Scan(&path, &a.size, &a.mtime, &exts, &a.problem) == nil && json.Unmarshal([]byte(exts), &a.exts) == nil {
			ix.known[path] = a
		}
	}
}

// info is what's known of an archive, reading its headers only if it's new or changed.
func (ix *archiveIndex) info(path string) archiveInfo {
	ix.once.Do(ix.load)
	st, err := os.Stat(path)
	if err != nil {
		return archiveInfo{problem: gamearchive.ErrUnavailable.Code}
	}
	size, mtime := st.Size(), st.ModTime().Unix()
	if a, ok := ix.seen[path]; ok && a.size == size && a.mtime == mtime {
		return a
	}
	if a, ok := ix.known[path]; ok && a.size == size && a.mtime == mtime {
		ix.seen[path] = a
		return a
	}
	if ix.lister == nil {
		ix.lister = gamearchive.NewLister()
	}
	a := archiveInfo{size: size, mtime: mtime}
	l, err := ix.lister.List(path, gamearchive.ScanBudget)
	if err != nil {
		if a.problem = gamearchive.CodeOf(err); a.problem == "" {
			a.problem = gamearchive.ErrDamaged.Code
		}
	} else {
		a.exts = l.Exts()
	}
	ix.seen[path] = a
	ix.fresh[path] = err == nil || !gamearchive.Transient(err)
	return a
}

func (ix *archiveIndex) exts(path string) []string { return ix.info(path).exts }

// problem says why an archive couldn't be read, if that's why it isn't a game.
func (ix *archiveIndex) problem(path string) string {
	if !gamearchive.IsArchive(path) {
		return ""
	}
	return ix.info(path).problem
}

// save remembers what this scan read. prune: every folder was reachable, so an
// archive that wasn't seen is gone.
func (ix *archiveIndex) save(ctx context.Context, prune bool) {
	if ix.store == nil {
		return
	}
	ix.once.Do(ix.load)
	tx, err := ix.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()
	for path, lasting := range ix.fresh {
		if !lasting {
			continue // a hiccup, not an answer: ask again next scan
		}
		a := ix.seen[path]
		exts := a.exts
		if exts == nil {
			exts = []string{}
		}
		b, _ := json.Marshal(exts)
		if _, err := tx.ExecContext(ctx, `INSERT INTO game_archives(library_id, path, size, mtime, exts, problem) VALUES(?,?,?,?,?,?)
			ON CONFLICT(library_id, path) DO UPDATE SET size = excluded.size, mtime = excluded.mtime, exts = excluded.exts, problem = excluded.problem`,
			ix.libraryID, path, a.size, a.mtime, string(b), a.problem); err != nil {
			return
		}
	}
	if prune {
		for path := range ix.known {
			if _, ok := ix.seen[path]; !ok {
				if _, err := tx.ExecContext(ctx, `DELETE FROM game_archives WHERE library_id = ? AND path = ?`, ix.libraryID, path); err != nil {
					return
				}
			}
		}
	}
	_ = tx.Commit()
}
