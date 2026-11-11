package sdui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Viewport is the size of the requesting client's window in CSS pixels.
type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// RenderContext describes the requester a document is rendered for.
type RenderContext struct {
	// Platform is "web", "tv", "ios", "android", "desktop", …
	Platform string
	// Role is the requester's role ("admin", "user", "guest", …).
	Role string
	// Features lists enabled feature names.
	Features []string
	// Modules lists installed, enabled module ids.
	Modules []string
	// Flags are arbitrary boolean switches.
	Flags map[string]bool
	// Viewport is the client window size; zero means unknown.
	Viewport Viewport
	// Locale is a BCP 47 tag such as "en-GB".
	Locale string
	// Now is the render time; the zero value means time.Now().
	Now time.Time
	// Data is the root object that Bindings resolve against. It should be
	// built from JSON-compatible values (map[string]any, []any, string,
	// float64, bool, nil); Go structs are not traversed.
	Data map[string]any
	// ThemeOverrides maps module id → token name → colour, merged over the
	// document's ThemeRef.ModuleOverrides.
	ThemeOverrides map[string]map[string]string
}

// Render resolves doc for ctx. It deep-copies the document; removes every
// node whose Visibility evaluates false (with its subtree); resolves
// Bindings into Props (applying Default and Format); applies module token
// overrides into Style.Tokens for nodes whose Style.Module matches; strips
// Visibility and Bindings; and stamps Meta.GeneratedAt and Meta.ETag.
//
// Render does not validate; callers should Validate authored documents when
// they are stored. It returns an error only if the root is hidden or a
// binding has a malformed path.
func Render(doc Document, ctx RenderContext) (Document, error) {
	out := cloneDocument(doc)
	if ctx.Now.IsZero() {
		ctx.Now = time.Now()
	}
	overrides := mergeOverrides(doc.Theme, ctx.ThemeOverrides)

	root, keep, err := renderNode(out.Root, ctx, overrides, "/root")
	if err != nil {
		return Document{}, err
	}
	if !keep {
		return Document{}, fmt.Errorf("sdui: root node %q is hidden by its visibility condition", doc.Root.ID)
	}
	out.Root = root
	out.Meta.GeneratedAt = ctx.Now.UTC()
	out.Meta.ETag = ETag(out)
	return out, nil
}

func mergeOverrides(ref *ThemeRef, ctxOv map[string]map[string]string) map[string]map[string]string {
	merged := map[string]map[string]string{}
	add := func(src map[string]map[string]string) {
		for mod, toks := range src {
			if merged[mod] == nil {
				merged[mod] = map[string]string{}
			}
			for k, v := range toks {
				merged[mod][k] = v
			}
		}
	}
	if ref != nil {
		add(ref.ModuleOverrides)
	}
	add(ctxOv)
	return merged
}

func renderNode(n Node, ctx RenderContext, overrides map[string]map[string]string, path string) (Node, bool, error) {
	if n.Visibility != nil && !Evaluate(n.Visibility, ctx) {
		return Node{}, false, nil
	}
	if len(n.Bindings) > 0 {
		if n.Props == nil {
			n.Props = make(map[string]any, len(n.Bindings))
		}
		for _, prop := range sortedKeys(n.Bindings) {
			b := n.Bindings[prop]
			val, err := ResolveBinding(b, ctx.Data)
			if err != nil {
				return Node{}, false, fmt.Errorf("sdui: %s/bindings/%s: %w", path, prop, err)
			}
			if val != nil {
				n.Props[prop] = val
			}
			// A binding that resolves to nothing and has no Default leaves
			// any static prop value in place as the fallback.
		}
		if len(n.Props) == 0 {
			n.Props = nil
		}
	}
	if n.Style != nil && n.Style.Module != "" {
		if toks := overrides[n.Style.Module]; len(toks) > 0 {
			if n.Style.Tokens == nil {
				n.Style.Tokens = make(map[string]string, len(toks))
			}
			for k, v := range toks {
				n.Style.Tokens[k] = v
			}
		}
	}
	n.Visibility = nil
	n.Bindings = nil

	if len(n.Children) > 0 {
		kept := make([]Node, 0, len(n.Children))
		for i, c := range n.Children {
			rc, keep, err := renderNode(c, ctx, overrides, path+"/children/"+strconv.Itoa(i))
			if err != nil {
				return Node{}, false, err
			}
			if keep {
				kept = append(kept, rc)
			}
		}
		if len(kept) == 0 {
			kept = nil
		}
		n.Children = kept
	}
	return n, true, nil
}

// Evaluate reports whether c holds for ctx. A nil or zero condition is true.
// Every set field must hold; All/Any/Not compose nested conditions. Viewport
// bounds are ignored when the context viewport dimension is zero (unknown).
func Evaluate(c *Condition, ctx RenderContext) bool {
	if c == nil {
		return true
	}
	for i := range c.All {
		if !Evaluate(&c.All[i], ctx) {
			return false
		}
	}
	if len(c.Any) > 0 {
		ok := false
		for i := range c.Any {
			if Evaluate(&c.Any[i], ctx) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if c.Not != nil && Evaluate(c.Not, ctx) {
		return false
	}
	if c.Feature != "" && !containsString(ctx.Features, c.Feature) {
		return false
	}
	if c.Role != "" && ctx.Role != c.Role {
		return false
	}
	if c.Platform != "" && ctx.Platform != c.Platform {
		return false
	}
	if c.Module != "" && !containsString(ctx.Modules, c.Module) {
		return false
	}
	if w := ctx.Viewport.Width; w > 0 {
		if c.MinWidth > 0 && w < c.MinWidth {
			return false
		}
		if c.MaxWidth > 0 && w > c.MaxWidth {
			return false
		}
	}
	if h := ctx.Viewport.Height; h > 0 {
		if c.MinHeight > 0 && h < c.MinHeight {
			return false
		}
		if c.MaxHeight > 0 && h > c.MaxHeight {
			return false
		}
	}
	if c.Locale != "" && !localeMatches(ctx.Locale, c.Locale) {
		return false
	}
	if c.Flag != "" && !ctx.Flags[c.Flag] {
		return false
	}
	// c.When is reserved; it does not affect evaluation in v1.
	return true
}

func localeMatches(have, want string) bool {
	have = strings.ToLower(have)
	want = strings.ToLower(want)
	if have == want {
		return true
	}
	return strings.HasPrefix(have, want+"-") || strings.HasPrefix(have, want+"_")
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ResolveBinding looks up b.Path in data, falls back to b.Default when the
// path does not resolve (or resolves to nil), and applies b.Format. It
// returns nil when there is no value and no default. The error is non-nil
// only for a malformed path.
func ResolveBinding(b Binding, data map[string]any) (any, error) {
	segs, err := ParseBindingPath(b.Path)
	if err != nil {
		return nil, err
	}
	val, ok := Lookup(data, segs)
	if !ok || val == nil {
		val = b.Default
	}
	if val == nil {
		return nil, nil
	}
	if b.Format != "" {
		val = applyFormat(b.Format, val)
	}
	return val, nil
}

// Lookup walks data along segs. It understands map[string]any, []any and
// []string; any other container type stops the walk.
func Lookup(data map[string]any, segs []Segment) (any, bool) {
	var cur any = data
	if data == nil {
		cur = nil
	}
	for _, s := range segs {
		switch s.Kind {
		case KeySegment:
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, false
			}
			cur, ok = m[s.Key]
			if !ok {
				return nil, false
			}
		case IndexSegment:
			switch arr := cur.(type) {
			case []any:
				if s.Index >= len(arr) {
					return nil, false
				}
				cur = arr[s.Index]
			case []string:
				if s.Index >= len(arr) {
					return nil, false
				}
				cur = arr[s.Index]
			case []map[string]any:
				if s.Index >= len(arr) {
					return nil, false
				}
				cur = arr[s.Index]
			default:
				return nil, false
			}
		}
	}
	return cur, true
}

func applyFormat(format string, v any) any {
	switch {
	case format == "upper":
		return strings.ToUpper(stringify(v))
	case format == "lower":
		return strings.ToLower(stringify(v))
	case format == "title":
		return titleCase(stringify(v))
	case format == "duration":
		if f, ok := toFloat(v); ok {
			return formatDuration(f)
		}
		return stringify(v)
	case strings.HasPrefix(format, "date:"):
		layout := format[len("date:"):]
		if t, ok := toTime(v); ok {
			return t.Format(layout)
		}
		return stringify(v)
	}
	return v
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		out := r
		if unicode.IsSpace(prev) || prev == '-' {
			out = unicode.ToUpper(r)
		}
		prev = r
		return out
	}, s)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

func toTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
			if t, err := time.Parse(layout, x); err == nil {
				return t, true
			}
		}
	case float64, int, int64:
		f, _ := toFloat(v)
		return time.Unix(int64(f), 0).UTC(), true
	}
	return time.Time{}, false
}

// formatDuration renders seconds as "h:mm:ss" or "m:ss".
func formatDuration(seconds float64) string {
	if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		seconds = 0
	}
	total := int64(math.Round(seconds))
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// ---- deep copy ----

func cloneDocument(d Document) Document {
	out := d
	out.Root = cloneNode(d.Root)
	if d.Theme != nil {
		t := *d.Theme
		t.ModuleOverrides = cloneStringMapMap(d.Theme.ModuleOverrides)
		out.Theme = &t
	}
	if d.Actions != nil {
		out.Actions = make(map[string]Action, len(d.Actions))
		for k, a := range d.Actions {
			a.Params = cloneAnyMap(a.Params)
			out.Actions[k] = a
		}
	}
	return out
}

func cloneNode(n Node) Node {
	out := n
	out.Props = cloneAnyMap(n.Props)
	if n.Style != nil {
		s := *n.Style
		s.Tokens = cloneStringMap(n.Style.Tokens)
		out.Style = &s
	}
	if n.Visibility != nil {
		c := cloneCondition(*n.Visibility)
		out.Visibility = &c
	}
	if n.Bindings != nil {
		out.Bindings = make(map[string]Binding, len(n.Bindings))
		for k, b := range n.Bindings {
			b.Default = cloneAny(b.Default)
			out.Bindings[k] = b
		}
	}
	if n.Children != nil {
		out.Children = make([]Node, len(n.Children))
		for i, c := range n.Children {
			out.Children[i] = cloneNode(c)
		}
	}
	return out
}

func cloneCondition(c Condition) Condition {
	out := c
	if c.All != nil {
		out.All = make([]Condition, len(c.All))
		for i, x := range c.All {
			out.All[i] = cloneCondition(x)
		}
	}
	if c.Any != nil {
		out.Any = make([]Condition, len(c.Any))
		for i, x := range c.Any {
			out.Any[i] = cloneCondition(x)
		}
	}
	if c.Not != nil {
		n := cloneCondition(*c.Not)
		out.Not = &n
	}
	return out
}

func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneStringMapMap(m map[string]map[string]string) map[string]map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]map[string]string, len(m))
	for k, v := range m {
		out[k] = cloneStringMap(v)
	}
	return out
}

func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneAny(v)
	}
	return out
}

// cloneAny deep-copies JSON-like values; other types are shared.
func cloneAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return cloneAnyMap(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneAny(e)
		}
		return out
	case []string:
		return append([]string(nil), x...)
	case []map[string]any:
		out := make([]map[string]any, len(x))
		for i, e := range x {
			out[i] = cloneAnyMap(e)
		}
		return out
	}
	return v
}
