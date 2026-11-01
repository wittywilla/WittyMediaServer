package legal

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const repo = "../.."

// The core words: third-party notices say this, in these words, so no screen
// quietly says less than the others.
const disclaimer = "associated with, endorsed by or checked by"

func TestEveryKindExistsAndLinksToASection(t *testing.T) {
	sections := map[string]bool{}
	for _, s := range Full().Sections {
		if s.ID == "" || s.Title == "" || len(s.Body) == 0 {
			t.Errorf("section %+v is incomplete", s)
		}
		if sections[s.ID] {
			t.Errorf("section %q twice", s.ID)
		}
		sections[s.ID] = true
	}
	for id, n := range Short().Notices {
		if n.Text == "" {
			t.Errorf("notice %q has no words", id)
		}
		if !sections[n.Section] {
			t.Errorf("notice %q links to section %q, which isn't on the Legal page", id, n.Section)
		}
	}
	for _, id := range []string{"general", "searchResults", "webResults", "metadata", "webMedia", "tv", "games", "creators", "reader", "local", "thirdPartyServer", "connect", "cores", "modules", "sharing", "sharedView", "remote", "ratings", "flashing"} {
		if _, ok := Short().Notices[id]; !ok {
			t.Errorf("notice %q is missing", id)
		}
	}
}

// What the Legal page has to cover.
func TestLegalPageCoversEverything(t *testing.T) {
	want := map[string]string{
		"terms":           "Apache License 2.0",
		"no-warranty":     "AS IS",
		"no-affiliation":  "endorsed by",
		"third-party":     "not endorsed or certified by TMDB",
		"your-content":    "provide, host, own",
		"sharing":         "you're the one sharing",
		"trademarks":      "Blu-ray Disc Association",
		"games":           "BIOS",
		"copy-protection": "decrypts",
		"cryptography":    "encryption",
		"privacy":         "No telemetry",
		"licenses":        "THIRD_PARTY_LICENSES",
		"flashing":        "photosensitive",
		"ratings":         "guarantee",
		"server-owners":   "own notice",
		"contact":         "FILL IN",
	}
	body := map[string]string{}
	for _, s := range Full().Sections {
		body[s.ID] = strings.Join(s.Body, "\n")
	}
	for id, phrase := range want {
		if !strings.Contains(body[id], phrase) {
			t.Errorf("section %q should mention %q", id, phrase)
		}
	}
	if !strings.Contains(body["terms"], "isn't legal advice") || !strings.Contains(body["terms"], "Local law applies") {
		t.Error("the terms should say local law applies and that this isn't legal advice")
	}
}

func TestThirdPartyNoticesSayTheSameThing(t *testing.T) {
	for _, id := range []string{"searchResults", "webResults", "metadata", "webMedia", "tv", "games", "cores", "modules", "reader", "thirdPartyServer"} {
		if !strings.Contains(Short().Notices[id].Text, disclaimer) {
			t.Errorf("notice %q doesn't say %q", id, disclaimer)
		}
	}
	for _, id := range []string{"general", "local", "connect", "sharing", "sharedView", "remote"} {
		if !strings.Contains(Short().Notices[id].Text, "WittyMediaServer project") {
			t.Errorf("notice %q doesn't name the project", id)
		}
	}
	if !strings.Contains(Short().Ack, "responsible for having the rights") {
		t.Error("the first-run acknowledgement should say who's responsible")
	}
}

// The screens that show someone else's content, and the notice each one carries.
// A new screen goes here and in docs/LEGAL-NOTICES-CHECKLIST.md.
var screens = map[string][]string{
	"App.tsx":                            {"general"},
	"pages/SearchPage.tsx":               {"searchResults"},
	"pages/ItemPage.tsx":                 {"metadata"},
	"pages/ShowPage.tsx":                 {"metadata"},
	"components/ArtworkPicker.tsx":       {"webResults"},
	"pages/Podcasts.tsx":                 {"webResults"},
	"pages/RadioLibrary.tsx":             {"webMedia", "webResults"},
	"pages/Couch.tsx":                    {"thirdPartyServer"},
	"pages/Rigs.tsx":                     {"thirdPartyServer"},
	"pages/GameMeta.tsx":                 {"webResults"},
	"player/CdPlayer.tsx":                {"metadata", "webResults"},
	"pages/settings/Couch.tsx":           {"thirdPartyServer"},
	"pages/settings/WittyRigs.tsx":       {"games"},
	"pages/settings/GamingCustom.tsx":    {"cores"},
	"pages/AudioLibrary.tsx":             {"webMedia"},
	"pages/tv/GuidePage.tsx":             {"tv"},
	"pages/tv/TvSettings.tsx":            {"tv"},
	"pages/Games.tsx":                    {"games"},
	"pages/creators/ChannelsLibrary.tsx": {"creators"},
	"pages/creators/ShortsLibrary.tsx":   {"creators"},
	"pages/creators/WatchPage.tsx":       {"creators"},
	"pages/Books.tsx":                    {"reader"},
	"pages/settings/Metadata.tsx":        {"webResults"},
	"pages/settings/Gaming.tsx":          {"cores"},
	"pages/settings/Modules.tsx":         {"modules"},
	"pages/settings/Sharing.tsx":         {"sharing"},
	"pages/settings/RemoteAccess.tsx":    {"remote"},
	"components/ShareDialog.tsx":         {"sharing"},
	"pages/SharedPage.tsx":               {"sharedView"},
	"components/ConnectServer.tsx":       {"connect"},
	"pages/SetupWizard.tsx":              {"webResults"},
	"pages/SettingsPage.tsx":             {"local"},
	"pages/Profiles.tsx":                 {"ratings"},
}

func TestEveryScreenCarriesItsNotice(t *testing.T) {
	for file, kinds := range screens {
		b, err := os.ReadFile(filepath.Join(repo, "web/src", file))
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if !strings.Contains(src, "components/LegalNote'") && !strings.Contains(src, "./LegalNote'") {
			t.Errorf("%s doesn't import LegalNote", file)
		}
		for _, k := range kinds {
			if _, ok := Short().Notices[k]; !ok {
				t.Errorf("%s: no notice called %q", file, k)
			}
			if !strings.Contains(src, `kind="`+k+`"`) && !strings.Contains(src, `'`+k+`'`) {
				t.Errorf("%s doesn't show the %q notice", file, k)
			}
		}
	}
}

// Every kind a screen asks for has words (TypeScript also checks this at build time).
func TestEveryKindUsedExists(t *testing.T) {
	re := regexp.MustCompile(`<LegalNote kind="(\w+)"`)
	err := filepath.WalkDir(filepath.Join(repo, "web/src"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".tsx") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllSubmatch(b, -1) {
			if _, ok := Short().Notices[string(m[1])]; !ok {
				t.Errorf("%s asks for a notice that doesn't exist: %s", p, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The first-run tick-box is required, and is on the step that creates the account.
func TestSetupWizardAsksOnce(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repo, "web/src/pages/SetupWizard.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{`type="checkbox" required checked={ack}`, "acknowledgedLegal: ack", "legalAckText()", "|| !ack}"} {
		if !strings.Contains(src, want) {
			t.Errorf("the setup wizard's account step should have %q", want)
		}
	}
	if n := strings.Count(src, `kind="local"`); n != 0 {
		t.Errorf("the wizard shows the local-media notice once, as the tick-box, but also %d more", n)
	}
}

// The apps say what the server says.
func TestClientAppsUseTheSameWords(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repo, "clients/wittymediaclient/shared/data/strings.en.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]string
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s["legalConnect"] != Short().Notices["connect"].Text {
		t.Errorf("clients/wittymediaclient/shared/data/strings.en.json's legalConnect should match the connect notice:\n%s\n%s", s["legalConnect"], Short().Notices["connect"].Text)
	}
}

// Don't send anyone to a way around copy protection.
func TestNothingTellsPeopleToDecrypt(t *testing.T) {
	re := regexp.MustCompile(`(?i)(make|making|create)\s+(a\s+)?decrypted|decrypt\s+it\s+before|tool\s+that\s+is\s+legal`)
	for _, root := range []string{"internal", "modules", "cmd", "web/src", "docs", "README.md", "clients/wittymediaclient/shared"} {
		err := filepath.WalkDir(filepath.Join(repo, root), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(p) {
			case ".go", ".ts", ".tsx", ".md", ".json":
			default:
				return nil
			}
			if strings.HasSuffix(p, "legal_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if m := re.Find(b); m != nil {
				t.Errorf("%s says %q", p, m)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Addresses that are never contacted by themselves: links for people to click, the
// identifiers of file formats, examples, and things people type in. A new one here
// should be a decision: anything the server calls goes on the privacy list instead.
var notContacted = regexp.MustCompile(`(?i)(^|\.)(example\.(com|org|net)|example|localhost|127\.0\.0\.1|w3\.org|purl\.org|itunes\.com|www\.apple\.com|creativecommons\.org|schemas\.\S+|xmlsoap\.org|schema\.org|www\.w3\.org|video\.google\.com|shop\.silicondust\.com|info\.hdhomerun\.com|zapperbox\.com|support\.adth\.com|a3sa\.com|wikipedia\.org|fandom\.com|ffmpeg\.org|apache\.org|anilist\.co|tvmaze\.com|themoviedb\.org|dictionaryapi\.dev|naturalearthdata\.com|go\.dev|npmjs\.com|dev\.twitch\.tv|igdb\.com|rawg\.io|screenscraper\.fr|thegamesdb\.net|apps\.apple\.com|play\.google\.com|amazon\.com|radio-browser\.info)$`)

// "Nothing leaves except what's listed" has to stay true: a host written into the
// server's code that isn't on the privacy list fails here.
func TestEveryHostInTheCodeIsOnThePrivacyList(t *testing.T) {
	listed := map[string]bool{}
	for _, s := range Full().Services {
		for _, h := range s.Hosts {
			listed[h] = true
		}
	}
	url := regexp.MustCompile(`https?://([a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)
	for _, root := range []string{"internal", "modules", "cmd"} {
		err := filepath.WalkDir(filepath.Join(repo, root), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" || d.Name() == "web" || d.Name() == "java" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(b), "\n") {
				if t := strings.TrimSpace(line); strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") {
					continue
				}
				if c := strings.Index(line, " // "); c >= 0 {
					line = line[:c]
				}
				for _, m := range url.FindAllStringSubmatch(line, -1) {
					h := strings.ToLower(m[1])
					if !listed[h] && !notContacted.MatchString(h) {
						t.Errorf("%s:%d names %s, which isn't on the privacy list in internal/legal/legal.json (services)", p, i+1, h)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestOwnerPlaceholdersAreListedForTheOwner(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repo, "docs/LEGAL-NOTICES-CHECKLIST.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(Placeholders()) == 0 {
		return // all filled in
	}
	for _, p := range Placeholders() {
		if !strings.Contains(string(b), p) {
			t.Errorf("docs/LEGAL-NOTICES-CHECKLIST.md should tell the owner to fill in: %s", p)
		}
	}
}
