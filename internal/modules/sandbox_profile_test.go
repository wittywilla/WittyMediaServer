package modules

import (
	"strings"
	"testing"
)

// A folder's name goes into a Mac's sandbox profile. It mustn't be able to add a rule of its own.
func TestProfileQuotesAFoldersName(t *testing.T) {
	for _, evil := range []string{`/data/x"))(allow file-write* (subpath "/`, `/data/x\`, `/data/x\"))(allow default)(`, "/data/\"\n(allow network*)"} {
		got := helperProfile(0, "/prog", []string{evil})
		quoted := sbplString(evil)
		if !strings.Contains(got, `(allow file-write* (subpath `+quoted+`))`) {
			t.Errorf("%q wasn't quoted: %s", evil, got)
		}
		// Reading it back as the sandbox would: a string ends at the first quote with no backslash.
		inString, escaped, depth, bad := false, false, 0, false
		for _, c := range got {
			switch {
			case escaped:
				escaped = false
			case inString && c == '\\':
				escaped = true
			case c == '"':
				inString = !inString
			case !inString && c == '(':
				depth++
			case !inString && c == ')':
				depth--
				bad = bad || depth < 0
			}
		}
		if inString || depth != 0 || bad {
			t.Errorf("%q broke out of its string: %s", evil, got)
		}
	}
}

// What each thing a helper may be allowed does to its profile.
func TestProfileGivesOnlyWhatsAsked(t *testing.T) {
	bare := helperProfile(0, "/prog", nil)
	for _, want := range []string{`(deny network*)`, `(deny process-exec*)`, `(allow process-exec (literal "/prog"))`, `(deny process-fork)`, `(deny file-write*)`, `(deny signal (target others))`} {
		if !strings.Contains(bare, want) {
			t.Errorf("a bare helper's profile has no %s: %s", want, bare)
		}
	}
	all := helperProfile(SandboxNetwork|SandboxPrograms|SandboxWriteData, "/prog", []string{"/data"})
	for _, not := range []string{`(deny network*)`, `(deny process-exec*)`, `(deny process-fork)`} {
		if strings.Contains(all, not) {
			t.Errorf("a helper allowed it still has %s: %s", not, all)
		}
	}
	if !strings.Contains(all, `(allow file-write* (subpath "/data"))`) || !strings.Contains(all, `(deny file-write*)`) {
		t.Errorf("writing should be only to its folder: %s", all)
	}
	if strings.Contains(helperProfile(SandboxWriteData, "/prog", []string{"relative/path"}), "relative/path") {
		t.Error("a relative path would mean wherever the sandbox thinks it is")
	}
}

func TestWritableFoldersAreOnlyWhatsAsked(t *testing.T) {
	env := func(k string) string {
		return map[string]string{"WITTYMODULE_DATA": "/d", SandboxWriteEnv: "/w", "TMPDIR": "/t"}[k]
	}
	if got := writableFolders(0, []string{"-save", "/s"}, env); len(got) != 0 {
		t.Errorf("nothing asked for, and got %v", got)
	}
	if got := strings.Join(writableFolders(SandboxSaveArg, []string{"-save", "/s"}, env), ","); got != "/s" {
		t.Errorf("the save argument: %s", got)
	}
	if got := strings.Join(writableFolders(SandboxWriteData, nil, env), ","); got != "/d,/w,/t" {
		t.Errorf("its data: %s", got)
	}
}

// What the pages say always comes with a title and words when it's anything short of full.
func TestConfinementIsSaidInWords(t *testing.T) {
	for _, launcher := range []string{"", "/some/program"} {
		c := confinement(launcher)
		switch c.Level {
		case SandboxFull:
		case SandboxPartial, SandboxLimited, SandboxUnconfined:
			if c.Title == "" || c.Words == "" {
				t.Errorf("%q: %+v has nothing to show", launcher, c)
			}
		default:
			t.Errorf("%q: unknown level %q", launcher, c.Level)
		}
	}
	if c := confinement(""); c.Level == SandboxFull {
		t.Error("with no launcher nothing can be fully confined")
	}
}
