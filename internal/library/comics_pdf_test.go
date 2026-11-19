// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An encrypted PDF's title and author are encrypted too: the file's name stays, rather than
// what's left of them ("a5" by "!*D`FS|`S+," on a NASA monograph).
func TestEncryptedPDFKeepsItsFileName(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	info := "1 0 obj << /Title (\x1d\xd2\xeba5) /Author (!*D`FS|`S+,) >> endobj\n"
	for name, body := range map[string]string{
		"head.pdf":  "%PDF-1.6\n" + info + "trailer << /Info 1 0 R /Encrypt 9 0 R >>\n",
		"tail.pdf":  "%PDF-1.6\n" + info + strings.Repeat(" ", 100) + "trailer << /Info 1 0 R /Encrypt << /Filter /Standard >> >>\n",
		"plain.pdf": "%PDF-1.6\n" + info + "trailer << /Info 1 0 R >>\n",
	} {
		it := &Item{Path: write(name, body), Title: "From The File"}
		readPDFMeta(it)
		if name != "plain.pdf" && (it.Title != "From The File" || it.Artist != "") {
			t.Errorf("%s: title %q, author %q", name, it.Title, it.Artist)
		}
		// Not encrypted, but its strings still aren't text: the control character keeps it out.
		if name == "plain.pdf" && it.Title != "From The File" {
			t.Errorf("%s: title %q", name, it.Title)
		}
	}
	it := &Item{Path: write("good.pdf", "%PDF-1.4\n1 0 obj << /Title (A Real Title) /Author (Someone) >> endobj\ntrailer << /Info 1 0 R >>\n")}
	readPDFMeta(it)
	if it.Title != "A Real Title" || it.Artist != "Someone" {
		t.Errorf("plain PDF: %q by %q", it.Title, it.Artist)
	}
}

// A bookmark's /Title can come before the document's: the trailer's /Info says which is the
// document's own.
func TestPDFTitleIsTheInfoDictionarys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "b.pdf")
	body := "%PDF-1.4\n3 0 obj << /Title (Chapter One) /Parent 2 0 R >> endobj\n" +
		"7 0 obj << /Title (The Real Book) /Author (Someone) >> endobj\n" +
		"trailer << /Root 1 0 R /Info 7 0 R >>\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	it := &Item{Path: p}
	readPDFMeta(it)
	if it.Title != "The Real Book" || it.Artist != "Someone" {
		t.Errorf("%q by %q", it.Title, it.Artist)
	}
}
