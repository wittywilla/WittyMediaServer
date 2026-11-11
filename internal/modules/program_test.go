// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"bytes"
	"testing"

	"github.com/wittywilla/wittymodule"
)

// A program that isn't a helper is installed without being started: it has no greeting to give,
// and installing it used to wait for one until it failed.
func TestAProgramIsInstalledWithoutBeingStarted(t *testing.T) {
	w := newWorld(t)
	program := bytes.Replace(manifestJSON("0.2.0", "0.1.0", "0.x", wittymodule.API, "crash"), []byte(`"delivery":"helper"`), []byte(`"delivery":"program"`), 1)
	w.publish("0.2.0", program)
	w.builtinProgram = true
	m := w.manager()
	defer m.Close()
	check(t, m)
	if s := installWait(t, m); s.Version != "0.2.0" || s.Error != "" {
		t.Fatalf("%+v", s)
	}
	if _, _, ok := m.Program("wittyphotos"); !ok {
		t.Error("it can't be found to be started")
	}
}
