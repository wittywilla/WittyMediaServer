package docscheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

const repo = "../.."

// A guide's lines, without its fenced code.
func prose(text string) []string {
	var out []string
	fence := ""
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = t[:3]
			continue
		}
		out = append(out, line)
	}
	return out
}

var (
	headingRe    = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	headingLink  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	inlineCodeRe = regexp.MustCompile("`[^`]*`")
	linkRe       = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)[^)]*\)`)
)

// The address GitHub gives a heading. The web app's slug() in web/src/help/markdown.ts is the same.
func slug(h string) string {
	h = headingLink.ReplaceAllString(h, "$1")
	h = strings.NewReplacer("`", "", "**", "", "\\", "").Replace(h)
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), r == '_', r == '-':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune('-')
		}
	}
	return b.String()
}

func anchors(text string) map[string]bool {
	seen := map[string]int{}
	out := map[string]bool{}
	for _, line := range prose(text) {
		m := headingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		s := slug(m[1])
		if n := seen[s]; n > 0 {
			out[s+"-"+itoa(n)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	}
	return out
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + itoa(n%10)
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func docsFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(repo, "docs", "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no guides: %v", err)
	}
	return files
}

// Every link between the documents leads to a file and a heading that are there.
func TestLinksLeadSomewhere(t *testing.T) {
	files := docsFiles(t)
	for _, f := range []string{"README.md", "CHANGELOG.md", "CONTRIBUTING.md", "SECURITY.md"} {
		files = append(files, filepath.Join(repo, f))
	}
	for _, file := range files {
		text := read(t, file)
		name, _ := filepath.Rel(repo, file)
		for _, line := range prose(text) {
			for _, m := range linkRe.FindAllStringSubmatch(inlineCodeRe.ReplaceAllString(line, ""), -1) {
				href := m[1]
				if strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:") {
					continue
				}
				path, hash, _ := strings.Cut(href, "#")
				other := text
				if path != "" {
					target := filepath.Join(filepath.Dir(file), filepath.FromSlash(path))
					b, err := os.ReadFile(target)
					if err != nil {
						t.Errorf("%s links to %s, which isn't there", name, href)
						continue
					}
					other = string(b)
				}
				if hash != "" && strings.HasSuffix(strings.ToLower(path+"x.md"), ".md") && !anchors(other)[hash] {
					t.Errorf("%s links to %s: no such heading", name, href)
				}
			}
		}
	}
}

// A guide is either in the app's list or said to be left out, and what's left out isn't bundled.
func TestEveryGuideIsListedInHelp(t *testing.T) {
	src := read(t, filepath.Join(repo, "web/src/help/guides.ts"))
	listed := map[string]bool{}
	for _, m := range regexp.MustCompile(`file: '([^']+)'`).FindAllStringSubmatch(src, -1) {
		if listed[m[1]] {
			t.Errorf("%s is in the help list twice", m[1])
		}
		listed[m[1]] = true
	}
	left := regexp.MustCompile(`notInHelp = \[([^\]]*)\]`).FindStringSubmatch(src)
	if left == nil {
		t.Fatal("guides.ts has no notInHelp")
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(left[1], -1) {
		out[m[1]] = true
		if !strings.Contains(src, "'!../../../docs/"+m[1]+".md'") {
			t.Errorf("%s is in notInHelp but would still be built into the app", m[1])
		}
	}
	for _, f := range docsFiles(t) {
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		if !listed[name] && !out[name] {
			t.Errorf("docs/%s.md isn't in web/src/help/guides.ts: add it to GROUPS or notInHelp", name)
		}
		if listed[name] && out[name] {
			t.Errorf("docs/%s.md is both listed and left out", name)
		}
	}
	for name := range listed {
		if _, err := os.Stat(filepath.Join(repo, "docs", name+".md")); err == nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(repo, name+".md")); err != nil {
			t.Errorf("guides.ts lists %s, and there's no such file", name)
		}
	}
}

func webSources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	root := filepath.Join(repo, "web/src")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".tsx") || strings.HasSuffix(p, ".ts")) && !strings.HasSuffix(p, ".d.ts") {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[p] = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func guideText(t *testing.T, guideSlug string) (string, bool) {
	t.Helper()
	files := docsFiles(t)
	files = append(files, filepath.Join(repo, "CHANGELOG.md"), filepath.Join(repo, "SECURITY.md"))
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		if strings.ReplaceAll(strings.ToLower(name), "_", "-") == guideSlug {
			return read(t, f), true
		}
	}
	return "", false
}

// A link from a page to a guide leads to that guide and that heading.
func TestAppLinksToHelpLeadSomewhere(t *testing.T) {
	link := regexp.MustCompile(`[^.\w/]/help/([a-z0-9-]+)(?:#([a-z0-9_-]+))?`)
	short := regexp.MustCompile(`<HelpLink to="([a-z0-9-]+)(?:#([a-z0-9_-]+))?"`)
	for p, src := range webSources(t) {
		matches := append(link.FindAllStringSubmatch(src, -1), short.FindAllStringSubmatch(src, -1)...)
		for _, m := range matches {
			text, ok := guideText(t, m[1])
			if !ok {
				t.Errorf("%s links to help guide %q, and there's none", p, m[1])
				continue
			}
			if m[2] != "" && !anchors(text)[m[2]] {
				t.Errorf("%s links to %s#%s: no such heading", p, m[1], m[2])
			}
		}
	}
	guides := read(t, filepath.Join(repo, "web/src/help/guides.ts"))
	block := regexp.MustCompile(`(?s)settingsHelp: Record<string, string> = \{(.*?)\n\}`).FindStringSubmatch(guides)
	if block == nil {
		t.Fatal("guides.ts has no settingsHelp")
	}
	for _, m := range regexp.MustCompile(`: '([a-z0-9-]+)(?:#([a-z0-9_-]+))?'`).FindAllStringSubmatch(block[1], -1) {
		text, ok := guideText(t, m[1])
		if !ok {
			t.Errorf("settingsHelp names guide %q, and there's none", m[1])
		} else if m[2] != "" && !anchors(text)[m[2]] {
			t.Errorf("settingsHelp names %s#%s: no such heading", m[1], m[2])
		}
	}
}

// Every settings page has help, and a guide's "Settings → Repair" knows where that is.
func TestSettingsPagesHaveHelp(t *testing.T) {
	src := read(t, filepath.Join(repo, "web/src/pages/SettingsPage.tsx"))
	guides := read(t, filepath.Join(repo, "web/src/help/guides.ts"))
	for _, m := range regexp.MustCompile(`\{ to: '([a-z-]+)', label: '([^']+)'`).FindAllStringSubmatch(src, -1) {
		slugName, label := m[1], m[2]
		if !regexp.MustCompile(`\n\s*'?` + regexp.QuoteMeta(slugName) + `'?: '[a-z0-9#_-]+',`).MatchString(guides) {
			t.Errorf("settings page %s has no entry in settingsHelp", slugName)
		}
		if !strings.Contains(guides, "'"+label+"': '"+slugName+"'") && !strings.Contains(guides, label+": '"+slugName+"'") {
			t.Errorf("settingsPages doesn't say that %q is %s", label, slugName)
		}
	}
}

// Nobody is sent to GitHub, or to a file in the repository, to find out how something works.
func TestHelpIsNotOnGitHub(t *testing.T) {
	blob := regexp.MustCompile(`wittywilla/WittyMediaServer/(blob|tree|wiki)`)
	shown := regexp.MustCompile(`<code>docs/|[^/ ]*\bdocs/[A-Z_-]+\.md`)
	for p, src := range webSources(t) {
		if blob.MatchString(src) {
			t.Errorf("%s links to GitHub for help: use /help/<guide>", p)
		}
		if filepath.Base(p) == "guides.ts" {
			continue // the list of what's bundled
		}
		for _, line := range strings.Split(src, "\n") {
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "/*") {
				continue
			}
			if strings.Contains(line, "//") && strings.Index(line, "docs/") > strings.LastIndex(line, "//") {
				continue
			}
			if shown.MatchString(line) {
				t.Errorf("%s shows a repository path: %s", p, strings.TrimSpace(line))
			}
		}
	}
	messages := regexp.MustCompile(`["` + "`" + `][^"` + "`" + `]*\bdocs/[A-Z_-]+\.md`)
	for _, root := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(repo, root), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || strings.Contains(p, "mediatypes") {
				return nil
			}
			for _, line := range strings.Split(read(t, p), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				if messages.MatchString(line) {
					t.Errorf("%s tells people to read a repository file: %s", p, strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// One footer, with no version in it to go stale, and no HTML for the app to drop.
func TestGuidesAreTidy(t *testing.T) {
	const footer = "<sub>WittyMediaServer documentation</sub>"
	html := regexp.MustCompile(`^\s*</?[a-zA-Z][^>]*>`)
	for _, file := range docsFiles(t) {
		text := strings.TrimRight(strings.ReplaceAll(read(t, file), "\r\n", "\n"), "\n ")
		name := filepath.Base(file)
		if !strings.HasSuffix(text, "---\n\n"+footer) {
			t.Errorf("%s doesn't end with the footer", name)
		}
		if !strings.HasPrefix(text, "# ") {
			t.Errorf("%s doesn't start with its title", name)
		}
		for _, line := range prose(text) {
			if html.MatchString(line) && strings.TrimSpace(line) != footer {
				t.Errorf("%s has HTML, which the app won't show: %s", name, strings.TrimSpace(line))
			}
		}
	}
}

// Commands are wittymediaserver's. "wms" lives on only in file names and variables.
func TestProgramNamesAreCurrent(t *testing.T) {
	old := regexp.MustCompile(`(^|[^\w./-])(\./)?wms(\.exe)? (serve|doctor|config|backup|logs|metadata|dvd|bluray|cd|version|update|hash|uninstall)\b|cmd/wms\b|wms-tray|wms-setup`)
	allowed := []string{"CHANGELOG.md", "INSTALL.md", "UNINSTALL.md"}
	for _, file := range docsFiles(t) {
		if slices.Contains(allowed, filepath.Base(file)) {
			continue
		}
		if m := old.FindString(read(t, file)); m != "" {
			t.Errorf("%s uses an old program name: %q", filepath.Base(file), m)
		}
	}
}
