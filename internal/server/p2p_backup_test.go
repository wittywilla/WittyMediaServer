// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// A WittyRelay's access key is a key: a backup made with no password, which
// leaves keys out, leaves it out too, and keeps the rest of the settings.
func TestP2PBackupsWithNoPasswordLeaveOutRelayKeys(t *testing.T) {
	raw, _ := json.Marshal(p2pSettings{Enabled: true, Port: 40001, OwnRelays: []p2pOwnRelay{
		{Addr: "/ip4/198.51.100.1/udp/4001/quic-v1/p2p/12D3KooWD3eckifWpRn9wQpMG9R9hX3sD158z7EqHWmweQAJU5SA", Key: "relay-access-key-0123456789", HasKey: true},
	}})
	f := BackupScrubSettings()[p2pSettingKey]
	if f == nil {
		t.Fatal("the peer-to-peer settings aren't scrubbed in backups")
	}
	out := f(string(raw))
	if strings.Contains(out, "relay-access-key") {
		t.Fatalf("the key is still there: %s", out)
	}
	var back p2pSettings
	if err := json.Unmarshal([]byte(out), &back); err != nil || !back.Enabled || back.Port != 40001 || len(back.OwnRelays) != 1 || back.OwnRelays[0].HasKey || back.OwnRelays[0].Addr == "" {
		t.Fatalf("the rest didn't survive: %+v %v", back, err)
	}
	if f("not json") != "" {
		t.Fatal("unreadable settings were kept as they were")
	}
}
