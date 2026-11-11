// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"log/slog"
	"sort"
	"strings"
	"sync/atomic"
)

// SandboxFlag is what the server starts itself with to box a program in before it runs.
const SandboxFlag = "__module-sandbox"

// What the launcher is asked to allow and to insist on.
const (
	SandboxNetwork   = 1 << iota // it may have a network, and open devices for writing
	SandboxStrict                // a fork: if a limit can't be applied, it doesn't run
	SandboxDevices               // it may open devices for writing, as a graphics card is
	SandboxSaveArg               // it may write to the folder after its -save argument
	SandboxWriteData             // it may write to WITTYMODULE_DATA, and the folders in SandboxWriteEnv
	SandboxPrograms              // it may start other programs
)

// SandboxWriteEnv lists other folders a program may write to, as the environment.
const SandboxWriteEnv = "WITTYMODULE_WRITE"

// How well a helper is boxed in on this system.
const (
	SandboxFull       = "full"       // everything the security model promises
	SandboxPartial    = "partial"    // most of it, and the words say what's missing
	SandboxLimited    = "limited"    // some limits, which aren't a sandbox
	SandboxUnconfined = "unconfined" // nothing that would stop it doing harm
)

// SandboxInfo says, in words a person can act on, what a module's helper is held to here. Anything
// short of full is shown on the pages that add modules.
type SandboxInfo struct {
	Level string `json:"level"`
	Title string `json:"title,omitempty"`
	Words string `json:"words,omitempty"`
}

func unconfined() SandboxInfo {
	return SandboxInfo{Level: SandboxUnconfined, Title: "Modules aren't confined on this system",
		Words: "A module's helper runs with everything this server's account can do: it could change your files, start other programs, and send what it reads over the internet. Only modules from this project run here, and modules from other publishers are refused."}
}

// launcherFailed: helpers wouldn't start in the launcher here, and went on without it.
var launcherFailed atomic.Bool

// ownSandbox are the modules whose helper puts each program it starts in a sandbox of its own,
// as WittyPlay does with every game.
var ownSandbox = map[string]bool{"wittyplay": true}

// helperLauncher is the launcher a module's own helper starts through. A fork always has it, and
// it refuses forks on a Mac. A Mac won't start one sandbox inside another: sandbox-exec, run by a
// helper already in the launcher's, fails with "sandbox_apply: Operation not permitted", so every
// game failed to start, as "The emulator crashed". There, a helper that boxes in what it starts
// goes without the launcher, and what it starts keeps its own sandbox, the tighter of the two:
// WittyPlay's game runner has no network, starts nothing and writes only its saves.
func helperLauncher(system, id, launcher string) string {
	if system == "darwin" && ownSandbox[id] {
		return ""
	}
	return launcher
}

// runnerStopped begins the reason a version is out of use when its game runner kept stopping
// before the game was ready: the server reported each one (api_games.go), and three did it.
// It no longer does (liftGameFailureBlocks), so nothing new is blocked with this reason.
const runnerStopped = "it kept failing (the game runner stopped before the game was ready"

// liftNestedSandboxBlocks puts back in use what a Mac took out of use before helperLauncher. No
// game could start there, so a WittyPlay version that was tried went out of use for it, and the
// fault was this server's. Done once, at the first start with the fix, so a version that fails the
// same way after that stays out. It says whether the state changed and needs saving.
func liftNestedSandboxBlocks(system, id string, e *entry, log *slog.Logger) bool {
	if system != "darwin" || !ownSandbox[id] || e.state.Source != nil || e.state.NestedSandboxFixed {
		return false
	}
	e.state.NestedSandboxFixed = true
	vers := make([]string, 0, len(e.state.Blocked))
	for ver, why := range e.state.Blocked {
		if strings.HasPrefix(why, runnerStopped) {
			vers = append(vers, ver)
		}
	}
	sort.Strings(vers)
	for _, ver := range vers {
		log.Info("modules: putting a version back in use: no game could start in the server's sandbox on a Mac", "module", id, "version", ver, "was", e.state.Blocked[ver])
		delete(e.state.Blocked, ver)
		e.note("put-back", ver, "it was taken out of use because no game could start on a Mac, which was this server's fault, not the version's")
	}
	return true
}
