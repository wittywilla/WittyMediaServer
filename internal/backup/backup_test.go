// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

var (
	jpeg = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{7}, 200)...)
	ctx  = context.Background()
)

func mustExec(t *testing.T, d *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := d.ExecContext(ctx, q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func writeTestFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o640); err != nil {
		t.Fatal(err)
	}
}

// aServer makes a data folder with a little of everything in it.
func aServer(t *testing.T) (Folders, string) {
	t.Helper()
	dir := t.TempDir()
	f := Folders{Data: filepath.Join(dir, "data"), Images: filepath.Join(dir, "data", "cache", "images")}
	media := filepath.Join(dir, "media")
	if err := os.MkdirAll(f.Data, 0o750); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, d.DB, `INSERT INTO users(id, name, password_hash, is_admin, created_at, totp_secret, totp_on, recovery_codes) VALUES (1, 'kim', ?, 1, 1, 'TOTPPLANTEDSECRET', 1, 'RECOVERYPLANTED')`, string(hash))
	mustExec(t, d.DB, `INSERT INTO users(id, name, password_hash, is_admin, created_at) VALUES (2, 'sam', ?, 0, 1)`, string(hash))
	mustExec(t, d.DB, `INSERT INTO sessions(token, user_id, created_at, expires_at, device) VALUES ('SESSIONPLANTED', 1, 1, 9999999999, 'phone')`)
	mustExec(t, d.DB, `INSERT INTO libraries(id, name, type, created_at) VALUES (1, 'Films', 'movies', 1)`)
	mustExec(t, d.DB, `INSERT INTO library_paths(library_id, path) VALUES (1, ?)`, media)
	mustExec(t, d.DB, `INSERT INTO items(id, library_id, kind, path, title, sort_title, added_at, updated_at, size_bytes, mtime, poster) VALUES (1, 1, 'video', ?, 'The Treeline', 'treeline', 1, 1, 10, 1, 'cache:up-1.jpg')`, filepath.Join(media, "a.mkv"))
	mustExec(t, d.DB, `INSERT INTO items(id, library_id, kind, path, title, sort_title, added_at, updated_at, size_bytes, mtime) VALUES (2, 1, 'video', ?, 'Harbour Lights', 'harbour', 1, 1, 10, 1)`, filepath.Join(media, "b.mkv"))
	mustExec(t, d.DB, `INSERT INTO progress(user_id, item_id, position_sec, duration_sec, watched, updated_at) VALUES (1, 1, 120, 600, 0, 5)`)
	mustExec(t, d.DB, `INSERT INTO channels(id, number, name, enabled, definition, seed, rev, anchor_day, created_at, updated_at) VALUES (1, 7, 'Late Films', 1, '{}', 1, 1, 1, 1, 1)`)
	for k, v := range map[string]string{"tmdbKey": "TMDBPLANTEDKEY", "brandName": "Kim's server", "serverKey": "IDENTITYPLANTED", "javaPath": "/tmp/evil-java", "lyricsMirror": "http://192.168.1.2/", "remoteAccess": `{"enabled":true}`} {
		if err := d.SetSetting(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	d.Close()
	writeTestFile(t, filepath.Join(f.Images, "up-1.jpg"), jpeg)
	writeTestFile(t, filepath.Join(f.Images, "logos", "a.png"), []byte("\x89PNG\r\n\x1a\n1234567890"))
	writeTestFile(t, filepath.Join(f.Images, "not-a-picture.txt"), []byte("leftover"))
	writeTestFile(t, filepath.Join(f.Data, "gaming", "saves", "u1", "i1", "save.srm"), []byte("SAVEDATA"))
	writeTestFile(t, filepath.Join(f.Data, "gaming", "cores", "core.so"), []byte("\x7fELFcore"))
	writeTestFile(t, filepath.Join(f.Data, "branding", "logo.png"), []byte("\x89PNG\r\n\x1a\n1234567890"))
	writeTestFile(t, filepath.Join(f.Data, "modules", "state.json"), []byte(`{"wittydvd":{"active":"9.9.9","noAuto":true,"source":{"owner":"evil"}}}`))
	writeTestFile(t, filepath.Join(f.Data, "modules", "wittydvd", "versions", "9.9.9", "helper"), []byte("\x7fELFhelper"))
	writeTestFile(t, filepath.Join(f.Data, "metadata", "Films", "The Treeline [item-1].json"), []byte("{\n  \"id\": \"item-1\",\n  \"title\": \"The Treeline\"\n}\n"))
	writeTestFile(t, filepath.Join(f.Data, "lyrics-corrections", "1.json"), []byte(`{"lines":[]}`))
	writeTestFile(t, filepath.Join(f.Data, "logs", "wms.log"), []byte("LOGPLANTED"))
	return f, media
}

var everything = []string{PartLibrary, PartSettings, PartUsers, PartHistory, PartChannels, PartArtwork, PartSaves, PartModules, PartMetaFiles}

func entriesOf(t *testing.T, p string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = b
	}
	return out
}

func TestEveryTableIsClassified(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	names, err := objectNames(ctx, d.DB, "table")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if _, ok := tableParts[n]; !ok && !neverTables[n] {
			t.Errorf("table %s isn't in tableParts or neverTables in internal/backup/backup.go. Choose: which part of a backup is it, or does it stay out?", n)
		}
	}
}

func TestPlainBackupHoldsNoSecretsAndNoCode(t *testing.T) {
	f, _ := aServer(t)
	out, man, err := Create(ctx, Options{Folders: f, Parts: everything, ServerVersion: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if man.Protected || man.Crypt != nil {
		t.Fatal("no password was given")
	}
	all := entriesOf(t, out)
	for _, want := range []string{ManifestName, ReadmeName, dbEntry, "artwork/up-1.jpg", "artwork/logos/a.png", "saves/u1/i1/save.srm", "branding/logo.png", "modules/state.json"} {
		if _, ok := all[want]; !ok {
			t.Errorf("%s is missing", want)
		}
	}
	for name, b := range all {
		if strings.Contains(name, "core.so") || strings.Contains(name, "helper") || strings.Contains(name, "wms.log") || strings.HasSuffix(name, "not-a-picture.txt") || strings.HasPrefix(name, "lyrics-corrections") {
			t.Errorf("%s shouldn't be in it", name)
		}
		for _, planted := range []string{"TMDBPLANTEDKEY", "TOTPPLANTEDSECRET", "RECOVERYPLANTED", "SESSIONPLANTED", "IDENTITYPLANTED", "LOGPLANTED"} {
			if bytes.Contains(b, []byte(planted)) {
				t.Errorf("%s holds %s, even in free pages", name, planted)
			}
		}
	}
}

func TestProtectedBackupShowsNothing(t *testing.T) {
	f, _ := aServer(t)
	out, _, err := Create(ctx, Options{Folders: f, Parts: everything, Password: "a long password", ServerVersion: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	all := entriesOf(t, out)
	if len(all) != 3 {
		t.Fatalf("entries: %d", len(all))
	}
	for name, b := range all {
		// Nothing as short as an account's name: three bytes turn up in this much random data by chance.
		for _, planted := range []string{"The Treeline", "TMDBPLANTEDKEY", "SAVEDATA", "SQLite format 3"} {
			if bytes.Contains(b, []byte(planted)) {
				t.Errorf("%s shows %s in the open", name, planted)
			}
		}
	}
	if _, err := Inspect(ctx, out, "", f, nil); !errors.Is(err, ErrPasswordNeeded) {
		t.Fatalf("no password: %v", err)
	}
	if _, err := Inspect(ctx, out, "not the password", f, nil); !IsWrongPassword(err) {
		t.Fatalf("wrong password: %v", err)
	}
	in, err := Inspect(ctx, out, "a long password", f, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if !in.Protected || len(in.Accounts) != 2 {
		t.Fatalf("%+v", in)
	}
}

func TestProtectedPayloadCantBeChanged(t *testing.T) {
	var sealed bytes.Buffer
	k, err := DeriveKey("a long password")
	if err != nil {
		t.Fatal(err)
	}
	h, err := k.header()
	if err != nil {
		t.Fatal(err)
	}
	w, err := newSealWriter(&sealed, h, k.Secret)
	if err != nil {
		t.Fatal(err)
	}
	plain := bytes.Repeat([]byte("abcdefgh"), 400000) // over three chunks
	if _, err := w.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var back bytes.Buffer
	if err := openPayload(&back, bytes.NewReader(sealed.Bytes()), h, "a long password", 1<<40, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back.Bytes(), plain) {
		t.Fatal("not the same coming back")
	}
	chunk := 4 + chunkSize + 16
	cases := map[string][]byte{
		"cut off":        sealed.Bytes()[:chunk],
		"cut mid chunk":  sealed.Bytes()[:chunk+100],
		"a bit flipped":  append(append(append([]byte{}, sealed.Bytes()[:chunk+500]...), sealed.Bytes()[chunk+500]^1), sealed.Bytes()[chunk+501:]...),
		"chunks swapped": append(append(append([]byte{}, sealed.Bytes()[chunk:2*chunk]...), sealed.Bytes()[:chunk]...), sealed.Bytes()[2*chunk:]...),
		"extra at end":   append(append([]byte{}, sealed.Bytes()...), 1, 2, 3),
	}
	for name, b := range cases {
		if err := openPayload(io.Discard, bytes.NewReader(b), h, "a long password", 1<<40, nil); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	bad := *h
	bad.MemoryKiB = 1 << 30
	if err := bad.check(); err == nil {
		t.Error("a header asking for a terabyte of memory was accepted")
	}
}

// rewrap copies an archive, changing what change says, with a manifest that agrees.
func rewrap(t *testing.T, src, dst string, change func(name string, b []byte) ([]byte, bool), add map[string][]byte) {
	t.Helper()
	all := entriesOf(t, src)
	var man Manifest
	if err := json.Unmarshal(all[ManifestName], &man); err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for name, b := range all {
		if name == ManifestName {
			continue
		}
		if nb, keep := change(name, b); keep {
			out[name] = nb
		}
	}
	var files []FileEntry
	for name, b := range add {
		out[name] = b
	}
	for name, b := range out {
		if name == ReadmeName {
			continue
		}
		sum := sha256.Sum256(b)
		part := ""
		if s, _ := sourceFor(name); s != nil {
			part = s.Part
		}
		files = append(files, FileEntry{Path: name, Part: part, Size: int64(len(b)), SHA256: hex.EncodeToString(sum[:])})
	}
	man.Files = files
	mb, _ := json.Marshal(&man)
	out[ManifestName] = mb
	o, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	zw := zip.NewWriter(o)
	for name, b := range out {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// rawZip writes entries exactly as asked, for names and modes no honest tool writes.
func rawZip(t *testing.T, dst string, entries []*zip.FileHeader, body func(h *zip.FileHeader) []byte) {
	t.Helper()
	o, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	zw := zip.NewWriter(o)
	for _, h := range entries {
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body(h)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func manifestOnly() []byte {
	b, _ := json.Marshal(&Manifest{Format: FormatVersion, App: "WittyMediaServer", Schema: db.CurrentVersions(), Parts: []PartSummary{{ID: PartArtwork}}})
	return b
}

func wantRefused(t *testing.T, f Folders, p, what string) {
	t.Helper()
	in, err := Inspect(ctx, p, "", f, nil)
	if err == nil {
		in.Close()
		t.Errorf("%s: it was accepted", what)
		return
	}
	var r *Refusal
	if !errors.As(err, &r) {
		t.Errorf("%s: not a refusal: %v", what, err)
	}
	// Nothing outside the backups folder's own work area may have appeared.
	entries, _ := os.ReadDir(f.Dir())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".restore-check-") {
			t.Errorf("%s: left %s behind", what, e.Name())
		}
	}
}

// Every shape of hostile name, mode and size. Nothing may be accepted, and nothing may
// be written outside the work folder while it's looked at.
func TestHostileArchivesAreRefused(t *testing.T) {
	f, _ := aServer(t)
	dir := t.TempDir()
	cases := []struct {
		name string
		h    zip.FileHeader
	}{
		{"parent path", zip.FileHeader{Name: "artwork/../../evil.jpg"}},
		{"plain parent", zip.FileHeader{Name: "../evil.jpg"}},
		{"absolute", zip.FileHeader{Name: "/etc/passwd"}},
		{"backslash", zip.FileHeader{Name: `artwork\..\..\evil.jpg`}},
		{"drive letter", zip.FileHeader{Name: "C:/Windows/evil.jpg"}},
		{"alternate stream", zip.FileHeader{Name: "artwork/a.jpg:evil.exe"}},
		{"reserved name", zip.FileHeader{Name: "artwork/CON.jpg"}},
		{"trailing dot", zip.FileHeader{Name: "artwork/a.jpg."}},
		{"trailing space", zip.FileHeader{Name: "artwork/a.jpg "}},
		{"control character", zip.FileHeader{Name: "artwork/a\nb.jpg"}},
		{"text direction trick", zip.FileHeader{Name: "artwork/a\u202egpj.exe"}},
		{"empty folder name", zip.FileHeader{Name: "artwork//a.jpg"}},
		{"unknown folder", zip.FileHeader{Name: "secrets/a.jpg"}},
		{"a program", zip.FileHeader{Name: "saves/u1/run.exe"}},
		{"a script", zip.FileHeader{Name: "saves/u1/run.sh"}},
		{"a web page", zip.FileHeader{Name: "metadata/a.html"}},
		{"svg picture", zip.FileHeader{Name: "artwork/a.svg"}},
		{"module program", zip.FileHeader{Name: "modules/wittydvd/versions/1/helper"}},
	}
	for _, c := range cases {
		p := filepath.Join(dir, "case.zip")
		h := c.h
		h.Method = zip.Store
		rawZip(t, p, []*zip.FileHeader{{Name: ManifestName}, &h}, func(x *zip.FileHeader) []byte {
			if x.Name == ManifestName {
				return manifestOnly()
			}
			return jpeg
		})
		wantRefused(t, f, p, c.name)
	}

	// Modes: a link, or a device, in the ZIP's own directory.
	for name, mode := range map[string]os.FileMode{"symlink": os.ModeSymlink | 0o777, "device": os.ModeDevice | 0o600, "setuid": os.ModeSetuid | 0o755} {
		p := filepath.Join(dir, "mode.zip")
		h := &zip.FileHeader{Name: "artwork/link.jpg", Method: zip.Store}
		h.SetMode(mode)
		rawZip(t, p, []*zip.FileHeader{{Name: ManifestName}, h}, func(x *zip.FileHeader) []byte {
			if x.Name == ManifestName {
				return manifestOnly()
			}
			return []byte("/etc/passwd")
		})
		wantRefused(t, f, p, name)
	}

	// The same name twice, in different cases.
	p := filepath.Join(dir, "dupe.zip")
	rawZip(t, p, []*zip.FileHeader{{Name: ManifestName}, {Name: "artwork/a.jpg"}, {Name: "ARTWORK/A.JPG"}}, func(x *zip.FileHeader) []byte {
		if x.Name == ManifestName {
			return manifestOnly()
		}
		return jpeg
	})
	wantRefused(t, f, p, "duplicate")

	// Not a backup at all, a manifest from the future, junk.
	p = filepath.Join(dir, "junk.zip")
	writeTestFile(t, p, []byte("this is no zip"))
	wantRefused(t, f, p, "not a zip")
	rawZip(t, p, []*zip.FileHeader{{Name: "hello.txt"}}, func(*zip.FileHeader) []byte { return []byte("hi") })
	wantRefused(t, f, p, "no manifest")
	rawZip(t, p, []*zip.FileHeader{{Name: ManifestName}}, func(*zip.FileHeader) []byte {
		return []byte(`{"format":99,"app":"WittyMediaServer","serverVersion":"99.0"}`)
	})
	wantRefused(t, f, p, "newer format")
	rawZip(t, p, []*zip.FileHeader{{Name: ManifestName}}, func(*zip.FileHeader) []byte {
		b, _ := json.Marshal(&Manifest{Format: 1, App: "WittyMediaServer", Schema: db.Versions{Main: 99999}})
		return b
	})
	wantRefused(t, f, p, "newer database")
	rawZip(t, p, []*zip.FileHeader{{Name: ManifestName}}, func(*zip.FileHeader) []byte { return []byte(`{"format":1,"app":"Something Else"}`) })
	wantRefused(t, f, p, "someone else's")
}

// A ZIP bomb: small on disk, enormous when opened.
func TestZipBombsAreRefused(t *testing.T) {
	f, _ := aServer(t)
	p := filepath.Join(t.TempDir(), "bomb.zip")
	o, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(o)
	if w, err := zw.Create(ManifestName); err == nil {
		_, _ = w.Write(manifestOnly())
	}
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "saves/u1/big.srm", Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	zero := make([]byte, 1<<20)
	for i := 0; i < 64; i++ { // 64 MB of zeros, about 64 KB packed: nearly 1000 to 1
		if _, err := w.Write(zero); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	o.Close()
	wantRefused(t, f, p, "zip bomb")

	// Too many entries, and more than the total.
	oldE, oldT := maxEntries, maxTotalSize
	defer func() { maxEntries, maxTotalSize = oldE, oldT }()
	maxEntries = 2
	rawZip(t, p, []*zip.FileHeader{{Name: ManifestName}, {Name: "artwork/a.jpg"}, {Name: "artwork/b.jpg"}}, func(*zip.FileHeader) []byte { return jpeg })
	wantRefused(t, f, p, "too many entries")
	maxEntries = oldE
	maxTotalSize = 10
	wantRefused(t, f, p, "too much in total")
}

// A file that says less than it holds, by editing the sizes in a real ZIP's directory.
func TestSizesInTheDirectoryAreNotBelieved(t *testing.T) {
	f, _ := aServer(t)
	dir := t.TempDir()
	src, _, err := Create(ctx, Options{Folders: f, Parts: []string{PartArtwork}, Output: filepath.Join(dir, "good.zip")})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	// Central directory records hold the uncompressed size at offset 24.
	sig := []byte("PK\x01\x02")
	name := []byte("artwork/up-1.jpg")
	at := bytes.Index(b, sig)
	for at >= 0 && !bytes.Contains(b[at:min(at+46+len(name), len(b))], name) {
		next := bytes.Index(b[at+4:], sig)
		if next < 0 {
			t.Fatal("no directory entry")
		}
		at += 4 + next
	}
	b[at+24] = 10 // it holds 204 bytes
	bad := filepath.Join(dir, "lying.zip")
	writeTestFile(t, bad, b)
	wantRefused(t, f, bad, "a size that lies")
}

func TestChangesAfterTheBackupAreRefused(t *testing.T) {
	f, _ := aServer(t)
	dir := t.TempDir()
	src, _, err := Create(ctx, Options{Folders: f, Parts: everything, Output: filepath.Join(dir, "good.zip")})
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.zip")
	// With the manifest left alone: the hash catches it.
	all := entriesOf(t, src)
	o, err := os.Create(bad)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(o)
	for name, b := range all {
		w, _ := zw.Create(name)
		if name == "artwork/up-1.jpg" {
			b = append(append([]byte{}, b...), 1)
		}
		_, _ = w.Write(b)
	}
	zw.Close()
	o.Close()
	wantRefused(t, f, bad, "a changed picture")

	// A file the manifest doesn't list.
	o, _ = os.Create(bad)
	zw = zip.NewWriter(o)
	for name, b := range all {
		w, _ := zw.Create(name)
		_, _ = w.Write(b)
	}
	w, _ := zw.Create("artwork/added.jpg")
	_, _ = w.Write(jpeg)
	zw.Close()
	o.Close()
	wantRefused(t, f, bad, "an added file")

	// With a manifest that agrees, the contents are still judged.
	rewrap(t, src, bad, func(name string, b []byte) ([]byte, bool) {
		if name == "artwork/up-1.jpg" {
			return []byte("<svg onload=alert(1)>"), true
		}
		return b, true
	}, nil)
	wantRefused(t, f, bad, "not really a picture")
	rewrap(t, src, bad, func(name string, b []byte) ([]byte, bool) {
		if name == "saves/u1/i1/save.srm" {
			return []byte("\x7fELF\x02\x01\x01"), true
		}
		return b, true
	}, nil)
	wantRefused(t, f, bad, "a program posing as a save")
	rewrap(t, src, bad, func(_ string, b []byte) ([]byte, bool) { return b, true }, map[string][]byte{"themes/evil.zip": []byte("PK\x03\x04no")})
	wantRefused(t, f, bad, "a theme that isn't one")
}

// tamper runs SQL on the archive's database and returns a new archive with a manifest
// that agrees, which is what someone who wants this would do.
func tamper(t *testing.T, src, dst string, sqls ...string) {
	t.Helper()
	all := entriesOf(t, src)
	tmp := filepath.Join(t.TempDir(), "x.db")
	writeTestFile(t, tmp, all[dbEntry])
	d, err := db.OpenPlainFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range sqls {
		mustExec(t, d, q)
	}
	d.Close()
	nb, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	rewrap(t, src, dst, func(name string, b []byte) ([]byte, bool) {
		if name == dbEntry {
			return nb, true
		}
		return b, true
	}, nil)
}

func TestTamperedDatabasesAreRefused(t *testing.T) {
	f, _ := aServer(t)
	dir := t.TempDir()
	src, _, err := Create(ctx, Options{Folders: f, Parts: everything, Output: filepath.Join(dir, "good.zip")})
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.zip")
	for name, q := range map[string]string{
		"a view":             `CREATE VIEW sneaky AS SELECT * FROM users`,
		"a trigger":          `CREATE TRIGGER sneaky AFTER INSERT ON items BEGIN UPDATE users SET is_admin = 1; END`,
		"a virtual table":    `CREATE VIRTUAL TABLE sneaky USING fts5(a)`,
		"a computed column":  `ALTER TABLE items ADD COLUMN sneaky TEXT GENERATED ALWAYS AS (title) VIRTUAL`,
		"a wrong version":    `UPDATE schema SET version = 1`,
		"a version from far": `UPDATE schema SET version = 99999`,
	} {
		tamper(t, src, bad, q)
		wantRefused(t, f, bad, name)
	}
	// Not a database at all.
	rewrap(t, src, bad, func(name string, b []byte) ([]byte, bool) {
		if name == dbEntry {
			return []byte("DROP TABLE users; -- this is text"), true
		}
		return b, true
	}, nil)
	wantRefused(t, f, bad, "text instead of a database")
	// A database with its pages damaged.
	all := entriesOf(t, src)
	broken := append([]byte{}, all[dbEntry]...)
	for i := 4096; i < len(broken) && i < 4096*6; i += 3 {
		broken[i] ^= 0xFF
	}
	rewrap(t, src, bad, func(name string, b []byte) ([]byte, bool) {
		if name == dbEntry {
			return broken, true
		}
		return b, true
	}, nil)
	wantRefused(t, f, bad, "damaged pages")
}

// What can't be told apart from a real backup isn't refused: it's made harmless.
func TestHostileRowsAreMadeHarmless(t *testing.T) {
	f, media := aServer(t)
	dir := t.TempDir()
	src, _, err := Create(ctx, Options{Folders: f, Parts: everything, Output: filepath.Join(dir, "good.zip")})
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.zip")
	tamper(t, src, bad,
		`INSERT OR REPLACE INTO settings(key, value) VALUES ('javaPath', '/tmp/evil'), ('remoteAccess', '{"enabled":true}'), ('agents', '[{"url":"http://10.0.0.1/"}]'), ('logDir', '/etc')`,
		`INSERT INTO sessions(token, user_id, created_at, expires_at) VALUES ('planted', 1, 1, 9999999999)`,
		`INSERT INTO share_links(id, token_hash, user_id, kind, target_id, title, allow_download, expires_at, created_at) VALUES (9, 'planted', 1, 'item', 1, 'x', 1, 0, 1)`,
		`INSERT INTO items(id, library_id, kind, path, title, sort_title, added_at, updated_at, size_bytes, mtime) VALUES (50, 1, 'video', '/etc/passwd', 'x', 'x', 1, 1, 1, 1)`,
		`INSERT INTO items(id, library_id, kind, path, title, sort_title, added_at, updated_at, size_bytes, mtime) VALUES (51, 1, 'video', '`+filepath.ToSlash(media)+`/../../etc/shadow', 'x', 'x', 1, 1, 1, 1)`,
		`INSERT INTO library_paths(library_id, path) VALUES (1, '/etc'), (1, '/')`,
		`INSERT INTO library_paths(library_id, path) VALUES (1, '`+filepath.ToSlash(f.Data)+`/certs')`,
		`UPDATE items SET title = '<script>alert(1)</script>Treeline', overview = '<img src=x onerror=alert(1)>Plot', poster = '/etc/passwd', backdrop = 'javascript:alert(1)' WHERE id = 1`,
		`UPDATE items SET poster = 'cache:../../etc/passwd' WHERE id = 2`,
		`UPDATE users SET password_hash = 'plaintext' WHERE id = 2`,
	)
	in, err := Inspect(ctx, bad, "", f, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if _, err := in.Stage(ctx, f, StageOptions{Parts: everything}); err != nil {
		t.Fatal(err)
	}
	if r := Apply(f); r == nil || !r.OK {
		t.Fatalf("%+v", r)
	}
	d, err := db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	one := func(q string) string {
		var s sql.NullString
		if err := d.QueryRow(q).Scan(&s); err != nil {
			return "error: " + err.Error()
		}
		return s.String
	}
	for q, want := range map[string]string{
		`SELECT COUNT(*) FROM settings WHERE key IN ('javaPath','remoteAccess','agents','logDir')`: "0",
		`SELECT COUNT(*) FROM sessions`:                                                               "0",
		`SELECT COUNT(*) FROM share_links`:                                                            "0",
		`SELECT COUNT(*) FROM items WHERE id IN (50, 51)`:                                             "0",
		`SELECT COUNT(*) FROM library_paths WHERE path = '/etc' OR path = '/' OR path LIKE '%/certs'`: "0",
		`SELECT title FROM items WHERE id = 1`:                                                        "Treeline",
		`SELECT overview FROM items WHERE id = 1`:                                                     "Plot",
		`SELECT poster FROM items WHERE id = 1`:                                                       "",
		`SELECT backdrop FROM items WHERE id = 1`:                                                     "",
		`SELECT poster FROM items WHERE id = 2`:                                                       "",
		`SELECT password_hash FROM users WHERE id = 2`:                                                "",
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name NOT IN ('title_meta_item_gone', 'title_meta_show_gone', 'listings_item_gone', 'listings_show_gone', 'share_links_item_gone', 'share_links_album_gone', 'profile_watch_new', 'profile_watch_more', 'credits_item_gone', 'credits_show_gone')`: "0",
	} {
		if got := one(q); got != want {
			t.Errorf("%s: %q, want %q", q, got, want)
		}
	}
	if !containsWord(in.Dropped, "/etc") {
		t.Errorf("a person wasn't told: %v", in.Dropped)
	}
}

func containsWord(list []string, w string) bool {
	for _, s := range list {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// listTree says what's in a folder, as path and size, leaving out what's made fresh.
func listTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "backups/") || strings.HasPrefix(rel, "restore-") || strings.HasPrefix(rel, "wms.db") {
				return nil
			}
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

// The whole round: back up, wipe, restore, and everything's back, with and without a
// password. A safety copy holds what was there, and can be brought back.
func TestFullRoundTrip(t *testing.T) {
	for _, password := range []string{"", "a long password"} {
		t.Run(fmt.Sprintf("password=%v", password != ""), func(t *testing.T) {
			f, _ := aServer(t)
			out, man, err := Create(ctx, Options{Folders: f, Parts: everything, Password: password, SecretSettings: []string{"tmdbKey"}, ServerVersion: "1.0"})
			if err != nil {
				t.Fatal(err)
			}
			if len(man.Parts) != len(everything) {
				t.Fatalf("parts: %+v", man.Parts)
			}
			// As a reset would leave it, with a new account that must go.
			keep := filepath.Join(t.TempDir(), "keep.zip")
			if err := os.Rename(out, keep); err != nil {
				t.Fatal(err)
			}
			for _, p := range []string{"wms.db", "wms.db-wal", "wms.db-shm", "gaming", "cache", "branding", "modules", "metadata", "lyrics-corrections"} {
				_ = os.RemoveAll(filepath.Join(f.Data, p))
			}
			d, err := db.Open(f.Data)
			if err != nil {
				t.Fatal(err)
			}
			mustExec(t, d.DB, `INSERT INTO users(id, name, password_hash, is_admin, created_at) VALUES (1, 'newcomer', 'x', 1, 1)`)
			d.Close()

			in, err := Inspect(ctx, keep, password, f, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer in.Close()
			if len(in.Accounts) != 2 || in.Accounts[0].Name != "kim" || !in.Accounts[0].Admin {
				t.Fatalf("accounts: %+v", in.Accounts)
			}
			if len(in.Libraries) != 1 || in.Libraries[0].Items != 2 {
				t.Fatalf("libraries: %+v", in.Libraries)
			}
			if _, err := in.Stage(ctx, f, StageOptions{Parts: everything}); err != nil {
				t.Fatal(err)
			}
			// Nothing has changed yet, and it can still be taken back.
			if _, err := os.Stat(filepath.Join(f.Data, "branding")); err == nil {
				t.Fatal("restored before the next start")
			}
			r := Apply(f)
			if r == nil || !r.OK {
				t.Fatalf("%+v", r)
			}
			for _, p := range []string{"cache/images/up-1.jpg", "cache/images/logos/a.png", "gaming/saves/u1/i1/save.srm", "branding/logo.png", "modules/state.json", "metadata/Films/The Treeline [item-1].json"} {
				if _, err := os.Stat(filepath.Join(f.Data, filepath.FromSlash(p))); err != nil {
					t.Errorf("%s isn't back: %v", p, err)
				}
			}
			state, err := os.ReadFile(filepath.Join(f.Data, "modules", "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(state), "noAuto") || strings.Contains(string(state), "active") || strings.Contains(string(state), "evil") {
				t.Errorf("module settings came back with more than settings: %s", state)
			}
			d, err = db.Open(f.Data)
			if err != nil {
				t.Fatal(err)
			}
			count := func(q string) (n int) {
				if err := d.QueryRow(q).Scan(&n); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
				return n
			}
			for q, want := range map[string]int{
				`SELECT COUNT(*) FROM users WHERE name = 'newcomer'`:                                        0,
				`SELECT COUNT(*) FROM users WHERE name = 'kim' AND is_admin = 1`:                            1,
				`SELECT COUNT(*) FROM items`:                                                                2,
				`SELECT COUNT(*) FROM progress WHERE position_sec = 120`:                                    1,
				`SELECT COUNT(*) FROM channels WHERE name = 'Late Films'`:                                   1,
				`SELECT COUNT(*) FROM sessions`:                                                             0,
				`SELECT COUNT(*) FROM settings WHERE key = 'brandName'`:                                     1,
				`SELECT COUNT(*) FROM settings WHERE key = 'javaPath'`:                                      0,
				`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name = 'listings_item_gone'`: 1,
			} {
				if got := count(q); got != want {
					t.Errorf("%s: %d, want %d", q, got, want)
				}
			}
			wantSecrets := 0
			if password != "" {
				wantSecrets = 1
			}
			if got := count(`SELECT COUNT(*) FROM settings WHERE key = 'tmdbKey' AND value = 'TMDBPLANTEDKEY'`); got != wantSecrets {
				t.Errorf("API keys: %d, want %d", got, wantSecrets)
			}
			if got := count(`SELECT COUNT(*) FROM users WHERE totp_secret = 'TOTPPLANTEDSECRET' AND totp_on = 1`); got != wantSecrets {
				t.Errorf("two-factor: %d, want %d", got, wantSecrets)
			}
			d.Close()

			// Undo: the newcomer comes back, and what was restored is kept aside instead.
			if r.Safety == "" {
				t.Fatal("no safety copy")
			}
			if err := AskUndo(f, r.Safety); err != nil {
				t.Fatal(err)
			}
			if r := Apply(f); r == nil || !r.OK {
				t.Fatalf("undo: %+v", r)
			}
			d, err = db.Open(f.Data)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			var name string
			if err := d.QueryRow(`SELECT name FROM users`).Scan(&name); err != nil || name != "newcomer" {
				t.Fatalf("undo: %q %v", name, err)
			}
			if _, err := os.Stat(filepath.Join(f.Data, "branding")); err == nil {
				t.Error("undo left what was restored")
			}
		})
	}
}

// Only what was chosen changes. Numbers that depend on what's replaced can't stay.
func TestPartialRestore(t *testing.T) {
	f, _ := aServer(t)
	dir := t.TempDir()
	out, _, err := Create(ctx, Options{Folders: f, Parts: everything, Output: filepath.Join(dir, "b.zip")})
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, d.DB, `UPDATE items SET title = 'Changed since' WHERE id = 1`)
	mustExec(t, d.DB, `UPDATE settings SET value = 'Changed name' WHERE key = 'brandName'`)
	mustExec(t, d.DB, `INSERT INTO custom_consoles(id, data, aliases, cores, created_at, updated_at) VALUES ('c', '{}', '', '', 1, 1)`)

	in, err := Inspect(ctx, out, "", f, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	// Asking for something without what it depends on is refused.
	if _, err := in.Stage(ctx, f, StageOptions{Parts: []string{PartHistory}, LiveDB: d.DB}); err == nil {
		t.Fatal("watch history without the library was allowed")
	}
	if _, err := in.Stage(ctx, f, StageOptions{Parts: []string{PartSettings}, LiveDB: d.DB}); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if r := Apply(f); r == nil || !r.OK {
		t.Fatalf("%+v", r)
	}
	d, err = db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	one := func(q string) string {
		var s string
		if err := d.QueryRow(q).Scan(&s); err != nil {
			return err.Error()
		}
		return s
	}
	if got := one(`SELECT value FROM settings WHERE key = 'brandName'`); got != "Kim's server" {
		t.Errorf("settings weren't restored: %q", got)
	}
	if got := one(`SELECT title FROM items WHERE id = 1`); got != "Changed since" {
		t.Errorf("the library changed: %q", got)
	}
	if got := one(`SELECT COUNT(*) FROM sessions`); got != "1" {
		t.Errorf("sign-ins should stay when accounts aren't replaced: %q", got)
	}
	if got := one(`SELECT COUNT(*) FROM custom_consoles`); got != "1" {
		t.Errorf("what no backup holds should stay: %q", got)
	}
	if got := one(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger'`); got != "10" {
		t.Errorf("triggers: %q", got)
	}
}

// If the swap can't finish, everything goes back. And after a power cut, the next start
// finds the journal and does the same.
func TestRollback(t *testing.T) {
	f, _ := aServer(t)
	dir := t.TempDir()
	out, _, err := Create(ctx, Options{Folders: f, Parts: everything, Output: filepath.Join(dir, "b.zip")})
	if err != nil {
		t.Fatal(err)
	}
	in, err := Inspect(ctx, out, "", f, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if _, err := in.Stage(ctx, f, StageOptions{Parts: everything}); err != nil {
		t.Fatal(err)
	}
	before := listTree(t, f.Data)
	// A database that doesn't open makes it a failed restore, whatever else went well.
	writeTestFile(t, filepath.Join(f.Data, stagedName, "database", "wms.db"), []byte("not a database at all, sorry"))
	r := Apply(f)
	if r == nil || r.OK || !strings.Contains(r.Error, "put back") {
		t.Fatalf("%+v", r)
	}
	if got := listTree(t, f.Data); strings.Join(got, "\n") != strings.Join(before, "\n") {
		t.Errorf("not as it was:\n%v\n%v", before, got)
	}
	if Pending(f) != nil {
		t.Error("the request is still there")
	}
	d, err := db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM users WHERE name = 'kim'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("the database didn't come back: %d %v", n, err)
	}
	d.Close()

	// A power cut: the journal says it's part way.
	if _, err := in.Stage(ctx, f, StageOptions{Parts: everything}); err != nil {
		t.Fatal(err)
	}
	req := Pending(f)
	safety := filepath.Join(f.Dir(), "before-restore-20260101-000000")
	if err := os.MkdirAll(safety, 0o700); err != nil {
		t.Fatal(err)
	}
	j := &journal{State: "applying", Mode: "restore", Parts: req.Parts, Swaps: planSwaps(f, safety, req)}
	for i := range j.Swaps {
		if i > 3 {
			break
		}
		s := &j.Swaps[i]
		if _, err := os.Lstat(s.Target); err == nil {
			if err := moveWithin(s.Target, s.Aside); err != nil {
				t.Fatal(err)
			}
			s.Moved = true
		}
		if _, err := os.Lstat(s.Source); s.Source != "" && err == nil {
			if err := moveWithin(s.Source, s.Target); err != nil {
				t.Fatal(err)
			}
			s.In = true
		}
	}
	if err := saveJournal(safety, j); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.Data, requestName)); err != nil {
		t.Fatal(err)
	}
	if r := Apply(f); r == nil || !r.OK || r.Mode != "recovery" {
		t.Fatalf("%+v", r)
	}
	if got := listTree(t, f.Data); strings.Join(got, "\n") != strings.Join(before, "\n") {
		t.Errorf("not as it was after a power cut:\n%v\n%v", before, got)
	}
}

// An older database comes forward by the migrations a server would run.
func TestOlderDatabaseIsMigrated(t *testing.T) {
	f, _ := aServer(t)
	dir := t.TempDir()
	cur := db.CurrentVersions()
	older := cur
	older.Metadata = 1
	older.Music = 0
	olderFile := filepath.Join(dir, "older.db")
	d, err := db.BuildAt(ctx, olderFile, older)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, d, `INSERT INTO libraries(id, name, type, created_at) VALUES (1, 'Old Films', 'movies', 1)`)
	d.Close()
	raw, err := os.ReadFile(olderFile)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	man, _ := json.Marshal(&Manifest{Format: 1, App: "WittyMediaServer", ServerVersion: "0.1", Schema: older, Parts: []PartSummary{{ID: PartLibrary}},
		Files: []FileEntry{{Path: dbEntry, Size: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])}}})
	p := filepath.Join(dir, "older.zip")
	rawZip(t, p, []*zip.FileHeader{{Name: ManifestName}, {Name: dbEntry}}, func(h *zip.FileHeader) []byte {
		if h.Name == ManifestName {
			return man
		}
		return raw
	})
	in, err := Inspect(ctx, p, "", f, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if !in.Older || len(in.Libraries) != 1 {
		t.Fatalf("%+v", in)
	}
	if _, err := in.Stage(ctx, f, StageOptions{Parts: []string{PartLibrary}}); err != nil {
		t.Fatal(err)
	}
	if r := Apply(f); r == nil || !r.OK {
		t.Fatalf("%+v", r)
	}
	nd, err := db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer nd.Close()
	v, err := db.ReadVersions(ctx, nd.DB, "")
	if err != nil || v != cur {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestScheduleKeepsTheNewest(t *testing.T) {
	f, _ := aServer(t)
	if err := os.MkdirAll(f.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("wms-backup-auto-2026010%d-000000.zip", i+1)
		writeTestFile(t, filepath.Join(f.Dir(), name), []byte("x"))
		at := time.Now().Add(time.Duration(i-10) * time.Hour)
		_ = os.Chtimes(filepath.Join(f.Dir(), name), at, at)
	}
	writeTestFile(t, filepath.Join(f.Dir(), "wms-backup-20200101-000000.zip"), []byte("by hand"))
	removed := Prune(f, 2)
	if len(removed) != 3 {
		t.Fatalf("%v", removed)
	}
	var left []string
	for _, b := range List(f) {
		left = append(left, b.Name)
	}
	sort.Strings(left)
	if strings.Join(left, ",") != "wms-backup-20200101-000000.zip,wms-backup-auto-20260104-000000.zip,wms-backup-auto-20260105-000000.zip" {
		t.Fatalf("%v", left)
	}
}

func TestBackupNamesCantBePaths(t *testing.T) {
	for _, n := range []string{"../wms-backup-x.zip", `..\wms-backup-x.zip`, "wms-backup-/../x.zip", "/etc/passwd", "wms-backup-x.zip/../../y", "x.zip", "wms-backup-X.zip", "wms-backup-..zip"} {
		if ValidName(n) {
			t.Errorf("%q passed", n)
		}
	}
	if !ValidName("wms-backup-20260101-000000.zip") {
		t.Error("a real one failed")
	}
}

// What a core worked out for itself is made again for nothing and can run to gigabytes.
func TestACoresCacheStaysOutOfABackup(t *testing.T) {
	for rel, want := range map[string]bool{"3/12/system/pcsx2/cache/shaders.bin": true, "system/pcsx2/cache/a": true, "3/12/system/pcsx2/memcards/card.ps2": false, "3/12/save.srm": false, "3/cache/x": false} {
		if got := isCoreCache(rel); got != want {
			t.Errorf("%s: %v", rel, got)
		}
	}
}
