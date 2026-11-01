package library

import (
	"context"
	"encoding/xml"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ebook"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
)

// Playlist files in a music library become collections of the songs they name. Only
// songs in the library count. A playlist is a list of names, so a web address, or a
// path leading out of the library, simply finds nothing: a playlist can't make the
// server fetch or read anything.

const playlistKind = "playlist"

const (
	maxPlaylistFile    = 16 << 20
	maxPlaylistEntries = 100000
)

// ParseEntries reads the entries of a playlist file, in order, as written.
func ParseEntries(format string, b []byte) []string {
	text := ebook.DecodeText(b)
	switch format {
	case "m3u":
		return parseM3U(text)
	case "pls":
		return parsePLS(text)
	case "xspf":
		return parseXSPF(text)
	case "asx":
		return parseASX(text)
	}
	return nil
}

func parseM3U(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\xEF\xBB\xBF"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
		if len(out) >= maxPlaylistEntries {
			break
		}
	}
	return out
}

func parsePLS(text string) []string {
	type entry struct {
		n    int
		file string
	}
	var entries []entry
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || len(k) < 5 || !strings.EqualFold(k[:4], "file") {
			continue
		}
		if n, err := strconv.Atoi(k[4:]); err == nil && n > 0 && len(entries) < maxPlaylistEntries {
			entries = append(entries, entry{n, strings.TrimSpace(v)})
		}
	}
	// Numbered, usually in order.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].n < entries[j-1].n && i < 1<<14; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.file
	}
	return out
}

func parseXSPF(text string) []string {
	d := xml.NewDecoder(strings.NewReader(text))
	d.Strict = false
	var out []string
	inLocation := false
	for len(out) < maxPlaylistEntries {
		tok, err := d.Token()
		if err == io.EOF || err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			inLocation = t.Name.Local == "location"
		case xml.EndElement:
			inLocation = false
		case xml.CharData:
			if inLocation {
				if v := strings.TrimSpace(string(t)); v != "" {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

// parseASX reads ASX and WPL: an element with a reference in an attribute.
func parseASX(text string) []string {
	d := xml.NewDecoder(strings.NewReader(text))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	var out []string
	for len(out) < maxPlaylistEntries {
		tok, err := d.Token()
		if err != nil {
			break
		}
		if t, ok := tok.(xml.StartElement); ok {
			switch strings.ToLower(t.Name.Local) {
			case "ref", "media":
				for _, a := range t.Attr {
					if k := strings.ToLower(a.Name.Local); k == "href" || k == "src" {
						out = append(out, strings.TrimSpace(a.Value))
					}
				}
			}
		}
	}
	return out
}

// playlistTarget resolves an entry to a path, relative to the playlist's folder or
// given in full. A web address has no path here.
func playlistTarget(dir, entry string) string {
	entry = strings.TrimSpace(entry)
	if strings.HasPrefix(strings.ToLower(entry), "file://") {
		if u, err := url.Parse(entry); err == nil {
			entry = strings.TrimPrefix(filepath.FromSlash(u.Path), string(filepath.Separator)+string(filepath.Separator))
			if len(entry) > 2 && entry[0] == filepath.Separator && entry[2] == ':' {
				entry = entry[1:] // file:///C:/Music
			}
		}
	} else if strings.Contains(entry, "://") {
		return ""
	}
	entry = strings.ReplaceAll(entry, `\`, string(filepath.Separator))
	if !filepath.IsAbs(entry) {
		entry = filepath.Join(dir, entry)
	}
	return filepath.Clean(entry)
}

// importPlaylists makes a collection of each playlist file, or brings it up to date.
// They belong to the first administrator and are shared, like the songs. A file that
// finds no songs makes nothing.
func (s *Scanner) importPlaylists(ctx context.Context, lib *Library, files []candidate) {
	if len(files) == 0 {
		return
	}
	var owner int64
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE is_admin = 1 ORDER BY id LIMIT 1`).Scan(&owner); err != nil {
		return // nobody to give them to yet: the next scan will
	}
	for _, c := range files {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := s.importPlaylist(ctx, lib, c, owner); err != nil {
			s.minorFailure(ctx, "could not read a playlist", err, "file", c.path)
		}
	}
}

func (s *Scanner) importPlaylist(ctx context.Context, lib *Library, c candidate, owner int64) error {
	f := mediatypes.Find(c.path, mediatypes.Playlist)
	if f == nil || c.size > maxPlaylistFile {
		return nil
	}
	raw, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(c.path)
	var ids []int64
	seen := map[int64]bool{}
	for _, e := range ParseEntries(f.ID, raw) {
		p := playlistTarget(dir, e)
		if p == "" {
			continue
		}
		it, err := s.Store.itemByPath(ctx, lib.ID, p)
		if err == nil && !seen[it.ID] {
			seen[it.ID] = true
			ids = append(ids, it.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	name := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	// Its file's path in the description says which file it came from: the same name
	// in another folder is another playlist.
	from := "Playlist file: " + c.path
	var id int64
	switch err := s.Store.DB.QueryRowContext(ctx, `SELECT id FROM collections WHERE owner_id = ? AND description = ? LIMIT 1`, owner, from).Scan(&id); err {
	case nil:
		if _, err := s.Store.DB.ExecContext(ctx, `DELETE FROM collection_items WHERE collection_id = ?`, id); err != nil {
			return err
		}
	default:
		col := &Collection{OwnerID: owner, Name: name, Description: from, Kind: "manual", Shared: true}
		if err := s.Store.SaveCollection(ctx, col); err != nil {
			return err
		}
		id = col.ID
	}
	// Lists keep the order of their dates: one apart keeps the file's order.
	now := db.Now()
	for i, x := range ids {
		if _, err := s.Store.DB.ExecContext(ctx, `INSERT OR IGNORE INTO collection_items(collection_id, item_id, added_at) VALUES(?,?,?)`, id, x, now+int64(i)); err != nil {
			return err
		}
	}
	return nil
}
