package ipc_test

import (
	"encoding/json"
	"testing"

	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittyplay/ipc"
)

// What a version's manifest says it speaks must be what it was compiled with.
func TestManifestNamesThisProtocol(t *testing.T) {
	var m struct {
		Capabilities map[string][]string `json:"capabilities"`
	}
	if err := json.Unmarshal(wittyplay.ManifestJSON, &m); err != nil {
		t.Fatal(err)
	}
	if got := m.Capabilities["ipc"]; len(got) != 1 || got[0] != ipc.Version {
		t.Fatalf("the manifest says %v and the code says %q", got, ipc.Version)
	}
}
