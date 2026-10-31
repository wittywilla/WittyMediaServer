package gamemeta

import "strings"

// Putting what several sources said into one answer. Sources are in the owner's order
// and the first with a value wins, except for scores, age ratings and pictures, where
// there's room for everyone.

// Answer is one source's answer, with what the server needs to know about the source.
type Answer struct {
	Source string
	Info   *Info
}

// Credit is shown wherever a source's data is, as its terms ask.
type Credit struct {
	Source  string `json:"source"`
	Name    string `json:"name"`
	Site    string `json:"site"`
	Text    string `json:"text"`
	License string `json:"license"`
	URL     string `json:"url,omitempty"` // this game's page there
}

// Merged is the result of merging.
type Merged struct {
	Title, Tags, Description, Released, Developer, Publisher, Franchise string
	Genres                                                              []string
	Players                                                             int
	Scores                                                              []Score
	Ages                                                                []AgeRating
	// Posters and Backdrops are every candidate, best first: if one can't be fetched
	// the next is tried.
	Posters, Backdrops []string
	Art                []ArtRef
	MatchSource        string
	MatchID            string
	MatchMethod        string
	Confidence         int
	// Used lists the sources whose data ended up in this.
	Used []string
}

// MaxArt is how many extra pictures a game keeps track of. They're only fetched when
// someone looks.
const MaxArt = 12

// AcceptConfidence is the least sureness for a source's answer to be used at all.
const AcceptConfidence = 76

// RenameConfidence is the least sureness for a source's name to replace the file's own.
// A close match may be a hack or a variant of the game it's close to, which still gets its
// details, but is called what its file calls it.
const RenameConfidence = 90

// Rating is what to sort by, out of 10: critics if any, otherwise users.
func (m *Merged) Rating() *float64 {
	for _, kind := range []string{"critic", "user"} {
		var sum float64
		n := 0
		for _, s := range m.Scores {
			if s.Kind == kind {
				sum += s.Value
				n++
			}
		}
		if n > 0 {
			v := float64(int(sum/float64(n)+0.5)) / 10
			return &v
		}
	}
	return nil
}

// ContentRating is the rating kept for parental controls, as "ESRB T".
func (m *Merged) ContentRating() string {
	if a := Strictest(m.Ages); a != nil {
		return a.System + " " + a.Value
	}
	return ""
}

// Merge puts answers together. A manual choice comes first whatever the order is.
func Merge(answers []Answer) *Merged {
	m := &Merged{}
	used := map[string]bool{}
	ordered := make([]Answer, 0, len(answers))
	for _, a := range answers {
		if a.Info != nil && a.Info.Method == MethodManual {
			ordered = append(ordered, a)
		}
	}
	for _, a := range answers {
		if a.Info != nil && a.Info.Method != MethodManual && a.Info.Confidence >= AcceptConfidence {
			ordered = append(ordered, a)
		}
	}
	take := func(dst *string, v, source string) {
		if *dst == "" && v != "" {
			*dst = v
			used[source] = true
		}
	}
	seenScore, seenAge, seenArt := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, a := range ordered {
		in, src := a.Info, a.Source
		if in.Confidence > m.Confidence {
			m.MatchSource, m.MatchID, m.MatchMethod, m.Confidence = src, in.ID, in.Method, in.Confidence
		}
		if in.Confidence >= RenameConfidence {
			take(&m.Title, in.Title, src)
			take(&m.Tags, in.Tags, src)
		}
		take(&m.Description, in.Description, src)
		take(&m.Released, in.Released, src)
		take(&m.Developer, in.Developer, src)
		take(&m.Publisher, in.Publisher, src)
		take(&m.Franchise, in.Franchise, src)
		if len(m.Genres) == 0 && len(in.Genres) > 0 {
			m.Genres = in.Genres
			used[src] = true
		}
		if m.Players == 0 && in.Players > 0 {
			m.Players = in.Players
			used[src] = true
		}
		for _, s := range in.Scores {
			if k := strings.ToLower(s.Source + "|" + s.Kind); !seenScore[k] {
				seenScore[k] = true
				m.Scores = append(m.Scores, s)
				used[src] = true
			}
		}
		for _, x := range in.Ages {
			if !seenAge[x.System] {
				seenAge[x.System] = true
				m.Ages = append(m.Ages, x)
				used[src] = true
			}
		}
		if in.Poster != "" {
			m.Posters = append(append(m.Posters, in.Poster), in.MorePosters...)
			used[src] = true
		}
		if in.Backdrop != "" {
			m.Backdrops = append(append(m.Backdrops, in.Backdrop), in.MoreBackdrops...)
			used[src] = true
		}
		for _, x := range in.Art {
			if len(m.Art) < MaxArt && !seenArt[x.Ref] {
				seenArt[x.Ref] = true
				m.Art = append(m.Art, x)
				used[src] = true
			}
		}
	}
	for _, a := range answers {
		if used[a.Source] {
			m.Used = append(m.Used, a.Source)
		}
	}
	return m
}

// Gaps says whether anything worth asking another source for is still missing.
func (m *Merged) Gaps() bool {
	return m.Description == "" || len(m.Posters) == 0 || len(m.Backdrops) == 0 || len(m.Ages) == 0 || len(m.Scores) == 0 || len(m.Genres) == 0 || m.Developer == ""
}
