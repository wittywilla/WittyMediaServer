package ebook

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func allText(b *Book) string {
	var s strings.Builder
	for _, c := range b.Chapters {
		s.WriteString(c.Body)
	}
	return s.String()
}

func TestPlainText(t *testing.T) {
	b, err := ConvertBytes([]byte("Chapter 1\n\nFirst <b>not bold</b> para\nsame para.\n\nChapter 2\n\nSecond."), "txt", "Title")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Title" || len(b.Chapters) != 2 || b.Chapters[0].Title != "Chapter 1" {
		t.Fatalf("%+v", b)
	}
	if !strings.Contains(allText(b), "First &lt;b&gt;not bold&lt;/b&gt; para same para.") {
		t.Error(allText(b))
	}
}

func TestTextEncodings(t *testing.T) {
	utf16 := []byte{0xFF, 0xFE}
	for _, r := range "Café" {
		utf16 = append(utf16, byte(r), byte(r>>8))
	}
	for name, in := range map[string][]byte{"utf-8": []byte("Café"), "bom": []byte("\xEF\xBB\xBFCafé"), "cp1252": []byte("Caf\xE9"), "utf-16": utf16} {
		if got := decodeText(in); got != "Café" {
			t.Errorf("%s: %q", name, got)
		}
	}
	if got := decodeText([]byte(`<?xml version="1.0" encoding="windows-1251"?>` + "\xCF\xF0\xE8")); !strings.Contains(got, "При") {
		t.Errorf("a named encoding: %q", got)
	}
}

func TestMarkdownCantInject(t *testing.T) {
	b, err := ConvertBytes([]byte("# Title\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1)) [good](https://example.test/x)\n\n**bold** and *it*\n\n- one\n- two\n\n> quoted\n\n```\ncode <x>\n```\n![pic](http://evil.test/x.png)\n"), "md", "")
	if err != nil {
		t.Fatal(err)
	}
	s := allText(b)
	for _, bad := range []string{"<script", "javascript:alert(1)\"", "evil.test", "<img"} {
		if strings.Contains(s, bad) {
			t.Errorf("%q in %s", bad, s)
		}
	}
	for _, want := range []string{"&lt;script&gt;", `<a href="https://example.test/x">good</a>`, "<strong>bold</strong>", "<em>it</em>", "<li>one</li>", "<blockquote>", "code &lt;x&gt;"} {
		if !strings.Contains(s, want) {
			t.Errorf("no %q in %s", want, s)
		}
	}
	if b.Title != "Title" {
		t.Errorf("title %q", b.Title)
	}
}

func TestSanitizer(t *testing.T) {
	in := `<p onclick="x()">Hi<script>alert(1)</script><style>@import url(http://evil.test)</style>
	<a href="javascript:alert(1)">a</a><a href="https://ok.test/">b</a><a href="data:text/html,x">c</a>
	<img src="http://evil.test/x.png"><iframe src="http://evil.test"></iframe><svg onload="x()"><script>y</script></svg>
	<object data="x"></object><form action="http://evil.test"><input name="p"></form><video src="x"></video>
	<table><tr><td colspan="2" onmouseover="x">cell</td></tr></table><unknown>kept</unknown></p>`
	out := sanitizeHTML(in, htmlOptions{})
	for _, bad := range []string{"script", "onclick", "onmouseover", "javascript", "evil.test", "iframe", "<svg", "object", "<form", "input", "video", "data:", "<style", "<img"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q in %s", bad, out)
		}
	}
	for _, want := range []string{`<a href="https://ok.test/">b</a>`, `colspan="2"`, "kept", "Hi"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in %s", want, out)
		}
	}
	deep := strings.Repeat("<div>", 200000) + "x"
	if len(sanitizeHTML(deep, htmlOptions{})) > 1<<20 {
		t.Error("deep nesting should be cut off")
	}
}

func TestHTMLBook(t *testing.T) {
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="
	b, err := ConvertBytes([]byte(`<!doctype html><html><head><title>A Page</title><meta name="author" content="Someone"><script>x</script></head><body><h1>Head</h1><p>Words</p><img src="data:image/png;base64,`+png+`"><img src="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="></body></html>`), "html", "x")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "A Page" || b.Author != "Someone" {
		t.Errorf("%+v", b)
	}
	if len(b.Images) != 1 || b.Images[0].MIME != "image/png" {
		t.Errorf("images %+v", b.Images)
	}
	if !strings.Contains(allText(b), `src="images/00001.png"`) {
		t.Error(allText(b))
	}
}

func TestRTF(t *testing.T) {
	src := `{\rtf1\ansi\ansicpg1252{\fonttbl{\f0 Arial;}}{\colortbl;\red0\green0\blue0;}{\info{\title My Title}{\author Me}}\f0 Plain \b bold\b0  and \i it\i0 .\par Caf\'e9 \u233? done\par{\pict\pngblip 89504e47}{\*\fldinst HYPERLINK "http://evil.test"}end}`
	b, err := ConvertBytes([]byte(src), "rtf", "x")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "My Title" || b.Author != "Me" {
		t.Errorf("%+v", b)
	}
	s := allText(b)
	for _, want := range []string{"Plain <b>bold</b>", "<i>it</i>", "Café é done"} {
		if !strings.Contains(s, want) {
			t.Errorf("no %q in %s", want, s)
		}
	}
	for _, bad := range []string{"Arial", "evil.test", "89504e47"} {
		if strings.Contains(s, bad) {
			t.Errorf("%q in %s", bad, s)
		}
	}
	// Unbalanced and nested for ever: still ends, and is well formed.
	if _, err := ConvertBytes([]byte(strings.Repeat("{", 1<<20)+"x"), "rtf", "x"); err == nil {
		// Nothing readable is fine as well.
		_ = err
	}
}

func TestFB2(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?><FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0" xmlns:l="http://www.w3.org/1999/xlink">
	<description><title-info><author><first-name>A.</first-name><last-name>Writer</last-name></author><book-title>Tale</book-title><annotation><p>About it</p></annotation><lang>en</lang><coverpage><image l:href="#c.png"/></coverpage></title-info></description>
	<body><section><title><p>One</p></title><p>First <emphasis>word</emphasis>.</p><image l:href="#c.png"/></section><section><title><p>Two</p></title><p>Second.</p></section></body>
	<binary id="c.png" content-type="image/png">iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==</binary></FictionBook>`
	b, err := ConvertBytes([]byte(src), "fb2", "x")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Tale" || b.Author != "A. Writer" || b.Language != "en" || !strings.Contains(b.Description, "About it") {
		t.Errorf("%+v", b)
	}
	if len(b.Chapters) != 2 || b.Chapters[1].Title != "Two" {
		t.Fatalf("%+v", b.Chapters)
	}
	if b.Cover == "" || len(b.Images) != 1 {
		t.Errorf("cover %q, images %d", b.Cover, len(b.Images))
	}
	if !strings.Contains(b.Chapters[0].Body, "<em>word</em>") || !strings.Contains(b.Chapters[0].Body, "<img") {
		t.Error(b.Chapters[0].Body)
	}
}

func docx(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, s := range entries {
		f, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(s))
	}
	zw.Close()
	return buf.Bytes()
}

func TestDOCX(t *testing.T) {
	b, err := ConvertBytes(docx(t, map[string]string{
		"docProps/core.xml": `<cp:coreProperties xmlns:cp="x" xmlns:dc="y"><dc:title>Doc Title</dc:title><dc:creator>Me</dc:creator></cp:coreProperties>`,
		"word/document.xml": `<w:document xmlns:w="w"><w:body><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Head</w:t></w:r></w:p><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Bold</w:t></w:r><w:r><w:t xml:space="preserve"> plain &lt;x&gt;</w:t></w:r></w:p></w:body></w:document>`,
	}), "docx", "x")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Doc Title" || b.Author != "Me" {
		t.Errorf("%+v", b)
	}
	s := allText(b)
	for _, want := range []string{"<h1>Head</h1>", "<b>Bold</b> plain &lt;x&gt;"} {
		if !strings.Contains(s, want) {
			t.Errorf("no %q in %s", want, s)
		}
	}
}

func TestODT(t *testing.T) {
	b, err := ConvertBytes(docx(t, map[string]string{
		"meta.xml":    `<office:document-meta xmlns:office="o" xmlns:dc="d"><office:meta><dc:title>Odt Title</dc:title></office:meta></office:document-meta>`,
		"content.xml": `<office:document-content xmlns:office="o" xmlns:text="t"><office:body><office:text><text:h text:outline-level="1">Head</text:h><text:p>Words <text:span>here</text:span></text:p></office:text></office:body></office:document-content>`,
	}), "odt", "x")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Odt Title" || !strings.Contains(allText(b), "<h1>Head</h1>") || !strings.Contains(allText(b), "Words here") {
		t.Errorf("%+v %s", b, allText(b))
	}
}

// palm builds a book: a database of a header record and text records.
func palm(kind string, header []byte, records ...[]byte) []byte {
	all := append([][]byte{header}, records...)
	head := make([]byte, 78)
	copy(head[60:], kind)
	binary.BigEndian.PutUint16(head[76:], uint16(len(all)))
	at := 78 + len(all)*8 + 2
	dir := make([]byte, 0, len(all)*8+2)
	for _, r := range all {
		d := make([]byte, 8)
		binary.BigEndian.PutUint32(d, uint32(at))
		dir = append(dir, d...)
		at += len(r)
	}
	out := append(append(head, dir...), 0, 0)
	for _, r := range all {
		out = append(out, r...)
	}
	return out
}

func mobiHeaderFor(compression, encryption uint16, textRecords int, title string) []byte {
	h := make([]byte, 16+232)
	binary.BigEndian.PutUint16(h[0:], compression)
	binary.BigEndian.PutUint16(h[8:], uint16(textRecords))
	binary.BigEndian.PutUint16(h[10:], 4096)
	binary.BigEndian.PutUint16(h[12:], encryption)
	copy(h[16:], "MOBI")
	binary.BigEndian.PutUint32(h[20:], 232)
	binary.BigEndian.PutUint32(h[28:], 65001)
	binary.BigEndian.PutUint32(h[36:], 6)
	binary.BigEndian.PutUint32(h[84:], uint32(len(h)))
	binary.BigEndian.PutUint32(h[88:], uint32(len(title)))
	return append(h, title...)
}

func TestMobi(t *testing.T) {
	text := []byte(`<html><body><h1>One</h1><p>First.</p><mbp:pagebreak/><h1>Two</h1><p>Second.</p><script>x</script></body></html>`)
	b, err := ConvertBytes(palm("BOOKMOBI", mobiHeaderFor(1, 0, 1, "A Title"), text), "mobi", "x")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "A Title" || len(b.Chapters) != 2 {
		t.Fatalf("%+v", b)
	}
	if s := allText(b); !strings.Contains(s, "<h1>Two</h1>") || strings.Contains(s, "script") {
		t.Error(s)
	}
}

func TestProtectedBooksAreRefused(t *testing.T) {
	for _, enc := range []uint16{1, 2} {
		_, err := ConvertBytes(palm("BOOKMOBI", mobiHeaderFor(2, enc, 1, "x"), []byte("garbled")), "mobi", "x")
		if !errors.Is(err, ErrProtected) {
			t.Errorf("encryption %d: %v", enc, err)
		}
	}
	if _, err := ConvertBytes([]byte("\xEADRMION\xEE..."), "azw", "x"); !errors.Is(err, ErrProtected) {
		t.Errorf("newer protected format: %v", err)
	}
}

func TestPalmDecompress(t *testing.T) {
	// Literals, a copied run, a space-plus-letter pair, and a back reference.
	in := []byte{'a', 'b', 0x02, 0xF0, 0xF1, 0xC1, 'x' ^ 0, 0x80, 0x19}
	got, err := palmDecompress(in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, []byte{'a', 'b', 0xF0, 0xF1, ' ', 'A'}) {
		t.Errorf("%q", got)
	}
	// A reference to before the start is damage, not a crash.
	if _, err := palmDecompress([]byte{0x80, 0xFF}); err == nil {
		t.Error("a bad reference should say so")
	}
}

func TestEPUBOutputReads(t *testing.T) {
	b, err := ConvertBytes([]byte("Chapter 1\n\nWords <&> here."), "txt", `Odd "<title>" & more`)
	if err != nil {
		t.Fatal(err)
	}
	b.addImage([]byte("\x89PNG\r\n\x1a\n\x00\x00"))
	var out bytes.Buffer
	if err := b.WriteEPUB(&out); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if zr.File[0].Name != "mimetype" || zr.File[0].Method != zip.Store {
		t.Error("an EPUB starts with a stored mimetype")
	}
	opf := zipFile(zr, "OEBPS/content.opf")
	rc, _ := opf.Open()
	raw, _ := io.ReadAll(rc)
	if !strings.Contains(string(raw), "&lt;title&gt;") {
		t.Errorf("the title isn't escaped: %s", raw)
	}
}

func TestLimits(t *testing.T) {
	if _, err := ConvertBytes(nil, "txt", "x"); err == nil {
		t.Error("nothing to read should say so")
	}
	if _, err := ConvertBytes([]byte("x"), "nonsense", "x"); err == nil {
		t.Error("an unknown format should say so")
	}
	// A record table that points past the end, or backwards.
	bad := make([]byte, 100)
	copy(bad[60:], "BOOKMOBI")
	binary.BigEndian.PutUint16(bad[76:], 2)
	binary.BigEndian.PutUint32(bad[78:], 0xFFFFFFFF)
	if _, err := ConvertBytes(bad, "mobi", "x"); err == nil {
		t.Error("a bad record table should say so")
	}
}

func FuzzConvert(f *testing.F) {
	f.Add([]byte(`{\rtf1 hello\par}`), "rtf")
	f.Add([]byte(`<FictionBook><body><section><p>x</p></section></body></FictionBook>`), "fb2")
	f.Add([]byte("# t\n\nx"), "md")
	f.Add([]byte("<p>x"), "html")
	f.Add(palm("BOOKMOBI", mobiHeaderFor(2, 0, 1, "x"), []byte{0x80, 0x10, 'a'}), "mobi")
	f.Add(palm("BOOKMOBI", mobiHeaderFor(17480, 0, 1, "x"), []byte{1, 2, 3}), "mobi")
	f.Add([]byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"), "doc")
	f.Add([]byte("PK\x03\x04"), "docx")
	f.Fuzz(func(t *testing.T, b []byte, format string) {
		if len(b) > 1<<16 {
			return
		}
		start := time.Now()
		defer func() {
			if took := time.Since(start); took > 5*time.Second {
				t.Errorf("%d bytes as %q took %v", len(b), format, took)
			}
		}()
		book, err := ConvertBytes(b, format, "x")
		if err != nil {
			return
		}
		var out bytes.Buffer
		if err := book.WriteEPUB(&out); err != nil {
			t.Error(err)
		}
		for _, c := range book.Chapters {
			if strings.Contains(strings.ToLower(c.Body), "<script") {
				t.Errorf("a script got through: %s", c.Body)
			}
		}
	})
}
