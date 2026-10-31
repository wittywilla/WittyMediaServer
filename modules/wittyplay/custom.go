package wittyplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// A settings file is what a person writes or imports to set options the menus don't
// show, and to make controls and presets of their own. It's JSON, it's data, and it's
// hostile until proven otherwise: its size, depth and counts are limited before
// anything is looked at, and only what's known here comes out of it.
//
//	{
//	  "wittyplay": 1,
//	  "settings": {"display.scanlines": 0.4},
//	  "coreOptions": {"fceumm_sndquality": "High"},
//	  "controls": [{"label": "Sound", "type": "choice", "option": "fceumm_sndquality", "choices": ["Low", "High"]}],
//	  "presets": [{"name": "Crisp", "settings": {"display.scaling": "pixel"}}]
//	}

// Limits on a settings file.
const (
	MaxCustomSize  = 256 << 10
	MaxCustomDepth = 8
	MaxCustomNodes = 20000
	MaxCustomText  = 1024
)

// Issue is something about a settings file, with where it is when that's known.
type Issue struct {
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// CustomReport is what came of reading a settings file. Errors stop it being saved.
// Ignored things are harmless, and are shown so that nothing vanishes silently.
type CustomReport struct {
	Errors  []Issue `json:"errors"`
	Ignored []Issue `json:"ignored"`
}

// OK says the file can be saved.
func (r *CustomReport) OK() bool { return len(r.Errors) == 0 }

// node is one value in the file, with where it began.
type node struct {
	line, col int
	kind      byte // o object, a array, s string, n number, b bool, z null
	keys      []string
	fields    []*node
	items     []*node
	str       string
	num       float64
	flag      bool
}

func (n *node) field(key string) *node {
	for i, k := range n.keys {
		if k == key {
			return n.fields[i]
		}
	}
	return nil
}

func (n *node) value() any {
	switch n.kind {
	case 's':
		return n.str
	case 'n':
		return n.num
	case 'b':
		return n.flag
	}
	return nil
}

type customReader struct {
	src   []byte
	dec   *json.Decoder
	nodes int
	lines []int // offset each line starts at
}

func (c *customReader) where(off int64) (line, col int) {
	lo, hi := 0, len(c.lines)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if int64(c.lines[mid]) <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1, int(off) - c.lines[lo] + 1
}

// tokenStart is where the token just read began, found from where it ended.
func (c *customReader) tokenStart(end int64) int64 {
	i := int(end)
	if i > len(c.src) {
		i = len(c.src)
	}
	if i > 0 {
		i--
	}
	if c.src[i] == '"' {
		for i--; i > 0; i-- {
			if c.src[i] == '"' && c.src[i-1] != '\\' {
				return int64(i)
			}
		}
		return 0
	}
	for i > 0 && !bytes.ContainsRune([]byte(" \t\r\n,:[{"), rune(c.src[i-1])) {
		i--
	}
	return int64(i)
}

var errTooMuch = errors.New("too much")

func (c *customReader) read(depth int) (*node, error) {
	tok, err := c.dec.Token()
	if err != nil {
		return nil, err
	}
	return c.readFrom(tok, depth)
}

func (c *customReader) readFrom(tok json.Token, depth int) (*node, error) {
	c.nodes++
	if c.nodes > MaxCustomNodes {
		return nil, fmt.Errorf("%w: a settings file can hold %d values at most", errTooMuch, MaxCustomNodes)
	}
	line, col := c.where(c.tokenStart(c.dec.InputOffset()))
	n := &node{line: line, col: col}
	switch v := tok.(type) {
	case json.Delim:
		if depth >= MaxCustomDepth {
			return nil, fmt.Errorf("%w: it nests deeper than %d levels", errTooMuch, MaxCustomDepth)
		}
		if v == '{' {
			n.kind = 'o'
			for c.dec.More() {
				kt, err := c.dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := kt.(string)
				if !ok {
					return nil, errors.New("expected a name")
				}
				kl, kc := c.where(c.tokenStart(c.dec.InputOffset()))
				if len(key) > MaxCustomText {
					return nil, fmt.Errorf("%w: a name of %d characters", errTooMuch, len(key))
				}
				if n.field(key) != nil {
					return nil, &customSyntax{kl, kc, fmt.Sprintf("%q appears twice here", key)}
				}
				child, err := c.read(depth + 1)
				if err != nil {
					return nil, err
				}
				child.line, child.col = kl, kc // a problem with a value is shown at its name
				n.keys, n.fields = append(n.keys, key), append(n.fields, child)
			}
		} else {
			n.kind = 'a'
			for c.dec.More() {
				child, err := c.read(depth + 1)
				if err != nil {
					return nil, err
				}
				n.items = append(n.items, child)
			}
		}
		if _, err := c.dec.Token(); err != nil { // the closing bracket
			return nil, err
		}
	case string:
		if len(v) > MaxCustomText {
			return nil, fmt.Errorf("%w: a text of %d characters", errTooMuch, len(v))
		}
		n.kind, n.str = 's', v
	case json.Number:
		f, err := v.Float64()
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, &customSyntax{line, col, "this number is out of range"}
		}
		n.kind, n.num = 'n', f
	case bool:
		n.kind, n.flag = 'b', v
	default:
		n.kind = 'z'
	}
	return n, nil
}

type customSyntax struct {
	line, col int
	msg       string
}

func (e *customSyntax) Error() string { return e.msg }

// parseCustom reads the file into a tree, or says where it stopped making sense.
func parseCustom(src []byte) (*node, *Issue) {
	if len(src) > MaxCustomSize {
		return nil, &Issue{Message: fmt.Sprintf("This file is too large: it can be %d KB at most.", MaxCustomSize>>10)}
	}
	c := &customReader{src: src, dec: json.NewDecoder(bytes.NewReader(src)), lines: []int{0}}
	c.dec.UseNumber()
	for i, b := range src {
		if b == '\n' {
			c.lines = append(c.lines, i+1)
		}
	}
	root, err := c.read(0)
	if err == nil {
		if _, terr := c.dec.Token(); !errors.Is(terr, io.EOF) {
			off := c.dec.InputOffset()
			if terr != nil {
				var se *json.SyntaxError
				if errors.As(terr, &se) {
					off = se.Offset
				}
			}
			l, co := c.where(max(off-1, 0))
			return nil, &Issue{Line: l, Column: co, Message: "There's something after the end of the settings."}
		}
		return root, nil
	}
	var cs *customSyntax
	var se *json.SyntaxError
	switch {
	case errors.As(err, &cs):
		return nil, &Issue{Line: cs.line, Column: cs.col, Message: cs.msg}
	case errors.As(err, &se):
		l, co := c.where(max(se.Offset-1, 0))
		if strings.Contains(se.Error(), "unexpected end") {
			return nil, &Issue{Line: l, Column: co, Message: "The file ends before the settings do. Is a closing bracket missing?"}
		}
		return nil, &Issue{Line: l, Column: co, Message: "This isn't valid JSON here: " + plainJSONError(se.Error())}
	case errors.Is(err, errTooMuch):
		return nil, &Issue{Message: "This file holds too much: " + strings.TrimPrefix(err.Error(), "too much: ") + "."}
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		l, co := c.where(int64(max(len(src)-1, 0)))
		return nil, &Issue{Line: l, Column: co, Message: "The file ends before the settings do. Is a closing bracket missing?"}
	}
	return nil, &Issue{Message: "This isn't valid JSON: " + plainJSONError(err.Error())}
}

func plainJSONError(s string) string {
	s = strings.TrimPrefix(s, "invalid character ")
	s = strings.ReplaceAll(s, "looking for beginning of value", "where a value should begin")
	return s
}

// CustomInput is what checking a file knows about the game it's for: the options its
// core offers. Nil means it isn't known, and those checks wait until a game runs.
type CustomInput struct {
	Offered []OptionInfo
}

// ParseCustom checks a settings file and returns what can be used from it.
func ParseCustom(src []byte, in CustomInput) (*Layer, CustomReport) {
	rep := CustomReport{Errors: []Issue{}, Ignored: []Issue{}}
	src = bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(src)) == 0 {
		rep.Errors = append(rep.Errors, Issue{Line: 1, Column: 1, Message: "The file is empty. Write {} for one with nothing in it."})
		return nil, rep
	}
	root, bad := parseCustom(src)
	if bad != nil {
		rep.Errors = append(rep.Errors, *bad)
		return nil, rep
	}
	if root.kind != 'o' {
		rep.Errors = append(rep.Errors, Issue{Line: root.line, Column: root.col, Message: "A settings file is one { … } object."})
		return nil, rep
	}
	v := &customCheck{rep: &rep}
	l := &Layer{}
	for i, key := range root.keys {
		n := root.fields[i]
		switch key {
		case "wittyplay":
			if n.kind != 'n' || n.num != 1 {
				v.errorAt(n, key, `The only version so far is "wittyplay": 1.`)
			}
		case "name", "description":
			if n.kind != 's' {
				v.errorAt(n, key, "This should be text.")
			}
		case "settings":
			l.Values = v.values(n, key)
		case "coreOptions":
			l.CoreOptions = v.coreOptions(n, key)
		case "controls":
			l.Controls = v.controls(n, key)
		case "presets":
			l.Presets = v.presets(n, key)
		default:
			v.ignore(n, key, "This isn't something a settings file can set, so it's ignored.")
		}
	}
	if in.Offered != nil {
		v.checkOffered(root, l, in.Offered)
	}
	if !rep.OK() {
		return nil, rep
	}
	return l, rep
}

type customCheck struct{ rep *CustomReport }

func (v *customCheck) errorAt(n *node, path, msg string) {
	v.rep.Errors = append(v.rep.Errors, Issue{Line: n.line, Column: n.col, Path: path, Message: msg})
}

func (v *customCheck) ignore(n *node, path, msg string) {
	v.rep.Ignored = append(v.rep.Ignored, Issue{Line: n.line, Column: n.col, Path: path, Message: msg})
}

func (v *customCheck) values(n *node, path string) map[string]any {
	if n.kind != 'o' {
		v.errorAt(n, path, "This should be a { … } of setting names and values.")
		return nil
	}
	out := map[string]any{}
	for i, key := range n.keys {
		c := n.fields[i]
		p := path + "." + key
		s := settingByKey[key]
		if s == nil {
			v.ignore(c, p, "There's no setting with this name, so it's ignored.")
			continue
		}
		clean, err := CleanValue(s, c.value())
		if err != nil {
			v.errorAt(c, p, s.Label+" "+err.Error()+".")
			continue
		}
		out[key] = clean
	}
	return out
}

func optionText(n *node) (string, bool) {
	switch n.kind {
	case 's':
		return n.str, true
	case 'n':
		return strconv.FormatFloat(n.num, 'f', -1, 64), true
	case 'b':
		if n.flag {
			return "enabled", true
		}
		return "disabled", true
	}
	return "", false
}

func (v *customCheck) coreOptions(n *node, path string) map[string]string {
	if n.kind != 'o' {
		v.errorAt(n, path, "This should be a { … } of option names and values.")
		return nil
	}
	if len(n.keys) > MaxCoreOptions {
		v.errorAt(n, path, fmt.Sprintf("That's too many core options: %d at most.", MaxCoreOptions))
		return nil
	}
	out := map[string]string{}
	for i, key := range n.keys {
		c := n.fields[i]
		p := path + "." + key
		if !goodOptionKey(key) {
			v.errorAt(c, p, "A core option's name is letters, numbers, _ . : and - only.")
			continue
		}
		text, ok := optionText(c)
		if !ok || !goodOptionValue(text) {
			v.errorAt(c, p, "A core option's value is short text, a number, or true or false.")
			continue
		}
		out[key] = text
	}
	return out
}

func (v *customCheck) label(n *node, path string, required bool) string {
	if n == nil {
		return ""
	}
	if n.kind != 's' || len(n.str) > MaxLabel || (required && strings.TrimSpace(n.str) == "") {
		v.errorAt(n, path, fmt.Sprintf("This should be text of up to %d characters.", MaxLabel))
		return ""
	}
	for _, r := range n.str {
		if r < 0x20 || r == 0x7f {
			v.errorAt(n, path, "This has characters that can't be shown.")
			return ""
		}
	}
	return n.str
}

func (v *customCheck) controls(n *node, path string) []CustomControl {
	if n.kind != 'a' {
		v.errorAt(n, path, "This should be a [ … ] of controls.")
		return nil
	}
	if len(n.items) > MaxCustomControls {
		v.errorAt(n, path, fmt.Sprintf("That's too many controls: %d at most.", MaxCustomControls))
		return nil
	}
	var out []CustomControl
	for i, c := range n.items {
		p := fmt.Sprintf("%s[%d]", path, i)
		if c.kind != 'o' {
			v.errorAt(c, p, "A control is a { … } with a label, a type and an option.")
			continue
		}
		before := len(v.rep.Errors)
		ctl := CustomControl{}
		if l := c.field("label"); l != nil {
			ctl.Label = v.label(l, p+".label", true)
		} else {
			v.errorAt(c, p, `This control needs a "label".`)
		}
		ctl.Help = v.label(c.field("help"), p+".help", false)
		if o := c.field("option"); o == nil || o.kind != 's' || !goodOptionKey(o.str) {
			at := c
			if o != nil {
				at = o
			}
			v.errorAt(at, p+".option", `This control needs an "option": the name of the core option it sets.`)
		} else {
			ctl.Option = o.str
		}
		t := c.field("type")
		if t == nil || t.kind != 's' {
			v.errorAt(c, p+".type", `This control needs a "type": "toggle", "choice" or "slider".`)
			continue
		}
		ctl.Type = t.str
		switch t.str {
		case "toggle":
			ctl.On, ctl.Off = "enabled", "disabled"
			if on := c.field("on"); on != nil {
				if text, ok := optionText(on); ok && goodOptionValue(text) {
					ctl.On = text
				} else {
					v.errorAt(on, p+".on", "This should be the option's value when on.")
				}
			}
			if off := c.field("off"); off != nil {
				if text, ok := optionText(off); ok && goodOptionValue(text) {
					ctl.Off = text
				} else {
					v.errorAt(off, p+".off", "This should be the option's value when off.")
				}
			}
		case "choice":
			ctl.Choices = v.choiceList(c.field("choices"), c, p+".choices")
		case "slider":
			if list := c.field("values"); list != nil {
				ctl.Choices = v.choiceList(list, c, p+".values")
			} else if list := c.field("choices"); list != nil {
				ctl.Choices = v.choiceList(list, c, p+".choices")
			} else {
				ctl.Choices = v.sliderRange(c, p)
			}
		default:
			v.errorAt(t, p+".type", `The type is "toggle", "choice" or "slider".`)
		}
		for j, key := range c.keys {
			switch key {
			case "label", "help", "type", "option", "on", "off", "choices", "values", "min", "max", "step":
			default:
				v.ignore(c.fields[j], p+"."+key, "A control has no such part, so it's ignored.")
			}
		}
		if len(v.rep.Errors) == before {
			out = append(out, ctl)
		}
	}
	return out
}

func (v *customCheck) choiceList(n, owner *node, path string) []Choice {
	if n == nil {
		v.errorAt(owner, path, "This control needs a list of values.")
		return nil
	}
	if n.kind != 'a' || len(n.items) == 0 || len(n.items) > MaxControlChoices {
		v.errorAt(n, path, fmt.Sprintf("This should be a [ … ] of 1 to %d values.", MaxControlChoices))
		return nil
	}
	out := make([]Choice, 0, len(n.items))
	for i, it := range n.items {
		p := fmt.Sprintf("%s[%d]", path, i)
		if it.kind == 'o' {
			val := it.field("value")
			text, ok := "", false
			if val != nil {
				text, ok = optionText(val)
			}
			if !ok || !goodOptionValue(text) {
				v.errorAt(it, p, `This should have a "value", and can have a "label".`)
				continue
			}
			lab := v.label(it.field("label"), p+".label", false)
			if lab == "" {
				lab = text
			}
			out = append(out, Choice{text, lab})
			continue
		}
		text, ok := optionText(it)
		if !ok || !goodOptionValue(text) {
			v.errorAt(it, p, "This should be the option's value: short text or a number.")
			continue
		}
		out = append(out, Choice{text, text})
	}
	return out
}

func (v *customCheck) sliderRange(c *node, p string) []Choice {
	lo, hi, st := c.field("min"), c.field("max"), c.field("step")
	if lo == nil || hi == nil || lo.kind != 'n' || hi.kind != 'n' {
		v.errorAt(c, p, `A slider needs a "values" list, or a "min" and "max".`)
		return nil
	}
	step := 1.0
	if st != nil {
		if st.kind != 'n' || st.num <= 0 {
			v.errorAt(st, p+".step", "The step is a number above 0.")
			return nil
		}
		step = st.num
	}
	count := (hi.num-lo.num)/step + 1
	if hi.num < lo.num || count > MaxControlChoices {
		v.errorAt(c, p, fmt.Sprintf("From min to max by step has to come to %d values at most.", MaxControlChoices))
		return nil
	}
	out := make([]Choice, 0, int(count))
	for i := 0; i < int(count+1e-9); i++ {
		text := strconv.FormatFloat(lo.num+float64(i)*step, 'f', -1, 64)
		out = append(out, Choice{text, text})
	}
	return out
}

func (v *customCheck) presets(n *node, path string) []Preset {
	if n.kind != 'a' {
		v.errorAt(n, path, "This should be a [ … ] of presets.")
		return nil
	}
	if len(n.items) > MaxPresets {
		v.errorAt(n, path, fmt.Sprintf("That's too many presets: %d at most.", MaxPresets))
		return nil
	}
	var out []Preset
	seen := map[string]bool{}
	for i, c := range n.items {
		p := fmt.Sprintf("%s[%d]", path, i)
		if c.kind != 'o' {
			v.errorAt(c, p, "A preset is a { … } with a name.")
			continue
		}
		pr := Preset{}
		if name := c.field("name"); name != nil {
			pr.Name = v.label(name, p+".name", true)
		} else {
			v.errorAt(c, p, `This preset needs a "name".`)
		}
		if seen[pr.Name] && pr.Name != "" {
			v.errorAt(c, p, fmt.Sprintf("There are two presets named %q.", pr.Name))
		}
		seen[pr.Name] = true
		pr.Description = v.label(c.field("description"), p+".description", false)
		if s := c.field("settings"); s != nil {
			pr.Values = v.values(s, p+".settings")
		}
		if s := c.field("coreOptions"); s != nil {
			pr.CoreOptions = v.coreOptions(s, p+".coreOptions")
		}
		for j, key := range c.keys {
			switch key {
			case "name", "description", "settings", "coreOptions":
			default:
				v.ignore(c.fields[j], p+"."+key, "A preset has no such part, so it's ignored.")
			}
		}
		out = append(out, pr)
	}
	return out
}

// checkOffered lists what the core won't take, at the lines it's written on.
func (v *customCheck) checkOffered(root *node, l *Layer, offered []OptionInfo) {
	at := func(key string) *node {
		if co := root.field("coreOptions"); co != nil {
			if n := co.field(key); n != nil {
				return n
			}
		}
		return root
	}
	for _, issue := range CheckOptions(l.CoreOptions, offered) {
		key := strings.TrimPrefix(issue.Path, "coreOptions.")
		v.ignore(at(key), issue.Path, upperFirst(issue.Message)+".")
	}
	known := map[string]bool{}
	for _, o := range offered {
		known[o.Key] = true
	}
	if c := root.field("controls"); c != nil {
		for i, ctl := range l.Controls {
			if !known[ctl.Option] && i < len(c.items) {
				v.ignore(c.items[i], fmt.Sprintf("controls[%d].option", i), fmt.Sprintf("This core has no option %q, so this control won't show for it.", ctl.Option))
			}
		}
	}
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Document turns a layer back into the text of a settings file.
func (l *Layer) Document() []byte {
	doc := struct {
		Version int `json:"wittyplay"`
		Layer
	}{1, *l}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return []byte(`{"wittyplay": 1}`)
	}
	return b
}
