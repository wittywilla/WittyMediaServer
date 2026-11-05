package desktop

import (
	"strings"
	"testing"
)

var three = []string{"Open WittyMediaServer", "Start another server…", "Exit"}

func TestOsascript(t *testing.T) {
	args := osascriptArgs("Running", `It's at "here"`, three)
	line := args[len(args)-1]
	if !strings.Contains(line, `buttons {"Exit", "Start another server…", "Open WittyMediaServer"}`) ||
		!strings.Contains(line, `default button "Open WittyMediaServer"`) || !strings.Contains(line, `cancel button "Exit"`) ||
		!strings.Contains(line, `It's at \"here\"`) {
		t.Fatal(line)
	}
	if one := osascriptArgs("", "x", []string{"OK"}); strings.Contains(one[len(one)-1], "cancel button") {
		t.Fatal(one)
	}
	for out, want := range map[string]int{
		"button returned:Start another server…\n": 1,
		"button returned:Open WittyMediaServer":   0,
		"button returned:Something else":          -1,
		"":                                        -1,
	} {
		if got := osascriptAnswer(out, three); got != want {
			t.Errorf("%q: %d, want %d", out, got, want)
		}
	}
}

func TestZenity(t *testing.T) {
	args := strings.Join(zenityArgs("A <b>", "x & y", three), "|")
	for _, want := range []string{"--question", "--ok-label=Open WittyMediaServer", "--cancel-label=Exit", "--extra-button=Start another server…", "A &lt;b&gt;", "x &amp; y"} {
		if !strings.Contains(args, want) {
			t.Errorf("%s missing %s", args, want)
		}
	}
	if one := zenityArgs("", "x", []string{"OK"}); one[0] != "--info" {
		t.Fatal(one)
	}
	for _, tc := range []struct {
		code int
		out  string
		want int
	}{{0, "", 0}, {1, "Start another server…\n", 1}, {1, "", 2}, {5, "", -1}, {-1, "", -1}} {
		if got := zenityAnswer(tc.code, tc.out, three); got != tc.want {
			t.Errorf("%d %q: %d, want %d", tc.code, tc.out, got, tc.want)
		}
	}
}

func TestKdialog(t *testing.T) {
	if a := strings.Join(kdialogArgs("T", "x", three), "|"); !strings.Contains(a, "--yesnocancel") || !strings.Contains(a, "--no-label|Start another server…") {
		t.Fatal(a)
	}
	if a := kdialogArgs("", "x", three[:2]); a[2] != "--yesno" {
		t.Fatal(a)
	}
	for code, want := range map[int]int{0: 0, 1: 1, 2: 2, 3: -1} {
		if got := kdialogAnswer(code, three); got != want {
			t.Errorf("%d: %d, want %d", code, got, want)
		}
	}
}
