package wittycouch

import (
	"encoding/base64"
	"os/exec"
	"strings"
	"testing"
)

func target() Target {
	return Target{RigName: "Living Room", Address: "192.168.1.20", UUID: "abc", AppID: 7, AppName: "Space Game"}
}

func kinds(ms []Method) string {
	var k []string
	for _, m := range ms {
		k = append(k, m.Kind)
	}
	return strings.Join(k, ",")
}

func TestPlanPerDevice(t *testing.T) {
	for _, c := range []struct {
		d      Device
		inPage string
		want   string
	}{
		{Device{"desktop", "windows"}, "", "command,steps"},
		{Device{"desktop", "linux"}, "https://x/", "client,command,steps"},
		{Device{"browser", "windows"}, "", "command,steps"},
		{Device{"browser", "mac"}, "https://x/", "client,command,steps"},
		{Device{"browser", "android"}, "", "steps"},
		{Device{"browser", "ios"}, "https://x/", "client,steps"},
		{Device{"android", "android"}, "", "intent,steps"},
		{Device{"android", "androidtv"}, "", "intent,steps"},
		{Device{"android", "firetv"}, "", "intent,steps"},
		{Device{"apple", "ios"}, "", "steps"},
		{Device{"apple", "tvos"}, "", "steps"},
	} {
		if got := kinds(Plan(c.d, target(), c.inPage != "", "")); got != c.want {
			t.Errorf("%+v: %s, want %s", c.d, got, c.want)
		}
	}
	ms := Plan(Device{"android", "androidtv"}, target(), false, "")
	if ms[0].Package != "com.limelight" || ms[0].Activity != "com.limelight.ShortcutTrampoline" || ms[0].Extras["UUID"] != "abc" || ms[0].Extras["AppId"] != "7" {
		t.Errorf("%+v", ms[0])
	}
	away := target()
	away.Address, away.Away = "", true
	if got := kinds(Plan(Device{"desktop", "windows"}, away, false, "")); got != "" {
		t.Errorf("away with nothing to connect to: %s", got)
	}
	if got := kinds(Plan(Device{"browser", "windows"}, away, true, "")); got != "client" {
		t.Errorf("away with the client: %s", got)
	}
}

func TestHostileAppNames(t *testing.T) {
	for _, name := range []string{"--quit-after", "-x", "a\nb", ""} {
		tg := target()
		tg.AppName = name
		if MoonlightArgs(tg) != nil {
			t.Errorf("%q would go on a command line", name)
		}
		if got := kinds(Plan(Device{"desktop", "windows"}, tg, false, "")); strings.Contains(got, "command") {
			t.Errorf("%q: %s", name, got)
		}
	}
	tg := target()
	tg.AppName = `It's; rm -rf / $(x) "y"`
	ms := Plan(Device{"browser", "linux"}, tg, false, "")
	want := `moonlight 'stream' '192.168.1.20' 'It'\''s; rm -rf / $(x) "y"'`
	if ms[0].Text != want {
		t.Errorf("command line: %s", ms[0].Text)
	}
	tg = target()
	tg.Port = 48000
	if a := MoonlightArgs(tg); a[1] != "192.168.1.20:48000" {
		t.Errorf("%v", a)
	}
	tg.Address = "fd00::1"
	if a := MoonlightArgs(tg); a[1] != "[fd00::1]:48000" {
		t.Errorf("%v", a)
	}
	tg.Port = 0
	if a := MoonlightArgs(tg); a[1] != "[fd00::1]" {
		t.Errorf("%v", a)
	}
	if PairArgs("-x", 0, "1234") != nil || PairArgs("pc", 0, "12") != nil {
		t.Error("bad pair arguments")
	}
}

// What someone pastes has to be one word to the shell that reads it, whatever the PC calls
// its game. These ask each shell itself; PowerShell's is only parsed, never run.
func TestAppNameStaysOneWordInEachShell(t *testing.T) {
	names := []string{`It's; exit 9`, "x\u2019; exit 9; \u2019", "x\u2018; exit 9; \u2018", "x\u201a; exit 9; \u201a", "x\u201b; exit 9; \u201b", "\u2019''\u2018", `$(exit 9) "y"`}
	if sh, err := exec.LookPath("sh"); err == nil {
		for _, name := range names {
			out, err := exec.Command(sh, "-c", "printf %s "+quoted("linux", name)).Output()
			if err != nil || string(out) != name {
				t.Errorf("sh: %q came out as %q: %v", name, out, err)
			}
		}
	}
	ps, err := exec.LookPath("powershell")
	if err != nil {
		return
	}
	parse := func(text string) []string {
		line := base64.StdEncoding.EncodeToString([]byte(text))
		script := `$errs=$null;$toks=$null;$a=[System.Management.Automation.Language.Parser]::ParseInput([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + line + `')),[ref]$toks,[ref]$errs);$p=$a.EndBlock.Statements;if($errs.Count -or $p.Count -ne 1){'statements='+$p.Count;exit};$e=$p[0].PipelineElements[0].CommandElements;$e.Count;[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($e[$e.Count-1].Value))`
		out, err := exec.Command(ps, "-NoProfile", "-NonInteractive", "-Command", script).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.Fields(string(out))
	}
	// Doubling only the plain quote is what let a name out: this check has to see that.
	if got := parse("moonlight 'x\u2019; exit 9; \u2019'"); len(got) != 1 || got[0] != "statements=3" {
		t.Errorf("the parser should split a curly quote that isn't doubled: %q", got)
	}
	for _, name := range names {
		lines := parse(commandLine("windows", []string{"stream", "pc", name}))
		out := strings.Join(lines, " ")
		if len(lines) != 2 || lines[0] != "4" {
			t.Errorf("PowerShell: %q reads as %q", name, out)
			continue
		}
		got, err := base64.StdEncoding.DecodeString(lines[1])
		if err != nil || string(got) != name {
			t.Errorf("PowerShell: %q came out as %q", name, got)
		}
	}
}

func TestInstallLinksAreTheProjects(t *testing.T) {
	for _, os := range []string{"windows", "mac", "linux", "android", "androidtv", "firetv", "ios", "tvos", "chromeos"} {
		i := InstallFor(os)
		if i == nil || i.License != "GPL-3.0" || !strings.HasPrefix(i.Source, "https://github.com/moonlight-stream/") {
			t.Errorf("%s: %+v", os, i)
		}
	}
	if InstallFor("toaster") != nil {
		t.Error("toaster")
	}
}
