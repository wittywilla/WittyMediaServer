// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package update

import (
	"crypto/ed25519"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymodule"
)

func withdrawalList(t *testing.T, module string, serial int, versions ...string) []byte {
	t.Helper()
	w := wittymodule.Withdrawals{Module: module, Serial: serial}
	for _, v := range versions {
		w.Versions = append(w.Versions, wittymodule.Withdrawn{Version: v, Severity: wittymodule.SeveritySecurity, Reason: "it had a hole"})
	}
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWithdrawnVersionIsNeverOffered(t *testing.T) {
	rels := []ghRelease{
		{TagName: "v1.1.0", Assets: []GHAsset{{Name: ArchiveName("1.1.0", "linux", "amd64"), URL: "https://github.com/a"}}},
		{TagName: "v1.2.0", Assets: []GHAsset{{Name: ArchiveName("1.2.0", "linux", "amd64"), URL: "https://github.com/b"}}},
		{TagName: "v1.3.0-alpha", Prerelease: true, Assets: []GHAsset{{Name: ArchiveName("1.3.0-alpha", "linux", "amd64"), URL: "https://github.com/c"}}},
		{TagName: "withdrawals"},
	}
	if r := latest(rels, "1.0.0", "linux", "amd64", nil); r == nil || r.Version != "1.2.0" {
		t.Fatalf("someone on a full release gets the newest full release: %+v", r)
	}
	if r := latest(rels, "1.0.0-alpha", "linux", "amd64", nil); r == nil || r.Version != "1.3.0-alpha" {
		t.Fatalf("someone on a test version goes on testing: %+v", r)
	}
	w := &wittymodule.Withdrawals{Module: serverModuleName, Serial: 1, Versions: []wittymodule.Withdrawn{{Version: "1.2.0", Severity: wittymodule.SeveritySecurity}}}
	if r := latest(rels, "1.0.0", "linux", "amd64", w); r == nil || r.Version != "1.1.0" {
		t.Fatalf("a withdrawn version was offered: %+v", r)
	}
}

// What a feed that lies can and can't do with the list.
func TestWithdrawalListIsSignedAndOnlyGoesForward(t *testing.T) {
	key, pub := newKey(t)
	file := filepath.Join(t.TempDir(), "kept.json")
	trustedKeysFor = func() []ed25519.PublicKey { return []ed25519.PublicKey{pub} }
	defer func() { trustedKeysFor = TrustedKeys }()
	u := New("1.0.0", t.TempDir(), nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	u.LoadWithdrawals(file)

	take := func(list []byte) error { return u.takeWithdrawals(list, Sign(key, list)) }
	if err := take(withdrawalList(t, serverModuleName, 2, "1.0.0")); err != nil {
		t.Fatal(err)
	}
	if u.Status().Withdrawn == "" {
		t.Fatal("the running version was withdrawn, and nothing says so")
	}
	// An older list brings nothing back.
	if err := take(withdrawalList(t, serverModuleName, 1)); err != nil {
		t.Fatal(err)
	}
	if u.Status().Withdrawn == "" {
		t.Error("an older list brought a withdrawn version back")
	}
	// A list with no signature from a key this server trusts.
	otherKey, _ := newKey(t)
	unsigned := withdrawalList(t, serverModuleName, 9)
	if err := u.takeWithdrawals(unsigned, Sign(otherKey, unsigned)); err == nil {
		t.Error("a list signed by someone else was taken")
	}
	if u.Status().Withdrawn == "" {
		t.Error("a forged list changed what is withdrawn")
	}
	// A module's list, however well signed.
	if err := take(withdrawalList(t, "wittydvd", 9)); err == nil {
		t.Error("a module's list was taken as the server's")
	}
	// It survives a restart, so hiding the list doesn't undo it.
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
	again := New("1.0.0", t.TempDir(), nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	again.LoadWithdrawals(file)
	if again.Status().Withdrawn == "" {
		t.Error("the list was forgotten at restart")
	}
}
