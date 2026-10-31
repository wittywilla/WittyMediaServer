package libretro

import (
	"testing"
	"unsafe"

	"github.com/wittywilla/wittyplay/engine"
)

func fakeValues(c *Core, pairs ...string) [maxCoreValues]coreValue {
	var v [maxCoreValues]coreValue
	for i := 0; i+1 < len(pairs); i += 2 {
		v[i/2] = coreValue{c.pin(pairs[i]), c.pin(pairs[i+1])}
	}
	return v
}

// A core hands over these structures in memory, laid out as libretro's header says.
func TestTheNewestOptionFormatIsRead(t *testing.T) {
	c := &Core{vars: map[string]*byte{}, options: map[string]string{"second": "x", "third": "../../etc/passwd"}}
	cats := []coreCategory{{c.pin("video"), c.pin("Video"), c.pin("About the picture")}, {}}
	defs := []coreDefV2{
		{key: c.pin("first"), desc: c.pin("First"), descCategorized: c.pin("F"), info: c.pin("What it does"), infoCategorized: c.pin("i"), category: c.pin("video"), values: fakeValues(c, "a", "Option A", "b", "Option B"), defaultValue: c.pin("b")},
		{key: c.pin("second"), desc: c.pin("Second"), values: fakeValues(c, "w", "", "x", "")},
		{key: c.pin("third"), desc: c.pin("Third"), values: fakeValues(c, "p", "P", "q", "Q"), defaultValue: c.pin("nonsense")},
		{key: c.pin("nothing"), desc: c.pin("No values")},
		{},
	}
	o := coreOptionsV2{categories: &cats[0], definitions: &defs[0]}
	if !c.environment(envSetCoreOptionsV2, unsafe.Pointer(&o)) {
		t.Fatal("refused")
	}
	list := c.Options()
	if len(list) != 3 {
		t.Fatalf("got %+v", list)
	}
	f := list[0]
	if f.Label != "First" || f.Info != "What it does" || f.Category != "Video" || f.Default != "b" || f.Value != "b" || f.Labels[0] != "Option A" || cString(c.vars["first"]) != "b" {
		t.Errorf("first: %+v", f)
	}
	if s := list[1]; s.Value != "x" || s.Default != "w" || s.Labels[1] != "x" {
		t.Errorf("a value from the server should win, and a value with no name is its own name: %+v", s)
	}
	if s := list[2]; s.Value != "p" || s.Default != "p" {
		t.Errorf("neither a bad default nor a value it doesn't list should count: %+v", s)
	}
}

func TestTheOlderOptionFormatsAreRead(t *testing.T) {
	c := &Core{vars: map[string]*byte{}}
	defs := []coreDefV1{{key: c.pin("k"), desc: c.pin("K"), info: c.pin("Info"), values: fakeValues(c, "1", "One", "2", "Two"), defaultValue: c.pin("2")}, {}}
	in := coreIntl{us: unsafe.Pointer(&defs[0])}
	if !c.environment(envSetCoreOptionsIntl, unsafe.Pointer(&in)) {
		t.Fatal("refused")
	}
	if l := c.Options(); len(l) != 1 || l[0].Value != "2" || l[0].Labels[1] != "Two" || l[0].Info != "Info" {
		t.Fatalf("got %+v", l)
	}
	c2 := &Core{vars: map[string]*byte{}}
	defs2 := []coreDefV2{{key: c2.pin("k"), desc: c2.pin("K"), values: fakeValues(c2, "1", "One")}, {}}
	o := coreOptionsV2{definitions: &defs2[0]}
	in2 := coreIntl{us: unsafe.Pointer(&o)}
	if !c2.environment(envSetCoreOptionsV2Intl, unsafe.Pointer(&in2)) {
		t.Fatal("refused")
	}
	if l := c2.Options(); len(l) != 1 || l[0].Key != "k" {
		t.Fatalf("got %+v", l)
	}
	// The call that only asks whether the command exists.
	for _, cmd := range []uint32{envSetCoreOptions, envSetCoreOptionsIntl, envSetCoreOptionsV2, envSetCoreOptionsV2Intl, envSetCoreOptionsDisplay, envSetCoreOptionsUpdateCB} {
		if !c.environment(cmd, nil) {
			t.Errorf("command %d", cmd)
		}
	}
}

func TestACoreCanHideAndShowOptions(t *testing.T) {
	c := &Core{vars: map[string]*byte{}}
	c.noteOption("k", "K", []string{"a"}, "a")
	show := func(key string, on bool) {
		d := struct {
			key     *byte
			visible bool
		}{c.pin(key), on}
		c.environment(envSetCoreOptionsDisplay, unsafe.Pointer(&d))
	}
	var _ engine.OptionsChanged = c
	show("k", false)
	if !c.Options()[0].Hidden || !c.OptionsChanged() {
		t.Fatal("hiding wasn't noted")
	}
	if c.OptionsChanged() {
		t.Fatal("it should say so once")
	}
	show("k", false)
	if c.OptionsChanged() {
		t.Fatal("nothing changed that time")
	}
	show("no such option", true)
	show("k", true)
	if c.Options()[0].Hidden || !c.OptionsChanged() {
		t.Fatal("showing wasn't noted")
	}
}

func TestAnOptionOnlyTakesWhatTheCoreListed(t *testing.T) {
	c := &Core{vars: map[string]*byte{}}
	c.noteOption("k", "Label", []string{"a", "b"}, "a")
	c.vars["k"] = c.pin("a")
	for _, bad := range []struct{ key, value string }{{"k", "c"}, {"k", ""}, {"k", "../../etc/passwd"}, {"other", "a"}} {
		if c.SetOption(bad.key, bad.value) {
			t.Errorf("took %q = %q", bad.key, bad.value)
		}
		if c.varDirty {
			t.Errorf("told the core about %q = %q", bad.key, bad.value)
		}
	}
	if !c.SetOption("k", "b") {
		t.Fatal("refused a listed value")
	}
	if !c.varDirty || cString(c.vars["k"]) != "b" || c.Options()[0].Value != "b" {
		t.Fatalf("the core wasn't told: dirty %v, %q", c.varDirty, cString(c.vars["k"]))
	}
	// Setting what it already is isn't a change, and costs the core nothing.
	c.varDirty = false
	if !c.SetOption("k", "b") || c.varDirty {
		t.Fatal("a value that didn't change was reported as changed")
	}
}

func TestAnOptionDeclaredAgainReplacesItself(t *testing.T) {
	c := &Core{vars: map[string]*byte{}}
	c.noteOption("k", "Old", []string{"a"}, "a")
	c.noteOption("j", "J", []string{"x"}, "x")
	c.noteOption("k", "New", []string{"a", "b"}, "b")
	if got := c.Options(); len(got) != 2 || got[0].Label != "New" || got[1].Key != "j" {
		t.Fatalf("got %+v", got)
	}
}

func TestWhatACoreIsToldAboutOutput(t *testing.T) {
	c := &Core{}
	if c.avEnable() != 3 {
		t.Fatalf("it should start with both on: %d", c.avEnable())
	}
	c.SetOutput(false, true, true)
	if c.avEnable() != 2 || !c.fastForward {
		t.Fatalf("got %d, %v", c.avEnable(), c.fastForward)
	}
	c.SetOutput(true, false, false)
	if c.avEnable() != 1 || c.fastForward {
		t.Fatalf("got %d, %v", c.avEnable(), c.fastForward)
	}
}
