// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymodule"
)

// WittyPlay's helper starts each game in sandbox-exec. Started the way the manager starts it, a
// helper like it can do that; in the launcher's sandbox, as every game was started before, it
// can't, because a Mac won't start one sandbox inside another.
func TestMacHelperCanBoxInItsPrograms(t *testing.T) {
	if !profileWorks() {
		t.Skip("sandbox-exec isn't here, or refuses the helper's profile")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	raw, err := json.Marshal(wittymodule.Manifest{ID: "wittyplay", Name: "WittyPlay", Version: "0.3.4", Repo: "wittywilla/WittyPlay",
		Delivery: wittymodule.DeliveryHelper, Entry: "wittyplay-module", API: MinAPI, MinServer: "0.4.0", MaxServer: "0.5.x",
		Permissions: []string{wittymodule.PermissionNetwork, wittymodule.PermissionPrograms, wittymodule.PermissionDevices, wittymodule.PermissionData}})
	if err != nil {
		t.Fatal(err)
	}
	man, err := wittymodule.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, wittymodule.ManifestFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wittyplay-module"), helperBytes(t), 0o755); err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: Config{Launcher: self}, dir: t.TempDir(), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	start := func(launcher string) *proc {
		p := m.newProcFor("wittyplay", &entry{}, man, dir)
		p.verify = nil // the test's files aren't signed
		if launcher != "" {
			p.launch = launcher
		}
		t.Cleanup(p.Close)
		return p
	}

	p := start("")
	if p.launch != "" {
		t.Fatalf("WittyPlay's helper starts through %q, want no launcher", p.launch)
	}
	if _, err := p.Call("sandbox", nil, nil); err != nil {
		t.Errorf("WittyPlay's helper couldn't start a program in a sandbox: %v", err)
	}

	// The launcher's sandbox, as WittyPlay's helper had before. If this ever works, a Mac has
	// learnt to nest sandboxes, and WittyPlay's helper can go back in the launcher's (ownSandbox).
	p = start(self)
	_, err = p.Call("sandbox", nil, nil)
	if err == nil {
		t.Fatal("a program in the launcher's sandbox started a sandbox of its own: a Mac nests them now, so ownSandbox can go")
	}
	if !strings.Contains(err.Error(), "sandbox_apply") {
		t.Errorf("in the launcher's sandbox it failed, but not as expected: %v", err)
	}
	if p.launch != self {
		t.Error("the helper didn't start in the launcher, so this didn't try what it meant to")
	}
}
