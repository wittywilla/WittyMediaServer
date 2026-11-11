// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"log/slog"
	"sort"
	"strings"
)

// gameModule is the module whose games used to count against it.
const gameModule = "wittyplay"

// liftGameFailureBlocks puts back in use a WittyPlay version that games failing to start took out
// of use, anywhere. One console's core crashing, or a console drawn on a graphics card on a server
// without one, used to count against WittyPlay itself, and three in a row took every game away
// from every client. Only WittyPlay's helper failing counts now (stream.ErrHelper, in the server's
// api_games.go), and those failures are recorded against the core instead. A version that went
// out of use that way goes back once, at the first start since. One that goes out of use again
// after that has failed in a way that does count, and stays out.
func liftGameFailureBlocks(id string, e *entry, log *slog.Logger) bool {
	if id != gameModule || e.state.Source != nil || e.state.GameFailuresUncounted {
		return false
	}
	e.state.GameFailuresUncounted = true
	vers := make([]string, 0, len(e.state.Blocked))
	for ver, why := range e.state.Blocked {
		if strings.HasPrefix(why, runnerStopped) {
			vers = append(vers, ver)
		}
	}
	sort.Strings(vers)
	for _, ver := range vers {
		log.Info("modules: putting a version back in use: games that couldn't start no longer count against it", "module", id, "version", ver, "was", e.state.Blocked[ver])
		delete(e.state.Blocked, ver)
		e.note("put-back", ver, "it was taken out of use because games kept failing to start, which is about those games and their cores, not this version")
	}
	return true
}
