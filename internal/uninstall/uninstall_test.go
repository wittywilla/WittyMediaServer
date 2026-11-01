package uninstall

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// fakeSys answers like a machine and writes down what it's asked. A program name it has an
// answer for exists; everything else fails, as a missing program does.
type fakeSys struct {
	answers map[string]string
	calls   []string
	stopped []string
	reg     map[string]string // HIVE\key\name -> value; HIVE\key -> ""
}

func (f *fakeSys) Run(name string, args ...string) (string, error) {
	call := filepath.Base(name) + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	for k, v := range f.answers {
		if strings.HasPrefix(call, k) {
			return v, nil
		}
	}
	return "", errors.New("exit status 1")
}
func (f *fakeSys) StopProgramsIn(dir string) { f.stopped = append(f.stopped, dir) }
func (f *fakeSys) RegKeyExists(h, k string) bool {
	_, ok := f.reg[h+`\`+k]
	return ok
}
func (f *fakeSys) RegValue(h, k, n string) (string, bool) {
	v, ok := f.reg[h+`\`+k+`\`+n]
	return v, ok
}
func (f *fakeSys) RegDeleteKey(h, k string) error {
	for key := range f.reg {
		if key == h+`\`+k || strings.HasPrefix(key, h+`\`+k+`\`) {
			delete(f.reg, key)
		}
	}
	return nil
}
func (f *fakeSys) RegDeleteValue(h, k, n string) error {
	delete(f.reg, h+`\`+k+`\`+n)
	return nil
}
func (f *fakeSys) called(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tree lists every file under root, as slash paths.
func tree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if rel, _ := filepath.Rel(root, p); rel != "." && !d.IsDir() {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func dirGone(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		t.Errorf("%s is still there", p)
	}
}

// fillData writes a data folder with what a server leaves in one, a library in a folder of
// its own, and a file someone put there.
func fillData(t *testing.T, dir string) {
	t.Helper()
	for _, f := range []string{"wms.db", "wms.db-wal", "cache/images/a.jpg", "logs/wms.log", "modules/wittyplay/versions/1/x", "gaming/cores/c.dll", "themes/t.json"} {
		write(t, filepath.Join(dir, filepath.FromSlash(f)), "x")
	}
}

func noFacts(string) (DataFacts, error) { return DataFacts{}, nil }

type winMachine struct {
	root                                               string
	programFiles, programData, local, roaming, perUser string
	sys                                                *fakeSys
	env                                                *Env
}

func newWindows(t *testing.T) *winMachine {
	t.Helper()
	root := t.TempDir()
	w := &winMachine{root: root, sys: &fakeSys{answers: map[string]string{}, reg: map[string]string{}}}
	vars := map[string]string{
		"SystemRoot": filepath.Join(root, "Windows"), "ProgramFiles": filepath.Join(root, "Program Files"), "ProgramData": filepath.Join(root, "ProgramData"),
		"LOCALAPPDATA": filepath.Join(root, "Users/me/AppData/Local"), "APPDATA": filepath.Join(root, "Users/me/AppData/Roaming"),
		"USERPROFILE": filepath.Join(root, "Users/me"), "PUBLIC": filepath.Join(root, "Users/Public"),
	}
	w.programFiles = filepath.Join(vars["ProgramFiles"], winName)
	w.programData = filepath.Join(vars["ProgramData"], winName)
	w.local, w.roaming = filepath.Join(vars["LOCALAPPDATA"], winName), filepath.Join(vars["APPDATA"], winName)
	w.perUser = filepath.Join(vars["LOCALAPPDATA"], "Programs", winName)
	w.env = &Env{GOOS: "windows", Getenv: func(k string) string { return vars[k] }, Home: vars["USERPROFILE"], IsAdmin: true, Sys: w.sys, Libraries: noFacts}
	return w
}

// A machine with the most that could have piled up: a per-machine MSI with its service and
// firewall rule, an old per-user copy from the setup program of 0.5.2, its boot task, shortcuts,
// startup entries and data folders.
func (w *winMachine) fill(t *testing.T) {
	t.Helper()
	for _, f := range []string{"WittyMediaServer.exe", "Uninstall WittyMediaServer.exe", "package-info.json", "bin/wittymediaserver.exe", "bin/ffmpeg.exe", "bin/ffprobe.exe", "docs/LICENSE", ".update/old-files.txt", "WittyMediaServer.exe.old"} {
		write(t, filepath.Join(w.programFiles, filepath.FromSlash(f)), "x")
	}
	for _, f := range []string{"WittyMediaServer.exe", "wms.exe", "wms-setup.exe", "bin/ffmpeg.exe", "wittyplay-host.exe", "icon.ico", "LICENSE", "README.md"} {
		write(t, filepath.Join(w.perUser, filepath.FromSlash(f)), "x")
	}
	fillData(t, w.local)
	fillData(t, w.programData)
	write(t, filepath.Join(w.roaming, "wms.json"), `{"logLevel":"info"}`)
	write(t, filepath.Join(w.env.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`, winName+".lnk"), "x")
	write(t, filepath.Join(w.env.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`, winName+".lnk"), "x")
	w.sys.answers = map[string]string{
		"sc.exe qc":                      `BINARY_PATH_NAME : "C:\Program Files\WittyMediaServer\bin\wittymediaserver.exe" serve`,
		"sc.exe":                         "",
		"schtasks.exe /Query":            "<Task/>",
		"schtasks.exe":                   "",
		"netsh.exe advfirewall firewall": "",
		"msiexec.exe":                    "",
	}
	w.sys.reg[`HKLM\`+UninstallKey] = ""
	w.sys.reg[`HKLM\`+UninstallKey+`\ProductCode`] = "{11111111-2222-3333-4444-555555555555}"
	w.sys.reg[`HKCU\`+UninstallKey] = ""
	w.sys.reg[`HKCU\`+runKey+`\`+winName] = "x"
}

func TestWindowsFullCleanLeavesNothing(t *testing.T) {
	w := newWindows(t)
	before := tree(t, w.root)
	w.fill(t)
	pl := MakePlan(w.env)
	res := Execute(pl, Options{RemoveData: true, RemoveRecordings: true})
	if !res.OK() {
		t.Fatal(res.Report())
	}
	if after := tree(t, w.root); strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("left behind:\n%s", strings.Join(after, "\n"))
	}
	for _, d := range []string{w.programFiles, w.perUser, w.local, w.programData, w.roaming} {
		dirGone(t, d)
	}
	for _, want := range []string{"sc.exe delete WittyMediaServer", "schtasks.exe /Delete /TN WittyMediaServer /F", "msiexec.exe /x {11111111-2222-3333-4444-555555555555} /qn /norestart", "netsh.exe advfirewall firewall delete rule name=WittyMediaServer"} {
		if !w.sys.called(want) {
			t.Errorf("didn't run %s; ran %v", want, w.sys.calls)
		}
	}
	if len(w.sys.reg) != 0 {
		t.Errorf("registry left: %v", w.sys.reg)
	}
}

func TestWindowsKeepDataKeepsTheLibrary(t *testing.T) {
	w := newWindows(t)
	w.fill(t)
	res := Execute(MakePlan(w.env), Options{})
	if !res.OK() {
		t.Fatal(res.Report())
	}
	for _, f := range []string{"wms.db", "cache/images/a.jpg", "modules/wittyplay/versions/1/x"} {
		for _, d := range []string{w.local, w.programData} {
			if !exists(filepath.Join(d, filepath.FromSlash(f))) {
				t.Errorf("%s was removed from %s", f, d)
			}
		}
	}
	if !exists(filepath.Join(w.roaming, "wms.json")) {
		t.Error("the settings file went")
	}
	dirGone(t, w.programFiles)
	dirGone(t, w.perUser)
}

// Things it didn't make stay, wherever they are, and it says so.
func TestLeavesWhatItDidNotMake(t *testing.T) {
	w := newWindows(t)
	w.fill(t)
	mine := []string{
		filepath.Join(w.local, "my notes.txt"),
		filepath.Join(w.programFiles, "somebody's plugin.dll"),
		filepath.Join(w.programData, "Movies", "A Made-Up Film.mkv"),
	}
	for _, f := range mine {
		write(t, f, "mine")
	}
	res := Execute(MakePlan(w.env), Options{RemoveData: true})
	for _, f := range mine {
		if !exists(f) {
			t.Errorf("%s was removed", f)
		}
		if !strings.Contains(res.Report(), f) && !strings.Contains(res.Report(), filepath.Dir(f)) {
			t.Errorf("the report doesn't mention %s:\n%s", f, res.Report())
		}
	}
	// The folders that hold them stay; what was ours is gone.
	if exists(filepath.Join(w.local, "wms.db")) || exists(filepath.Join(w.programFiles, "bin")) {
		t.Error("our own files stayed")
	}
}

// A library inside the data folder: media is never touched, whichever folder it's in.
func TestALibraryInADataFolderStays(t *testing.T) {
	w := newWindows(t)
	fillData(t, w.local)
	film := filepath.Join(w.local, "cache", "movies", "A Made-Up Film.mkv")
	write(t, film, "film")
	w.env.Libraries = func(d string) (DataFacts, error) {
		return DataFacts{Libraries: []string{filepath.Join(w.local, "cache", "movies")}}, nil
	}
	res := Execute(MakePlan(w.env), Options{RemoveData: true})
	if !exists(film) {
		t.Fatal("a film was deleted")
	}
	if !strings.Contains(res.Report(), "library") {
		t.Errorf("the report doesn't say why:\n%s", res.Report())
	}
}

func TestRecordingsHaveTheirOwnQuestion(t *testing.T) {
	w := newWindows(t)
	fillData(t, w.local)
	rec := filepath.Join(w.local, "recordings", "A Made-Up Show.ts")
	write(t, rec, "tv")
	pl := MakePlan(w.env)
	if !pl.Has(GroupRecordings) {
		t.Fatal("recordings aren't in the plan")
	}
	Execute(pl, Options{RemoveData: true})
	if !exists(rec) {
		t.Fatal("removing data took the recordings")
	}
	Execute(MakePlan(w.env), Options{RemoveData: true, RemoveRecordings: true})
	if exists(rec) {
		t.Fatal("asked to remove recordings, they stayed")
	}
}

func TestAFolderWithTheRightNameIsNotEnough(t *testing.T) {
	w := newWindows(t)
	// Someone else's program folder, with our name on it.
	write(t, filepath.Join(w.programFiles, "their.exe"), "x")
	// A chosen data folder holding no database of ours.
	other := filepath.Join(w.root, "tank")
	write(t, filepath.Join(other, "cache", "x"), "theirs")
	planned := &Plan{}
	planDataFolder(w.env, planned, other, false, true)
	if len(planned.Items) != 0 {
		t.Fatalf("took %s for a data folder: %+v", other, planned.Items)
	}
	Execute(MakePlan(w.env), Options{RemoveData: true})
	if !exists(filepath.Join(w.programFiles, "their.exe")) {
		t.Fatal("removed a folder that isn't ours")
	}
}

// A folder chosen in wms.json is reported and left; a data folder chosen there is followed.
func TestChosenFolders(t *testing.T) {
	w := newWindows(t)
	cache := filepath.Join(w.root, "fast", "cache")
	data := filepath.Join(w.root, "big", "library data")
	write(t, filepath.Join(cache, "thumb.jpg"), "x")
	fillData(t, data)
	write(t, filepath.Join(w.roaming, "wms.json"), `{"cacheDir":`+quote(cache)+`,"dataDir":`+quote(data)+`}`)
	res := Execute(MakePlan(w.env), Options{RemoveData: true})
	if !exists(filepath.Join(cache, "thumb.jpg")) {
		t.Error("a chosen folder was emptied")
	}
	if !strings.Contains(res.Report(), cache) {
		t.Errorf("the chosen folder isn't reported:\n%s", res.Report())
	}
	dirGone(t, data)
	dirGone(t, w.roaming)
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}

// Replacing a copy of 0.5.2 clears its parts and keeps its library.
func TestTakeOverOldCopy(t *testing.T) {
	w := newWindows(t)
	for _, f := range []string{"WittyMediaServer.exe", "wms.exe", "wms-setup.exe", "bin/ffmpeg.exe", "wittyplay-host.exe", "icon.ico", "LICENSE", "README.md"} {
		write(t, filepath.Join(w.perUser, filepath.FromSlash(f)), "x")
	}
	fillData(t, w.local)
	w.sys.answers = map[string]string{"schtasks.exe": "", "netsh.exe": ""}
	w.sys.reg[`HKCU\`+UninstallKey] = ""
	w.sys.reg[`HKCU\`+UninstallKey+`\InstallLocation`] = w.perUser
	w.sys.reg[`HKCU\`+runKey+`\`+winName] = `"` + filepath.Join(w.perUser, "WittyMediaServer.exe") + `"`
	did := TakeOver(w.env)
	if len(did) == 0 {
		t.Fatal("nothing was done")
	}
	dirGone(t, w.perUser)
	if !exists(filepath.Join(w.local, "wms.db")) {
		t.Error("the library went")
	}
	if len(w.sys.reg) != 0 {
		t.Errorf("registry left: %v", w.sys.reg)
	}
	if !w.sys.called("schtasks.exe /Delete") || !w.sys.called("netsh.exe advfirewall firewall delete") {
		t.Errorf("ran %v", w.sys.calls)
	}
}

// The new copy went to the same folder as the old one: only the old parts go.
func TestTakeOverInTheSameFolder(t *testing.T) {
	w := newWindows(t)
	for _, f := range []string{"WittyMediaServer.exe", "wms.exe", "wms-setup.exe", "bin/ffmpeg.exe", "bin/wittymediaserver.exe", "package-info.json"} {
		write(t, filepath.Join(w.perUser, filepath.FromSlash(f)), "x")
	}
	TakeOver(w.env)
	for _, f := range []string{"WittyMediaServer.exe", "bin/wittymediaserver.exe", "package-info.json"} {
		if !exists(filepath.Join(w.perUser, filepath.FromSlash(f))) {
			t.Errorf("%s went", f)
		}
	}
	for _, f := range []string{"wms.exe", "wms-setup.exe"} {
		if exists(filepath.Join(w.perUser, f)) {
			t.Errorf("%s stayed", f)
		}
	}
}

func TestServiceDataFolderIsFollowed(t *testing.T) {
	cases := map[string]string{
		`"C:\P\bin\wittymediaserver.exe" serve -data "D:\My Data" -listen :9797`: `D:\My Data`,
		`Environment=WMS_DATA_DIR=/tank/wms`:                                     `/tank/wms`,
		`Arguments>serve -data D:\x -listen :9797</Arguments`:                    `D:\x`,
		`serve -listen :9797`:                                                    "",
	}
	for in, want := range cases {
		if got := dataDirFromCommand(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func newUnix(t *testing.T) (*Env, *fakeSys, string) {
	t.Helper()
	root := t.TempDir()
	sys := &fakeSys{answers: map[string]string{}, reg: map[string]string{}}
	home := filepath.Join(root, "home", "me")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{}
	return &Env{GOOS: "linux", Getenv: func(k string) string { return vars[k] }, Home: home, IsAdmin: true, Prefix: root, Sys: sys, Libraries: noFacts}, sys, root
}

// A copy that install.sh of 0.5.2 put there, with a custom data folder: everything goes.
func TestLinuxOldInstallShCopy(t *testing.T) {
	e, sys, root := newUnix(t)
	before := tree(t, root)
	write(t, filepath.Join(root, "etc/systemd/system/wittymediaserver.service"),
		"[Service]\nEnvironment=WMS_DATA_DIR=/tank/wms\nExecStart=/srv/wms/wms serve\n")
	for _, f := range []string{"wms", "WittyMediaServer", "bin/ffmpeg", "wittyplay-host", "icon.png", "LICENSE"} {
		write(t, filepath.Join(root, "srv/wms", filepath.FromSlash(f)), "x")
	}
	fillData(t, filepath.Join(root, "tank/wms"))
	write(t, filepath.Join(root, "usr/share/applications/wittymediaserver.desktop"), "x")
	sys.answers["id wms"] = "uid=999"
	sys.answers["systemctl"] = ""
	sys.answers["userdel"] = ""
	res := Execute(MakePlan(e), Options{RemoveData: true})
	if !res.OK() {
		t.Fatal(res.Report())
	}
	if after := tree(t, root); strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("left behind:\n%s", strings.Join(after, "\n"))
	}
	dirGone(t, filepath.Join(root, "tank/wms"))
	dirGone(t, filepath.Join(root, "srv/wms"))
	if !sys.called("systemctl disable --now wittymediaserver.service") || !sys.called("userdel wms") {
		t.Errorf("ran %v", sys.calls)
	}
}

// A .deb is the package manager's to remove, and the question decides remove or purge.
func TestLinuxDebPackage(t *testing.T) {
	for _, removeData := range []bool{false, true} {
		e, sys, root := newUnix(t)
		sys.answers["dpkg-query"] = "install ok installed"
		sys.answers["apt-get"] = ""
		fillData(t, filepath.Join(root, "var/lib/wittymediaserver"))
		write(t, filepath.Join(root, "usr/share/applications/wittymediaserver.desktop"), "x")
		res := Execute(MakePlan(e), Options{RemoveData: removeData})
		if !res.OK() {
			t.Fatal(res.Report())
		}
		if !sys.called("apt-get remove -y wittymediaserver") {
			t.Errorf("ran %v", sys.calls)
		}
		if got := sys.called("apt-get purge -y wittymediaserver"); got != removeData {
			t.Errorf("removeData %v: purge %v", removeData, got)
		}
		if exists(filepath.Join(root, "var/lib/wittymediaserver/wms.db")) == removeData {
			t.Errorf("removeData %v: the database is wrong", removeData)
		}
		// The package's own files are its own to remove.
		if !exists(filepath.Join(root, "usr/share/applications/wittymediaserver.desktop")) {
			t.Error("removed a file the package manager owns")
		}
	}
}

func TestMacFullCleanLeavesNothing(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "Users", "me")
	sys := &fakeSys{answers: map[string]string{"pkgutil --pkgs": "com.wittywilla.wittymediaserver.app\ncom.wittywilla.wittymediaserver.service\n", "pkgutil --forget": "", "launchctl": ""}, reg: map[string]string{}}
	e := &Env{GOOS: "darwin", Getenv: func(string) string { return "" }, Home: home, IsAdmin: true, Prefix: root, Sys: sys, Libraries: noFacts}
	for _, f := range []string{
		"Applications/WittyMediaServer.app/Contents/MacOS/WittyMediaServer",
		"Applications/WittyMediaServer.app/Contents/Helpers/wittymediaserver",
		"Applications/Uninstall WittyMediaServer.app/Contents/MacOS/Uninstall WittyMediaServer",
		"usr/local/bin/wittymediaserver",
		"Users/me/Library/LaunchAgents/com.wittywilla.wittymediaserver.tray.plist",
		"Users/me/Library/Caches/com.wittywilla.wittymediaserver/x",
		"Users/me/Library/Preferences/com.wittywilla.wittymediaserver.plist",
		"Users/you/Library/LaunchAgents/com.wittywilla.wittymediaserver.tray.plist",
	} {
		write(t, filepath.Join(root, filepath.FromSlash(f)), "x")
	}
	write(t, filepath.Join(root, "Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist"),
		"<dict><key>WMS_DATA_DIR</key><string>/Users/me/Library/Application Support/WittyMediaServer</string></dict>")
	fillData(t, filepath.Join(root, "Users/me/Library/Application Support/WittyMediaServer"))
	fillData(t, filepath.Join(root, "Users/you/Library/Application Support/WittyMediaServer"))
	write(t, filepath.Join(root, "Users/me/Library/Application Support/WittyMediaServer/wms.json"), "{}")
	write(t, filepath.Join(root, "Users/me/Movies/A Made-Up Film.mkv"), "film")
	res := Execute(MakePlan(e), Options{RemoveData: true})
	if !res.OK() {
		t.Fatal(res.Report())
	}
	if got := strings.Join(tree(t, root), "\n"); got != "Users/me/Movies/A Made-Up Film.mkv" {
		t.Errorf("left behind:\n%s", got)
	}
	if !sys.called("launchctl bootout system") || !sys.called("pkgutil --forget com.wittywilla.wittymediaserver.app") {
		t.Errorf("ran %v", sys.calls)
	}
}

func TestCommandLineAsksInPlainWords(t *testing.T) {
	cases := []struct {
		name, input string
		args        Args
		code        int
		dbStays     bool
	}{
		{"keeps data when told no", "n\ny\n", Args{}, 0, true},
		{"removes data when told yes", "y\ny\n", Args{}, 0, false},
		{"changes nothing unless go-ahead", "y\nn\n", Args{}, 1, true},
		{"no questions with the answers given", "", Args{Yes: true, Options: Options{RemoveData: true}}, 0, false},
		{"--yes alone keeps data", "", Args{Yes: true}, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, _, root := newUnix(t)
			db := filepath.Join(root, "var/lib/wittymediaserver/wms.db")
			fillData(t, filepath.Dir(db))
			var out bytes.Buffer
			if code := runCLI(e, c.args, strings.NewReader(c.input), &out); code != c.code {
				t.Fatalf("exit %d, want %d\n%s", code, c.code, out.String())
			}
			if exists(db) != c.dbStays {
				t.Errorf("database there: %v, want %v\n%s", exists(db), c.dbStays, out.String())
			}
			if !strings.Contains(out.String(), "never touched") {
				t.Errorf("doesn't say media is safe:\n%s", out.String())
			}
		})
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	e, sys, root := newUnix(t)
	fillData(t, filepath.Join(root, "var/lib/wittymediaserver"))
	before := tree(t, root)
	var out bytes.Buffer
	if code := runCLI(e, Args{DryRun: true}, strings.NewReader(""), &out); code != 0 {
		t.Fatal(out.String())
	}
	if strings.Join(before, "\n") != strings.Join(tree(t, root), "\n") {
		t.Error("files changed")
	}
	for _, c := range sys.calls {
		if !strings.HasPrefix(c, "dpkg-query") && !strings.HasPrefix(c, "rpm -q") && !strings.HasPrefix(c, "id ") {
			t.Errorf("ran %s", c)
		}
	}
	if !strings.Contains(out.String(), "wms.db") {
		t.Errorf("doesn't list the data:\n%s", out.String())
	}
}

func TestNeedsAdmin(t *testing.T) {
	e, _, root := newUnix(t)
	e.IsAdmin = false
	fillData(t, filepath.Join(root, "var/lib/wittymediaserver"))
	var out bytes.Buffer
	if code := runCLI(e, Args{Yes: true, Options: Options{RemoveData: true}}, strings.NewReader(""), &out); code != 1 {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	if !exists(filepath.Join(root, "var/lib/wittymediaserver/wms.db")) {
		t.Error("carried on without the rights")
	}
}

func TestParseArgs(t *testing.T) {
	a, err := ParseArgs([]string{"/S", "/RemoveData", "--program-folder", `C:\x y`, "--report", "r.txt"})
	if err != nil || !a.Yes || !a.RemoveData || a.ProgramFolder != `C:\x y` || a.Report != "r.txt" {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := ParseArgs([]string{"--remove-data", "--keep-data"}); err == nil {
		t.Error("both answers at once were taken")
	}
	if _, err := ParseArgs([]string{"--everything"}); err == nil {
		t.Error("an unknown option was taken")
	}
	b, _ := ParseArgs(a.Flags())
	if !b.Worker || !b.Yes || !b.RemoveData || b.ProgramFolder != a.ProgramFolder || b.Report != a.Report {
		t.Errorf("flags don't round-trip: %+v", b)
	}
}

// Anything the server starts keeping at the top of its data folder has to be known here, or
// it'd be left behind on a full clean-up.
func TestEveryDataFolderIsKnown(t *testing.T) {
	root := filepath.Join("..", "..")
	re := regexp.MustCompile(`(?:DataDir|dataDir|\.Data), "([^"]+)"`)
	var missing []string
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if strings.Contains(filepath.ToSlash(p), "internal/uninstall/") {
				return nil
			}
			for _, line := range strings.Split(string(b), "\n") {
				// Not a folder: the -data flag's own name, and arguments passed to the server.
				if strings.Contains(line, "fs.StringVar") || strings.Contains(line, `"-listen"`) {
					continue
				}
				for _, m := range re.FindAllStringSubmatch(line, -1) {
					name := m[1]
					if _, ok := dataEntry(name); !ok && name != RecordingsEntry {
						missing = append(missing, name+" in "+filepath.ToSlash(p))
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(missing) > 0 {
		t.Errorf("add these to DataEntries in data.go:\n  %s", strings.Join(missing, "\n  "))
	}
}
