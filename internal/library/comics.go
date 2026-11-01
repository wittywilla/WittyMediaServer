package library

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ebook"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
)

// Comics in any archive, whatever their names claim: many a .cbr is really a ZIP and
// many a .cbz a RAR. ZIP is read in place. RAR, 7-Zip and TAR are hostile enough to
// keep out of the server, so the same child process that reads game archives lists and
// unpacks them, into a folder of the cache that's trimmed as it grows.

const (
	// comicReadAhead pages are unpacked per request. Reading a solid archive means
	// reading from its start, so it is worth doing a few at once.
	comicReadAhead = 12
	maxComicPage   = 256 << 20
	// maxComicCache is how much unpacked comic is kept, and maxConvertedCache how many
	// converted books. Anything older goes first.
	maxComicCache     = int64(4) << 30
	maxConvertedCache = int64(2) << 30
)

// ErrNoReader says a book can't be opened here, and why.
var ErrNoReader = errors.New("this kind of book can't be opened here")

type comicList struct {
	mtime time.Time
	pages []string
}

var (
	comicLists   = map[string]comicList{}
	comicListsMu sync.Mutex
	comicUnpack  sync.Mutex // one at a time: each costs a process and a disk
)

// IsZipComic is comicIsZip for the server's handlers.
func IsZipComic(p string) bool { return comicIsZip(p) }

// ComicPageWeb is comicPageWeb for the server's handlers.
func ComicPageWeb(name string) bool { return comicPageWeb(name) }

// comicIsZip reads the file's first bytes: the name doesn't decide.
func comicIsZip(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	var b [4]byte
	if _, err := io.ReadFull(f, b[:]); err != nil {
		return false
	}
	return string(b[:]) == "PK\x03\x04" || string(b[:]) == "PK\x05\x06"
}

// pageNames sorts the pictures of an archive into reading order.
func pageNames(names []string) []string {
	var pages []string
	for _, n := range names {
		low := strings.ToLower(n)
		if strings.HasPrefix(low, "__macosx") || strings.Contains(low, "/.") || strings.HasPrefix(path.Base(low), ".") {
			continue
		}
		if comicPageExt(low) {
			pages = append(pages, n)
		}
		if len(pages) >= maxComicPages {
			break
		}
	}
	sort.Slice(pages, func(i, j int) bool { return naturalLess(pages[i], pages[j]) })
	return pages
}

func comicPageExt(low string) bool {
	switch path.Ext(low) {
	case ".jpg", ".jpeg", ".jfif", ".png", ".webp", ".gif", ".avif", ".bmp", ".jxl", ".tif", ".tiff", ".heic":
		return true
	}
	return false
}

// comicPageWeb: browsers show this kind of page as it is. Others are converted.
func comicPageWeb(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".jfif", ".png", ".webp", ".gif", ".avif", ".bmp":
		return true
	}
	return false
}

// ComicPageList lists a comic's pages, by reading its archive's directory.
func (s *Scanner) ComicPageList(p string) ([]string, error) {
	if comicIsZip(p) {
		return ArchivePages(p)
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	comicListsMu.Lock()
	cur, ok := comicLists[p]
	comicListsMu.Unlock()
	if ok && cur.mtime.Equal(st.ModTime()) {
		return cur.pages, nil
	}
	l, err := gamearchive.List(p)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(l.Entries))
	for i, e := range l.Entries {
		names[i] = e.Name
	}
	pages := pageNames(names)
	comicListsMu.Lock()
	if len(comicLists) >= 64 {
		comicLists = map[string]comicList{}
	}
	comicLists[p] = comicList{st.ModTime(), pages}
	comicListsMu.Unlock()
	return pages, nil
}

func (s *Scanner) comicDir(p string, st os.FileInfo) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", p, st.Size(), st.ModTime().UnixNano())))
	return filepath.Join(s.CacheDir, "comics", hex.EncodeToString(h[:10]))
}

// ComicPageFile puts a page of a RAR, 7-Zip or TAR comic in the cache and returns its
// file. A few pages after it come out too.
func (s *Scanner) ComicPageFile(ctx context.Context, p string, pages []string, n int) (string, error) {
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	dir := s.comicDir(p, st)
	name := func(i int) string { return fmt.Sprintf("%05d%s", i, strings.ToLower(path.Ext(pages[i]))) }
	out := filepath.Join(dir, name(n))
	if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
		return out, nil
	}
	comicUnpack.Lock()
	defer comicUnpack.Unlock()
	if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
		return out, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var want []gamearchive.Want
	for i := n; i < len(pages) && i < n+comicReadAhead; i++ {
		if _, err := os.Stat(filepath.Join(dir, name(i))); err != nil {
			want = append(want, gamearchive.Want{Name: pages[i], Out: name(i)})
		}
	}
	uctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if _, err := gamearchive.Unpack(uctx, p, dir, want, int64(len(want))*maxComicPage); err != nil {
		return "", err
	}
	trimCache(filepath.Join(s.CacheDir, "comics"), maxComicCache)
	return out, nil
}

// trimCache removes the oldest folders or files until a cache holds no more than limit.
func trimCache(root string, limit int64) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type item struct {
		path string
		size int64
		mod  time.Time
	}
	var items []item
	var total int64
	for _, e := range entries {
		p := filepath.Join(root, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		size := info.Size()
		if e.IsDir() {
			size = 0
			inner, _ := os.ReadDir(p)
			for _, f := range inner {
				if i, err := f.Info(); err == nil {
					size += i.Size()
				}
			}
		}
		items = append(items, item{p, size, info.ModTime()})
		total += size
	}
	if total <= limit {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.Before(items[j].mod) })
	for _, it := range items {
		if total <= limit {
			return
		}
		if os.RemoveAll(it.path) == nil {
			total -= it.size
		}
	}
}

// comicCover makes the cover of a comic from its first page.
func (s *Scanner) comicCover(ctx context.Context, archive, out string) error {
	pages, err := s.ComicPageList(archive)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return errors.New("this comic has no pages")
	}
	if comicIsZip(archive) {
		return s.makeCover(ctx, archive, pages[0], out)
	}
	p, err := s.ComicPageFile(ctx, archive, pages, 0)
	if err != nil {
		return err
	}
	return s.Tools.Still(ctx, append([]string{"-f", "image2", "-max_pixels", maxCoverPixels}, ffmpeg.FileInput(p)...), 0, 400, out)
}

// ComicPageImage makes sure a page can be shown: pictures a browser can't show become
// a JPEG, cached beside the unpacked pages.
func (s *Scanner) ComicPageImage(ctx context.Context, archive, name string, read func() (io.ReadCloser, error)) (string, error) {
	h := sha1.Sum([]byte(archive + "|" + name))
	dir := filepath.Join(s.CacheDir, "comics", "pages")
	key := hex.EncodeToString(h[:12])
	out := filepath.Join(dir, key+".jpg")
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	src, err := read()
	if err != nil {
		return "", err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(dir, "page-*"+path.Ext(name))
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(src, maxComicPage)); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()
	if err := s.Tools.ImageThumb(ctx, tmp.Name(), 0, 2560, 0, out); err != nil {
		return "", err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Books converted for the reader
// ---------------------------------------------------------------------------

var bookConvert sync.Mutex // conversions read a whole book into memory, so one at a time

// BookEPUB returns the EPUB a book reads as: itself for an EPUB, otherwise one made
// once from the book, kept in the cache and named after the book's identity.
func (s *Scanner) BookEPUB(it *Item) (string, error) {
	if it.Container == "epub" {
		return it.Path, nil
	}
	if !ebook.Convertible(it.Container) {
		return "", ErrNoReader
	}
	st, err := os.Stat(it.Path)
	if err != nil {
		return "", err
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", it.Path, st.Size(), st.ModTime().UnixNano())))
	dir := filepath.Join(s.CacheDir, "books")
	out := filepath.Join(dir, hex.EncodeToString(h[:12])+".epub")
	if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
		return out, nil
	}
	bookConvert.Lock()
	defer bookConvert.Unlock()
	if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
		return out, nil
	}
	book, err := ebook.Convert(it.Path, it.Container)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "book-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	err = book.WriteEPUB(tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), out); err != nil {
		return "", err
	}
	trimCache(dir, maxConvertedCache)
	return out, nil
}

// ---------------------------------------------------------------------------
// PDF: details and a cover without drawing anything
// ---------------------------------------------------------------------------

const maxPDFScan = 32 << 20

var (
	pdfPagesRe  = regexp.MustCompile(`/Type\s*/Pages\b[^>]{0,400}?/Count\s+(\d{1,6})`)
	pdfCountRe  = regexp.MustCompile(`/Count\s+(\d{1,6})[^>]{0,400}?/Type\s*/Pages\b`)
	pdfTitleRe  = regexp.MustCompile(`/Title\s*\(((?:[^()\\]|\\.){1,400})\)`)
	pdfAuthorRe = regexp.MustCompile(`/Author\s*\(((?:[^()\\]|\\.){1,400})\)`)
)

// readPDFMeta reads what's cheap to find in a PDF: pages, title and author when they
// aren't in compressed streams, and the first JPEG for a cover, which for a scanned
// book is its first page. It reads the start of the file only.
func readPDFMeta(it *Item) (cover []byte) {
	f, err := os.Open(it.Path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxPDFScan))
	if err != nil {
		return nil
	}
	pages := 0
	for _, re := range []*regexp.Regexp{pdfPagesRe, pdfCountRe} {
		for _, m := range re.FindAllSubmatch(b, 64) {
			if n, _ := strconv.Atoi(string(m[1])); n > pages {
				pages = n
			}
		}
	}
	if pages > 0 {
		it.RuntimeSec = float64(pages)
	}
	if m := pdfTitleRe.FindSubmatch(b); m != nil {
		if t := pdfString(m[1]); t != "" && it.Album == "" {
			it.Title = t
		}
	}
	if m := pdfAuthorRe.FindSubmatch(b); m != nil {
		if a := pdfString(m[1]); a != "" {
			it.Artist, it.AlbumArtist = a, a
		}
	}
	// The first stream that's a JPEG, by its filter and by its bytes.
	for from := 0; from < len(b); {
		i := bytes.Index(b[from:], []byte("/DCTDecode"))
		if i < 0 {
			break
		}
		at := from + i
		rel := bytes.Index(b[at:min(len(b), at+2048)], []byte("stream"))
		if rel < 0 {
			from = at + 10
			continue
		}
		start := at + rel + len("stream")
		for start < len(b) && (b[start] == '\r' || b[start] == '\n') {
			start++
		}
		if !bytes.HasPrefix(b[start:], []byte("\xFF\xD8\xFF")) {
			from = start
			continue
		}
		end := bytes.Index(b[start:], []byte("endstream"))
		if end <= 0 {
			break
		}
		return b[start : start+end]
	}
	return nil
}

// pdfString decodes a PDF literal string; one in UTF-16 stays out rather than garbled.
func pdfString(b []byte) string {
	if bytes.HasPrefix(b, []byte("\xFE\xFF")) || bytes.HasPrefix(b, []byte("\\376\\377")) {
		return ""
	}
	s := strings.NewReplacer(`\(`, "(", `\)`, ")", `\\`, `\`, `\n`, " ", `\r`, " ", `\t`, " ").Replace(string(b))
	if bytes.ContainsAny([]byte(s), "\x00\x01\x02\x03\x04\x05\x06\x07\x08") {
		return ""
	}
	return strings.TrimSpace(strings.ToValidUTF8(s, ""))
}

// ---------------------------------------------------------------------------
// DjVu: through DjVuLibre, when it's installed
// ---------------------------------------------------------------------------

// djvuTool finds a program of DjVuLibre, which isn't part of this server. Nothing is
// looked for in the folder the server runs from.
func djvuTool(name string) string {
	p, err := exec.LookPath(name)
	if err != nil || !filepath.IsAbs(p) {
		return ""
	}
	return p
}

// HasDjVu says DjVu pages can be drawn on this machine.
func HasDjVu() bool { return djvuTool("ddjvu") != "" }

// ErrNeedsDjVu: the book is here, the program that draws its pages isn't.
var ErrNeedsDjVu = errors.New("DjVu books need the DjVuLibre tools (ddjvu) on the server. You can still download this one")

// djvuPages counts pages from the file's own directory, without any program.
func djvuPages(p string) int {
	f, err := os.Open(p)
	if err != nil {
		return 0
	}
	defer f.Close()
	var h [48]byte
	n, _ := io.ReadFull(f, h[:])
	if n < 16 || string(h[:8]) != "AT&TFORM" {
		return 0
	}
	switch string(h[12:16]) {
	case "DJVU":
		return 1
	case "DJVM":
		// DIRM: flags, then a two-byte count of the components (pages and shared parts).
		if n >= 16+11 && string(h[16:20]) == "DIRM" {
			return int(h[25])<<8 | int(h[26])
		}
	}
	return 0
}

// DjvuPageList names a DjVu book's pages in the form the comic reader uses.
func DjvuPageList(p string) []string {
	n := 0
	if tool := djvuTool("djvused"); tool != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, tool, "-e", "n", p).Output(); err == nil {
			n, _ = strconv.Atoi(strings.TrimSpace(string(out)))
		}
	}
	if n <= 0 {
		n = djvuPages(p)
	}
	if n > maxComicPages {
		n = maxComicPages
	}
	pages := make([]string, n)
	for i := range pages {
		pages[i] = strconv.Itoa(i+1) + ".jpg"
	}
	return pages
}

// DjvuPageFile draws one page, once, as a JPEG in the cache. It has a time limit, so a
// hostile file costs a moment; ddjvu is a separate program, so a crash costs nothing.
func (s *Scanner) DjvuPageFile(ctx context.Context, it *Item, page int) (string, error) {
	tool := djvuTool("ddjvu")
	if tool == "" {
		return "", ErrNeedsDjVu
	}
	st, err := os.Stat(it.Path)
	if err != nil {
		return "", err
	}
	dir := s.comicDir(it.Path, st)
	out := filepath.Join(dir, fmt.Sprintf("%05d.jpg", page))
	if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
		return out, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp := filepath.Join(dir, fmt.Sprintf("%05d.tmp.tif", page))
	defer os.Remove(tmp)
	dctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	abs, err := filepath.Abs(it.Path) // so no name can read as an option
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(dctx, tool, "-format=tiff", "-page="+strconv.Itoa(page+1), "-size=2000x2800", abs, tmp)
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ddjvu: %w: %s", err, strings.TrimSpace(string(b)))
	}
	if err := s.Tools.ImageThumb(dctx, tmp, 0, 2560, 0, out); err != nil {
		return "", err
	}
	trimCache(filepath.Join(s.CacheDir, "comics"), maxComicCache)
	return out, nil
}

// readComicMeta counts a comic's pages and says how to make its cover.
func (s *Scanner) readComicMeta(it *Item) func(context.Context, string) error {
	pages, err := s.ComicPageList(it.Path)
	if err != nil {
		s.Log.Debug("could not list a comic's pages", "file", it.Path, "err", err)
		return nil
	}
	it.RuntimeSec = float64(len(pages)) // books: length in pages
	if len(pages) == 0 {
		return nil
	}
	archive := it.Path
	return func(ctx context.Context, out string) error { return s.comicCover(ctx, archive, out) }
}
