package library

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
)

// Which sources are in use: built from the server's settings, and rebuilt when they
// change. Keys live in the settings table like every other secret: masked by the API,
// and never logged.

// GameSourceSetting is one source's place in the owner's list.
type GameSourceSetting struct {
	ID string `json:"id"`
	On bool   `json:"on"`
}

// GameSourceKeys are the settings that hold what sources need. They are secrets.
var GameSourceKeys = []string{"igdbClientId", "igdbClientSecret", "rawgKey", "tgdbKey", "ssUser", "ssPassword", "ssDevId", "ssDevPassword"}

// defaultSourceOrder is who's believed first until the owner says otherwise. The best
// data first; Wikidata last, as it often has plain summaries when nothing else does.
var defaultSourceOrder = []string{"igdb", "screenscraper", "libretro", "libretroart", "wikidata", "rawg", "tgdb"}

// GameSourceOrder reads the owner's list, with any source it doesn't mention added at
// the end, switched on.
func GameSourceOrder(raw, wikidataSetting string) []GameSourceSetting {
	var saved []GameSourceSetting
	_ = json.Unmarshal([]byte(raw), &saved)
	seen := map[string]bool{}
	out := make([]GameSourceSetting, 0, len(defaultSourceOrder))
	for _, s := range saved {
		if slices.Contains(defaultSourceOrder, s.ID) && !seen[s.ID] {
			seen[s.ID] = true
			out = append(out, s)
		}
	}
	for _, id := range defaultSourceOrder {
		if !seen[id] {
			// Someone who turned Wikidata off for films hasn't asked for it to be asked about games.
			on := !(id == "wikidata" && wikidataSetting == "0")
			out = append(out, GameSourceSetting{ID: id, On: on})
		}
	}
	return out
}

func (g *GameMeta) setting(ctx context.Context, key string) string {
	return g.Store.DB.GetSetting(ctx, key, "")
}

// sources builds the source list, reusing the last one if nothing has changed.
func (g *GameMeta) sources(ctx context.Context) *sourceSet {
	var sig strings.Builder
	settings := map[string]string{}
	for _, k := range GameSourceKeys {
		v := g.setting(ctx, k)
		settings[k] = v
		sig.WriteString(k + "=" + v + "\n")
	}
	order := GameSourceOrder(g.setting(ctx, "gameSources"), g.Store.DB.GetSetting(ctx, "wikidata", "1"))
	for _, o := range order {
		sig.WriteString(o.ID)
		if o.On {
			sig.WriteString("+")
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.set != nil && g.set.signature == sig.String() {
		return g.set
	}
	set := &sourceSet{signature: sig.String(), all: map[string]gamemeta.Source{}}
	if g.Built != nil {
		for _, s := range g.Built() {
			set.all[s.About().ID] = s
		}
	} else {
		for _, s := range g.build(settings) {
			set.all[s.About().ID] = s
		}
	}
	for _, o := range order {
		src := set.all[o.ID]
		if src == nil {
			continue
		}
		set.order = append(set.order, o.ID)
		if o.On && usable(src, settings) {
			set.on = append(set.on, o.ID)
		}
	}
	for _, src := range set.all {
		if m, ok := src.(gamemeta.Metered); ok {
			m.SetMeter(g.meterFor(src.About().ID, m))
		}
	}
	g.set = set
	return set
}

// usable: everything the source needs has been given.
func usable(src gamemeta.Source, settings map[string]string) bool {
	about := src.About()
	if about.Unavailable != "" {
		return false
	}
	for _, f := range about.Fields {
		if settings[f.Setting] == "" && f.Hint == "" {
			return false
		}
	}
	return true
}

func (g *GameMeta) build(s map[string]string) []gamemeta.Source {
	client := g.HTTP
	if client == nil {
		client = catalogClient(45 * time.Second)
	}
	cache := filepath.Join(g.CacheDir, "gamemeta")
	db := &gamemeta.Libretro{HTTP: client, Dir: filepath.Join(cache, "libretro")}
	return []gamemeta.Source{
		db,
		&gamemeta.LibretroArt{DB: db},
		&gamemeta.WikidataSource{HTTP: client, Dir: filepath.Join(cache, "wikidata")},
		&gamemeta.IGDB{ClientID: s["igdbClientId"], Secret: s["igdbClientSecret"], HTTP: client},
		&gamemeta.ScreenScraper{User: s["ssUser"], Password: s["ssPassword"], DevUser: s["ssDevId"], DevPass: s["ssDevPassword"], HTTP: client},
		&gamemeta.RAWG{Key: s["rawgKey"], HTTP: client},
		&gamemeta.TGDB{Key: s["tgdbKey"], HTTP: client},
	}
}

// Requests sources with an allowance have spent are counted in the settings table,
// so a restart doesn't forget them.

type meterState struct {
	Period string `json:"period"`
	Used   int    `json:"used"`
}

var meterMu sync.Mutex

func periodKey(period string, now time.Time) string {
	if period == "day" {
		return now.Format("2006-01-02")
	}
	return now.Format("2006-01")
}

func periodEnd(period string, now time.Time) time.Time {
	if period == "day" {
		return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	}
	return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
}

func (g *GameMeta) meterFor(id string, m gamemeta.Metered) func(int) error {
	limit, period := m.Allowance()
	key := "gameSourceUse." + id
	return func(n int) error {
		if limit <= 0 {
			return nil
		}
		meterMu.Lock()
		defer meterMu.Unlock()
		ctx := context.Background()
		now := time.Now()
		st := meterState{Period: periodKey(period, now)}
		var saved meterState
		if json.Unmarshal([]byte(g.Store.DB.GetSetting(ctx, key, "")), &saved) == nil && saved.Period == st.Period {
			st.Used = saved.Used
		}
		if st.Used+n > limit {
			return &gamemeta.RateLimited{Until: periodEnd(period, now), Why: "this " + period + "'s allowance is used up"}
		}
		st.Used += n
		_ = g.Store.DB.SetSetting(ctx, key, toJSON(st))
		return nil
	}
}

// GameSourceView is one source as the setup wizard, Settings and the apps show it.
type GameSourceView struct {
	gamemeta.About
	On bool `json:"on"`
	// Ready: it has everything it needs, so being on means it's in use.
	Ready bool `json:"ready"`
	// Saved lists which of its fields have a value. The values never leave the server.
	Saved []string `json:"saved"`
	// Used and Allowance: requests spent this period, and how many it may spend.
	Used      int    `json:"used,omitempty"`
	Allowance int    `json:"allowance,omitempty"`
	Period    string `json:"period,omitempty"`
}

// SourceViews lists every source, in the owner's order.
func (g *GameMeta) SourceViews(ctx context.Context) []GameSourceView {
	set := g.sources(ctx)
	settings := map[string]string{}
	for _, k := range GameSourceKeys {
		settings[k] = g.setting(ctx, k)
	}
	on := map[string]bool{}
	for _, o := range GameSourceOrder(g.setting(ctx, "gameSources"), g.Store.DB.GetSetting(ctx, "wikidata", "1")) {
		on[o.ID] = o.On
	}
	out := make([]GameSourceView, 0, len(set.order))
	for _, id := range set.order {
		src := set.all[id]
		v := GameSourceView{About: src.About(), On: on[id], Ready: usable(src, settings), Saved: []string{}}
		for _, f := range v.Fields {
			if settings[f.Setting] != "" {
				v.Saved = append(v.Saved, f.Setting)
			}
		}
		if m, ok := src.(gamemeta.Metered); ok {
			v.Allowance, v.Period = m.Allowance()
			var st meterState
			if json.Unmarshal([]byte(g.Store.DB.GetSetting(ctx, "gameSourceUse."+id, "")), &st) == nil && st.Period == periodKey(v.Period, time.Now()) {
				v.Used = st.Used
			}
		}
		out = append(out, v)
	}
	return out
}

// SetSourceOrder saves the owner's list. Answers are kept, so a new order only needs
// them merged again; whatever a newly switched-on source can add, it's asked.
func (g *GameMeta) SetSourceOrder(ctx context.Context, list []GameSourceSetting) error {
	clean := GameSourceOrder(toJSON(list), g.Store.DB.GetSetting(ctx, "wikidata", "1"))
	if err := g.Store.DB.SetSetting(ctx, "gameSources", toJSON(clean)); err != nil {
		return err
	}
	return g.RequeueAll(ctx)
}

// KeysChanged is told when a source's key was saved, so games can be asked about again.
func (g *GameMeta) KeysChanged(ctx context.Context) {
	_ = g.RequeueAll(ctx)
}

// TestSource tries a source with these details, or the saved ones where a detail is
// left out, without saving anything. The answer is safe to show.
func (g *GameMeta) TestSource(ctx context.Context, id string, fields map[string]string) (string, error) {
	settings := map[string]string{}
	for _, k := range GameSourceKeys {
		settings[k] = g.setting(ctx, k)
	}
	for k, v := range fields {
		if slices.Contains(GameSourceKeys, k) && strings.TrimSpace(v) != "" && !strings.Contains(v, "•") {
			settings[k] = strings.TrimSpace(v)
		}
	}
	var src gamemeta.Source
	if g.Built != nil {
		for _, s := range g.Built() {
			if s.About().ID == id {
				src = s
			}
		}
	} else {
		for _, s := range g.build(settings) {
			if s.About().ID == id {
				src = s
			}
		}
	}
	if src == nil {
		return "", ErrNotFound
	}
	for _, f := range src.About().Fields {
		if settings[f.Setting] == "" && f.Hint == "" {
			return "", errors.New("Fill in " + f.Label + " first.")
		}
	}
	msg, err := src.Test(ctx)
	switch {
	case err == nil:
		return msg, nil
	case errors.Is(err, gamemeta.ErrKey):
		return "", errors.New("The service didn't accept these details. Check that you copied all of them.")
	case errors.Is(err, gamemeta.ErrNotSet):
		return "", errors.New("This source needs more details before it can be tried.")
	case errors.Is(err, context.DeadlineExceeded):
		return "", errors.New("No answer from the service. Is this server online?")
	}
	return "", err
}

// SourceAbout is every source's description, built with no secrets, for places that
// list them without using them: the credits, the docs.
func SourceAbout() []gamemeta.About {
	g := &GameMeta{}
	var out []gamemeta.About
	for _, s := range g.build(map[string]string{}) {
		out = append(out, s.About())
	}
	return out
}
