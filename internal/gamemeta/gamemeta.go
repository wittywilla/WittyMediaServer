// Package gamemeta finds out about video games: what they're called, what they
// are, who made them, how they were rated and what they look like. See
// docs/METADATA.md.
//
// A Source is one place to ask. Sources only answer questions: they don't touch
// the library, and they hold no secrets beyond the ones they were built with.
// internal/library decides when to ask, keeps the answers and merges them.
//
// Everything a source says comes from outside, so it is checked here: answers are
// size-limited and parsed into plain fields, text is stripped of markup, and
// addresses are only ever web addresses.
package gamemeta

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// How a match was made. A checksum names one exact file; a serial one exact disc.
const (
	MethodChecksum = "checksum"
	MethodSerial   = "serial"
	MethodName     = "name"
	MethodManual   = "manual"
)

// AcceptScore is the least a name match needs to be used without asking. A wrong
// game's cover is worse than no cover.
const AcceptScore = 80

// ErrNotFound: the source answered, and doesn't know this game.
var ErrNotFound = errors.New("not known to this source")

// ErrNotSet: this source needs a key that hasn't been given.
var ErrNotSet = errors.New("this source needs a key")

// ErrKey: the source refused the key.
var ErrKey = errors.New("the key was not accepted")

// RateLimited: the source asked us to stop for now, or the allowance we keep for it
// is used up. Nothing is wrong with this game: ask again after Until.
type RateLimited struct {
	Until time.Time
	Why   string
}

func (e *RateLimited) Error() string { return "rate limited: " + e.Why }

// IsRetryable says whether an error means "ask again later" rather than "no".
func IsRetryable(err error) bool {
	return err != nil && !errors.Is(err, ErrNotFound)
}

// Query is what's known about a game file before asking.
type Query struct {
	Console string
	// Title is the cleaned name. Tags are what cleaning took off: regions, revisions.
	Title string
	Tags  string
	// CRCs are CRC-32s of the game's files, best guess first.
	CRCs []uint32
	// Size is the size of the first file those CRCs came from.
	Size int64
	// Serial is a disc's product code, such as SLUS-00594.
	Serial string
}

// Score is one rating out of 100, from one reviewer or the source's own users.
type Score struct {
	Source string  `json:"source"` // who gave the score, such as IGDB or Famitsu
	Kind   string  `json:"kind"`   // critic | user
	Value  float64 `json:"value"`  // out of 100
	Votes  int     `json:"votes,omitempty"`
	URL    string  `json:"url,omitempty"`
}

// AgeRating is one rating board's verdict.
type AgeRating struct {
	System string `json:"system"` // ESRB, PEGI, CERO, USK, ACB, GRAC, ClassInd, ELSPA
	Value  string `json:"value"`  // as the board writes it: T, 12, B
	Age    *int   `json:"age"`    // the youngest age it's meant for, nil if the board hasn't decided
}

// Info is what one source says about one game. Every field is optional.
type Info struct {
	ID          string   `json:"id"`
	Title       string   `json:"title,omitempty"`
	Tags        string   `json:"tags,omitempty"`
	Description string   `json:"description,omitempty"`
	Released    string   `json:"released,omitempty"` // YYYY, YYYY-MM or YYYY-MM-DD
	Developer   string   `json:"developer,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Players     int      `json:"players,omitempty"` // the most who can play at once
	Franchise   string   `json:"franchise,omitempty"`
	Poster      string   `json:"poster,omitempty"`
	Backdrop    string   `json:"backdrop,omitempty"`
	// More of each, to try if the first can't be fetched.
	MorePosters   []string    `json:"morePosters,omitempty"`
	MoreBackdrops []string    `json:"moreBackdrops,omitempty"`
	Art           []ArtRef    `json:"art,omitempty"`
	Scores        []Score     `json:"scores,omitempty"`
	Ages          []AgeRating `json:"ages,omitempty"`
	URL           string      `json:"url,omitempty"` // the game's page at the source
	// Method and Confidence say how sure the source is that this is the right game.
	Method     string `json:"method,omitempty"`
	Confidence int    `json:"confidence,omitempty"`
}

// ArtRef is one picture. Ref is a web address, or a source's own reference (see
// Referrer). It may be private to the server, so it's never sent to a browser.
type ArtRef struct {
	Ref  string `json:"ref"`
	Kind string `json:"kind"` // screenshot | title | artwork
}

// Candidate is one possible game in a search, for someone to choose from.
type Candidate struct {
	Source  string `json:"source"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Year    int    `json:"year,omitempty"`
	Console string `json:"console,omitempty"`
	Poster  string `json:"poster,omitempty"`
	Score   int    `json:"score"`
}

// KeyField is one thing the owner has to type in for a source.
type KeyField struct {
	Setting string `json:"setting"` // the server setting that holds it
	Label   string `json:"label"`
	Hint    string `json:"hint,omitempty"`
}

// About describes a source in words, for the setup wizard, Settings and the apps.
type About struct {
	ID string `json:"id"`
	// Name and Site as the source's terms ask for it to be credited.
	Name string `json:"name"`
	Site string `json:"site"`
	// Why is one line on what this source is good for.
	Why string `json:"why"`
	// Gives lists what it can provide.
	Gives []string `json:"gives"`
	// Fields are what it needs. None: it works straight away.
	Fields []KeyField `json:"fields"`
	// KeyURL is where to get a key, and Cost what that costs.
	KeyURL string   `json:"keyUrl,omitempty"`
	Cost   string   `json:"cost"`
	Steps  []string `json:"steps,omitempty"`
	// Limit is how much it may be asked, in words.
	Limit string `json:"limit"`
	// Sends says what leaves this server when it's asked.
	Sends string `json:"sends"`
	// Credit is the attribution its terms ask for, shown with its data.
	Credit string `json:"credit"`
	// License of its data, in a few words.
	License string `json:"license"`
	// Unavailable says why it can't be used on this build, if it can't.
	Unavailable string `json:"unavailable,omitempty"`
}

// Source is one place to ask about games.
type Source interface {
	About() About
	// Lookup finds the game a file is. ErrNotFound when it doesn't know it.
	Lookup(ctx context.Context, q Query) (*Info, error)
	// Search lists games that might be meant, best first.
	Search(ctx context.Context, console, title string) ([]Candidate, error)
	// Fetch gets one game by the source's own id.
	Fetch(ctx context.Context, console, id string) (*Info, error)
	// Test says whether the source can be used, trying its key if it has one.
	Test(ctx context.Context) (string, error)
}

// Preparer is a source that does better asked in bulk: it's told a batch of games
// before they're looked up one at a time, and can fetch them all in a few requests.
type Preparer interface {
	Prepare(ctx context.Context, console string, qs []Query) error
}

// Referrer is a source whose pictures can't be stored as plain addresses, because
// fetching them takes credentials.
type Referrer interface {
	PictureURL(ref string) (string, error)
}

// Metered is a source with a monthly or daily allowance. Local work and requests
// that didn't count don't call Spend.
type Metered interface {
	// Allowance is how many requests to spend per period ("month" or "day"), leaving a
	// margin. 0: no allowance to keep track of.
	Allowance() (n int, period string)
	// SetMeter gives the source a way to count what it spends.
	SetMeter(spend func(n int) error)
}

// GapFiller is a source tight enough on requests that it's only asked when something
// is still missing.
type GapFiller interface {
	GapsOnly() bool
}

func errf(source, format string, a ...any) error {
	return fmt.Errorf(source+": "+format, a...)
}
