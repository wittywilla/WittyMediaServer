// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package connectsteps is the one list of the steps a device goes through to
// connect from away, and of the reasons one can fail, with the plain words to
// show for each and the next thing to try. The server's attempt history, its
// Check connection, the connector library and every app use the same codes:
// the apps keep a copy of steps.json (WittyMediaClient shared/data/connect-steps.json).
package connectsteps

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed steps.json
var raw []byte

// The steps, in order.
const (
	Read   = "read"   // code or link read
	Found  = "found"  // server found
	Secure = "secure" // encrypted connection made
	SignIn = "signin" // signed in
)

// Step is one step of connecting.
type Step struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Failed string `json:"failed"`
}

// Code is one reason a step can fail.
type Code struct {
	Code    string   `json:"code"`
	Step    string   `json:"step"`
	Reasons []string `json:"reasons"`
	Match   []string `json:"match"`
	Say     string   `json:"say"`
	Next    string   `json:"next"`
}

// File is steps.json.
type File struct {
	Version int    `json:"version"`
	Steps   []Step `json:"steps"`
	Codes   []Code `json:"codes"`
}

var file = func() File {
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		panic("connectsteps: steps.json: " + err.Error())
	}
	return f
}()

// Raw is steps.json as it is, for serving to apps.
func Raw() []byte { return raw }

// Steps are the steps, in order.
func Steps() []Step { return append([]Step(nil), file.Steps...) }

// Codes are every code, in the order they are tried.
func Codes() []Code { return append([]Code(nil), file.Codes...) }

// Lookup finds a code. ok is false for one the list doesn't have.
func Lookup(code string) (Code, bool) {
	for _, c := range file.Codes {
		if c.Code == code {
			return c, true
		}
	}
	return Code{}, false
}

// StepIndex is where a step comes in the order, or -1.
func StepIndex(id string) int {
	for i, s := range file.Steps {
		if s.ID == id {
			return i
		}
	}
	return -1
}

// Failure is a step that failed, why, and what to do: what an app shows and
// what the server's attempt history keeps.
type Failure struct {
	Step  string `json:"step"`
	Code  string `json:"code"`
	Say   string `json:"say"`
	Next  string `json:"next"`
	Error string `json:"error,omitempty"` // the technical words it came from
}

// Classify names the failure behind a reason (a server's answer, such as
// "expired") and an error's words. A reason is looked for first, then the
// words; with neither known, the failure is "unknown" at step, the step that
// was being tried.
func Classify(reason, text, step string) Failure {
	reason = strings.TrimSpace(reason)
	lower := strings.ToLower(text)
	pick := func(c Code) Failure {
		f := Failure{Step: c.Step, Code: c.Code, Say: c.Say, Next: c.Next, Error: text}
		if f.Step == "" {
			f.Step = step
		}
		return f
	}
	if reason != "" {
		for _, c := range file.Codes {
			for _, r := range c.Reasons {
				if r == reason {
					return pick(c)
				}
			}
		}
	}
	if lower != "" {
		for _, c := range file.Codes {
			for _, m := range c.Match {
				if strings.Contains(lower, m) {
					return pick(c)
				}
			}
		}
	}
	u, _ := Lookup("unknown")
	return pick(u)
}

// Of is the failure for a code already known, at its own step.
func Of(code string) Failure {
	c, ok := Lookup(code)
	if !ok {
		c, _ = Lookup("unknown")
	}
	return Failure{Step: c.Step, Code: c.Code, Say: c.Say, Next: c.Next}
}
