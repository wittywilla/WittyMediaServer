package wittymoonlight

import (
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
	want := `moonlight 'stream' '192.168.1.20' 'It''s; rm -rf / $(x) "y"'`
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
