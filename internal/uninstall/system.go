package uninstall

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/wittywilla/wittymediaserver/internal/layout"
)

func indexOf(s, sub string) int { return strings.Index(s, sub) }

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// Here is this machine, as the running program sees it.
func Here() *Env {
	home, _ := os.UserHomeDir()
	root := ""
	if p := layout.Current(); p.Kind != layout.KindSource {
		root = p.Root
	}
	return &Env{GOOS: runtime.GOOS, Getenv: os.Getenv, Home: home, IsAdmin: isAdmin(), SelfRoot: root, Sys: realSystem{}, Libraries: ReadLibraries}
}

// ReadLibraries asks a data folder's database, read-only, which folders are libraries and
// which folders were chosen in Settings.
func ReadLibraries(dataDir string) (DataFacts, error) {
	name := strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(filepath.ToSlash(filepath.Join(dataDir, "wms.db")))
	if strings.Contains(name, ":") && !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	d, err := sql.Open("sqlite", "file:"+name+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return DataFacts{}, err
	}
	defer d.Close()
	d.SetMaxOpenConns(1)
	var f DataFacts
	rows, err := d.Query(`SELECT path FROM library_paths`)
	if err != nil {
		return f, fmt.Errorf("reading the libraries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil && p != "" {
			f.Libraries = append(f.Libraries, p)
		}
	}
	if err := rows.Err(); err != nil {
		return f, err
	}
	chosen := map[string]string{"logDir": "a log folder", "dvrFolder": "a folder for recorded TV"}
	for key, what := range chosen {
		var v string
		if d.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v) == nil && v != "" {
			f.Chosen = append(f.Chosen, Note{v, "you chose it as " + what + ": delete it yourself if you no longer want it"})
		}
	}
	return f, nil
}

func runProgram(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if len(text) > 300 {
			text = text[:300]
		}
		if text != "" {
			return string(out), fmt.Errorf("%w: %s", err, text)
		}
	}
	return string(out), err
}
