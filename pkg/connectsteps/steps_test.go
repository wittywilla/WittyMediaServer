// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package connectsteps

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTheListHoldsTogether(t *testing.T) {
	want := []string{Read, Found, Secure, SignIn}
	steps := Steps()
	if len(steps) != len(want) {
		t.Fatalf("steps: %v", steps)
	}
	for i, s := range steps {
		if s.ID != want[i] || s.Title == "" || s.Failed == "" {
			t.Errorf("step %d: %+v", i, s)
		}
	}
	seen := map[string]bool{}
	reasons := map[string]string{}
	for _, c := range Codes() {
		if seen[c.Code] {
			t.Errorf("%s twice", c.Code)
		}
		seen[c.Code] = true
		if c.Code != "unknown" && StepIndex(c.Step) < 0 {
			t.Errorf("%s: no step %q", c.Code, c.Step)
		}
		if c.Say == "" || c.Next == "" || !strings.HasSuffix(c.Say, ".") || !strings.HasSuffix(c.Next, ".") {
			t.Errorf("%s: say and next are whole sentences: %q %q", c.Code, c.Say, c.Next)
		}
		for _, m := range c.Match {
			if m != strings.ToLower(m) {
				t.Errorf("%s: match %q isn't lower case", c.Code, m)
			}
		}
		for _, r := range c.Reasons {
			if other, ok := reasons[r]; ok {
				t.Errorf("reason %q is both %s and %s", r, other, c.Code)
			}
			reasons[r] = c.Code
		}
	}
	if !seen["unknown"] {
		t.Error("no unknown")
	}
}

// The words the connector and the server really say, and what each is: the
// cases in steps.json, which every app's tests answer too.
func TestClassify(t *testing.T) {
	var f struct {
		Cases []struct{ Reason, Text, Step, Code, At string }
	}
	if err := json.Unmarshal(raw, &f); err != nil || len(f.Cases) < 10 {
		t.Fatalf("cases: %v %d", err, len(f.Cases))
	}
	for _, c := range f.Cases {
		got := Classify(c.Reason, c.Text, c.Step)
		if got.Code != c.Code || got.Step != c.At || got.Say == "" || got.Next == "" {
			t.Errorf("Classify(%q, %q, %q) = %+v, want %s at %s", c.Reason, c.Text, c.Step, got, c.Code, c.At)
		}
	}
	if f := Of("expired"); f.Step != SignIn || f.Say == "" {
		t.Errorf("Of: %+v", f)
	}
	if f := Of("nope"); f.Code != "unknown" {
		t.Errorf("Of(nope): %+v", f)
	}
}
