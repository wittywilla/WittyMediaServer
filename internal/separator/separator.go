// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package separator finds a stem separator the owner already installed (Demucs, Spleeter,
// audio-separator), so karaoke needs no configuration: the server looks on the PATH and in
// the usual places such tools are installed, and uses the best it finds. It never installs
// anything, and what it finds is a program on this machine, never a name from a request.
package separator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Found is a separator on this machine.
type Found struct {
	// Name is what it is called for people: "Demucs", "Spleeter", "audio-separator".
	Name string `json:"name"`
	// Version is what the program says its version is, "" when it doesn't say.
	Version string `json:"version,omitempty"`
	// Path is the program that runs.
	Path string `json:"-"`
	// Words are the command, with {input} and {output}, as mediaextras.SeparatorCommand takes it.
	Words []string `json:"-"`
}

// Options say where to look. The zero value looks where a real machine has them.
type Options struct {
	// Dirs are searched in order. Nil means the PATH and then UsualDirs.
	Dirs []string
	// Python names the interpreters tried for "python -m demucs". Nil means python3, python.
	Python []string
	// Timeout for each question asked of a program (its version, whether a module imports).
	Timeout time.Duration
}

// kinds are the separators known, best first. Demucs separates best; Spleeter is faster and
// rougher; audio-separator wraps several models and names its files its own way.
var kinds = []struct {
	name, bin string
	args      []string
}{
	{"Demucs", "demucs", []string{"-n", "htdemucs", "-o", "{output}", "{input}"}},
	{"Spleeter", "spleeter", []string{"separate", "-p", "spleeter:4stems", "-o", "{output}", "{input}"}},
	{"audio-separator", "audio-separator", []string{"{input}", "--output_dir", "{output}", "--output_format", "FLAC"}},
}

// UsualDirs are where pip, pipx, Homebrew and Conda put programs, beyond the PATH. A server
// started as a service often has a short PATH that leaves these out.
func UsualDirs() []string {
	var out []string
	home, _ := os.UserHomeDir()
	add := func(parts ...string) { out = append(out, filepath.Join(parts...)) }
	glob := func(pattern string) {
		m, _ := filepath.Glob(pattern)
		if len(m) > 16 {
			m = m[:16]
		}
		out = append(out, m...)
	}
	if runtime.GOOS == "windows" {
		if home != "" {
			glob(filepath.Join(home, "AppData", "Roaming", "Python", "Python*", "Scripts"))
			glob(filepath.Join(home, "AppData", "Local", "Programs", "Python", "Python*", "Scripts"))
			add(home, ".local", "bin")
			add(home, "miniconda3", "Scripts")
			add(home, "anaconda3", "Scripts")
		}
		return out
	}
	if home != "" {
		add(home, ".local", "bin")
		glob(filepath.Join(home, "Library", "Python", "*", "bin"))
		add(home, "miniconda3", "bin")
		add(home, "anaconda3", "bin")
		add(home, "miniforge3", "bin")
	}
	out = append(out, "/opt/homebrew/bin", "/usr/local/bin", "/opt/conda/bin", "/usr/bin")
	return out
}

func (o Options) dirs() []string {
	if o.Dirs != nil {
		return o.Dirs
	}
	return append(filepath.SplitList(os.Getenv("PATH")), UsualDirs()...)
}

// SearchDirs are the folders Discover looks in, in order, for the log when it finds nothing.
func (o Options) SearchDirs() []string { return o.dirs() }

// look finds an executable called name in dirs.
func look(name string, dirs []string) string {
	names := []string{name}
	if runtime.GOOS == "windows" {
		names = []string{name + ".exe", name + ".cmd", name + ".bat"}
	}
	for _, d := range dirs {
		if d == "" || !filepath.IsAbs(d) {
			continue
		}
		for _, n := range names {
			p := filepath.Join(d, n)
			fi, err := os.Stat(p)
			if err != nil || fi.IsDir() {
				continue
			}
			if runtime.GOOS != "windows" && fi.Mode()&0o111 == 0 {
				continue
			}
			return p
		}
	}
	return ""
}

// ask runs a program with a few arguments and answers the first line it prints, if it
// ends well and in time.
func ask(ctx context.Context, timeout time.Duration, prog string, args ...string) (string, bool) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, prog, args...)
	cmd.WaitDelay = 2 * time.Second
	b, err := cmd.Output()
	if err != nil {
		return "", false
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	line = strings.TrimSpace(line)
	if len(line) > 60 {
		line = line[:60]
	}
	return line, true
}

// version is the last word of what "--version" prints ("spleeter 2.4.0" is "2.4.0"), ""
// when the program has no such answer.
func version(ctx context.Context, timeout time.Duration, prog string) string {
	line, ok := ask(ctx, timeout, prog, "--version")
	if !ok || line == "" {
		return ""
	}
	f := strings.Fields(line)
	v := f[len(f)-1]
	if v == "" || !(v[0] >= '0' && v[0] <= '9' || v[0] == 'v') {
		return ""
	}
	return v
}

// Discover looks for a separator, best first: a demucs program, then Python's demucs
// module, then spleeter, then audio-separator. It answers nil when there is none.
func Discover(ctx context.Context, o Options) *Found {
	dirs := o.dirs()
	for i, k := range kinds {
		if p := look(k.bin, dirs); p != "" {
			f := &Found{Name: k.name, Path: p, Words: append([]string{p}, k.args...), Version: version(ctx, o.Timeout, p)}
			return f
		}
		if i > 0 {
			continue
		}
		// Demucs installed with pip but without its launcher on any path we know.
		pys := o.Python
		if pys == nil {
			pys = []string{"python3", "python"}
		}
		for _, py := range pys {
			p := look(py, dirs)
			if p == "" {
				continue
			}
			if v, ok := ask(ctx, o.Timeout, p, "-c", "import demucs;print(getattr(demucs,'__version__',''))"); ok {
				return &Found{Name: k.name, Path: p, Version: v, Words: append([]string{p, "-m", "demucs"}, k.args...)}
			}
		}
	}
	return nil
}
