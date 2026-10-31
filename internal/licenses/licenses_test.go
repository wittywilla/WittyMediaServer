package licenses

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const regenerate = "run: go run ./internal/licenses/gen (with npm ci done in web/), and read what it lists"

func names() map[string]Component {
	out := map[string]Component{}
	for _, c := range All() {
		out[c.Name] = c
	}
	return out
}

func TestEveryComponentSaysWhatItIs(t *testing.T) {
	groups := map[string]bool{}
	for _, g := range Groups {
		groups[g.ID] = true
		if g.Title == "" || g.About == "" {
			t.Errorf("group %s says nothing", g.ID)
		}
	}
	last := -1
	order := map[string]int{}
	for i, g := range Groups {
		order[g.ID] = i
	}
	for i, c := range All() {
		if c.ID != i {
			t.Errorf("%s: id %d at %d", c.Name, c.ID, i)
		}
		if c.Name == "" || c.License == "" || !strings.HasPrefix(c.URL, "https://") || !groups[c.Group] {
			t.Errorf("incomplete: %+v", c)
		}
		if strings.Contains(strings.ToLower(c.License), "unknown") {
			t.Errorf("%s: license %q", c.Name, c.License)
		}
		if order[c.Group] < last {
			t.Errorf("%s is out of its group's order", c.Name)
		}
		last = order[c.Group]
		text, ok := Text(c.ID)
		if ok != c.HasText {
			t.Errorf("%s: says %v, has %v", c.Name, c.HasText, ok)
		}
		if c.HasText && len(text) < 100 {
			t.Errorf("%s: its license text is only %q", c.Name, text)
		}
		if (c.Group == GroupServer || c.Group == GroupWeb) && !c.HasText {
			t.Errorf("%s is built in, so it needs its license text", c.Name)
		}
	}
	if _, ok := Text(-1); ok {
		t.Error("id -1")
	}
	if _, ok := Text(len(All())); ok {
		t.Error("an id past the end")
	}
}

func TestWhatMustBeSaidIsSaid(t *testing.T) {
	all := names()
	if !strings.Contains(all["The Movie Database (TMDB)"].Note, "not endorsed or certified by TMDB") {
		t.Error("TMDB requires this exact notice")
	}
	if c := all["github.com/hashicorp/golang-lru/v2"]; c.License != "MPL-2.0" {
		t.Errorf("golang-lru: %+v", c)
	}
	if c := all["FFmpeg"]; !strings.Contains(c.License, "GPL") {
		t.Errorf("FFmpeg: %+v", c)
	}
	var cores int
	for _, c := range All() {
		if c.Group == GroupCores {
			cores++
		}
	}
	if cores == 0 {
		t.Error("no game cores listed")
	}
}

// A new dependency has to appear on the list before it can be merged.
func TestEveryGoLibraryIsListed(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	root := filepath.Join("..", "..")
	targets := []string{"./cmd/wms", "./cmd/wms-tray", "./cmd/wms-setup"}
	cmds, _ := filepath.Glob(filepath.Join(root, "modules", "*", "cmd", "*"))
	for _, c := range cmds {
		rel, _ := filepath.Rel(root, c)
		targets = append(targets, "./"+filepath.ToSlash(rel))
	}
	cmd := exec.Command("go", append([]string{"list", "-e", "-deps", "-f", `{{with .Module}}{{if not .Main}}{{.Path}}{{end}}{{end}}`}, targets...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	b, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	all := names()
	for _, p := range strings.Fields(string(b)) {
		if strings.HasPrefix(p, "github.com/wittywilla/") {
			continue
		}
		if _, ok := all[p]; !ok {
			t.Errorf("%s is compiled in but isn't on the list of licenses: %s", p, regenerate)
		}
	}
}

func TestEveryWebPackageIsListed(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "web", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		t.Fatal(err)
	}
	all := names()
	for p := range pkg.Dependencies {
		if strings.HasPrefix(p, "@wittywilla/") {
			continue
		}
		if _, ok := all[p]; !ok {
			t.Errorf("%s is in the web app but isn't on the list of licenses: %s", p, regenerate)
		}
	}
}
