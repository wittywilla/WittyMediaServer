package modules

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymodule"
)

// What the server wrote when every game on a Mac failed in the launcher's sandbox.
const macBlock = runnerStopped + ": The emulator crashed while running this game. Another core may work; you can choose one in Settings, Consoles & cores.)"

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

// A version a Mac took out of use while no game could start there is put back once, and only that.
func TestLiftNestedSandboxBlocks(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	blocked := func() *entry {
		man := &wittymodule.Manifest{ID: "wittyplay", Name: "WittyPlay", Version: "0.1.0", Delivery: wittymodule.DeliveryHelper}
		return &entry{builtin: man, man: man, known: map[string]*knownRelease{}, state: moduleState{Active: "0.3.4", Blocked: map[string]string{
			"0.3.4": macBlock,
			"0.3.3": runnerStopped + ": ipc: the runner said nothing)",
			"0.3.2": "files have changed since they were installed.",
			"0.3.1": "it kept failing (it stopped without answering)",
		}, Events: []Event{{Kind: "failed", Version: "0.3.4", Detail: macBlock}, {Kind: "taken-out-of-use", Version: "0.3.4", Detail: macBlock}}}}
	}
	e := blocked()
	if !liftNestedSandboxBlocks("darwin", "wittyplay", e, log) {
		t.Fatal("nothing changed on a Mac")
	}
	for _, ver := range []string{"0.3.4", "0.3.3"} {
		if r := e.state.Blocked[ver]; r != "" {
			t.Errorf("%s is still out of use: %s", ver, r)
		}
	}
	for _, ver := range []string{"0.3.2", "0.3.1"} {
		if e.state.Blocked[ver] == "" {
			t.Errorf("%s was put back, but it went out of use for its own reasons", ver)
		}
	}
	var back []string
	for _, ev := range e.state.Events {
		if ev.Kind == "put-back" {
			back = append(back, ev.Version)
		}
	}
	if len(back) != 2 || back[0] != "0.3.3" || back[1] != "0.3.4" {
		t.Errorf("events say %v were put back, want 0.3.3 and 0.3.4", back)
	}
	if st := e.status("wittyplay"); st.LastFail != "" {
		t.Errorf("the page still shows the failure that was this server's fault: %q", st.LastFail)
	}

	// Once only: failing the same way after the fix, it stays out.
	e.state.Blocked["0.3.4"] = macBlock
	if liftNestedSandboxBlocks("darwin", "wittyplay", e, log) || e.state.Blocked["0.3.4"] == "" {
		t.Error("a version that failed again after the fix was put back again")
	}

	for _, c := range []struct {
		name, system, id string
		e                *entry
	}{
		{"on Linux", "linux", "wittyplay", blocked()},
		{"on Windows", "windows", "wittyplay", blocked()},
		{"another module", "darwin", tunerID, blocked()},
		{"a fork", "darwin", "wittyplay", func() *entry { e := blocked(); e.state.Source = &Source{Repo: "someone/WittyPlay"}; return e }()},
	} {
		if liftNestedSandboxBlocks(c.system, c.id, c.e, log) || c.e.state.Blocked["0.3.4"] == "" {
			t.Errorf("%s: a version was put back", c.name)
		}
	}
}

// A server starting with the fix puts the version back, and says so in its saved state, so it
// doesn't again. A Mac's sandbox was the reason there; anywhere else, it's that games failing to
// start no longer count against WittyPlay (liftGameFailureBlocks).
func TestStartPutsBackWhatTheMacSandboxTookOutOfUse(t *testing.T) {
	dir := t.TempDir()
	builtin, err := json.Marshal(wittymodule.Manifest{ID: "wittyplay", Name: "WittyPlay", Version: "0.1.0", Repo: "wittywilla/WittyPlay",
		Delivery: wittymodule.DeliveryHelper, Entry: "wittyplay-module", API: wittymodule.API, MinServer: "0.1.0", MaxServer: "0.x"})
	if err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(map[string]moduleState{"wittyplay": {Active: "0.3.4", Blocked: map[string]string{"0.3.4": macBlock}}})
	if err := os.WriteFile(filepath.Join(dir, "state.json"), state, 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{Dir: dir, Server: "0.6.2-alpha", Builtins: [][]byte{builtin}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]moduleState
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	now := m.mods["wittyplay"].state
	m.mu.Unlock()
	if now.Blocked["0.3.4"] != "" {
		t.Errorf("0.3.4 is still out of use: %s", now.Blocked["0.3.4"])
	}
	// On a Mac the sandbox fix puts it back; anywhere, games that couldn't start no longer count.
	if s := saved["wittyplay"]; s.Blocked["0.3.4"] != "" || s.NestedSandboxFixed != (goos() == "darwin") || !s.GameFailuresUncounted {
		t.Errorf("the saved state doesn't say it was put back: %s", b)
	}
}
