// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package ebook turns book formats a reader can't show directly into EPUB, which it
// can. It knows Kindle (MOBI, AZW, KF8) and Palm books, FictionBook, Word (DOCX and
// DOC), OpenDocument, Rich Text, Markdown, web pages and plain text.
//
// Every input is treated as hostile. Sizes are checked against what was really read,
// allocations are bounded, every archive entry has a limit, and everything that ends
// up in a chapter has been through the sanitiser (sanitize.go): no scripts, no styles
// that can reach out, no links but to the web, no images but the book's own.
//
// Books with copy protection are refused with ErrProtected. Nothing here removes it.
package ebook

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Limits on what is converted. A real book is far below all of them.
const (
	MaxInput      = 256 << 20 // the source file
	maxPlainInput = 64 << 20  // text formats: more than the longest novels by far
	maxEntry      = 96 << 20  // one file inside a ZIP-based format, unpacked
	maxImages     = 4096
	maxImage      = 32 << 20
	maxImagesSum  = 512 << 20
	maxChapters   = 4096
	maxBookText   = 256 << 20 // the text of a book once unpacked
)

// ErrProtected: the book has copy protection, which this server doesn't touch.
var ErrProtected = errors.New("this book has copy protection, so it can't be opened here")

// ErrEncrypted: the file has a password.
var ErrEncrypted = errors.New("this file is protected by a password, so it can't be opened here")

// ErrTooBig: the book is over the limits.
var ErrTooBig = errors.New("this book is too large to convert")

// Chapter is one file of the book. Body is sanitised XHTML, the inside of a body tag.
type Chapter struct {
	Title string
	Body  string
}

// Image is a picture of the book, stored at images/<Name>.
type Image struct {
	Name string
	MIME string
	Data []byte
}

// Book is what a conversion produces.
type Book struct {
	Title       string
	Author      string
	Language    string
	Description string
	Date        string
	Chapters    []Chapter
	Images      []Image
	// Cover names the image that is the book's cover ("" = none).
	Cover string

	imageBytes int
}

// addImage keeps a picture if its bytes really are one of the kinds a browser shows,
// and returns its stored name ("" when it was refused). SVG never qualifies: as a
// picture in a chapter it could carry a script.
func (b *Book) addImage(data []byte) string {
	mime, ext := imageType(data)
	if mime == "" || len(data) > maxImage || len(b.Images) >= maxImages || b.imageBytes+len(data) > maxImagesSum {
		return ""
	}
	name := fmt.Sprintf("%05d%s", len(b.Images)+1, ext)
	b.Images = append(b.Images, Image{Name: name, MIME: mime, Data: data})
	b.imageBytes += len(data)
	return name
}

func imageType(b []byte) (mime, ext string) {
	switch {
	case bytes.HasPrefix(b, []byte("\xFF\xD8\xFF")):
		return "image/jpeg", ".jpg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", ".png"
	case bytes.HasPrefix(b, []byte("GIF8")):
		return "image/gif", ".gif"
	case len(b) > 12 && bytes.HasPrefix(b, []byte("RIFF")) && string(b[8:12]) == "WEBP":
		return "image/webp", ".webp"
	case bytes.HasPrefix(b, []byte("BM")):
		return "image/bmp", ".bmp"
	}
	return "", ""
}

// addChapter adds a chapter when it holds anything.
func (b *Book) addChapter(title, body string) {
	if strings.TrimSpace(body) == "" || len(b.Chapters) >= maxChapters {
		return
	}
	if title == "" {
		title = firstHeading(body)
	}
	if title == "" {
		title = "Part " + strconv.Itoa(len(b.Chapters)+1)
	}
	b.Chapters = append(b.Chapters, Chapter{Title: title, Body: body})
}

var headingRe = regexp.MustCompile(`(?is)<h[1-3][^>]*>(.*?)</h[1-3]>`)
var tagRe = regexp.MustCompile(`<[^>]*>`)

func firstHeading(body string) string {
	m := headingRe.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	t := strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(m[1], "")))
	if len(t) > 120 {
		t = t[:120]
	}
	return strings.ToValidUTF8(t, "")
}

// Convert reads a file of a format (a registry id) as a Book.
func Convert(path, format string) (*Book, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a file")
	}
	limit := int64(MaxInput)
	switch format {
	case "txt", "md", "rtf", "html", "fb2":
		limit = maxPlainInput
	}
	if st.Size() > limit {
		return nil, ErrTooBig
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ConvertBytes(data, format, strings.TrimSuffix(baseName(path), extOf(path)))
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func extOf(p string) string {
	base := baseName(p)
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		return base[i:]
	}
	return ""
}

// ConvertBytes is Convert for what is already read. title names the book when it
// doesn't say. A panic from a decoder costs this book, not the server.
func ConvertBytes(data []byte, format, title string) (book *Book, err error) {
	defer func() {
		if p := recover(); p != nil {
			book, err = nil, fmt.Errorf("could not read this book (%v)", p)
		}
	}()
	switch format {
	case "txt":
		book = fromText(data)
	case "md":
		book = fromMarkdown(data)
	case "html":
		book, err = fromHTML(data)
	case "rtf":
		book, err = fromRTF(data)
	case "fb2":
		book, err = fromFB2(data)
	case "docx":
		book, err = fromDOCX(data)
	case "odt":
		book, err = fromODT(data)
	case "doc":
		book, err = fromDOC(data)
	case "mobi", "azw", "azw3", "pdb":
		book, err = fromMobi(data)
	default:
		return nil, fmt.Errorf("no converter for %q", format)
	}
	if err != nil {
		return nil, err
	}
	if book.Title == "" {
		book.Title = title
	}
	if len(book.Chapters) == 0 {
		return nil, errors.New("this book has no readable text")
	}
	return book, nil
}

// Convertible says this format (a registry id) goes through this package.
func Convertible(format string) bool {
	switch format {
	case "txt", "md", "html", "rtf", "fb2", "docx", "odt", "doc", "mobi", "azw", "azw3", "pdb":
		return true
	}
	return false
}

// WriteEPUB stores the book as an EPUB 2 file, the form the reader takes.
func (b *Book) WriteEPUB(w io.Writer) error {
	zw := zip.NewWriter(w)
	stored := func(name string, data []byte) error {
		// The modified time is fixed, so a book converted twice comes out the same.
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	deflated := func(name string, data []byte) error {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	if err := stored("mimetype", []byte("application/epub+zip")); err != nil {
		return err
	}
	if err := deflated("META-INF/container.xml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)); err != nil {
		return err
	}
	var manifest, spine, nav strings.Builder
	for i, c := range b.Chapters {
		name := fmt.Sprintf("c%04d.xhtml", i+1)
		page := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<html xmlns="http://www.w3.org/1999/xhtml"><head><title>` + xmlText(c.Title) + `</title></head><body>` + c.Body + `</body></html>`
		if err := deflated("OEBPS/"+name, []byte(page)); err != nil {
			return err
		}
		fmt.Fprintf(&manifest, `<item id="c%d" href="%s" media-type="application/xhtml+xml"/>`, i+1, name)
		fmt.Fprintf(&spine, `<itemref idref="c%d"/>`, i+1)
		fmt.Fprintf(&nav, `<navPoint id="n%d" playOrder="%d"><navLabel><text>%s</text></navLabel><content src="%s"/></navPoint>`, i+1, i+1, xmlText(c.Title), name)
	}
	coverMeta := ""
	for i, im := range b.Images {
		if err := stored("OEBPS/images/"+im.Name, im.Data); err != nil {
			return err
		}
		fmt.Fprintf(&manifest, `<item id="i%d" href="images/%s" media-type="%s"/>`, i+1, im.Name, im.MIME)
		if im.Name == b.Cover {
			coverMeta = fmt.Sprintf(`<meta name="cover" content="i%d"/>`, i+1)
		}
	}
	lang := b.Language
	if lang == "" {
		lang = "und"
	}
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package version="2.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">witty-converted</dc:identifier><dc:title>` + xmlText(b.Title) + `</dc:title><dc:language>` + xmlText(lang) + `</dc:language>`
	if b.Author != "" {
		opf += `<dc:creator>` + xmlText(b.Author) + `</dc:creator>`
	}
	if b.Description != "" {
		opf += `<dc:description>` + xmlText(b.Description) + `</dc:description>`
	}
	if b.Date != "" {
		opf += `<dc:date>` + xmlText(b.Date) + `</dc:date>`
	}
	opf += coverMeta + `</metadata><manifest><item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>` + manifest.String() + `</manifest><spine toc="ncx">` + spine.String() + `</spine></package>`
	if err := deflated("OEBPS/content.opf", []byte(opf)); err != nil {
		return err
	}
	ncx := `<?xml version="1.0" encoding="UTF-8"?>
<ncx version="2005-1" xmlns="http://www.daisy.org/z3986/2005/ncx/"><head/><docTitle><text>` + xmlText(b.Title) + `</text></docTitle><navMap>` + nav.String() + `</navMap></ncx>`
	if err := deflated("OEBPS/toc.ncx", []byte(ncx)); err != nil {
		return err
	}
	return zw.Close()
}

// xmlText escapes text for XML, and drops what XML doesn't allow at all.
func xmlText(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return r
		case r < 0x20 || r == 0xFFFE || r == 0xFFFF:
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	return html.EscapeString(s)
}

// readZip opens a ZIP-based format with limits: entries are read through readEntry.
func readZip(data []byte) (*zip.Reader, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(zr.File) > 100000 {
		return nil, ErrTooBig
	}
	return zr, nil
}

func readEntry(f *zip.File) ([]byte, error) {
	if f.Flags&0x1 != 0 {
		return nil, ErrEncrypted
	}
	if f.UncompressedSize64 > maxEntry {
		return nil, ErrTooBig
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	// The size in the header is only what the file says.
	b, err := io.ReadAll(io.LimitReader(rc, maxEntry+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxEntry {
		return nil, ErrTooBig
	}
	return b, nil
}

func zipFile(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, name) {
			return f
		}
	}
	return nil
}

func zipBytes(zr *zip.Reader, name string) ([]byte, error) {
	f := zipFile(zr, name)
	if f == nil {
		return nil, fmt.Errorf("no %s in this file", name)
	}
	return readEntry(f)
}
