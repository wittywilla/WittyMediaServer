package gamemeta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// ScreenScraper. Its API may only go in free software, and it identifies each program
// with a developer id. That id belongs to whoever builds the program, not to the
// person running it, so it goes in at build time (see DevID below) and never in the
// repository. A self-built server has none until its builder asks for one, and then
// the source says so and steps aside. Fields for the owner's own ScreenScraper account
// raise how much they may ask.
//
// Logins travel in addresses here, so nothing built from them is ever stored, logged
// or sent to a browser: pictures are kept as "ss:" references and only turned into
// addresses at the moment the server fetches them.

// Set when building a copy to give out, with -ldflags "-X ...gamemeta.DevID=... -X ...gamemeta.DevPassword=...".
var (
	DevID       = ""
	DevPassword = ""
)

func init() { logging.AddSecret(DevPassword) }

// ScreenScraper asks screenscraper.fr.
type ScreenScraper struct {
	User, Password string
	// DevUser and DevPass are what the owner typed, if they have developer access of
	// their own. Otherwise the ones built in are used.
	DevUser, DevPass string
	SoftName         string
	HTTP             *http.Client
	// Base is only changed by tests.
	Base string

	once sync.Once
	c    *Client
}

func (s *ScreenScraper) init() {
	s.once.Do(func() {
		if s.DevUser == "" || s.DevPass == "" {
			s.DevUser, s.DevPass = DevID, DevPassword
		}
		s.c = NewClient("ScreenScraper", s.HTTP, 1200*time.Millisecond)
		s.c.Secrets = []string{s.User, s.Password, s.DevUser, s.DevPass}
		if s.Base == "" {
			s.Base = "https://api.screenscraper.fr/api2"
		}
		if s.SoftName == "" {
			s.SoftName = "WittyMediaServer"
		}
	})
}

// About implements Source.
func (s *ScreenScraper) About() About {
	s.init()
	a := About{ID: "screenscraper", Name: "ScreenScraper", Site: "https://www.screenscraper.fr",
		Why:   "Recognises files by checksum across almost every console, with box art, screenshots and scores.",
		Gives: []string{"description", "dates", "developer", "publisher", "genres", "players", "cover", "screenshots", "backdrop", "score", "age ratings"},
		Fields: []KeyField{{Setting: "ssUser", Label: "ScreenScraper user name"}, {Setting: "ssPassword", Label: "ScreenScraper password"},
			{Setting: "ssDevId", Label: "Developer id", Hint: "Only if your copy of the server has none built in"},
			{Setting: "ssDevPassword", Label: "Developer password", Hint: "Only if your copy of the server has none built in"}},
		KeyURL: "https://www.screenscraper.fr/membreinscription.php", Cost: "Free. A donation raises the limits",
		Steps: []string{"Make a free account at screenscraper.fr.", "Type its user name and password below. They are kept on this server and sent only to ScreenScraper.",
			"A copy of the server with no developer id built in also needs one: ask on ScreenScraper's forum."},
		Limit: "About 20,000 requests a day and 1 at a time on a free account. This server asks every 1.2 seconds and stops when told to.",
		Sends: "Game names, console names, file checksums and sizes", Credit: "Data and images from ScreenScraper",
		License: "CC BY-NC-SA 4.0. For free software only; data is kept on this server",
	}
	if s.DevUser == "" {
		a.Unavailable = "This copy of the server has no ScreenScraper developer id, which this source needs. To use your own, ask on ScreenScraper's forum and fill it in below."
	}
	return a
}

// Allowance implements Metered: stay under a free account's daily limit.
func (s *ScreenScraper) Allowance() (int, string) { return 18000, "day" }

// SetMeter implements Metered.
func (s *ScreenScraper) SetMeter(spend func(int) error) { s.init(); s.c.meter = spend }

func (s *ScreenScraper) params() (url.Values, error) {
	s.init()
	if s.DevUser == "" || s.DevPass == "" {
		return nil, ErrNotSet
	}
	q := url.Values{"devid": {s.DevUser}, "devpassword": {s.DevPass}, "softname": {s.SoftName}, "output": {"json"}}
	if s.User != "" {
		q.Set("ssid", s.User)
		q.Set("sspassword", s.Password)
	}
	return q, nil
}

// ask gets an answer, with ScreenScraper's own meanings for its status codes.
func (s *ScreenScraper) ask(ctx context.Context, page string, q url.Values, out any) error {
	base, err := s.params()
	if err != nil {
		return err
	}
	for k, v := range q {
		base[k] = v
	}
	req, err := http.NewRequestWithContext(ctx, "GET", s.Base+"/"+page+"?"+base.Encode(), nil)
	if err != nil {
		return s.c.scrub(err)
	}
	res, err := s.c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case 200:
	case 404:
		return ErrNotFound
	case 401:
		return s.c.gate.pause(10*time.Minute, "closed to non-members for now")
	case 403:
		return ErrKey
	case 423, 426:
		return s.c.gate.pause(time.Hour, res.Status)
	case 429:
		return s.c.gate.pause(time.Minute, res.Status)
	case 430, 431:
		return s.c.gate.pause(6*time.Hour, "today's allowance is used up")
	default:
		return fmt.Errorf("ScreenScraper answered %s", res.Status)
	}
	raw := limitedBody(res)
	if err := json.Unmarshal(raw, out); err != nil {
		// Not JSON: it answers some failures in plain text with a 200.
		if strings.Contains(strings.ToLower(string(raw)), "erreur") {
			return ErrNotFound
		}
		return fmt.Errorf("ScreenScraper sent something that isn't JSON")
	}
	return nil
}

// ssText is one region's or language's version of a field.
type ssText struct {
	Langue string `json:"langue"`
	Region string `json:"region"`
	Text   string `json:"text"`
	Type   string `json:"type"`
}

type ssMedia struct {
	Type   string `json:"type"`
	Region string `json:"region"`
	Parent string `json:"parent"`
}

type ssGame struct {
	ID        json.RawMessage `json:"id"`
	Noms      []ssText        `json:"noms"`
	Synopsis  []ssText        `json:"synopsis"`
	Dates     []ssText        `json:"dates"`
	Editeur   ssText          `json:"editeur"`
	Developpr ssText          `json:"developpeur"`
	Joueurs   ssText          `json:"joueurs"`
	Note      ssText          `json:"note"`
	Classif   []ssText        `json:"classifications"`
	Genres    []struct {
		Noms []ssText `json:"noms"`
	} `json:"genres"`
	Systeme struct {
		ID json.RawMessage `json:"id"`
	} `json:"systeme"`
	Medias []ssMedia `json:"medias"`
}

func rawID(r json.RawMessage) string {
	var n json.Number
	if json.Unmarshal(r, &n) == nil {
		return n.String()
	}
	var t string
	if json.Unmarshal(r, &t) == nil {
		return t
	}
	return ""
}

// pick chooses one region's or language's text: English or the US if there is one.
func pick(list []ssText) string {
	for _, want := range []string{"en", "us", "wor", "eu", "ss", "uk"} {
		for _, t := range list {
			if (t.Langue == want || t.Region == want) && t.Text != "" {
				return t.Text
			}
		}
	}
	if len(list) > 0 {
		return list[0].Text
	}
	return ""
}

// ssPicture makes a reference to one of a game's pictures. It holds no login.
func ssPicture(system, game, media string) string {
	return "ss:" + system + ":" + game + ":" + media
}

// PictureURL implements Referrer. The address it makes holds logins: it is only ever
// used to fetch, on the server.
func (s *ScreenScraper) PictureURL(ref string) (string, error) {
	parts := strings.SplitN(strings.TrimPrefix(ref, "ss:"), ":", 3)
	if len(parts) != 3 {
		return "", errors.New("not a ScreenScraper picture")
	}
	q, err := s.params()
	if err != nil {
		return "", err
	}
	q.Del("output")
	q.Set("systemeid", parts[0])
	q.Set("jeuid", parts[1])
	q.Set("media", parts[2])
	q.Set("outputformat", "png")
	q.Set("maxwidth", "1280")
	return s.Base + "/mediaJeu.php?" + q.Encode(), nil
}

// ssMediaName finds one kind of media, the first region it's listed under.
func ssMediaName(g *ssGame, kind string) string {
	var fallback string
	for _, m := range g.Medias {
		if m.Type != kind || m.Parent != "jeu" && m.Parent != "" {
			continue
		}
		if m.Region == "" {
			return kind
		}
		if m.Region == "us" || m.Region == "wor" {
			return kind + "(" + m.Region + ")"
		}
		if fallback == "" {
			fallback = kind + "(" + m.Region + ")"
		}
	}
	return fallback
}

func (s *ScreenScraper) info(g *ssGame) *Info {
	id := rawID(g.ID)
	sys := rawID(g.Systeme.ID)
	in := &Info{ID: id, Title: Line(pick(g.Noms)), Description: Text(pick(g.Synopsis)), Released: cleanDate(pick(g.Dates)),
		Developer: Line(g.Developpr.Text), Publisher: Line(g.Editeur.Text), URL: "https://www.screenscraper.fr/gameinfos.php?gameid=" + url.QueryEscape(id)}
	var genres []string
	for _, x := range g.Genres {
		genres = append(genres, pick(x.Noms))
	}
	in.Genres = lines(genres, 8)
	// "1-4" and "2" both turn up.
	if p := g.Joueurs.Text; p != "" {
		if i := strings.LastIndexAny(p, "-+"); i >= 0 {
			p = p[i+1:]
		}
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n > 0 && n <= 64 {
			in.Players = n
		}
	}
	if n, err := strconv.ParseFloat(strings.TrimSpace(g.Note.Text), 64); err == nil && n > 0 && n <= 20 {
		in.Scores = append(in.Scores, Score{Source: "ScreenScraper", Kind: "user", Value: n * 5, URL: in.URL})
	}
	for _, c := range g.Classif {
		if r, ok := NewAge(c.Type, c.Text); ok {
			in.Ages = append(in.Ages, r)
		}
	}
	if m := ssMediaName(g, "box-2D"); m != "" && sys != "" {
		in.Poster = ssPicture(sys, id, m)
	}
	if m := ssMediaName(g, "fanart"); m != "" && sys != "" {
		in.Backdrop = ssPicture(sys, id, m)
	}
	if m := ssMediaName(g, "ss"); m != "" && sys != "" {
		in.Art = append(in.Art, ArtRef{Ref: ssPicture(sys, id, m), Kind: "screenshot"})
		if in.Backdrop == "" {
			in.Backdrop = ssPicture(sys, id, m)
		}
	}
	if m := ssMediaName(g, "sstitle"); m != "" && sys != "" {
		in.Art = append(in.Art, ArtRef{Ref: ssPicture(sys, id, m), Kind: "title"})
	}
	return in
}

// Lookup implements Source.
func (s *ScreenScraper) Lookup(ctx context.Context, q Query) (*Info, error) {
	p := platforms[q.Console]
	if len(q.CRCs) > 0 {
		cq := url.Values{"crc": {fmt.Sprintf("%08X", q.CRCs[0])}, "romtype": {"rom"}, "romnom": {q.Title}}
		if q.Size > 0 {
			cq.Set("romtaille", strconv.FormatInt(q.Size, 10))
		}
		if len(p.ss) > 0 {
			cq.Set("systemeid", strconv.Itoa(p.ss[0]))
		}
		var out struct {
			Response struct {
				Jeu *ssGame `json:"jeu"`
			} `json:"response"`
		}
		err := s.ask(ctx, "jeuInfos.php", cq, &out)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if err == nil && out.Response.Jeu != nil && rawID(out.Response.Jeu.ID) != "" {
			in := s.info(out.Response.Jeu)
			in.Method, in.Confidence = MethodChecksum, 100
			return in, nil
		}
	}
	list, err := s.search(ctx, q.Console, q.Title)
	if err != nil {
		return nil, err
	}
	bestI, best := -1, 0
	for i := range list {
		names := make([]string, 0, len(list[i].Noms))
		for _, n := range list[i].Noms {
			names = append(names, n.Text)
		}
		if _, score := Best(q.Title, names); score > best {
			bestI, best = i, score
		}
	}
	if bestI < 0 || best < AcceptScore {
		return nil, ErrNotFound
	}
	in := s.info(&list[bestI])
	in.Method, in.Confidence = MethodName, nameConfidence(best)
	return in, nil
}

func (s *ScreenScraper) search(ctx context.Context, console, title string) ([]ssGame, error) {
	q := url.Values{"recherche": {title}}
	if p := platforms[console]; len(p.ss) > 0 {
		q.Set("systemeid", strconv.Itoa(p.ss[0]))
	}
	var out struct {
		Response struct {
			Jeux []ssGame `json:"jeux"`
		} `json:"response"`
	}
	if err := s.ask(ctx, "jeuRecherche.php", q, &out); err != nil {
		return nil, err
	}
	return out.Response.Jeux, nil
}

// Search implements Source.
func (s *ScreenScraper) Search(ctx context.Context, console, title string) ([]Candidate, error) {
	list, err := s.search(ctx, console, title)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for i := range list {
		g := &list[i]
		name := pick(g.Noms)
		if rawID(g.ID) == "" || name == "" {
			continue
		}
		// No picture: its address would hold logins.
		out = append(out, Candidate{Source: "screenscraper", ID: rawID(g.ID), Title: Line(name), Year: firstYear(pick(g.Dates)), Score: NameScore(title, name)})
	}
	return out, nil
}

// Fetch implements Source.
func (s *ScreenScraper) Fetch(ctx context.Context, console, id string) (*Info, error) {
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return nil, ErrNotFound
	}
	var out struct {
		Response struct {
			Jeu *ssGame `json:"jeu"`
		} `json:"response"`
	}
	if err := s.ask(ctx, "jeuInfos.php", url.Values{"gameid": {id}}, &out); err != nil {
		return nil, err
	}
	if out.Response.Jeu == nil {
		return nil, ErrNotFound
	}
	in := s.info(out.Response.Jeu)
	in.Method, in.Confidence = MethodManual, 100
	return in, nil
}

// Test implements Source.
func (s *ScreenScraper) Test(ctx context.Context) (string, error) {
	var out struct {
		Response struct {
			User struct {
				MaxPerDay json.RawMessage `json:"maxrequestsperday"`
			} `json:"ssuser"`
		} `json:"response"`
	}
	page := "ssinfraInfos.php"
	if s.User != "" {
		page = "ssuserInfos.php"
	}
	if err := s.ask(ctx, page, nil, &out); err != nil {
		return "", err
	}
	if n := rawID(out.Response.User.MaxPerDay); n != "" {
		return "ScreenScraper accepted these details. Your account may make " + n + " requests a day.", nil
	}
	return "ScreenScraper accepted these details.", nil
}
