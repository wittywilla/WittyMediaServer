package library

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wittywilla/wittymediaserver/internal/ebook"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"github.com/wittywilla/wittymediaserver/internal/metafolder"
	"github.com/wittywilla/wittymediaserver/internal/zipsafe"
)

// Books, comics and manga. Files that share a name in a folder are one
// book in several formats (downloads from digital libraries often come as
// CBZ + EPUB + PDF plus scans, OCR text and DRM-locked copies); the best
// readable format is used and the rest is ignored.

// Formats of one book, best first: comics prefer page images, text prefers EPUB, and
// plain text comes last, since it's also what scans and notes are saved as.
var bookRank = map[string]int{"cbz": 1, "epub": 2, "pdf": 3, "cbr": 4, "cb7": 5, "cbt": 6, "azw3": 7, "mobi": 8, "azw": 9, "fb2": 10, "docx": 11, "odt": 12,
	"djvu": 13, "doc": 14, "rtf": 15, "pdb": 16, "html": 17, "md": 18, "txt": 19}

// junkTextName: text beside books that isn't one. Text this small isn't one either.
var junkTextName = regexp.MustCompile(`(?i)^(read ?me|license|licence|copying|changelog|changes|history|notes?|info|index|toc|contents|authors|credits|contributing|install|manifest|about|description|comicinfo)\b`)

const minTextBook = 2 << 10

// Files from digital-library downloads that are not a readable book.
var junkBookRe = regexp.MustCompile(`(?i)(_lcp\.epub|_jp2\.zip|_djvu\.(txt|xml)|_hocr.*|_chocr.*|_scandata\.xml|_page_numbers\.json|_daisy.*|_text\.pdf)$`)

func discoverBooks(root string, step walkStep) ([]candidate, error) {
	type pick struct {
		c    candidate
		rank int
	}
	best := map[string]pick{} // folder + name without extension -> best format
	var bad unreadable
	var unnamed sniffBudget
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err := step.next(); err != nil {
			return err
		}
		if err != nil {
			bad.note(err)
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || strings.HasSuffix(strings.ToLower(name), "_daisy") || strings.EqualFold(name, "__MACOSX")) {
				return filepath.SkipDir
			}
			return nil
		}
		if junkBookRe.MatchString(name) || strings.HasPrefix(name, "._") {
			return nil
		}
		st, err := d.Info()
		if err != nil || !mediaEntry(root, p, d) {
			return nil
		}
		f := mediatypes.Find(name, mediatypes.Comic, mediatypes.Book)
		format := ""
		switch {
		case f == nil:
			if f = unnamed.sniff(p, name, st.Size(), mediatypes.Comic, mediatypes.Book); f == nil {
				return nil
			}
			format = f.ID
		case f.ID == "txt" || f.ID == "md" || f.ID == "html":
			stem := strings.TrimSuffix(name, filepath.Ext(name))
			if st.Size() < minTextBook || junkTextName.MatchString(stem) || !nameHolds(p, name, mediatypes.Book) {
				return nil
			}
		case f.Confirm && !nameHolds(p, name, mediatypes.Book):
			return nil
		}
		rank := bookRank[f.ID]
		key := filepath.Join(filepath.Dir(p), strings.TrimSuffix(name, filepath.Ext(name)))
		cur, ok := best[key]
		wins := !ok || rank < cur.rank
		// A locked EPUB gives way to any other copy of the same book. This only opens a
		// file when there are two copies to choose between.
		if ok && wins && f.ID == "epub" && epubFileLocked(p) {
			wins = false
		} else if ok && !wins && strings.EqualFold(filepath.Ext(cur.c.path), ".epub") && epubFileLocked(cur.c.path) {
			wins = true
		}
		if wins {
			best[key] = pick{candidate{path: p, kind: "book", size: st.Size(), mtime: st.ModTime().Unix(), format: format}, rank}
		}
		return nil
	})
	out := make([]candidate, 0, len(best))
	for _, v := range best {
		out = append(out, v.c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, bad.with(err)
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
	if k := c.unchanged(); k != nil {
		if k.missing {
			return s.Store.restore(ctx, k.id)
		}
		return nil
	}
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
	format := c.format
	if format == "" {
		if f := mediatypes.Find(c.path, mediatypes.Comic, mediatypes.Book); f != nil {
			format = f.ID
		}
	}
	it.Container = format
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
	switch {
	case format == "epub":
		cover = s.readEPUBMeta(ctx, it, it.Path)
	case format == "cbz" && comicIsZip(c.path):
		cover = s.readCBZMeta(it)
	case format == "cbz", format == "cbr", format == "cb7", format == "cbt":
		// The name may not say what's inside: a ZIP goes the way of a CBZ.
		if comicIsZip(c.path) {
			cover = s.readCBZMeta(it)
		} else {
			cover = s.readComicMeta(it)
		}
	case format == "pdf":
		if img := readPDFMeta(it); img != nil {
			cover = func(ctx context.Context, out string) error { return s.coverFromBytes(ctx, img, ".jpg", out) }
		}
	case format == "djvu":
		pages := djvuPages(c.path)
		it.RuntimeSec = float64(pages)
		if pages > 0 && HasDjVu() {
			cover = func(ctx context.Context, out string) error {
				p, err := s.DjvuPageFile(ctx, it, 0)
				if err != nil {
					return err
				}
				return s.Tools.Still(ctx, append([]string{"-f", "image2", "-max_pixels", maxCoverPixels}, ffmpeg.FileInput(p)...), 0, 400, out)
			}
		}
	case ebook.Convertible(format):
		// Its details and cover come from the EPUB the reader will use.
		if p, err := s.BookEPUB(it); err == nil {
			cover = s.readEPUBMeta(ctx, it, p)
		} else {
			s.minorFailure(ctx, "could not convert a book for the reader", err, "file", c.path)
		}
	}
	// Digital-library sidecars: <id>_meta.xml and __ia_thumb.jpg.
	if metas, _ := filepath.Glob(filepath.Join(dir, "*_meta.xml")); len(metas) == 1 {
		applyArchiveMeta(it, metas[0], number == "")
	}
	cleanBook(it)
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

// coverFromBytes makes a cover from a picture that's already in memory.
func (s *Scanner) coverFromBytes(ctx context.Context, img []byte, ext, out string) error {
	if err := os.MkdirAll(s.ImagesDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.ImagesDir, "cover-*"+ext)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(img); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return s.Tools.Still(ctx, append([]string{"-f", "image2", "-max_pixels", maxCoverPixels}, ffmpeg.FileInput(tmp.Name())...), 0, 400, out)
}

// makeCover turns an image inside an archive into a 400 px cover.
func (s *Scanner) makeCover(ctx context.Context, archive, entry, out string) error {
	zr, err := openZip(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	f, err := zr.Open(entry)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := os.MkdirAll(s.ImagesDir, 0o755); err != nil {
		return err
	}
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
	names := make([]string, 0, min(len(zr.File), maxComicPages))
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() && comicPageExt(strings.ToLower(f.Name)) {
			names = append(names, f.Name)
		}
		if len(names) >= maxComicPages {
			break
		}
	}
	return pageNames(names)
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
	zr, err := openZip(it.Path)
	if err != nil {
		return nil
	}
	defer zr.Close()
	pages := ComicPages(zr.Reader)
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

// Finding copy protection, so that a book can be refused with a plain message instead of
// showing unreadable chapters. Nothing here removes it or reads what it protects.

// fontObfuscation are the two ways a publisher scrambles an embedded font so it can't simply
// be copied out for other uses. The book's own text isn't touched, so a book with only this
// reads as it should, in the reader's own font.
var fontObfuscation = map[string]bool{
	"http://www.idpf.org/2008/embedding": true,
	"http://ns.adobe.com/pdf/enc#RC":     true,
}

// fontFile and bookDocument say what a file listed in encryption.xml is to the reader.
func fontFile(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".otf", ".ttf", ".ttc", ".woff", ".woff2", ".eot", ".pfb", ".pfa", ".dfont", ".cff", ".odttf":
		return true
	}
	return false
}

func bookDocument(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".xhtml", ".xht", ".html", ".htm", ".opf", ".ncx", ".xml":
		return true
	}
	return false
}

// epubEntry finds a file by name in any case: META-INF isn't always spelled the same.
func epubEntry(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, name) {
			return f
		}
	}
	return nil
}

// epubProtection says why a book can't be read: ebook.ErrEncrypted for a password on the ZIP,
// ebook.ErrProtected for copy protection, nil for anything else.
func epubProtection(zr *zip.Reader) error {
	for _, f := range zr.File {
		if f.Flags&0x1 != 0 {
			return ebook.ErrEncrypted
		}
	}
	f := epubEntry(zr, "META-INF/encryption.xml")
	if f == nil {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return epubLooksLocked(zr)
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, 8<<20))
	if err != nil {
		return epubLooksLocked(zr)
	}
	var doc struct {
		Data []struct {
			Method struct {
				Algorithm string `xml:"Algorithm,attr"`
			} `xml:"EncryptionMethod"`
			Target struct {
				URI string `xml:"URI,attr"`
			} `xml:"CipherData>CipherReference"`
		} `xml:"EncryptedData"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return epubLooksLocked(zr)
	}
	for _, d := range doc.Data {
		target := d.Target.URI
		if u, err := url.PathUnescape(target); err == nil {
			target = u
		}
		switch {
		case fontObfuscation[strings.TrimSpace(d.Method.Algorithm)]:
			if bookDocument(target) { // a chapter under it doesn't read, whatever it's called
				return ebook.ErrProtected
			}
		case !fontFile(target): // a font on its own locked away doesn't stop the book reading
			return ebook.ErrProtected
		}
	}
	return nil
}

// epubLooksLocked decides when encryption.xml can't be read: a licence beside it, or chapters
// that aren't text, are what a locked book looks like. Otherwise it's only a damaged list.
func epubLooksLocked(zr *zip.Reader) error {
	for _, n := range []string{"META-INF/rights.xml", "META-INF/license.lcpl", "META-INF/sinf.xml"} {
		if epubEntry(zr, n) != nil {
			return ebook.ErrProtected
		}
	}
	for _, f := range zr.File {
		if strings.EqualFold(path.Dir(f.Name), "META-INF") || !bookDocument(f.Name) || strings.HasSuffix(strings.ToLower(f.Name), ".opf") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		head := make([]byte, 4096)
		n, _ := io.ReadFull(rc, head)
		rc.Close()
		return unreadableText(head[:n])
	}
	return nil
}

// unreadableText: any chapter is text, and encrypted data can't look like it for long.
func unreadableText(b []byte) error {
	if bytes.HasPrefix(b, []byte{0xFF, 0xFE}) || bytes.HasPrefix(b, []byte{0xFE, 0xFF}) { // UTF-16
		return nil
	}
	// The last few bytes may be part of a character cut off by the 4096.
	for cut := 0; cut < 4 && cut <= len(b); cut++ {
		if s := b[:len(b)-cut]; utf8.Valid(s) && !bytes.Contains(s, []byte{0}) {
			return nil
		}
	}
	return ebook.ErrProtected
}

// ReadEPUB parses an EPUB's package document and table of contents. A book with copy protection
// or a password stops here.
func ReadEPUB(zr *zip.Reader) (*EPUBInfo, error) {
	if err := epubProtection(zr); err != nil {
		return nil, err
	}
	return parseEPUB(zr)
}

func parseEPUB(zr *zip.Reader) (*EPUBInfo, error) {
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

// readEPUBMeta reads a book's details from an EPUB: the file itself, or the one made
// from it for the reader. A locked book still gets its title and author, which are kept in
// the clear, so that it's clear which book the message is about when someone opens it.
func (s *Scanner) readEPUBMeta(ctx context.Context, it *Item, file string) func(context.Context, string) error {
	zr, err := openZip(file)
	if err != nil {
		return nil
	}
	defer zr.Close()
	locked := epubProtection(zr.Reader)
	if locked != nil {
		s.minorFailure(ctx, "a book can't be opened", locked, "file", file)
	}
	info, err := parseEPUB(zr.Reader)
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
	if info.Cover == "" || locked != nil {
		return nil
	}
	archive, entry := file, info.Cover
	return func(ctx context.Context, out string) error { return s.makeCover(ctx, archive, entry, out) }
}

// applyArchiveMeta reads a digital-library *_meta.xml for title, author
// and description (titles only when the file name gave no number).
func applyArchiveMeta(it *Item, p string, title bool) {
	b, err := readSmall(p, maxNFO)
	if err != nil {
		return
	}
	var m struct {
		Title       string   `xml:"title"`
		Creator     []string `xml:"creator"`
		Description string   `xml:"description"`
		Date        string   `xml:"date"`
		Subject     []string `xml:"subject"`
	}
	if line, msg := metafolder.DecodeXML(b, &m); msg != "" {
		slog.Default().Warn("could not read a metadata file; ignoring it", "cat", "library", "file", p, "line", line, "err", msg)
		return
	}
	if m.Title = metafolder.Line(m.Title, 300); title && m.Title != "" && it.Album == "" {
		it.Title = strings.TrimSpace(parenRe.ReplaceAllString(m.Title, ""))
	}
	if it.Artist == "" && len(m.Creator) > 0 {
		c := metafolder.Line(m.Creator[0], 300)
		it.Artist, it.AlbumArtist = c, c
	}
	if it.Overview == "" && m.Description != "" {
		it.Overview = metafolder.Text(plain(m.Description), 20000)
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
	zr      *zipFile
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
	// Whether it's a locked book, once somebody has asked.
	protection error
	checked    bool
}

func sweepArchives() {
	for range time.Tick(archiveIdle / 2) {
		func() {
			archiveCache.Lock()
			defer archiveCache.Unlock()
			for k, c := range archiveCache.m {
				if c.readers == 0 && time.Since(c.used) > archiveIdle {
					c.zr.Close()
					delete(archiveCache.m, k)
				}
			}
		}()
	}
}

// zipFile is a zip read through a handle already open, so it is the file that was checked.
type zipFile struct {
	*zip.Reader
	f *os.File
}

func (z *zipFile) Close() error { return z.f.Close() }

// bookZip is more than a book or comic ever holds: well over 100,000 pages.
var bookZip = zipsafe.Limits{MaxEntries: 200000, MaxDirectory: 16 << 20, MaxName: 4 << 10, MaxFolders: 1 << 20}

// newZipFile takes over f.
func newZipFile(f *os.File, st fs.FileInfo) (*zipFile, error) {
	err := zipsafe.Check(f, st.Size(), bookZip)
	var zr *zip.Reader
	if err == nil {
		zr, err = zip.NewReader(f, st.Size())
	}
	if err == nil {
		err = zipsafe.CheckNames(zr, bookZip)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return &zipFile{Reader: zr, f: f}, nil
}

// openZip opens a zip found by a scan.
func openZip(p string) (*zipFile, error) {
	f, err := os.OpenFile(p, os.O_RDONLY|openNoBlock, 0)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil || st.IsDir() || !plainMode(st.Mode()) {
		f.Close()
		return nil, fs.ErrInvalid
	}
	return newZipFile(f, st)
}

// OpenArchive returns an open archive of a library (call the release func when done). The
// file is opened again each time, so one replaced since doesn't get by on the copy kept open.
func (s *Store) OpenArchive(ctx context.Context, p string) (*zip.Reader, func(), error) {
	f, st, err := s.OpenLibraryFile(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	return keepArchive(p, f, st)
}

// OpenBook opens what the reader reads for a book: the library's own file, or the copy made
// from it in the cache, which isn't in a library and which only this server writes. A book
// that's locked stops here, so none of its files go out.
func (s *Store) OpenBook(ctx context.Context, it *Item, book string) (*zip.Reader, func(), error) {
	var zr *zip.Reader
	var done func()
	var err error
	if book == it.Path {
		zr, done, err = s.OpenArchive(ctx, book)
	} else {
		var f *os.File
		if f, err = os.Open(book); err != nil {
			return nil, nil, err
		}
		var st fs.FileInfo
		if st, err = f.Stat(); err != nil {
			f.Close()
			return nil, nil, err
		}
		zr, done, err = keepArchive(book, f, st)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := archiveProtection(zr, book); err != nil {
		done()
		return nil, nil, err
	}
	return zr, done, nil
}

// archiveProtection is epubProtection, remembered while the archive stays open: every chapter
// and picture of a book asks.
func archiveProtection(zr *zip.Reader, p string) error {
	archiveCache.Lock()
	c := archiveCache.m[p]
	archiveCache.Unlock()
	if c != nil && c.zr.Reader == zr && c.checked {
		return c.protection
	}
	err := epubProtection(zr)
	if c != nil && c.zr.Reader == zr {
		archiveCache.Lock()
		c.protection, c.checked = err, true
		archiveCache.Unlock()
	}
	return err
}

// epubFileLocked: this EPUB is known to be one that can't be opened.
func epubFileLocked(p string) bool {
	zr, err := openZip(p)
	if err != nil {
		return false
	}
	defer zr.Close()
	return epubProtection(zr.Reader) != nil
}

func keepArchive(p string, f *os.File, st fs.FileInfo) (*zip.Reader, func(), error) {
	archiveCache.sweeper.Do(func() { logging.Loop("archive sweep", sweepArchives, "cat", "library") })
	archiveCache.Lock()
	defer archiveCache.Unlock()
	cur := archiveCache.m[p]
	if cur != nil && cur.mtime.Equal(st.ModTime()) {
		f.Close()
		cur.used, cur.readers = time.Now(), cur.readers+1
		return cur.zr.Reader, func() { release(cur) }, nil
	}
	if cur != nil { // the file was replaced
		delete(archiveCache.m, p)
		if cur.readers == 0 {
			cur.zr.Close()
		} else {
			cur.stale = true
		}
	}
	zr, err := newZipFile(f, st)
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
	return zr.Reader, func() { release(c) }, nil
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
func (s *Store) ArchiveEPUB(ctx context.Context, p string) (*EPUBInfo, []int, error) {
	zr, done, err := s.OpenArchive(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	defer done()
	return archiveEPUB(zr, p)
}

// ArchiveEPUBOf is ArchiveEPUB for what the reader reads for a book, as OpenBook.
func (s *Store) ArchiveEPUBOf(ctx context.Context, it *Item, book string) (*EPUBInfo, []int, error) {
	zr, done, err := s.OpenBook(ctx, it, book)
	if err != nil {
		return nil, nil, err
	}
	defer done()
	return archiveEPUB(zr, book)
}

func archiveEPUB(zr *zip.Reader, p string) (*EPUBInfo, []int, error) {
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
func (s *Store) ArchivePages(ctx context.Context, p string) ([]string, error) {
	zr, done, err := s.OpenArchive(ctx, p)
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
