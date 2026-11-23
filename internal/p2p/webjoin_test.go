// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"encoding/hex"
	"testing"
)

// The code typed on a device and taken over the web port: the apps compute the same keys and proofs
// (WittyMediaClient, desktop/code.js and its test hold these same values), so a change here is a
// change of the wire format, to be made in both.
func TestWebJoinVectors(t *testing.T) {
	find, proof, err := DeriveWebJoin("k7qm 2xhd")
	if err != nil {
		t.Fatal(err)
	}
	dn, sn := make([]byte, 16), make([]byte, 16)
	for i := range dn {
		dn[i], sn[i] = byte(i), byte(100+i)
	}
	for _, c := range []struct{ what, got, want string }{
		{"find", hex.EncodeToString(find), "7570bed5d42ed757a21d20977ab7222b47d6a3ad37e493928f642d59bda56ed9"},
		{"proof", hex.EncodeToString(proof), "5258c623e6e31dee14dc3a9139e33dae09f7b558b63211be3989babcb0e6002d"},
		{"server proof", hex.EncodeToString(WebJoinProof(proof, "server", dn, sn, "ed25519-key-text")), "7e3132a96c5430476c4ea7c809fa9ef4748f853b0af4ba078d35a4f3f02af2fe"},
	} {
		if c.got != c.want {
			t.Errorf("%s: %s, want %s", c.what, c.got, c.want)
		}
	}
	// A proof for the web port is never one for the peer-to-peer join, and the roles differ.
	if hex.EncodeToString(WebJoinProof(proof, "device", dn, sn, "ed25519-key-text")) == "7e3132a96c5430476c4ea7c809fa9ef4748f853b0af4ba078d35a4f3f02af2fe" {
		t.Error("the device's proof is the server's")
	}
	if _, _, err := DeriveWebJoin("K7QM-2XHU"); err == nil {
		t.Error("a code with a U was taken")
	}
}
