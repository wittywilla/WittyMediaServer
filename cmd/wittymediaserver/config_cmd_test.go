package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/config"
)

func TestListenInCommand(t *testing.T) {
	for _, c := range []struct {
		words []string
		want  string
	}{
		// What the Windows service of 0.6 was registered with, as the installer reads it back.
		{[]string{`#%C:\Program Files\WittyMediaServer\bin\wittymediaserver.exe`, "--windows-service", "serve", "-data", `C:\ProgramData\WittyMediaServer`, "-listen", ":8181"}, "8181"},
		{[]string{"serve", "-listen=:9000"}, "9000"},
		{[]string{"serve", "--listen", `":7000"`}, "7000"},
		{[]string{"serve", "-listen", "127.0.0.1:8181"}, ""}, // an address of its own isn't just a port
		{[]string{"serve", "-data", "x"}, ""},
		{nil, ""},
	} {
		if got := listenInCommand(c.words); got != c.want {
			t.Errorf("%q: got %q, want %q", c.words, got, c.want)
		}
	}
}

// An upgrade carries the old service's port into the data folder's settings.ini, and the next
// start uses it. An explicit -port wins; nothing to carry leaves nothing behind.
func TestConfigPortCarriesOver(t *testing.T) {
	t.Setenv("WMS_LISTEN", "")
	t.Setenv("WMS_CONFIG", "")
	empty := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(empty, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	settings := config.DataSettingsFile(data)

	if code := cmdConfigPort([]string{"-data", data, "-config", empty, "-port", "", "-from-command", "serve", "-data", data}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(settings); err == nil {
		t.Fatal("nothing to carry over, but a file was written")
	}

	if code := cmdConfigPort([]string{"-data", data, "-config", empty, "-in-data", "-port", "", "-from-command", `#%C:\wms.exe`, "--windows-service", "serve", "-listen", ":8181"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	c, err := config.ParseOnly([]string{"-data", data, "-config", empty})
	if err != nil || c.Listen != ":8181" || c.ListenFrom != config.FromSettings {
		t.Fatalf("after carrying over: %+v %v", c, err)
	}

	if code := cmdConfigPort([]string{"-data", data, "-config", empty, "-port", "8282", "-from-command", "-listen", ":8181"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if s, _ := config.ReadSettings(settings); s.Port != 8282 {
		t.Errorf("-port should win: %+v", s)
	}

	if code := cmdConfigPort([]string{"-data", data, "-config", empty, "70000"}); code != 2 {
		t.Errorf("70000 was taken: exit %d", code)
	}
}
