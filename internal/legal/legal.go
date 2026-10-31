// Package legal holds the words the project says about itself, in one place: the
// short notices shown beside content (notices.json) and the full Legal page
// (legal.json). The web app bundles the same files, and native apps read them
// from the API, so every screen says the same thing. docs/LEGAL-NOTICES-CHECKLIST.md
// says which screen takes which notice.
package legal

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sync"
)

//go:embed notices.json
var noticesJSON []byte

//go:embed legal.json
var legalJSON []byte

// A Notice is one short notice. Section is the part of the Legal page it links to.
type Notice struct {
	Text    string `json:"text"`
	Section string `json:"section"`
}

// Notices is everything a client needs to show the short notices. AckVersion goes
// up only when the first-run acknowledgement changes enough to ask again.
type Notices struct {
	Version    int               `json:"version"`
	AckVersion int               `json:"ackVersion"`
	LinkText   string            `json:"linkText"`
	Ack        string            `json:"ack"`
	Notices    map[string]Notice `json:"notices"`
}

// A Section is one heading of the Legal page. Each string in Body is a paragraph;
// one starting with "- " is a bullet. Inline, **bold**, [text](address) and
// [[FILL IN: ...]] are the only markup, and addresses starting with / are pages of this app.
type Section struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Body  []string `json:"body"`
}

// A Service is one outside service on the privacy list. Setting is the app page
// that controls it.
type Service struct {
	Name    string   `json:"name"`
	Hosts   []string `json:"hosts"`
	Gets    string   `json:"gets"`
	When    string   `json:"when"`
	Control string   `json:"control"`
	Setting string   `json:"setting"`
}

// Text is the whole Legal page.
type Text struct {
	Updated  string    `json:"updated"`
	Sections []Section `json:"sections"`
	Services []Service `json:"services"`
}

var (
	once    sync.Once
	notices Notices
	text    Text
)

func load() {
	once.Do(func() {
		if err := json.Unmarshal(noticesJSON, &notices); err != nil {
			panic("internal/legal/notices.json: " + err.Error())
		}
		if err := json.Unmarshal(legalJSON, &text); err != nil {
			panic("internal/legal/legal.json: " + err.Error())
		}
	})
}

// Short returns the short notices.
func Short() Notices {
	load()
	return notices
}

// Full returns the Legal page.
func Full() Text {
	load()
	return text
}

// fillIn finds what the project's owner still has to write in legal.json.
var fillIn = regexp.MustCompile(`\[\[FILL IN: ([^\]]+)\]\]`)

// Placeholders lists what the project's owner still has to fill in.
func Placeholders() []string {
	load()
	var out []string
	for _, s := range text.Sections {
		for _, p := range s.Body {
			for _, m := range fillIn.FindAllStringSubmatch(p, -1) {
				out = append(out, m[1])
			}
		}
	}
	return out
}

// Limits on what a server's owner can write about their own server.
const (
	MaxOwnerNotice  = 4000
	MaxOwnerContact = 300
)
