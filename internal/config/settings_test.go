package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/layout"
)

// usePackage points the settings files at a program folder of the given kind, for one test.
func usePackage(t *testing.T, kind string) string {
	t.Helper()
	dir := t.TempDir()
	oldDir, oldKind := programDir, kindOf
	programDir, kindOf = func() string { return dir }, func() string { return kind }
	t.Cleanup(func() { programDir, kindOf = oldDir, oldKind })
	return dir
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The address comes from, strongest first: -listen, WMS_LISTEN, "listen" in wms.json, the data
// folder's settings.ini, the one beside the program, and the default.
func TestListenPrecedence(t *testing.T) {
	prog := usePackage(t, layout.KindArchive)
	data := t.TempDir()
	empty := filepath.Join(t.TempDir(), FileName)
	writeFile(t, empty, `{}`)
	withListen := filepath.Join(t.TempDir(), FileName)
	writeFile(t, withListen, `{"listen": ":2222"}`)
	t.Setenv("WMS_LISTEN", "")
	t.Setenv("WMS_DATA_DIR", "")

	check := func(name string, args []string, listen, from string) {
		t.Helper()
		c, err := ParseOnly(append([]string{"-data", data}, args...))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c.Listen != listen || c.ListenFrom != from {
			t.Errorf("%s: got %s from %s, want %s from %s", name, c.Listen, c.ListenFrom, listen, from)
		}
	}
	check("nothing set", []string{"-config", empty}, ":9797", FromDefault)

	writeFile(t, filepath.Join(prog, SettingsName), SettingsText(4444, false))
	check("beside the program", []string{"-config", empty}, ":4444", FromSettings)

	writeFile(t, DataSettingsFile(data), "port = 5555\n")
	check("data folder copy", []string{"-config", empty}, ":5555", FromSettings)
	if c, _ := ParseOnly([]string{"-data", data, "-config", empty}); c.SettingsFile != DataSettingsFile(data) {
		t.Errorf("the port came from %s", c.SettingsFile)
	}

	check("wms.json", []string{"-config", withListen}, ":2222", FromConfig)

	// What config init wrote before 0.7.0 doesn't stand in the way of the settings file.
	oldStarter := filepath.Join(t.TempDir(), FileName)
	writeFile(t, oldStarter, `{"listen": ":9797"}`)
	check("an old starter wms.json", []string{"-config", oldStarter}, ":5555", FromSettings)

	t.Setenv("WMS_LISTEN", ":3333")
	check("WMS_LISTEN", []string{"-config", withListen}, ":3333", FromEnv)
	check("-listen", []string{"-config", withListen, "-listen", "127.0.0.1:1111"}, "127.0.0.1:1111", FromFlag)

	if _, err := ParseOnly([]string{"-data", data, "-config", empty, "-listen", ""}); err == nil {
		t.Error("an empty -listen should be refused")
	}
}

// Inside a Mac app nothing beside the program is read: changing the app would break its signature.
func TestNoSettingsInsideAMacApp(t *testing.T) {
	dir := t.TempDir()
	old := programDir
	programDir = func() string { return filepath.Join(dir, "WittyMediaServer.app", "Contents") }
	t.Cleanup(func() { programDir = old })
	if p := ProgramSettingsFile(); p != "" {
		t.Errorf("a Mac app has a settings file at %s", p)
	}
	if got := SettingsFiles(dir); len(got) != 1 || got[0] != DataSettingsFile(dir) {
		t.Errorf("read from %v", got)
	}
}

// A typo is skipped and reported, and the server still starts.
func TestSettingsProblems(t *testing.T) {
	usePackage(t, layout.KindArchive)
	data := t.TempDir()
	empty := filepath.Join(t.TempDir(), FileName)
	writeFile(t, empty, `{}`)
	t.Setenv("WMS_LISTEN", "")
	const bom = "\xef\xbb\xbf" // Notepad starts a file with one
	writeFile(t, DataSettingsFile(data), bom+"# notes\r\n[server]\r\nprot = 1\r\nport = 80800\r\nnonsense\r\n")
	c, err := ParseOnly([]string{"-data", data, "-config", empty})
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":9797" || c.ListenFrom != FromDefault {
		t.Errorf("a bad port should leave the default: %s from %s", c.Listen, c.ListenFrom)
	}
	if len(c.SettingsProblems) != 3 {
		t.Fatalf("problems: %q", c.SettingsProblems)
	}
	for i, want := range []string{"line 3: there's no setting called \"prot\"", "line 4: the port should be", "line 5:"} {
		if !strings.Contains(c.SettingsProblems[i], want) {
			t.Errorf("problem %d is %q, want %q", i, c.SettingsProblems[i], want)
		}
	}

	writeFile(t, DataSettingsFile(data), "port = 8123   # my port\n")
	if s, _ := ReadSettings(DataSettingsFile(data)); s.Port != 8123 || len(s.Problems) != 0 {
		t.Errorf("a note after the value: %+v", s)
	}
}

func TestSavePort(t *testing.T) {
	dir := t.TempDir()

	// A new file is the shipped one, with the port.
	fresh := filepath.Join(dir, "new", SettingsName)
	if err := SavePort(fresh, 8080); err != nil {
		t.Fatal(err)
	}
	if s, _ := ReadSettings(fresh); s.Port != 8080 || len(s.Problems) != 0 {
		t.Errorf("new file: %+v", s)
	}
	b, _ := os.ReadFile(fresh)
	if !strings.Contains(string(b), "data folder") {
		t.Error("the data folder's copy should say what it is")
	}

	// An edited one keeps its notes and line endings, and its second port line goes.
	edited := filepath.Join(dir, SettingsName)
	writeFile(t, edited, "# mine\r\n  port=1234 \r\n# port = 1\r\nport = 999\r\n")
	if err := SavePort(edited, 8081); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(edited)
	if want := "# mine\r\n  port = 8081\r\n# port = 1\r\n\r\n"; string(b) != want {
		t.Errorf("got %q, want %q", b, want)
	}

	// One with no port line gets one.
	writeFile(t, edited, "# nothing yet\n")
	_ = SavePort(edited, 9000)
	if s, _ := ReadSettings(edited); s.Port != 9000 {
		t.Errorf("appended: %+v", s)
	}

	if err := SavePort(edited, 70000); err == nil {
		t.Error("70000 is no port")
	}
}

// Every shipped file says the port it was made with, and nothing else.
func TestSettingsTextReadsBack(t *testing.T) {
	for _, dataCopy := range []bool{false, true} {
		p := filepath.Join(t.TempDir(), SettingsName)
		writeFile(t, p, SettingsText(9797, dataCopy))
		if s, err := ReadSettings(p); err != nil || s.Port != 9797 || len(s.Problems) != 0 {
			t.Errorf("dataCopy %v: %+v %v", dataCopy, s, err)
		}
	}
}

// Settings saves where it wins: the data folder's copy when there is one or an installer owns the
// program's folder, otherwise beside the program, where a person looks for it.
func TestSettingsSaveFile(t *testing.T) {
	prog := usePackage(t, layout.KindArchive)
	data := t.TempDir()
	if got := SettingsSaveFile(data); got != DataSettingsFile(data) {
		t.Errorf("nothing anywhere: %s", got)
	}
	writeFile(t, filepath.Join(prog, SettingsName), SettingsText(9797, false))
	if got := SettingsSaveFile(data); got != filepath.Join(prog, SettingsName) {
		t.Errorf("an archive's own file: %s", got)
	}
	writeFile(t, DataSettingsFile(data), "port = 1\n")
	if got := SettingsSaveFile(data); got != DataSettingsFile(data) {
		t.Errorf("the data copy wins: %s", got)
	}

	for _, kind := range []string{layout.KindMSI, layout.KindDeb, layout.KindRPM, layout.KindPkg, layout.KindFreeBSD} {
		prog := usePackage(t, kind)
		data := t.TempDir()
		writeFile(t, filepath.Join(prog, SettingsName), SettingsText(9797, false))
		if got := SettingsSaveFile(data); got != DataSettingsFile(data) {
			t.Errorf("%s: an installer's folder was chosen: %s", kind, got)
		}
	}
}

// A link in the data folder is never followed: an installer running as root would read what it
// points to, and write that back where the service can read it.
func TestSavePortRefusesLinks(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	writeFile(t, secret, "root only\n")
	link := filepath.Join(dir, "data", SettingsName)
	_ = os.MkdirAll(filepath.Dir(link), 0o755)
	if err := os.Symlink(secret, link); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	if err := SavePort(link, 8080); err == nil {
		t.Fatal("a link was followed")
	}
	if b, _ := os.ReadFile(secret); string(b) != "root only\n" {
		t.Errorf("the file it points to changed: %q", b)
	}
}
