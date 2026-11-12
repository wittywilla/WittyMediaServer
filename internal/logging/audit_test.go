// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package logging

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestNewCategoriesAndAliases(t *testing.T) {
	for _, id := range []string{"sdui", "modules-store", "emulators", "p2p", "remotectl", "profiles", "extras"} {
		if NormalizeCat(id) != id {
			t.Errorf("%s is not a category", id)
		}
	}
	for alias, want := range map[string]string{"themes": "sdui", "emulator": "emulators", "syncplay": "remotectl", "parental": "profiles", "karaoke": "extras", "store": "modules-store"} {
		if got := NormalizeCat(alias); got != want {
			t.Errorf("%s → %s, want %s", alias, got, want)
		}
	}
	seen := map[string]bool{}
	for _, c := range Categories {
		if seen[c.ID] || c.Label == "" || c.Description == "" {
			t.Errorf("category %q is repeated or has no words", c.ID)
		}
		if _, ok := ParseLevel(c.Default); !ok {
			t.Errorf("category %q starts at an unknown level", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestAuditIsNeverSilenced(t *testing.T) {
	l, log := newTest(t)
	l.SetCategoryLevel("modules-store", LevelOff)
	log.Info("ordinary line", "cat", "modules-store")
	for i := 0; i < floodLimit+10; i++ {
		Audit(context.Background(), log, "module override set", "cat", "modules-store", "module", "wittydvd", "by", "admin", "overridePassword", "PLANTED0override")
	}
	out := read(t, l)
	if strings.Contains(out, "ordinary line") {
		t.Error("an Off area still wrote an ordinary line")
	}
	if n := strings.Count(out, "module override set"); n != floodLimit+10 {
		t.Errorf("%d audit lines written, want %d: the flood limit or the level dropped some", n, floodLimit+10)
	}
	if !strings.Contains(out, "audit=yes") || strings.Contains(out, "PLANTED0override") {
		t.Errorf("audit line not marked, or its secret kept:\n%s", out)
	}
}

func TestFollowKinds(t *testing.T) {
	l, log := newTest(t)
	l.SetCategoryLevel("games", slog.LevelDebug)
	log.Info("game started", "cat", "games", "session", "abc123")
	log.Info("emulator up", "cat", "emulators", "session", "abc123")
	log.Info("game note without a session", "cat", "games")
	log.Info("party joined", "cat", "remotectl", "session", "party-1")
	log.Info("command", "cat", "remotectl", "session", "rc-9")
	log.Info("path changed", "cat", "p2p", "session", "p2p-4")
	log.Warn("client said", "cat", "clients", "session", "p2p-4")
	Audit(context.Background(), log, "device revoked", "cat", "p2p")

	got := func(kind string) string {
		page, err := l.Query(Filter{MinLevel: slog.LevelDebug, Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		var msgs []string
		for _, e := range page.Entries {
			msgs = append(msgs, e.Msg)
		}
		return strings.Join(msgs, "|")
	}
	cases := map[string]string{
		"game":  "game started|emulator up",
		"party": "party joined",
		"rc":    "command",
		"p2p":   "path changed|client said",
		"audit": "device revoked",
		"nope":  "",
	}
	for kind, want := range cases {
		if g := got(kind); g != want {
			t.Errorf("kind %s: %q, want %q", kind, g, want)
		}
	}
}

func TestRedactNewShapes(t *testing.T) {
	for in, want := range map[string]string{
		`{"code":"K7QM-2XHD"}`:       `{"code":"[hidden]"}`,
		`{"joinCode":"K7QM2XHD"}`:    `{"joinCode":"[hidden]"}`,
		`{"roomCode":"ABC234"}`:      `{"roomCode":"[hidden]"}`,
		`{"code":"timeout"}`:         `{"code":"timeout"}`, // an error's code is what a reader is after
		`{"code":"not-found"}`:       `{"code":"not-found"}`,
		"see ?k=1&kind=game":         "see ?k=1&kind=game", // k= only hidden in pairing addresses
		"/api/logs/entries?kind=p2p": "/api/logs/entries?kind=p2p",
	} {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%s) = %s, want %s", in, got, want)
		}
	}
}

// The server's own key stays hidden however many invites push newer secrets in.
func TestKeepSecretOutlastsTheRing(t *testing.T) {
	const seed = "PLANTED0p2pseed0value"
	KeepSecret(seed)
	for i := 0; i < 200; i++ {
		AddSecret(strings.Repeat("x", 6) + string(rune('a'+i%26)) + strings.Repeat("y", i%7) + "z" + string(rune('A'+i/26)))
	}
	if got := Redact("key " + seed + " in a line"); strings.Contains(got, seed) {
		t.Fatalf("the kept secret was pushed out: %s", got)
	}
}
