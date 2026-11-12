// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"sync"
	"time"

	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
	"github.com/wittywilla/wittymodule/service"
)

// Consoles that standalone emulators play. GameCube and Wii have a core as well; Xbox, PS3 and
// Wii U have only standalone emulators, and the Switch only an emulator program its owner
// installed. Such a console is ready when one of its emulators is set up and ready here, and
// Play on its games starts that emulator. See docs/GAMING.md, Standalone emulators.

// standaloneOffers asks WittyPlay, at most every few seconds, which emulators are set up for
// each console. Without WittyPlay, or with its emulators off, it's empty.
type standaloneOffers struct {
	mu   sync.Mutex
	at   time.Time
	last map[string][]emulatorOffer
}

const standaloneTTL = 5 * time.Second

func (s *Server) consoleOffers(ctx context.Context) map[string][]emulatorOffer {
	o := &s.games().standalone
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.last != nil && time.Since(o.at) < standaloneTTL {
		return o.last
	}
	out := map[string][]emulatorOffer{}
	if service.Running != nil && service.Running(wittyPlayID) {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		var got map[string][]emulatorOffer
		if err := callWittyPlay(cctx, "docker.ConsoleOffers", []any{s.games().systemDir()}, []any{&got}); err != nil {
			s.Log.Debug("games: asking WittyPlay which consoles its emulators play", "err", err)
		} else if got != nil {
			out = got
		}
	}
	o.last, o.at = out, time.Now()
	return out
}

// readyEmulator is the first emulator ready to play a console here, or nil.
func readyEmulator(offers []emulatorOffer) *emulatorOffer {
	for i := range offers {
		if offers[i].Ready {
			return &offers[i]
		}
	}
	return nil
}

// standaloneWords says what a console with no core needs, when nothing is set up to play it.
func standaloneWords(c *wittyplay.Console, offers []emulatorOffer) string {
	for _, o := range offers {
		if o.Why != "" {
			return o.Why
		}
	}
	if len(c.Emulators) == 0 {
		return c.Name + " games play in an emulator program you install yourself: WittyPlay offers none. An administrator can add one in Settings, Standalone emulators, Your own emulator programs."
	}
	return c.Name + " games play in a standalone emulator. An administrator can install one in Settings, Standalone emulators."
}

// applyStandalone marks a console view ready when an emulator plays it here and no core does.
func applyStandalone(v *consoleView, offers []emulatorOffer) {
	v.Emulators = offers
	if v.Ready {
		return
	}
	if e := readyEmulator(offers); e != nil {
		v.Ready, v.Reason, v.Message, v.Standalone = true, "", "", e.ID
		return
	}
	if len(v.Cores) == 0 {
		v.Reason, v.Message = "emulator", standaloneWords(&v.Console, offers)
	}
}
