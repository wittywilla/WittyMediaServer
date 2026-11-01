package modules

import (
	"path/filepath"
	"strings"
)

// sbplString quotes text for a sandbox profile, so a folder's name can't end its string and add a
// rule of its own. Only a quote and a backslash mean anything inside one.
func sbplString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// helperProfile is a Mac's sandbox for one helper. Reading stays open, as a helper reads your media
// wherever it is. It's here rather than beside the launcher so its text can be tested anywhere.
func helperProfile(mode int, program string, writable []string) string {
	var b strings.Builder
	b.WriteString(`(version 1)(allow default)`)
	if mode&SandboxNetwork == 0 {
		b.WriteString(`(deny network*)`)
	}
	if mode&SandboxPrograms == 0 {
		// Only what it was started as, which sandbox-exec is about to do once it's in the sandbox.
		b.WriteString(`(deny process-exec*)(allow process-exec (literal ` + sbplString(program) + `))(deny process-fork)`)
	}
	b.WriteString(`(deny signal (target others))`)
	b.WriteString(`(deny file-write*)(allow file-write-data (literal "/dev/null") (literal "/dev/stdout") (literal "/dev/stderr"))`)
	for _, d := range writable {
		if filepath.IsAbs(d) {
			b.WriteString(`(allow file-write* (subpath ` + sbplString(d) + `))`)
		}
	}
	return b.String()
}

// writableFolders are the folders a helper may write to, as the launcher is told them: real paths,
// since that's what a sandbox matches.
func writableFolders(mode int, args []string, env func(string) string) []string {
	var out []string
	if mode&SandboxSaveArg != 0 {
		for i, a := range args {
			if a == "-save" && i+1 < len(args) {
				out = append(out, realPath(args[i+1]))
			}
		}
	}
	if mode&SandboxWriteData != 0 {
		if d := env("WITTYMODULE_DATA"); d != "" {
			out = append(out, realPath(d))
		}
		for _, d := range filepath.SplitList(env(SandboxWriteEnv)) {
			out = append(out, realPath(d))
		}
		if d := env("TMPDIR"); d != "" {
			out = append(out, realPath(d))
		}
	}
	return out
}

// realPath is a path as a sandbox sees it: on a Mac, /tmp, /var and /etc are links.
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
