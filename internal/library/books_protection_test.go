package library

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ebook"
)

const (
	testContainer = `<container><rootfiles><rootfile full-path="OEBPS/book.opf"/></rootfiles></container>`
	testOPF       = `<package><metadata><title xmlns="http://purl.org/dc/elements/1.1/">A Made-up Book</title></metadata><manifest><item id="c" href="c.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c"/></spine></package>`
)

func encryptionXML(entries ...[2]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:enc="http://www.w3.org/2001/04/xmlenc#">`)
	for _, e := range entries {
		b.WriteString(`<enc:EncryptedData><enc:EncryptionMethod Algorithm="` + e[0] + `"/><enc:CipherData><enc:CipherReference URI="` + e[1] + `"/></enc:CipherData></enc:EncryptedData>`)
	}
	b.WriteString(`</encryption>`)
	return b.String()
}

func testEPUB(t *testing.T, extra map[string]string) *zip.Reader {
	t.Helper()
	files := map[string]string{"META-INF/container.xml": testContainer, "OEBPS/book.opf": testOPF, "OEBPS/c.xhtml": `<html><body>some words</body></html>`}
	for k, v := range extra {
		files[k] = v
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for k, v := range files {
		w, err := zw.Create(k)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(v))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return zr
}

const (
	idpfFonts  = "http://www.idpf.org/2008/embedding"
	adobeFonts = "http://ns.adobe.com/pdf/enc#RC"
	aes128     = "http://www.w3.org/2001/04/xmlenc#aes128-cbc"
	aes256     = "http://www.w3.org/2001/04/xmlenc#aes256-cbc"
)

func TestProtectedEPUBsAreRefused(t *testing.T) {
	ciphertext := string([]byte{0x00, 0xFF, 0x13, 0x80, 0xC0, 0x00, 0xFE, 0x01, 0x9C})
	cases := map[string]map[string]string{
		"a store's protection on its chapters": {
			"META-INF/encryption.xml": encryptionXML([2]string{aes128, "OEBPS/c.xhtml"}),
			"META-INF/rights.xml":     `<rights/>`,
		},
		"a licence-based protection": {
			"META-INF/encryption.xml": encryptionXML([2]string{aes256, "OEBPS/c.xhtml"}),
			"META-INF/license.lcpl":   `{}`,
		},
		"protection with its name in capitals": {
			"META-INF/ENCRYPTION.XML": encryptionXML([2]string{aes128, "OEBPS/c.xhtml"}),
		},
		"an algorithm nobody knows":    {"META-INF/encryption.xml": encryptionXML([2]string{"urn:made-up", "OEBPS/c.xhtml"})},
		"no algorithm at all":          {"META-INF/encryption.xml": encryptionXML([2]string{"", "OEBPS/c.xhtml"})},
		"pictures locked":              {"META-INF/encryption.xml": encryptionXML([2]string{aes128, "OEBPS/p1.jpg"})},
		"fonts and chapters both":      {"META-INF/encryption.xml": encryptionXML([2]string{idpfFonts, "OEBPS/f.otf"}, [2]string{aes128, "OEBPS/c.xhtml"})},
		"the font scramble on a chapter": {"META-INF/encryption.xml": encryptionXML([2]string{idpfFonts, "OEBPS/c.xhtml"})},
		"a list that can't be read, beside a licence": {
			"META-INF/encryption.xml": `<<<not xml`,
			"META-INF/rights.xml":     `<rights/>`,
		},
		"a list that can't be read, with chapters that aren't text": {
			"META-INF/encryption.xml": `<<<not xml`,
			"OEBPS/c.xhtml":           ciphertext,
		},
	}
	for what, extra := range cases {
		t.Run(what, func(t *testing.T) {
			_, err := ReadEPUB(testEPUB(t, extra))
			if !errors.Is(err, ebook.ErrProtected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

// What must simply work: a publisher's scrambled fonts are not protection of the book.
func TestUnprotectedEPUBsOpen(t *testing.T) {
	cases := map[string]map[string]string{
		"nothing listed":     nil,
		"the common scramble": {"META-INF/encryption.xml": encryptionXML([2]string{idpfFonts, "OEBPS/fonts/a.otf"})},
		"the other scramble":  {"META-INF/encryption.xml": encryptionXML([2]string{adobeFonts, "OEBPS/fonts/a.ttf"})},
		"many scrambled fonts": {"META-INF/encryption.xml": encryptionXML([2]string{idpfFonts, "OEBPS/a.otf"}, [2]string{adobeFonts, "OEBPS/b.woff"}, [2]string{idpfFonts, "OEBPS/c%20d.TTF"})},
		"a font locked and nothing else": {"META-INF/encryption.xml": encryptionXML([2]string{aes128, "OEBPS/a.otf"})},
		"an empty list":        {"META-INF/encryption.xml": encryptionXML()},
		"a damaged list, readable chapters": {"META-INF/encryption.xml": `<<<not xml`},
		"a store's leftovers on a book with no protection": {"META-INF/rights.xml": `<rights/>`},
	}
	for what, extra := range cases {
		t.Run(what, func(t *testing.T) {
			info, err := ReadEPUB(testEPUB(t, extra))
			if err != nil {
				t.Fatal(err)
			}
			if info.Title != "A Made-up Book" || len(info.Spine) != 1 {
				t.Errorf("read %+v", info)
			}
		})
	}
}

func TestPasswordEPUBIsRefused(t *testing.T) {
	zr := testEPUB(t, nil)
	zr.File[0].Flags |= 0x1
	if _, err := ReadEPUB(zr); !errors.Is(err, ebook.ErrEncrypted) {
		t.Errorf("got %v", err)
	}
}

// Text the check must not mistake for encrypted data.
func TestChaptersThatAreText(t *testing.T) {
	cut := bytes.Repeat([]byte("é"), 2048)[:4095] // ends halfway through a character
	for what, b := range map[string][]byte{
		"plain":         []byte("<html>words</html>"),
		"empty":         {},
		"cut at the end": cut,
		"UTF-16":        {0xFF, 0xFE, '<', 0, 'h', 0},
	} {
		if err := unreadableText(b); err != nil {
			t.Errorf("%s: %v", what, err)
		}
	}
}

// A locked book gives way to another copy of itself, whichever is found first.
func TestLockedEPUBGivesWay(t *testing.T) {
	root := t.TempDir()
	write := func(name string, extra map[string]string) string {
		t.Helper()
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		files := map[string]string{"META-INF/container.xml": testContainer, "OEBPS/book.opf": testOPF, "OEBPS/c.xhtml": `<html/>`}
		for k, v := range extra {
			files[k] = v
		}
		for k, v := range files {
			w, _ := zw.Create(k)
			_, _ = w.Write([]byte(v))
		}
		zw.Close()
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	locked := map[string]string{"META-INF/encryption.xml": encryptionXML([2]string{aes128, "OEBPS/c.xhtml"})}
	write("Locked.epub", locked)
	_ = os.WriteFile(filepath.Join(root, "Locked.pdf"), []byte("%PDF-1.7 a book"), 0o644)
	write("Alone.epub", locked) // no other copy: it stays, and says why when it's opened
	write("Open.epub", nil)
	_ = os.WriteFile(filepath.Join(root, "Open.pdf"), []byte("%PDF-1.7 a book"), 0o644)
	got, err := discoverBooks(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range got {
		names = append(names, filepath.Base(c.path))
	}
	if strings.Join(names, " ") != "Alone.epub Locked.pdf Open.epub" {
		t.Errorf("books: %v", names)
	}
}
