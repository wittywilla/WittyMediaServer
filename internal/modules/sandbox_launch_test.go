package modules

import (
	"io"
	"log/slog"
	"testing"

	"github.com/wittywilla/wittymodule"
)

// On a Mac, WittyPlay's helper starts without the launcher, as it puts each game in a sandbox of
// its own and a Mac can't start one inside another. Everything else, and every fork, keeps it.
func TestHelperLauncher(t *testing.T) {
	const launcher = "/server"
	for _, c := range []struct {
		system, id, want string
	}{
		{"darwin", "wittyplay", ""},
		{"darwin", tunerID, launcher},
		{"darwin", "wittybluray", launcher},
		{"darwin", "wittyphotos", launcher},
		{"linux", "wittyplay", launcher},
		{"windows", "wittyplay", launcher},
		{"freebsd", "wittyplay", launcher},
	} {
		if got := helperLauncher(c.system, c.id, launcher); got != c.want {
			t.Errorf("%s on %s: launcher %q, want %q", c.id, c.system, got, c.want)
		}
	}
}

// What the manager gives each helper on this system.
func TestHelperLauncherInUse(t *testing.T) {
	const launcher = "/server"
	m := &Manager{cfg: Config{Launcher: launcher}, dir: t.TempDir(), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	play := &wittymodule.Manifest{ID: "wittyplay", Permissions: []string{wittymodule.PermissionNetwork, wittymodule.PermissionPrograms,
		wittymodule.PermissionDevices, wittymodule.PermissionData}}
	tuner := &wittymodule.Manifest{ID: tunerID, Permissions: []string{wittymodule.PermissionNetwork, wittymodule.PermissionPrograms}}
	stock := func() *entry { return &entry{} }
	fork := func() *entry { return &entry{state: moduleState{Source: &Source{Repo: "someone/WittyPlay"}}} }

	want := launcher
	if goos() == "darwin" {
		want = ""
	}
	if got := m.newProcFor("wittyplay", stock(), play, t.TempDir()).launch; got != want {
		t.Errorf("WittyPlay's helper on %s starts through %q, want %q", goos(), got, want)
	}
	if got := m.newProcFor("wittyplay", fork(), play, t.TempDir()).launch; got != launcher {
		t.Errorf("a fork of WittyPlay starts through %q, want the launcher", got)
	}
	if got := m.newProcFor(tunerID, stock(), tuner, t.TempDir()).launch; got != launcher {
		t.Errorf("WittyTuner's helper starts through %q, want the launcher", got)
	}
}
