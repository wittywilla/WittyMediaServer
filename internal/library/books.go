package library

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Books, comics and manga. Files that share a name in a folder are one
// book in several formats (downloads from digital libraries often come as
// CBZ + EPUB + PDF plus scans, OCR text and DRM-locked copies); the best
// readable format is used and the rest is ignored.

// Readable formats, best first: comics prefer page images, text prefers EPUB.
var bookExts = map[string]int{".cbz": 1, ".epub": 2, ".pdf": 3, ".cbr": 4, ".cb7": 5, ".mobi": 6, ".azw3": 7}

// Files from digital-library downloads that are not a readable book.
var junkBookRe = regexp.MustCompile(`(?i)(_lcp\.epub|_jp2\.zip|_djvu\.(txt|xml)|_hocr.*|_chocr.*|_scandata\.xml|_page_numbers\.json|_daisy.*|_text\.pdf)$`)

func discoverBooks(root string) ([]candidate, error) {
	type pick struct {
		c    candidate
		rank int
	}
	best := map[string]pick{} // folder + name without extension -> best format
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || strings.HasSuffix(strings.ToLower(name), "_daisy") || strings.EqualFold(name, "__MACOSX")) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		rank, ok := bookExts[ext]
		if !ok || junkBookRe.MatchString(name) || strings.HasPrefix(name, "._") {
			return nil
		}
		st, err := d.Info()
		if err != nil || !mediaEntry(root, p, d) {
			return nil
		}
		key := filepath.Join(filepath.Dir(p), strings.TrimSuffix(name, filepath.Ext(name)))
		if cur, ok := best[key]; !ok || rank < cur.rank {
			best[key] = pick{candidate{path: p, kind: "book", size: st.Size(), mtime: st.ModTime().Unix()}, rank}
		}
		return nil
	})
	out := make([]candidate, 0, len(best))
	for _, v := range best {
		out = append(out, v.c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, err
}

// "Series Name 145 (2023) (Digital) (group)", "Series v03", "Series #12"
var comicNameRe = regexp.MustCompile(`^(.*?)[\s._-]+(?:v(?:ol(?:ume)?)?\.?\s*|#|c(?:h(?:apter)?)?\.?\s*)?(\d{1,4}(?:\.\d+)?)\s*$`)
var parenRe = regexp.MustCompile(`\s*[\[(][^\])]*[\])]`)
var yearParenRe = regexp.MustCompile(`\((\d{4})\)`)

// parseComicName splits a comic file name into series, number and year.
func parseComicName(name string) (series string, number string, year *int) {
	if m := yearParenRe.FindStringSubmatch(name); m != nil {
		if y, err := strconv.Atoi(m[1]); err == nil && y > 1900 && y < 2100 {
			year = &y
		}
	}
	clean := strings.TrimSpace(parenRe.ReplaceAllString(name, ""))
	if m := comicNameRe.FindStringSubmatch(clean); m != nil {
		return strings.TrimSpace(strings.TrimRight(m[1], " -_.")), m[2], year
	}
	return clean, "", year
}

func (s *Scanner) indexBook(ctx context.Context, lib *Library, c candidate) error {
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	if err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size {
		if existing.Missing {
			return s.Store.restore(ctx, existing.ID)
		}
		return nil
	}
	it := &Item{LibraryID: lib.ID, Kind: "book", Path: c.path, SizeBytes: c.size, MTime: c.mtime}
	if existing != nil {
		it.ID, it.AddedAt = existing.ID, existing.AddedAt
		it.Reading = existing.Reading
	}
	ext := strings.ToLower(filepath.Ext(c.path))
	it.Container = strings.TrimPrefix(ext, ".")
	dir := filepath.Dir(c.path)
	stem := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	series, number, year := parseComicName(stem)
	it.Title, it.Year = series, year
	if number != "" {
		it.Album = series // the series
		if n, err := strconv.Atoi(strings.Split(number, ".")[0]); err == nil {
			it.Track = &n
		}
		it.Title = series + " " + number
	}
	// A folder named "... (Manga)" or "Manga" reads right to left.
	if it.Reading == "" && strings.Contains(strings.ToLower(filepath.Base(dir)), "manga") {
		it.Reading = "rtl"
	}
	var cover func(context.Context, string) error
	switch ext {
	case ".epub":
		cover = s.readEPUBMeta(it)
	case ".cbz":
		cover = s.readCBZMeta(it)
	}
	// Digital-library sidecars: <id>_meta.xml and __ia_thumb.jpg.
	if metas, _ := filepath.Glob(filepath.Join(dir, "*_meta.xml")); len(metas) == 1 {
		applyArchiveMeta(it, metas[0], number == "")
	}
	it.SortTitle = SortTitle(it.Title)
	if it.Poster == "" {
		h := sha1.Sum([]byte(c.path))
		name := "b-" + hex.EncodeToString(h[:10]) + ".jpg"
		out := filepath.Join(s.ImagesDir, name)
		if st, err := os.Stat(out); err == nil && st.Size() > 0 && existing != nil {
			it.Poster = "cache:" + name
		} else if cover != nil {
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if cover(cctx, out) == nil {
				it.Poster = "cache:" + name
			}
			cancel()
		}
	}
	if it.Poster == "" {
		it.Poster = findArt(dir, stem, "cover", "folder", "__ia_thumb")
	}
	s.applyAgents(ctx, it, "books")
	return s.Store.upsertItem(ctx, it)
}

const maxCoverPixels = "100000000"

// makeCover turns an image inside an archive into a 400 px cover.
func (s *Scanner) makeCover(ctx context.Context, archive, entry, out string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	f, err := zr.Open(entry)
	if err != nil {
		return err
	}
	defer f.Close()
	tmp, err := os.CreateTemp(s.ImagesDir, "cover-*"+path.Ext(entry))
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(f, 64<<20)); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	// Read as a picture whatever its contents claim to be, and refuse sizes no
	// cover has: a tiny file can claim 30000 x 30000 pixels (gigabytes of memory).
	return s.Tools.Still(ctx, append([]string{"-f", "image2", "-max_pixels", maxCoverPixels}, ffmpeg.FileInput(tmp.Name())...), 0, 400, out)
}

// ---------------------------------------------------------------- CBZ

type comicInfo struct {
	Title     string `xml:"Title"`
	Series    string `xml:"Series"`
	Number    string `xml:"Number"`
	Volume    string `xml:"Volume"`
	Summary   string `xml:"Summary"`
	Year      int    `xml:"Year"`
	Writer    string `xml:"Writer"`
	Genre     string `xml:"Genre"`
	Manga     string `xml:"Manga"`
	AgeRating string `xml:"AgeRating"`
}

// maxComicPages: sorting is the slowest part of opening a comic, and an archive
// can hold millions of (empty) entries.
const maxComicPages = 50000

// ComicPages lists the page images of a comic archive in reading order.
func ComicPages(zr *zip.Reader) []string {
	var pages []string
	for _, f := range zr.File {
		n := f.Name
		low := strings.ToLower(n)
		if f.FileInfo().IsDir() || strings.HasPrefix(low, "__macosx") || strings.Contains(low, "/.") || strings.HasPrefix(path.Base(low), ".") {
			continue
		}
		switch path.Ext(low) {
		case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif":
			pages = append(pages, n)
		}
		if len(pages) >= maxComicPages {
			break
		}
	}
	sort.Slice(pages, func(i, j int) bool { return naturalLess(pages[i], pages[j]) })
	return pages
}

// naturalLess compares strings with numbers in numeric order ("p2" < "p10").
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		ai, bi := digitsPrefix(a), digitsPrefix(b)
		if ai > 0 && bi > 0 {
			na, _ := strconv.Atoi(a[:ai])
			nb, _ := strconv.Atoi(b[:bi])
			if na != nb {
				return na < nb
			}
			a, b = a[ai:], b[bi:]
			continue
		}
		ca, cb := strings.ToLower(a[:1]), strings.ToLower(b[:1])
		if ca != cb {
			return ca < cb
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digitsPrefix(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i
}

var comicAges = map[string]string{"everyone": "G", "everyone 10+": "PG", "g": "G", "pg": "PG", "teen": "PG-13", "kids to adults": "PG",
	"mature 17+": "R", "m": "R", "ma15+": "MA15+", "adults only 18+": "18", "x18+": "18", "r18+": "18"}

func (s *Scanner) readCBZMeta(it *Item) func(context.Context, string) error {
	zr, err := zip.OpenReader(it.Path)
	if err != nil {
		return nil
	}
	defer zr.Close()
	pages := ComicPages(&zr.Reader)
	it.RuntimeSec = float64(len(pages)) // books: length in pages
	for _, f := range zr.File {
		if strings.EqualFold(path.Base(f.Name), "ComicInfo.xml") {
			if rc, err := f.Open(); err == nil {
				var ci comicInfo
				if xml.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&ci) == nil {
					if ci.Series != "" {
						it.Album = ci.Series
					}
					if n := num(firstNonEmpty(ci.Number, ci.Volume)); n != nil {
						it.Track = n
					}
					if ci.Title != "" {
						it.Title = ci.Title
					} else if ci.Series != "" && ci.Number != "" {
						it.Title = ci.Series + " " + ci.Number
					}
					if ci.Summary != "" {
						it.Overview = ci.Summary
					}
					if ci.Year > 0 {
						y := ci.Year
						it.Year = &y
					}
					if ci.Writer != "" {
						it.Artist, it.AlbumArtist = ci.Writer, ci.Writer
					}
					if ci.Genre != "" {
						it.Genres = strings.Split(ci.Genre, ",")
						for i := range it.Genres {
							it.Genres[i] = strings.TrimSpace(it.Genres[i])
						}
					}
					if strings.HasPrefix(strings.ToLower(ci.Manga), "yes") {
						it.Reading = "rtl"
					}
					if r := comicAges[strings.ToLower(strings.TrimSpace(ci.AgeRating))]; r != "" {
						it.ContentRating = r
					}
				}
				rc.Close()
			}
			break
		}
	}
	if len(pages) == 0 {
		return nil
	}
	first, archive := pages[0], it.Path
	return func(ctx context.Context, out string) error { return s.makeCover(ctx, archive, first, out) }
}

// ---------------------------------------------------------------- EPUB

type opfPackage struct {
	Metadata struct {
		Title       []string `xml:"title"`
		Creator     []string `xml:"creator"`
		Description string   `xml:"description"`
		Date        string   `xml:"date"`
		Subject     []string `xml:"subject"`
		Language    string   `xml:"language"`
		Meta        []struct {
			Name     string `xml:"name,attr"`
			Content  string `xml:"content,attr"`
			Property string `xml:"property,attr"`
			Value    string `xml:",chardata"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MediaType  string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine struct {
		Toc   string `xml:"toc,attr"`
		Items []struct {
			IDRef  string `xml:"idref,attr"`
			Linear string `xml:"linear,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

// EPUBInfo is what the reader needs: the chapters in order and the contents.
type EPUBInfo struct {
	Root   string     `json:"-"`
	Spine  []string   `json:"spine"` // paths inside the archive
	TOC    []TOCEntry `json:"toc"`
	Title  string     `json:"title"`
	Author string     `json:"author"`
	Cover  string     `json:"-"`
	opf    *opfPackage
}

type TOCEntry struct {
	Title string `json:"title"`
	Href  string `json:"href"`
}

// ReadEPUB parses an EPUB's package document and table of contents.
func ReadEPUB(zr *zip.Reader) (*EPUBInfo, error) {
	open := func(name string) ([]byte, error) {
		f, err := zr.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, 8<<20))
	}
	cont, err := open("META-INF/container.xml")
	if err != nil {
		return nil, errors.New("not an EPUB (no container.xml)")
	}
	var c struct {
		Rootfiles []struct {
			Path string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := xml.Unmarshal(cont, &c); err != nil || len(c.Rootfiles) == 0 {
		return nil, errors.New("not an EPUB (bad container.xml)")
	}
	opfPath := c.Rootfiles[0].Path
	raw, err := open(opfPath)
	if err != nil {
		return nil, err
	}
	var pkg opfPackage
	if err := xml.Unmarshal(raw, &pkg); err != nil {
		return nil, err
	}
	base := path.Dir(opfPath)
	resolve := func(href string) string {
		href = strings.SplitN(href, "#", 2)[0]
		if base == "." {
			return path.Clean(href)
		}
		return path.Clean(path.Join(base, href))
	}
	info := &EPUBInfo{Root: base, opf: &pkg}
	if len(pkg.Metadata.Title) > 0 {
		info.Title = strings.TrimSpace(pkg.Metadata.Title[0])
	}
	if len(pkg.Metadata.Creator) > 0 {
		info.Author = strings.TrimSpace(pkg.Metadata.Creator[0])
	}
	byID := map[string]string{}
	var navHref, coverID string
	for _, m := range pkg.Manifest {
		byID[m.ID] = m.Href
		if strings.Contains(m.Properties, "nav") {
			navHref = m.Href
		}
		if strings.Contains(m.Properties, "cover-image") {
			info.Cover = resolve(m.Href)
		}
	}
	for _, m := range pkg.Metadata.Meta {
		if m.Name == "cover" {
			coverID = m.Content
		}
	}
	if info.Cover == "" && coverID != "" && byID[coverID] != "" {
		info.Cover = resolve(byID[coverID])
	}
	for _, it := range pkg.Spine.Items {
		if h := byID[it.IDRef]; h != "" && it.Linear != "no" {
			info.Spine = append(info.Spine, resolve(h))
		}
	}
	// Contents: the EPUB 2 NCX (most books, EPUB 3 included, still ship one).
	if ncx := byID[pkg.Spine.Toc]; ncx != "" {
		if b, err := open(resolve(ncx)); err == nil {
			var doc struct {
				Points []ncxPoint `xml:"navMap>navPoint"`
			}
			if xml.Unmarshal(b, &doc) == nil {
				ncxBase := path.Dir(resolve(ncx))
				var walk func([]ncxPoint, int)
				walk = func(ps []ncxPoint, depth int) {
					for _, p := range ps {
						href := path.Clean(path.Join(ncxBase, p.Content.Src))
						if strings.Contains(p.Content.Src, "#") {
							href += "#" + strings.SplitN(p.Content.Src, "#", 2)[1]
						}
						info.TOC = append(info.TOC, TOCEntry{Title: strings.Repeat("  ", depth) + strings.TrimSpace(p.Label), Href: href})
						if depth < 2 {
							walk(p.Children, depth+1)
						}
					}
				}
				walk(doc.Points, 0)
			}
		}
	}
	_ = navHref // EPUB 3 nav documents are rendered by the reader's chapter list instead
	if len(info.Spine) == 0 {
		return nil, errors.New("this EPUB has no readable chapters")
	}
	return info, nil
}

type ncxPoint struct {
	Label   string `xml:"navLabel>text"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Children []ncxPoint `xml:"navPoint"`
}

func (s *Scanner) readEPUBMeta(it *Item) func(context.Context, string) error {
	zr, err := zip.OpenReader(it.Path)
	if err != nil {
		return nil
	}
	defer zr.Close()
	info, err := ReadEPUB(&zr.Reader)
	if err != nil {
		return nil
	}
	md := info.opf.Metadata
	if info.Title != "" {
		it.Title = info.Title
	}
	if info.Author != "" {
		it.Artist, it.AlbumArtist = info.Author, info.Author
	}
	if md.Description != "" {
		it.Overview = plain(md.Description)
	}
	if y := yearOf(md.Date); y != nil {
		it.Year = y
	}
	if len(md.Subject) > 0 {
		it.Genres = md.Subject
	}
	for _, m := range md.Meta {
		switch {
		case m.Name == "calibre:series" || m.Property == "belongs-to-collection":
			it.Album = firstNonEmpty(m.Content, m.Value)
		case m.Name == "calibre:series_index" || m.Property == "group-position":
			it.Track = num(firstNonEmpty(m.Content, m.Value))
		}
	}
	it.RuntimeSec = float64(len(info.Spine)) // books: length in chapters
	if info.Cover == "" {
		return nil
	}
	archive, entry := it.Path, info.Cover
	return func(ctx context.Context, out string) error { return s.makeCover(ctx, archive, entry, out) }
}

// applyArchiveMeta reads a digital-library *_meta.xml for title, author
// and description (titles only when the file name gave no number).
func applyArchiveMeta(it *Item, p string, title bool) {
	b, err := os.ReadFile(p)
	if err != nil || len(b) > 4<<20 {
		return
	}
	var m struct {
		Title       string   `xml:"title"`
		Creator     []string `xml:"creator"`
		Description string   `xml:"description"`
		Date        string   `xml:"date"`
		Subject     []string `xml:"subject"`
	}
	if xml.Unmarshal(b, &m) != nil {
		return
	}
	if title && m.Title != "" && it.Album == "" {
		it.Title = strings.TrimSpace(parenRe.ReplaceAllString(m.Title, ""))
	}
	if it.Artist == "" && len(m.Creator) > 0 {
		it.Artist, it.AlbumArtist = m.Creator[0], m.Creator[0]
	}
	if it.Overview == "" && m.Description != "" {
		it.Overview = plain(m.Description)
	}
	if it.Year == nil {
		it.Year = yearOf(m.Date)
	}
}

// ---------------------------------------------------------------- open archives

// Archives stay open briefly so a reader turning pages does not re-read the
// zip directory for every page. They close once nobody has read them for a
// while: an open file can't be renamed or deleted on Windows.
var archiveCache = struct {
	sync.Mutex
	m       map[string]*cachedArchive
	sweeper sync.Once
}{m: map[string]*cachedArchive{}}

const archiveIdle = 2 * time.Minute

type cachedArchive struct {
	zr      *zip.ReadCloser
	mtime   time.Time
	used    time.Time
	pages   []string
	readers int
	// Closed once the last reader is done, when the file changed meanwhile.
	stale bool
	// Parsed once: the contents and word counts of an EPUB (reading every
	// chapter to count its words takes a moment on a small machine).
	epub  *EPUBInfo
	words []int
}

func sweepArchives() {
	for range time.Tick(archiveIdle / 2) {
		archiveCache.Lock()
		for k, c := range archiveCache.m {
			if c.readers == 0 && time.Since(c.used) > archiveIdle {
				c.zr.Close()
				delete(archiveCache.m, k)
			}
		}
		archiveCache.Unlock()
	}
}

// OpenArchive returns an open archive (call the release func when done).
func OpenArchive(p string) (*zip.Reader, func(), error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, nil, err
	}
	archiveCache.sweeper.Do(func() { go sweepArchives() })
	archiveCache.Lock()
	defer archiveCache.Unlock()
	cur := archiveCache.m[p]
	if cur != nil && cur.mtime.Equal(st.ModTime()) {
		cur.used, cur.readers = time.Now(), cur.readers+1
		return &cur.zr.Reader, func() { release(cur) }, nil
	}
	if cur != nil { // the file was replaced
		delete(archiveCache.m, p)
		if cur.readers == 0 {
			cur.zr.Close()
		} else {
			cur.stale = true
		}
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, nil, err
	}
	// The file changed: close the copy of it kept open, once nobody reads it.
	if c := archiveCache.m[p]; c != nil {
		if c.readers == 0 {
			c.zr.Close()
		} else {
			c.stale = true
		}
		delete(archiveCache.m, p)
	}
	// Keep at most 8 open; close the least recently used idle one.
	if len(archiveCache.m) >= 8 {
		var oldK string
		var old *cachedArchive
		for k, c := range archiveCache.m {
			if c.readers == 0 && (old == nil || c.used.Before(old.used)) {
				oldK, old = k, c
			}
		}
		if old != nil {
			old.zr.Close()
			delete(archiveCache.m, oldK)
		}
	}
	c := &cachedArchive{zr: zr, mtime: st.ModTime(), used: time.Now(), readers: 1}
	archiveCache.m[p] = c
	return &zr.Reader, func() { release(c) }, nil
}

func release(c *cachedArchive) {
	archiveCache.Lock()
	defer archiveCache.Unlock()
	c.readers--
	if c.stale && c.readers == 0 {
		c.zr.Close()
	}
}

// ArchiveEPUB is ReadEPUB and WordCounts for a cached archive.
func ArchiveEPUB(p string) (*EPUBInfo, []int, error) {
	zr, done, err := OpenArchive(p)
	if err != nil {
		return nil, nil, err
	}
	defer done()
	archiveCache.Lock()
	c := archiveCache.m[p]
	if c != nil && c.epub != nil {
		defer archiveCache.Unlock()
		return c.epub, c.words, nil
	}
	archiveCache.Unlock()
	info, err := ReadEPUB(zr)
	if err != nil {
		return nil, nil, err
	}
	words := WordCounts(zr, info.Spine)
	if c != nil {
		archiveCache.Lock()
		c.epub, c.words = info, words
		archiveCache.Unlock()
	}
	return info, words, nil
}

// ArchivePages is ComicPages for a cached archive.
func ArchivePages(p string) ([]string, error) {
	zr, done, err := OpenArchive(p)
	if err != nil {
		return nil, err
	}
	defer done()
	archiveCache.Lock()
	c := archiveCache.m[p]
	archiveCache.Unlock()
	if c != nil && c.pages != nil {
		return c.pages, nil
	}
	pages := ComicPages(zr)
	if c != nil {
		archiveCache.Lock()
		c.pages = pages
		archiveCache.Unlock()
	}
	return pages, nil
}
