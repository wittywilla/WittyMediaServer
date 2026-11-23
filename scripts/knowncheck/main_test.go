// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	known := `{"format":1,"serial":3,"generated":"a","modules":[
		{"id":"a","name":"A","icon":{"sha256":"11"},"versions":[{"version":"1.0.0"}]},
		{"id":"gone","name":"Gone"}]}`
	same := `{"format":1,"serial":57,"generated":"b","modules":[
		{"id":"gone","name":"Gone"},
		{"id":"a","name":"A","icon":{"sha256":"11"},"versions":[{"version":"2.0.0"}]}]}`
	if d, err := compare([]byte(known), []byte(same)); err != nil || len(d) != 0 {
		t.Fatalf("versions, serial, date and order are not differences: %v %v", d, err)
	}
	live := `{"format":2,"serial":57,"modules":[
		{"id":"a","name":"A","icon":{"sha256":"22"}},
		{"id":"new","name":"New"}]}`
	d, err := compare([]byte(known), []byte(live))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(d, "\n")
	for _, want := range []string{`"format"`, "a icon:", "gone: built in, but no longer published", "new: published, but not built in"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if len(d) != 4 {
		t.Errorf("want 4 differences, got:\n%s", got)
	}
	if _, err := compare([]byte(`{"modules":1}`), []byte(live)); err == nil {
		t.Error("a file with no list of modules must be refused")
	}
}
