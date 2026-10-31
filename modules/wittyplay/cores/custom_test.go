package cores

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wittywilla/wittyplay/testcore/fakecore"
)

func testStore(t *testing.T) *CustomStore {
	t.Helper()
	dir := t.TempDir()
	return &CustomStore{Dir: filepath.Join(dir, "custom"), System: filepath.Join(dir, "system"), Placed: []string{filepath.Join(dir, "cores")},
		Limits: Limits{Library: 1 << 24, Package: 1 << 24, Unpack: 1 << 24, Entries: 200}}
}

// goodCoreBytes is a library that passes every check made before a test-load.
func goodCoreBytes(t *testing.T) []byte {
	t.Helper()
	p, err := fakecore.Build(t.TempDir(), "x", fakecore.Good)
	if err != nil {
		t.Skip(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type zipEntry struct {
	name string
	body []byte
	mode os.FileMode
}

func makeZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			h.SetMode(e.mode)
		}
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestParseBinary(t *testing.T) {
	pe := func(machine uint16, chars uint16) []byte {
		b := make([]byte, 0x100)
		copy(b, "MZ")
		binary.LittleEndian.PutUint32(b[60:], 0x40)
		copy(b[0x40:], "PE\x00\x00")
		binary.LittleEndian.PutUint16(b[0x44:], machine)
		binary.LittleEndian.PutUint16(b[0x44+18:], chars)
		return b
	}
	elf := func(class, data byte, ty, machine uint16) []byte {
		b := make([]byte, 64)
		copy(b, "\x7fELF")
		b[4], b[5] = class, data
		binary.LittleEndian.PutUint16(b[16:], ty)
		binary.LittleEndian.PutUint16(b[18:], machine)
		return b
	}
	macho := func(cpu, ty uint32) []byte {
		b := make([]byte, 64)
		binary.LittleEndian.PutUint32(b, 0xfeedfacf)
		binary.LittleEndian.PutUint32(b[4:], cpu)
		binary.LittleEndian.PutUint32(b[12:], ty)
		return b
	}
	fat := func() []byte {
		b := make([]byte, 0x2000)
		binary.BigEndian.PutUint32(b, 0xcafebabe)
		binary.BigEndian.PutUint32(b[4:], 2)
		binary.BigEndian.PutUint32(b[8:], 0x01000007)
		binary.BigEndian.PutUint32(b[16:], 0x1000)
		binary.BigEndian.PutUint32(b[28:], 0x0100000c)
		binary.BigEndian.PutUint32(b[36:], 0x1800)
		binary.LittleEndian.PutUint32(b[0x180c:], 6)
		binary.LittleEndian.PutUint32(b[0x100c:], 6)
		return b
	}
	for _, c := range []struct {
		name         string
		file         []byte
		goos, goarch string
		matches, lib bool
	}{
		{"windows core on windows", pe(0x8664, 0x2022), "windows", "amd64", true, true},
		{"windows program, not a library", pe(0x8664, 0x0022), "windows", "amd64", true, false},
		{"32-bit windows core", pe(0x14c, 0x2022), "windows", "amd64", false, true},
		{"windows core on linux", pe(0x8664, 0x2022), "linux", "amd64", false, true},
		{"linux core on linux", elf(2, 1, 3, 62), "linux", "amd64", true, true},
		{"linux program", elf(2, 1, 2, 62), "linux", "amd64", true, false},
		{"arm core on x86", elf(2, 1, 3, 183), "linux", "amd64", false, true},
		{"linux core on windows", elf(2, 1, 3, 62), "windows", "amd64", false, true},
		{"mac core on a mac", macho(0x0100000c, 6), "darwin", "arm64", true, true},
		{"intel mac core on arm", macho(0x01000007, 6), "darwin", "arm64", false, true},
		{"universal mac core", fat(), "darwin", "arm64", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseBinary(bytes.NewReader(c.file), c.goos, c.goarch)
			if err != nil {
				t.Fatal(err)
			}
			if got.Matches != c.matches || got.Library != c.lib {
				t.Errorf("got %+v", got)
			}
		})
	}
	for _, c := range []struct {
		name string
		file []byte
		says string
	}{
		{"empty", nil, "empty"},
		{"text", []byte("hello there, this is not a library"), "isn't a program library"},
		{"web page", []byte("<!DOCTYPE html><html>"), "web page"},
		{"script", []byte("#!/bin/sh\nrm -rf /"), "script"},
		{"zip", []byte("PK\x03\x04aaaa"), "zip"},
		{"gzip", []byte{0x1f, 0x8b, 8, 0, 0, 0, 0, 0}, "gzip"},
		{"damaged windows header", append([]byte("MZ"), make([]byte, 100)...), "damaged"},
		{"windows header pointing nowhere", func() []byte { b := pe(0x8664, 0x2022); binary.LittleEndian.PutUint32(b[60:], 0xfffffff0); return b }(), "damaged"},
		{"java class", func() []byte {
			b := make([]byte, 16)
			binary.BigEndian.PutUint32(b, 0xcafebabe)
			binary.BigEndian.PutUint32(b[4:], 65)
			return b
		}(), "isn't a program library"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseBinary(bytes.NewReader(c.file), "linux", "amd64")
			if err == nil || !strings.Contains(err.Error(), c.says) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestStageRefusesHostileUploads(t *testing.T) {
	good := goodCoreBytes(t)
	for _, c := range []struct {
		name, file string
		body       []byte
		says       string
	}{
		{"climbs out", "../../evil_libretro" + libraryExt(), good, "climb"},
		{"climbs out, Windows style", `..\..\evil_libretro.dll`, good, "climb"},
		{"absolute", "/etc/evil" + libraryExt(), good, "climb"},
		{"drive letter", `C:\Windows\evil.dll`, good, "climb"},
		{"stream name", "core.dll:hidden", good, "climb"},
		{"device name", "nul" + libraryExt(), good, "can't be used"},
		{"hidden", ".evil" + libraryExt(), good, "can't be used"},
		{"not a core", "readme.txt", []byte("hello"), "neither"},
		{"text named as a core", "fake_libretro" + libraryExt(), []byte("this is text"), "isn't a program library"},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := testStore(t)
			_, err := st.StageReader(bytes.NewReader(c.body), c.file)
			if err == nil || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("got %v", err)
			}
			if ents, _ := os.ReadDir(st.stagingDir()); len(ents) != 0 {
				t.Errorf("a refused file left %d things behind", len(ents))
			}
		})
	}
	t.Run("huge file", func(t *testing.T) {
		st := testStore(t)
		st.Limits.Package = 1 << 16
		_, err := st.StageReader(io.LimitReader(zeroes{}, 1<<30), "big_libretro"+libraryExt())
		if err == nil || !strings.Contains(err.Error(), "larger") {
			t.Fatalf("got %v", err)
		}
		if ents, _ := os.ReadDir(st.stagingDir()); len(ents) != 0 {
			t.Errorf("left %d things behind", len(ents))
		}
	})
	t.Run("another computer's core", func(t *testing.T) {
		p, err := fakecore.Foreign(t.TempDir(), "other_libretro")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(p)
		_, err = testStore(t).StageReader(bytes.NewReader(b), filepath.Base(p))
		if err == nil || !strings.Contains(err.Error(), "built for") {
			t.Fatalf("got %v", err)
		}
	})
}

type zeroes struct{}

func (zeroes) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestPackages(t *testing.T) {
	good := goodCoreBytes(t)
	lib := "abc_libretro" + libraryExt()
	other := "abc_libretro.so"
	if libraryExt() == ".so" {
		other = "abc_libretro.dll"
	}
	info := []byte("display_name = \"Abc\"\nlicense = \"GPLv3\"\nsupported_extensions = \"abc|ABD\"\nfirmware_count = 1\nfirmware0_path = \"abc.rom\"\nfirmware0_desc = \"Abc BIOS\"\nfirmware0_opt = \"false\"\nrequired_hw_api = \"OpenGL\"\n")

	t.Run("a full package", func(t *testing.T) {
		st := testStore(t)
		s, err := st.StageReader(bytes.NewReader(makeZip(t,
			zipEntry{name: lib, body: good}, zipEntry{name: "abc_libretro.info", body: info}, zipEntry{name: "README.md", body: []byte("hi")},
			zipEntry{name: "assets/shaders/a.txt", body: []byte("a")}, zipEntry{name: "system/abc/b.bin", body: []byte("b")},
		)), "abc.zip")
		if err != nil {
			t.Fatal(err)
		}
		if !s.HasInfo || s.Info.License != "GPLv3" || s.Info.DisplayName != "Abc" || !s.Info.HWRender || s.Assets != 1 || s.System != 1 {
			t.Errorf("got %+v, info %+v", s, s.Info)
		}
		if len(s.Info.Firmware) != 1 || s.Info.Firmware[0].File != "abc.rom" || !s.Info.Firmware[0].Required {
			t.Errorf("firmware = %+v", s.Info.Firmware)
		}
		if strings.Join(s.Info.Extensions, ",") != "abc,abd" {
			t.Errorf("extensions = %v", s.Info.Extensions)
		}
		if len(s.Ignored) != 1 || s.Ignored[0] != "README.md" {
			t.Errorf("ignored = %v", s.Ignored)
		}
		added, kept, err := st.Commit(s, "abc")
		if err != nil {
			t.Fatal(err)
		}
		if len(added) != 1 || added[0] != "abc/b.bin" || len(kept) != 0 {
			t.Errorf("added %v, kept %v", added, kept)
		}
		if st.AssetsPath("abc", s.SHA256) == "" || st.ReadVersionInfo("abc", s.SHA256) == nil {
			t.Error("the assets and info should be with the version")
		}
		if got := st.Verify("abc", s.SHA256); got.State != "ok" {
			t.Errorf("verify = %+v", got)
		}
		if _, err := os.Stat(filepath.Join(st.System, "abc", "b.bin")); err != nil {
			t.Error(err)
		}
	})
	t.Run("a BIOS file already there is never overwritten", func(t *testing.T) {
		st := testStore(t)
		_ = os.MkdirAll(st.System, 0o755)
		mine := filepath.Join(st.System, "bios.bin")
		_ = os.WriteFile(mine, []byte("my own"), 0o644)
		s, err := st.StageReader(bytes.NewReader(makeZip(t, zipEntry{name: lib, body: good}, zipEntry{name: "system/bios.bin", body: []byte("theirs")})), "abc.zip")
		if err != nil {
			t.Fatal(err)
		}
		added, kept, err := st.Commit(s, "abc")
		if err != nil {
			t.Fatal(err)
		}
		if len(added) != 0 || len(kept) != 1 {
			t.Errorf("added %v, kept %v", added, kept)
		}
		if b, _ := os.ReadFile(mine); string(b) != "my own" {
			t.Errorf("the BIOS is now %q", b)
		}
		if err := st.RemoveCore("abc", added); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(mine); string(b) != "my own" {
			t.Error("removing a core took a file that wasn't its")
		}
	})
	for _, c := range []struct {
		name    string
		entries []zipEntry
		says    string
	}{
		{"climbs out", []zipEntry{{name: lib, body: good}, {name: "assets/../../../evil.txt", body: []byte("x")}}, "climbs out"},
		{"absolute", []zipEntry{{name: lib, body: good}, {name: "/etc/passwd", body: []byte("x")}}, "climbs out"},
		{"backslashes", []zipEntry{{name: lib, body: good}, {name: `assets\..\..\evil.txt`, body: []byte("x")}}, "climbs out"},
		{"drive letter", []zipEntry{{name: lib, body: good}, {name: `C:/evil.txt`, body: []byte("x")}}, "climbs out"},
		{"device name", []zipEntry{{name: lib, body: good}, {name: "assets/CON.txt", body: []byte("x")}}, "a name that can be used"},
		{"a link", []zipEntry{{name: lib, body: good}, {name: "assets/link", body: []byte("/etc/passwd"), mode: os.ModeSymlink | 0o777}}, "link"},
		{"named twice", []zipEntry{{name: lib, body: good}, {name: "assets/a", body: []byte("x")}, {name: "ASSETS/A", body: []byte("y")}}, "twice"},
		{"no core", []zipEntry{{name: "readme.txt", body: []byte("x")}}, "no core"},
		{"another computer's core", []zipEntry{{name: other, body: good}}, "another kind"},
		{"several cores", []zipEntry{{name: lib, body: good}, {name: "b_libretro" + libraryExt(), body: good}}, "several"},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := testStore(t)
			_, err := st.StageReader(bytes.NewReader(makeZip(t, c.entries...)), "p.zip")
			if err == nil || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("got %v", err)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(st.Dir), "evil.txt")); err == nil {
				t.Error("a file escaped")
			}
		})
	}
	t.Run("a zip bomb stops at the limit", func(t *testing.T) {
		st := testStore(t)
		st.Limits.Unpack = 1 << 16
		_, err := st.StageReader(bytes.NewReader(makeZip(t, zipEntry{name: lib, body: good}, zipEntry{name: "assets/bomb", body: make([]byte, 1<<24)})), "p.zip")
		if err == nil || !strings.Contains(err.Error(), "unpacks to more") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("too many files", func(t *testing.T) {
		st := testStore(t)
		st.Limits.Entries = 3
		e := []zipEntry{{name: lib, body: good}}
		for i := range 5 {
			e = append(e, zipEntry{name: fmt.Sprintf("assets/%d", i), body: []byte("x")})
		}
		_, err := st.StageReader(bytes.NewReader(makeZip(t, e...)), "p.zip")
		if err == nil || !strings.Contains(err.Error(), "more than 3") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestVersionsAndTampering(t *testing.T) {
	good := goodCoreBytes(t)
	st := testStore(t)
	name := "abc_libretro" + libraryExt()
	first, err := st.StageReader(bytes.NewReader(good), name)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Commit(first, "abc"); err != nil {
		t.Fatal(err)
	}
	v2 := append(append([]byte{}, good...), 1, 2, 3, 4)
	second, err := st.StageReader(bytes.NewReader(v2), name)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 == second.SHA256 {
		t.Fatal("the two versions should differ")
	}
	if _, _, err := st.Commit(second, "abc"); err != nil {
		t.Fatal(err)
	}
	if st.Verify("abc", first.SHA256).State != "ok" || st.Verify("abc", second.SHA256).State != "ok" {
		t.Fatal("both versions should still be there")
	}
	p := st.CorePath("abc", second.SHA256)
	if err := os.WriteFile(p, append(v2, 9), 0o755); err != nil {
		t.Fatal(err)
	}
	got := st.Verify("abc", second.SHA256)
	if got.State != "changed" || got.Found == "" || got.Found == second.SHA256 {
		t.Errorf("a changed file: %+v", got)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if st.Verify("abc", second.SHA256).State != "missing" {
		t.Error("a missing file should say so")
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink("/bin/sh", p); err != nil {
			t.Fatal(err)
		}
		if st.Verify("abc", second.SHA256).State != "link" {
			t.Error("a link should be refused")
		}
	}
	third, err := st.StageReader(bytes.NewReader(good), name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(third.Library, []byte("swapped after the test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Commit(third, "abc"); err == nil || !strings.Contains(err.Error(), "changed after") {
		t.Errorf("a file swapped after testing: %v", err)
	}
}

func TestTheSameFileAgainKeepsWhatCameWithIt(t *testing.T) {
	good := goodCoreBytes(t)
	st := testStore(t)
	lib := "abc_libretro" + libraryExt()
	first, err := st.StageReader(bytes.NewReader(makeZip(t, zipEntry{name: lib, body: good}, zipEntry{name: "assets/one.txt", body: []byte("1")})), "p.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Commit(first, "abc"); err != nil {
		t.Fatal(err)
	}
	again, err := st.StageReader(bytes.NewReader(good), lib)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Commit(again, "abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.AssetsPath("abc", first.SHA256), "one.txt")); err != nil {
		t.Errorf("the assets went: %v", err)
	}
	if st.Verify("abc", first.SHA256).State != "ok" {
		t.Error("the file itself should still be right")
	}
}

func TestPlacedFiles(t *testing.T) {
	good := goodCoreBytes(t)
	st := testStore(t)
	folder := st.Placed[0]
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "here_libretro" + libraryExt()
	if err := os.WriteFile(filepath.Join(folder, name), good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "here_libretro.info"), []byte(`license = "MIT"`), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "out_libretro"+libraryExt())
	if err := os.WriteFile(outside, good, 0o755); err != nil {
		t.Fatal(err)
	}
	link := "link_libretro" + libraryExt()
	linked := os.Symlink(outside, filepath.Join(folder, link)) == nil

	var key, linkKey string
	for _, p := range st.ListPlaced() {
		switch p.Name {
		case name:
			key = p.Key
		case link:
			linkKey = p.Key
			if p.Refuse == "" {
				t.Error("a link should be listed as refused")
			}
		}
	}
	if key == "" {
		t.Fatal("the file wasn't listed")
	}
	for _, bad := range []string{"", "0/../../evil", "0/" + name + "/..", "9/" + name, filepath.Join(filepath.Dir(outside), filepath.Base(outside))} {
		if _, err := st.StagePlaced(bad); err == nil {
			t.Errorf("StagePlaced(%q) should fail", bad)
		}
	}
	if linked {
		if _, err := st.StagePlaced(linkKey); err == nil || !strings.Contains(err.Error(), "link") {
			t.Errorf("a link out of the folder: %v", err)
		}
	}
	s, err := st.StagePlaced(key)
	if err != nil {
		t.Fatal(err)
	}
	if !s.HasInfo || s.Info.License != "MIT" {
		t.Errorf("the info beside it should come along: %+v", s.Info)
	}
	if _, _, err := st.Commit(s, "here"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, name)); !os.IsNotExist(err) {
		t.Errorf("the placed file should have moved: %v", err)
	}
}

func TestSuggestID(t *testing.T) {
	for in, want := range map[string]string{
		"foo_libretro.dll":    "foo",
		"Foo Bar_libretro.so": "foo_bar",
		"mgba_libretro.dll":   "mgba-custom", // our table's name
		"snes9x_libretro.dll": "snes9x",      // known, so it keeps its non-commercial label
		"...weird...name.dll": "weird_name",
		"!!!.dll":             "custom-core",
	} {
		if got := SuggestID(in, ""); got != want {
			t.Errorf("SuggestID(%q) = %q, want %q", in, got, want)
		}
	}
	if got := SuggestID("core.dll", "My Core 2"); got != "my_core_2" {
		t.Errorf("a file with no real name takes the core's: %q", got)
	}
}

func TestParseInfoIsUntrusted(t *testing.T) {
	long := strings.Repeat("x", 5000)
	info := ParseInfo(strings.NewReader("corename = \"Evil\x07\x1b[2J\u202e" + long + "\"\nfirmware_count = 99999\nfirmware0_path = \"../../etc/passwd\"\nsupported_extensions = \"a|b/c|<x>|ok\"\nnot a line\n#comment\n"))
	if len([]rune(info.CoreName)) > 200 || strings.ContainsAny(info.CoreName, "\x07\x1b\u202e") {
		t.Errorf("core name = %q", info.CoreName)
	}
	if len(info.Firmware) != 1 {
		t.Errorf("firmware = %v", info.Firmware)
	}
	if strings.Join(info.Extensions, ",") != "a,ok" {
		t.Errorf("extensions = %v", info.Extensions)
	}
}

func TestCheckFetchURL(t *testing.T) {
	for _, bad := range []string{
		"", "not a url", "http://example.com/core.so", "ftp://example.com/core.so", "file:///etc/passwd", "gopher://example.com",
		"https://user:pw@example.com/x", "https://localhost/x", "https://LOCALHOST./x", "https://foo.localhost/x", "https://nas/x", "https://nas.local/x", "https://router.lan/x",
		"https://127.0.0.1/x", "https://10.0.0.1/x", "https://192.168.1.1/x", "https://172.16.0.1/x", "https://169.254.169.254/latest/meta-data", "https://[::1]/x", "https://[fe80::1]/x",
		"https://[::ffff:127.0.0.1]/x", "https://0.0.0.0/x",
	} {
		if _, err := CheckFetchURL(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
	for _, good := range []string{"https://example.com/core.so", "https://example.com:8443/a/b.zip?x=1"} {
		if _, err := CheckFetchURL(good); err != nil {
			t.Errorf("%q: %v", good, err)
		}
	}
}

// fetchClient points every name at a local test server, which is the only way a test
// can stand in for a public site, and refuses anything else the way the server's
// real client refuses a private address.
func fetchClient(ts *httptest.Server) *http.Client {
	tr := ts.Client().Transport.(*http.Transport).Clone()
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", ts.Listener.Addr().String())
	}
	return &http.Client{Transport: tr}
}

func TestStageURL(t *testing.T) {
	good := goodCoreBytes(t)
	var redirectTo string
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fine_libretro" + libraryExt():
			_, _ = w.Write(good)
		case "/fine.zip":
			_, _ = w.Write(makeZip(t, zipEntry{name: "fine_libretro" + libraryExt(), body: good}))
		case "/noname":
			_, _ = w.Write(good)
		case "/page":
			_, _ = w.Write([]byte("<!doctype html><html>not a core</html>"))
		case "/big":
			w.Header().Set("Content-Length", fmt.Sprint(int64(1)<<40))
			w.WriteHeader(200)
		case "/endless":
			for range 64 {
				_, _ = w.Write(make([]byte, 1<<20))
			}
		case "/redirect":
			http.Redirect(w, r, redirectTo, http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	st := testStore(t)
	st.Client = fetchClient(ts)
	ctx := context.Background()

	t.Run("a library", func(t *testing.T) {
		s, err := st.StageURL(ctx, "https://cores.example.com/fine_libretro"+libraryExt())
		if err != nil {
			t.Fatal(err)
		}
		defer st.Discard(s)
		if s.Source != "url" || !strings.Contains(s.SourceURL, "cores.example.com") || SuggestID(s.OrigName, "") != "fine" {
			t.Errorf("got %+v", s)
		}
	})
	t.Run("a zip", func(t *testing.T) {
		s, err := st.StageURL(ctx, "https://cores.example.com/fine.zip")
		if err != nil {
			t.Fatal(err)
		}
		st.Discard(s)
	})
	t.Run("a name that says nothing", func(t *testing.T) {
		s, err := st.StageURL(ctx, "https://cores.example.com/noname")
		if err != nil {
			t.Fatal(err)
		}
		st.Discard(s)
	})
	for _, c := range []struct {
		name, url, redirect, says string
	}{
		{"not https", "http://cores.example.com/fine.zip", "", "https"},
		{"a private address", "https://192.168.1.10/x", "", "public internet"},
		{"redirect to http", "https://cores.example.com/redirect", "http://cores.example.com/fine.zip", "https"},
		{"redirect to this machine", "https://cores.example.com/redirect", "https://127.0.0.1/secret", "public internet"},
		{"redirect to the cloud's metadata address", "https://cores.example.com/redirect", "https://169.254.169.254/latest", "public internet"},
		{"redirect to a local name", "https://cores.example.com/redirect", "https://nas.local/x", "public internet"},
		{"redirect loop", "https://cores.example.com/loop", "", "too many"},
		{"a web page", "https://cores.example.com/page", "", "web page"},
		{"not found", "https://cores.example.com/missing", "", "404"},
		{"claims to be huge", "https://cores.example.com/big", "", "larger"},
	} {
		t.Run(c.name, func(t *testing.T) {
			redirectTo = c.redirect
			_, err := st.StageURL(ctx, c.url)
			if err == nil || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("sends more than it may", func(t *testing.T) {
		small := testStore(t)
		small.Client = st.Client
		small.Limits.Package = 1 << 20
		_, err := small.StageURL(ctx, "https://cores.example.com/endless")
		if err == nil || !strings.Contains(err.Error(), "larger") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("no client, no fetching", func(t *testing.T) {
		if _, err := testStore(t).StageURL(ctx, "https://cores.example.com/x"); err == nil {
			t.Fatal("a store with no client must not fall back to an unprotected one")
		}
	})
}
