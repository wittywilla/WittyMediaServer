package wittydvdplayer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wittywilla/wittymodule"
)

func TestUOPLocks(t *testing.T) {
	if got := strings.Join(uopLocks(1<<0|1<<1|1<<19), ","); got != "seek,chapter,pause" {
		t.Errorf("locks: %s", got)
	}
	if uopLocks(0) != nil {
		t.Error("nothing forbidden, nothing locked")
	}
}

func TestManifest(t *testing.T) {
	m, err := wittymodule.ParseManifest(Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if m.Delivery != wittymodule.DeliveryHelper || m.Entry != "wittydvd-player" || !json.Valid(Manifest) {
		t.Errorf("%+v", m)
	}
}
