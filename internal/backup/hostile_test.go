package backup

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Settings that would let an archive change what this server may do, or who it is. Nothing checks
// a file with no password, so nothing secret or machine-wide is taken from one.
func TestPlainArchiveCantPlantSecretsOrSwitches(t *testing.T) {
	f, _ := aServer(t)
	dir := t.TempDir()
	src, _, err := Create(ctx, Options{Folders: f, Parts: everything, Output: filepath.Join(dir, "good.zip")})
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.zip")
	tamper(t, src, bad,
		`INSERT OR REPLACE INTO settings(key, value) VALUES ('serverKey', 'PLANTED'), ('serverId', 'PLANTED'), ('tmdbKey', 'PLANTED'), ('libtranscode.tdarrKey', 'PLANTED'), ('gamingCustomCores', '1'), ('brandName', 'Kept')`,
		`INSERT INTO ltc_rules(library_id, policy, output, allow_writes, updated_at, auto, auto_since) VALUES (1, '{}', 'replace', 1, 1, 1, 1)`,
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
	for q, want := range map[string]string{
		`SELECT COUNT(*) FROM settings WHERE value = 'PLANTED'`:                      "0",
		`SELECT COUNT(*) FROM settings WHERE key = 'gamingCustomCores'`:              "0",
		`SELECT value FROM settings WHERE key = 'brandName'`:                         "Kept",
		`SELECT auto FROM ltc_rules WHERE library_id = 1`:                            "0",
		`SELECT allow_writes FROM ltc_rules WHERE library_id = 1`:                    "0",
		`SELECT COUNT(*) FROM settings WHERE key LIKE 'schema.%' AND value = 'none'`: "0",
	} {
		var s sql.NullString
		if err := d.QueryRow(q).Scan(&s); err != nil {
			t.Errorf("%s: %v", q, err)
		} else if s.String != want {
			t.Errorf("%s: %q, want %q", q, s.String, want)
		}
	}
	if !containsWord(in.Dropped, "convert") {
		t.Errorf("a person wasn't told about the rule that went off: %v", in.Dropped)
	}
}

// A share on another machine: scanning it sends this machine's sign-in there. A browser can't name
// one unless the owner allows it, and neither can a file with no password. Device paths get round
// every check on the words of a path, so nothing brings those.
func TestArchiveCantNameAnotherMachine(t *testing.T) {
	f, _ := aServer(t)
	for _, c := range []struct {
		path, why string
	}{
		{`\\?\C:\Windows\System32`, "device"},
		{`\\.\C:\Windows`, "device"},
		{`//?/C:/Windows`, "device"},
	} {
		if got := dangerousRoot(c.path, f); !strings.Contains(got, c.why) {
			t.Errorf("%s: %q", c.path, got)
		}
	}
	dir := t.TempDir()
	src, _, err := Create(ctx, Options{Folders: f, Parts: everything, Output: filepath.Join(dir, "good.zip")})
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.zip")
	tamper(t, src, bad, `INSERT INTO library_paths(library_id, path) VALUES (1, '\\attacker.example\share'), (1, '//attacker.example/share')`)
	in, err := Inspect(ctx, bad, "", f, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	for _, l := range in.Libraries {
		for _, p := range l.Folders {
			if strings.Contains(p, "attacker") {
				t.Errorf("kept: %s", p)
			}
		}
	}
	if !containsWord(in.Dropped, "another machine") {
		t.Errorf("a person wasn't told: %v", in.Dropped)
	}
}

// Its numbers are read before anything is proven, and all this server's own backups ask for 64 MB.
func TestHeaderCantAskForMuchMemory(t *testing.T) {
	h := CryptHeader{Alg: "AES-256-GCM", KDF: "argon2id", Time: 3, MemoryKiB: 256 << 10, Threads: 2, Salt: "c2FsdHNhbHQ=", Prefix: "AAAAAAAAAA==", Chunk: chunkSize}
	if err := h.check(); err == nil {
		t.Error("256 MB was accepted")
	}
	h.MemoryKiB = kdfMemory
	if err := h.check(); err != nil {
		t.Errorf("what this server makes: %v", err)
	}
}

// What's used is what's read at the second pass, so it has to meet the same rules. The file can
// change between checking and restoring: another upload, or someone with the disk.
func TestStageChecksTheArchiveAgain(t *testing.T) {
	f, _ := aServer(t)
	dir := t.TempDir()
	src, _, err := Create(ctx, Options{Folders: f, Parts: []string{PartChannels, PartLibrary}, Output: filepath.Join(dir, "good.zip")})
	if err != nil {
		t.Fatal(err)
	}
	in, err := Inspect(ctx, src, "", f, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	swapped := filepath.Join(dir, "swapped.zip")
	all := entriesOf(t, src)
	o, err := os.Create(swapped)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(o)
	for name, b := range all {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	w, err := zw.Create("bumpers/added.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("\x7fELF a program")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	o.Close()
	if err := os.Rename(swapped, src); err != nil {
		t.Fatal(err)
	}
	_, err = in.Stage(ctx, f, StageOptions{Parts: []string{PartChannels, PartLibrary}})
	var r *Refusal
	if !errors.As(err, &r) {
		t.Fatalf("a program added after the check was unpacked: %v", err)
	}
	if _, serr := os.Stat(filepath.Join(f.Data, stagedName, "bumpers", "added.bin")); serr == nil {
		t.Error("it's still there")
	}
}

// The database comes out before anything has looked at how much room it needs, and most of what a
// ZIP bomb's ratio limit lets through is exactly this.
func TestDatabaseWontFillTheDisk(t *testing.T) {
	f, _ := aServer(t)
	src, _, err := Create(ctx, Options{Folders: f, Parts: []string{PartSettings}, Output: filepath.Join(t.TempDir(), "good.zip")})
	if err != nil {
		t.Fatal(err)
	}
	old := freeBytes
	defer func() { freeBytes = old }()
	freeBytes = func(string) int64 { return 1 }
	in, err := Inspect(ctx, src, "", f, nil)
	if in != nil {
		in.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "room") {
		t.Fatalf("with no room: %v", err)
	}
	entries, _ := os.ReadDir(f.Dir())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".restore-check-") {
			t.Errorf("%s was left behind", e.Name())
		}
	}
}

// Listing the backups reads each one's manifest, and an upload is listed like any other. This must
// not mean unpacking something enormous each time the page is opened.
func TestListingDoesntReadAHugeManifest(t *testing.T) {
	f, _ := aServer(t)
	if err := os.MkdirAll(f.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(f.Dir(), "wms-backup-upload-1.zip")
	o, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(o)
	w, err := zw.Create(ManifestName)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := json.Marshal(&Manifest{Format: 1, App: "WittyMediaServer", ServerVersion: "should-not-be-read"})
	if _, err := w.Write(append(m, bytes.Repeat([]byte(" "), int(maxListedManifest)+1)...)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	o.Close()
	list := List(f)
	if len(list) != 1 || list[0].Version != "" {
		t.Fatalf("%+v", list)
	}
	if time.Since(list[0].Created) > time.Hour {
		t.Errorf("it should still have a date: %v", list[0].Created)
	}
}
