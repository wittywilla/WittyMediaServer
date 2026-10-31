package libretro

import "testing"

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
