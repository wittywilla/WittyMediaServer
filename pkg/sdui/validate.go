// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Validation limits.
const (
	// MaxDepth is the deepest nesting Validate accepts (root is depth 1).
	MaxDepth = 32
	// MaxNodes is the largest node count Validate accepts.
	MaxNodes = 5000
	// MaxIDLength is the longest node ID Validate accepts.
	MaxIDLength = 128
)

// Issue is one validation problem, located by a JSON-pointer-like path into
// the document ("/root/children/2/props/title").
type Issue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// String renders the issue as "path: message".
func (i Issue) String() string { return i.Path + ": " + i.Message }

// ValidationError aggregates every problem found by Validate.
type ValidationError struct {
	Issues []Issue
}

// Error lists the issues, one per line, prefixed by a count.
func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "sdui: document is valid"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "sdui: %d validation issue(s):", len(e.Issues))
	for _, i := range e.Issues {
		b.WriteString("\n  ")
		b.WriteString(i.String())
	}
	return b.String()
}

func (e *ValidationError) add(path, format string, args ...any) {
	e.Issues = append(e.Issues, Issue{Path: path, Message: fmt.Sprintf(format, args...)})
}

// Validate checks doc against every schema rule and returns nil when it is
// valid, or a *ValidationError listing every problem found.
func Validate(doc *Document) error {
	ve := &ValidationError{}
	if doc == nil {
		ve.add("", "document is nil")
		return ve
	}
	if doc.Version != SchemaVersion {
		ve.add("/version", "unsupported schema version %d (want %d)", doc.Version, SchemaVersion)
	}
	if strings.TrimSpace(doc.Screen) == "" {
		ve.add("/screen", "screen must not be empty")
	}
	if doc.Root.Type != Screen {
		ve.add("/root/type", "root must be %q, got %q", Screen, doc.Root.Type)
	}
	if doc.Theme != nil {
		if strings.TrimSpace(doc.Theme.ID) == "" {
			ve.add("/theme/id", "theme id must not be empty")
		}
		for _, mod := range sortedKeys(doc.Theme.ModuleOverrides) {
			if !validModuleID(mod) {
				ve.add("/theme/moduleOverrides/"+mod, "invalid module id")
			}
			toks := doc.Theme.ModuleOverrides[mod]
			for _, tok := range sortedKeys(toks) {
				if !validIdent(tok) {
					ve.add("/theme/moduleOverrides/"+mod+"/"+tok, "invalid token name")
				}
				if err := ValidateColor(toks[tok]); err != nil {
					ve.add("/theme/moduleOverrides/"+mod+"/"+tok, "%v", err)
				}
			}
		}
	}
	if doc.Meta.TTLSeconds < 0 {
		ve.add("/meta/ttlSeconds", "must not be negative")
	}

	for _, name := range sortedKeys(doc.Actions) {
		a := doc.Actions[name]
		p := "/actions/" + name
		if !validIdent(name) {
			ve.add(p, "invalid action name")
		}
		if !knownActionType(a.Type) {
			ve.add(p+"/type", "unknown action type %q", a.Type)
		} else if actionNeedsTarget(a.Type) && strings.TrimSpace(a.Target) == "" {
			ve.add(p+"/target", "action type %q requires a target", a.Type)
		}
		if a.Params != nil {
			if _, err := json.Marshal(a.Params); err != nil {
				ve.add(p+"/params", "not JSON-serialisable: %v", err)
			}
		}
	}

	v := &validator{doc: doc, ve: ve, ids: map[string]string{}}
	v.node(&doc.Root, "/root", 1)
	if v.count > MaxNodes {
		ve.add("/root", "document has %d nodes; limit is %d", v.count, MaxNodes)
	}
	if len(ve.Issues) == 0 {
		return nil
	}
	return ve
}

type validator struct {
	doc   *Document
	ve    *ValidationError
	ids   map[string]string // id -> path of first occurrence
	count int
}

func (v *validator) node(n *Node, path string, depth int) {
	v.count++
	if depth > MaxDepth {
		v.ve.add(path, "nesting depth %d exceeds limit %d", depth, MaxDepth)
		return
	}
	if v.count > MaxNodes {
		return // reported once by the caller
	}

	switch {
	case n.ID == "":
		v.ve.add(path+"/id", "id must not be empty")
	case len(n.ID) > MaxIDLength:
		v.ve.add(path+"/id", "id longer than %d characters", MaxIDLength)
	case !validID(n.ID):
		v.ve.add(path+"/id", "id %q contains characters outside [A-Za-z0-9_.:-]", n.ID)
	default:
		if first, dup := v.ids[n.ID]; dup {
			v.ve.add(path+"/id", "duplicate id %q (first at %s)", n.ID, first)
		} else {
			v.ids[n.ID] = path
		}
	}

	spec, known := Spec(n.Type)
	if !known {
		v.ve.add(path+"/type", "unknown component type %q", n.Type)
	} else {
		if len(n.Children) > 0 && !spec.AllowsChildren {
			v.ve.add(path+"/children", "%s does not allow children", n.Type)
		}
		for _, rp := range spec.RequiredProps {
			_, inProps := n.Props[rp]
			_, inBind := n.Bindings[rp]
			if !inProps && !inBind {
				v.ve.add(path+"/props/"+rp, "%s requires prop %q", n.Type, rp)
			}
		}
	}
	if depth == 1 && n.Type != Screen {
		// already reported at /root/type
	} else if depth > 1 && n.Type == Screen {
		v.ve.add(path+"/type", "Screen may only appear at the root")
	}

	if n.Props != nil {
		if _, err := json.Marshal(n.Props); err != nil {
			v.ve.add(path+"/props", "not JSON-serialisable: %v", err)
		}
	}
	if n.Style != nil {
		v.style(n.Style, path+"/style")
	}
	if n.Action != "" {
		if _, ok := v.doc.Actions[n.Action]; !ok {
			v.ve.add(path+"/action", "action %q is not defined in /actions", n.Action)
		}
	}
	if n.Visibility != nil {
		v.condition(n.Visibility, path+"/visibility", 1)
	}
	for _, prop := range sortedKeys(n.Bindings) {
		b := n.Bindings[prop]
		bp := path + "/bindings/" + prop
		if !validIdent(prop) {
			v.ve.add(bp, "invalid prop name")
		}
		if _, err := ParseBindingPath(b.Path); err != nil {
			v.ve.add(bp+"/path", "%v", err)
		}
		if b.Format != "" && !validFormat(b.Format) {
			v.ve.add(bp+"/format", "unknown format %q", b.Format)
		}
		if b.Default != nil {
			if _, err := json.Marshal(b.Default); err != nil {
				v.ve.add(bp+"/default", "not JSON-serialisable: %v", err)
			}
		}
	}

	for i := range n.Children {
		c := &n.Children[i]
		cp := path + "/children/" + strconv.Itoa(i)
		if known && spec.AllowedChildren != nil && !containsType(spec.AllowedChildren, c.Type) {
			v.ve.add(cp+"/type", "%s is not an allowed child of %s", c.Type, n.Type)
		}
		v.node(c, cp, depth+1)
	}
}

func (v *validator) style(s *Style, path string) {
	colors := []struct{ name, val string }{
		{"background", s.Background}, {"foreground", s.Foreground},
		{"accent", s.Accent}, {"border", s.Border},
	}
	for _, c := range colors {
		if c.val == "" {
			continue
		}
		if err := ValidateColor(c.val); err != nil {
			v.ve.add(path+"/"+c.name, "%v", err)
		}
	}
	if s.Direction != "" && s.Direction != "row" && s.Direction != "column" {
		v.ve.add(path+"/direction", "must be \"row\" or \"column\"")
	}
	if s.Gap < 0 {
		v.ve.add(path+"/gap", "must not be negative")
	}
	if s.Columns < 0 {
		v.ve.add(path+"/columns", "must not be negative")
	}
	if s.Radius < 0 {
		v.ve.add(path+"/radius", "must not be negative")
	}
	if s.Elevation < 0 {
		v.ve.add(path+"/elevation", "must not be negative")
	}
	if s.Opacity < 0 || s.Opacity > 1 {
		v.ve.add(path+"/opacity", "must be between 0 and 1")
	}
	if s.AspectRatio != "" && !validAspect(s.AspectRatio) {
		v.ve.add(path+"/aspectRatio", "must look like \"16/9\"")
	}
	if s.Module != "" && !validModuleID(s.Module) {
		v.ve.add(path+"/module", "invalid module id")
	}
	for _, tok := range sortedKeys(s.Tokens) {
		if !validIdent(tok) {
			v.ve.add(path+"/tokens/"+tok, "invalid token name")
		}
		if err := ValidateColor(s.Tokens[tok]); err != nil {
			v.ve.add(path+"/tokens/"+tok, "%v", err)
		}
	}
}

func (v *validator) condition(c *Condition, path string, depth int) {
	if depth > MaxDepth {
		v.ve.add(path, "condition nesting too deep")
		return
	}
	if c.MinWidth < 0 || c.MaxWidth < 0 || c.MinHeight < 0 || c.MaxHeight < 0 {
		v.ve.add(path, "viewport bounds must not be negative")
	}
	if c.MinWidth > 0 && c.MaxWidth > 0 && c.MinWidth > c.MaxWidth {
		v.ve.add(path, "minWidth %d exceeds maxWidth %d", c.MinWidth, c.MaxWidth)
	}
	if c.MinHeight > 0 && c.MaxHeight > 0 && c.MinHeight > c.MaxHeight {
		v.ve.add(path, "minHeight %d exceeds maxHeight %d", c.MinHeight, c.MaxHeight)
	}
	if c.Module != "" && !validModuleID(c.Module) {
		v.ve.add(path+"/module", "invalid module id")
	}
	for i := range c.All {
		v.condition(&c.All[i], path+"/all/"+strconv.Itoa(i), depth+1)
	}
	for i := range c.Any {
		v.condition(&c.Any[i], path+"/any/"+strconv.Itoa(i), depth+1)
	}
	if c.Not != nil {
		v.condition(c.Not, path+"/not", depth+1)
	}
}

// ValidateColor reports whether s is an acceptable colour value: a token
// reference "token:<name>" (name matching [A-Za-z_][A-Za-z0-9_-]*), a hex
// colour (#rgb, #rgba, #rrggbb, #rrggbbaa) or a functional colour using
// rgb(), rgba(), hsl() or hsla(). Anything else — including named CSS
// colours, var() and url() — is rejected because the value is emitted into
// CSS by clients.
func ValidateColor(s string) error {
	if s == "" {
		return fmt.Errorf("colour must not be empty")
	}
	if strings.HasPrefix(s, "token:") {
		name := s[len("token:"):]
		if !validIdent(name) {
			return fmt.Errorf("invalid token reference %q", s)
		}
		return nil
	}
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		switch len(hex) {
		case 3, 4, 6, 8:
		default:
			return fmt.Errorf("hex colour %q must have 3, 4, 6 or 8 digits", s)
		}
		for _, r := range hex {
			if !isHex(r) {
				return fmt.Errorf("hex colour %q contains a non-hex digit", s)
			}
		}
		return nil
	}
	lower := strings.ToLower(s)
	for _, fn := range []string{"rgba(", "rgb(", "hsla(", "hsl("} {
		if strings.HasPrefix(lower, fn) {
			if !strings.HasSuffix(lower, ")") {
				return fmt.Errorf("colour %q is missing its closing parenthesis", s)
			}
			body := lower[len(fn) : len(lower)-1]
			if strings.TrimSpace(body) == "" {
				return fmt.Errorf("colour %q has no arguments", s)
			}
			for _, r := range body {
				switch {
				case r >= '0' && r <= '9', r == '.', r == ',', r == ' ', r == '%', r == '/', r == '-', r == '+':
				case r == 'd' || r == 'e' || r == 'g' || r == 'r' || r == 'a' || r == 't' || r == 'u' || r == 'n':
					// deg, rad, grad, turn
				default:
					return fmt.Errorf("colour %q contains unexpected character %q", s, r)
				}
			}
			return nil
		}
	}
	return fmt.Errorf("colour %q is not a hex, rgb()/hsl() or token: value", s)
}

// SegmentKind distinguishes binding path segments.
type SegmentKind int

// Segment kinds.
const (
	// KeySegment selects a map key (".name").
	KeySegment SegmentKind = iota
	// IndexSegment selects a slice index ("[3]").
	IndexSegment
)

// Segment is one step of a parsed binding path.
type Segment struct {
	Kind  SegmentKind
	Key   string
	Index int
}

// String renders the segment in path syntax.
func (s Segment) String() string {
	if s.Kind == IndexSegment {
		return "[" + strconv.Itoa(s.Index) + "]"
	}
	return "." + s.Key
}

// ParseBindingPath parses a JSONPath-lite expression. The grammar is
//
//	path    := "$" segment*
//	segment := "." ident | "[" digits "]"
//	ident   := [A-Za-z_] [A-Za-z0-9_-]*
//
// "$" alone selects the whole data object. Bracketed string keys, wildcards
// and filters are not supported.
func ParseBindingPath(p string) ([]Segment, error) {
	if p == "" {
		return nil, fmt.Errorf("binding path must not be empty")
	}
	if p[0] != '$' {
		return nil, fmt.Errorf("binding path %q must start with $", p)
	}
	var segs []Segment
	i := 1
	for i < len(p) {
		switch p[i] {
		case '.':
			i++
			start := i
			for i < len(p) && isIdentChar(p[i], i > start) {
				i++
			}
			if start == i {
				return nil, fmt.Errorf("binding path %q: expected identifier at offset %d", p, start)
			}
			segs = append(segs, Segment{Kind: KeySegment, Key: p[start:i]})
		case '[':
			i++
			start := i
			for i < len(p) && p[i] >= '0' && p[i] <= '9' {
				i++
			}
			if start == i || i >= len(p) || p[i] != ']' {
				return nil, fmt.Errorf("binding path %q: expected [digits] at offset %d", p, start-1)
			}
			n, err := strconv.Atoi(p[start:i])
			if err != nil || n < 0 {
				return nil, fmt.Errorf("binding path %q: bad index at offset %d", p, start)
			}
			segs = append(segs, Segment{Kind: IndexSegment, Index: n})
			i++ // ']'
		default:
			return nil, fmt.Errorf("binding path %q: unexpected %q at offset %d", p, p[i], i)
		}
	}
	return segs, nil
}

// ---- helpers ----

func knownActionType(t ActionType) bool {
	switch t {
	case Navigate, Play, OpenModal, CloseModal, Fetch, Toggle, Submit, External, Back:
		return true
	}
	return false
}

func actionNeedsTarget(t ActionType) bool {
	switch t {
	case CloseModal, Back:
		return false
	}
	return true
}

func validFormat(f string) bool {
	switch f {
	case "upper", "lower", "title", "duration":
		return true
	}
	return strings.HasPrefix(f, "date:") && len(f) > len("date:")
}

func containsType(list []ComponentType, t ComponentType) bool {
	for _, x := range list {
		if x == t {
			return true
		}
	}
	return false
}

func isHex(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isIdentChar(c byte, notFirst bool) bool {
	if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
		return true
	}
	return notFirst && ((c >= '0' && c <= '9') || c == '-')
}

// validIdent matches [A-Za-z_][A-Za-z0-9_-]*.
func validIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isIdentChar(s[i], i > 0) {
			return false
		}
	}
	return true
}

// validID matches [A-Za-z0-9_.:-]+.
func validID(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '_', c == '.', c == ':', c == '-':
		default:
			return false
		}
	}
	return true
}

// validModuleID matches [a-z][a-z0-9_-]* — module ids are lower-case.
func validModuleID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case i > 0 && ((c >= '0' && c <= '9') || c == '_' || c == '-'):
		default:
			return false
		}
	}
	return true
}

func validAspect(s string) bool {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n <= 0 {
			return false
		}
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
