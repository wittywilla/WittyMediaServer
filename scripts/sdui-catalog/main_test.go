// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// What it prints is the catalogue, whole: its revision and every table an app draws with.
func TestItPrintsTheCatalogue(t *testing.T) {
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Revision   int               `json:"revision"`
		CardStyles []json.RawMessage `json:"cardStyles"`
		Routes     []json.RawMessage `json:"routes"`
		Commands   []json.RawMessage `json:"commands"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	c := sdui.Catalog()
	if got.Revision != c.Revision || len(got.CardStyles) != len(c.CardStyles) || len(got.Routes) != len(c.Routes) || len(got.Commands) != len(c.Commands) || len(got.Commands) == 0 {
		t.Errorf("revision %d, %d styles, %d routes, %d commands", got.Revision, len(got.CardStyles), len(got.Routes), len(got.Commands))
	}
}
