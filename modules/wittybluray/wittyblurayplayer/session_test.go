package wittyblurayplayer

import (
	"testing"

	"github.com/wittywilla/wittymodule"
)

func TestLang3(t *testing.T) {
	if lang3("EN") != "eng" || lang3("fra") != "fra" || lang3("xx") != "" {
		t.Error("language codes")
	}
}

func TestManifest(t *testing.T) {
	m, err := wittymodule.ParseManifest(Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if m.Delivery != wittymodule.DeliveryHelper || m.Entry != "wittybluray-player" {
		t.Errorf("%+v", m)
	}
}
