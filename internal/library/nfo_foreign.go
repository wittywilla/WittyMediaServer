// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package library

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Other programs' .nfo files (a downloader's, a media centre's) are read unless a library says
// not to: some write a release's file name as the title and the day the file came out as the
// year. The server's own NFOs are read either way. And an administrator can clear a library's
// folders of every .nfo and .metadata.json file at once, to start again from the files' names
// and what is found online.

// SetLibraryForeignNFO says whether this library reads .nfo files other programs wrote.
func (s *Store) SetLibraryForeignNFO(ctx context.Context, id int64, on bool) error {
	v := 0
	if on {
		v = 1
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE libraries SET foreign_nfo = ? WHERE id = ?`, v, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// nfoUsable: this .nfo may be read for an item of lib. The server's own always may.
func nfoUsable(lib *Library, path string) bool {
	return lib == nil || lib.ForeignNFO || nfoIsOurs(path)
}

// usableNFOs is nfos without those lib doesn't read.
func usableNFOs(lib *Library, nfos []string) []string {
	if lib == nil || lib.ForeignNFO {
		return nfos
	}
	out := nfos[:0:0]
	for _, p := range nfos {
		if nfoIsOurs(p) {
			out = append(out, p)
		}
	}
	return out
}

// isMetadataFile: a file the purge removes. Media, artwork and subtitles are never touched.
func isMetadataFile(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".nfo") || strings.HasSuffix(n, ".metadata.json") || n == "metadata.json"
}

// PurgeMetadataFiles deletes every .nfo and .metadata.json file in a library's folders, and says
// how many went. A read-only library is refused, as for anything that would change its files.
func (s *Store) PurgeMetadataFiles(ctx context.Context, lib *Library) (int, error) {
	if lib.ReadOnly {
		return 0, ErrReadOnly
	}
	n := 0
	for _, root := range lib.Paths {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // a folder that can't be read is left as it is
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if d.Type().IsRegular() && isMetadataFile(d.Name()) {
				if os.Remove(p) == nil {
					n++
				}
			}
			return nil
		})
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// titleLibrary is the library a film, episode or show is in, nil when it can't be read.
func (s *Scanner) titleLibrary(ctx context.Context, t *Title) *Library {
	var id int64
	switch {
	case t.Item != nil:
		id = t.Item.LibraryID
	case t.Show != nil:
		id = t.Show.LibraryID
	default:
		return nil
	}
	lib, err := s.Store.Library(ctx, id)
	if err != nil {
		return nil
	}
	return lib
}

// RereadLibrary makes the next scan read every file of a library again, as if each had changed:
// what was read from files no longer there, or no longer used, goes. What people chose by hand
// stays, as it does for any file that changed.
func (s *Store) RereadLibrary(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET mtime = 0 WHERE library_id = ?`, id)
	return err
}
