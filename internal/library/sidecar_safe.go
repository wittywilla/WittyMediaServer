package library

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/metafolder"
)

// Files beside media can be edited by anyone who can write to a library folder, and come
// with downloads from anywhere. So everything read from one goes through here: size
// limited, parsed without believing any of it, cleaned to plain text of a sensible
// length, and a problem says which file and which line.

const maxNFOFile = metafolder.MaxNFO

func warnSidecar(path string, data []byte, err error) {
	line, msg := metafolder.JSONErrorLine(data, err)
	slog.Default().Warn("could not read a metadata file; ignoring it", "cat", "library", "file", path, "line", line, "err", msg)
}

// readSidecarXML reads an NFO into v. Tools often leave a web address after the end of
// the document, so it's cut at the last closing tag given.
func readSidecarXML(path, closing string, v any) bool {
	return readXMLCut(path, closing, false, v)
}

// readFirstSidecarXML is the same, cut at the first closing tag: a file for several
// episodes holds one document after another.
func readFirstSidecarXML(path, closing string, v any) bool {
	return readXMLCut(path, closing, true, v)
}

func readXMLCut(path, closing string, first bool, v any) bool {
	b, err := metafolder.ReadSidecar(path, maxNFOFile)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Default().Warn("could not read an NFO file; ignoring it", "cat", "library", "file", path, "err", err)
		}
		return false
	}
	at := bytes.LastIndex(b, []byte(closing))
	if first {
		at = bytes.Index(b, []byte(closing))
	}
	if at > 0 {
		b = b[:at+len(closing)]
	}
	if line, msg := metafolder.DecodeXML(b, v); msg != "" {
		slog.Default().Warn("could not read an NFO file; ignoring it", "cat", "library", "file", path, "line", line, "err", msg)
		return false
	}
	return true
}

func readSidecarJSON(path string, max int64, v any) bool {
	b, err := metafolder.ReadSidecar(path, max)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Default().Warn("could not read a metadata file; ignoring it", "cat", "library", "file", path, "err", err)
		}
		return false
	}
	if err := json.Unmarshal(b, v); err != nil {
		warnSidecar(path, b, err)
		return false
	}
	return true
}

func validYear(y int) bool { return y >= 1800 && y <= 3000 }

// firstWebAddress is the first thing in a list that's really a web address.
func firstWebAddress(list []string) string {
	for _, t := range list {
		if u, ok := metafolder.URL(t); ok {
			return u
		}
	}
	return ""
}

func cleanRef(s string) string {
	if u, ok := metafolder.ProviderID(s); ok {
		return u
	}
	return ""
}

func (m *nfoMovie) clean() {
	m.Title = metafolder.Line(m.Title, 300)
	m.SortTitle = metafolder.Line(m.SortTitle, 300)
	m.Plot = metafolder.Text(m.Plot, 20000)
	m.Outline = metafolder.Text(m.Outline, 20000)
	m.Tagline = metafolder.Line(m.Tagline, 500)
	m.Genres = metafolder.List(m.Genres, 30, 60)
	m.MPAA = metafolder.Line(m.MPAA, 20)
	m.Studio = metafolder.Line(m.Studio, 120)
	m.Premiered = metafolder.Line(m.Premiered, 20)
	m.TMDBID = cleanRef(m.TMDBID)
	if !validYear(m.Year) {
		m.Year = 0
	}
	if m.Rating < 0 || m.Rating > 10 || m.Rating != m.Rating {
		m.Rating = 0
	}
	if m.Runtime < 0 || m.Runtime > 100000 {
		m.Runtime = 0
	}
	for i := range m.UniqueIDs {
		m.UniqueIDs[i].ID = cleanRef(m.UniqueIDs[i].ID)
	}
	for i := range m.Ratings.Rating {
		if v := m.Ratings.Rating[i].Value; v < 0 || v > 10 || v != v {
			m.Ratings.Rating[i].Value = 0
		}
	}
}

func (m *sidecarJSON) clean() {
	m.Title = metafolder.Line(m.Title, 300)
	m.SortTitle = metafolder.Line(m.SortTitle, 300)
	if m.Year != nil && !validYear(*m.Year) {
		m.Year = nil
	}
	if m.Overview != nil {
		o := metafolder.Text(*m.Overview, 20000)
		m.Overview = &o
	}
	m.Tagline = metafolder.Line(m.Tagline, 500)
	m.Genres = metafolder.List(m.Genres, 30, 60)
	if m.Rating != nil && (*m.Rating < 0 || *m.Rating > 10 || *m.Rating != *m.Rating) {
		m.Rating = nil
	}
	if m.Provider != nil {
		p := strings.ToLower(metafolder.Line(*m.Provider, 40))
		m.Provider = &p
	}
	if m.ProviderID != nil {
		id := cleanRef(*m.ProviderID)
		m.ProviderID = &id
	}
}

func (m *nfoShow) clean() {
	m.Title = metafolder.Line(m.Title, 300)
	m.Plot = metafolder.Text(m.Plot, 20000)
	m.Genres = metafolder.List(m.Genres, 30, 60)
	m.MPAA = metafolder.Line(m.MPAA, 20)
	m.Studio = metafolder.Line(m.Studio, 120)
	m.Premiered = metafolder.Line(m.Premiered, 20)
	m.Status = metafolder.Line(m.Status, 40)
	m.TVDBID = cleanRef(m.TVDBID)
	if !validYear(m.Year) {
		m.Year = 0
	}
	if m.Rating < 0 || m.Rating > 10 || m.Rating != m.Rating {
		m.Rating = 0
	}
	for i := range m.UniqueIDs {
		m.UniqueIDs[i].ID = cleanRef(m.UniqueIDs[i].ID)
	}
}

func (m *nfoEpisode) clean() {
	m.Title = metafolder.Line(m.Title, 300)
	m.Plot = metafolder.Text(m.Plot, 20000)
	m.Aired = metafolder.Line(m.Aired, 20)
	if m.Rating < 0 || m.Rating > 10 || m.Rating != m.Rating {
		m.Rating = 0
	}
	if m.Season != nil && (*m.Season < 0 || *m.Season > 9999) {
		m.Season = nil
	}
	if m.Episode != nil && (*m.Episode < 0 || *m.Episode > 99999) {
		m.Episode = nil
	}
}

func (n *musicVideoNFO) clean() {
	n.Title = metafolder.Line(n.Title, 300)
	n.Artist = metafolder.Line(n.Artist, 300)
	n.Album = metafolder.Line(n.Album, 300)
	n.Plot = metafolder.Text(n.Plot, 20000)
	n.Genres = metafolder.List(n.Genres, 30, 60)
	n.Tags = metafolder.List(n.Tags, 50, 60)
	if !validYear(n.Year) {
		n.Year = 0
	}
}

func (c *cdSidecar) clean() {
	c.Title = metafolder.Line(c.Title, 300)
	c.Artist = metafolder.Line(c.Artist, 300)
	c.ReleaseID = metafolder.Line(c.ReleaseID, 80)
	if c.Year != nil && !validYear(*c.Year) {
		c.Year = nil
	}
	for i := range c.Tracks {
		c.Tracks[i].Title = metafolder.Line(c.Tracks[i].Title, 300)
		c.Tracks[i].Artist = metafolder.Line(c.Tracks[i].Artist, 300)
	}
}

func (v *videoSidecar) clean() {
	line := func(ps ...*string) {
		for _, p := range ps {
			*p = metafolder.Line(*p, 500)
		}
	}
	line(&v.Title, &v.FullTitle, &v.Uploader, &v.UploaderID, &v.Channel, &v.ChannelID, &v.Creator, &v.Artist, &v.Album, &v.Handle, &v.Platform, &v.ExtractorKey, &v.Extractor, &v.UploadDate, &v.Date, &v.ContentRating, &v.Playlist, &v.PlaylistTitle, &v.ID, &v.Type)
	v.Description = metafolder.Text(v.Description, 20000)
	v.Overview = metafolder.Text(v.Overview, 20000)
	addr := func(ps ...*string) {
		for _, p := range ps {
			u, ok := metafolder.URL(*p)
			if !ok {
				u = ""
			}
			*p = u
		}
	}
	addr(&v.UploaderURL, &v.ChannelURL, &v.URL, &v.WebpageURL)
	v.Tags = metafolder.List(v.Tags, 200, 100)
	v.Categories = metafolder.List(v.Categories, 50, 100)
	v.Genres = metafolder.List(v.Genres, 30, 60)
	for i := range v.Comments {
		v.Comments[i].Author = metafolder.Line(v.Comments[i].Author, 200)
		v.Comments[i].Text = metafolder.Text(v.Comments[i].Text, 10000)
	}
	v.Custom = cleanRawCustom(v.Custom)
}

// cleanRawCustom unpacks free-form details only when they are small enough to be kept at all:
// decoded, a few megabytes of "{}," take gigabytes of memory.
func cleanRawCustom(raw sideCustom) sideCustom {
	if len(raw) == 0 || len(raw) > 16<<10 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return nil
	}
	b, err := json.Marshal(cleanCustom(m, 0))
	if err != nil {
		return nil
	}
	return b
}

// cleanCustom cleans free-form extra details, however they nest, up to a point.
func cleanCustom(m map[string]any, depth int) map[string]any {
	if m == nil || depth > 6 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[metafolder.Line(k, 100)] = cleanAny(v, depth+1)
	}
	return out
}

func cleanAny(v any, depth int) any {
	switch x := v.(type) {
	case string:
		return metafolder.Text(x, 20000)
	case map[string]any:
		return cleanCustom(x, depth)
	case []any:
		if depth > 6 {
			return nil
		}
		out := make([]any, len(x))
		for i := range x {
			out[i] = cleanAny(x[i], depth+1)
		}
		return out
	}
	return v
}

// cleanBook cleans what a book said about itself: an EPUB's details, a comic's
// ComicInfo.xml, an archive's metadata file. Those come from inside a download.
func cleanBook(it *Item) {
	if t := metafolder.Line(it.Title, 300); t != "" {
		it.Title = t
	}
	it.Overview = metafolder.Text(it.Overview, 20000)
	it.Artist = metafolder.Line(it.Artist, 300)
	it.AlbumArtist = metafolder.Line(it.AlbumArtist, 300)
	it.Album = metafolder.Line(it.Album, 300)
	it.Genres = metafolder.List(it.Genres, 30, 60)
}
