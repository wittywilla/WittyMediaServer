package channels

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Suggestion is a channel an owner can add with one click: a complete
// definition, ready to save or to open in the editor first.
type Suggestion struct {
	Key         string     `json:"key"`
	Group       string     `json:"group"` // network | era | genre | show | classic
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Definition  Definition `json:"definition"`
	Stats       PoolStats  `json:"stats"`
	Exists      bool       `json:"exists"` // a channel of this name exists
}

// NetworkFact is a network in the library, with what the data can support.
type NetworkFact struct {
	Name     string `json:"name"`
	Shows    int    `json:"shows"`
	Episodes int    `json:"episodes"`
	Slots    int    `json:"slots"` // shows that know the day and time they aired
	FromYear int    `json:"fromYear"`
	ToYear   int    `json:"toYear"`
	Decades  []int  `json:"decades"`
}

// Networks lists the networks of the library's shows, with one spelling per
// network however the metadata wrote it.
func (s *Store) Networks(ctx context.Context) ([]NetworkFact, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT s.network, s.id, COUNT(i.id), MAX(IFNULL(s.air_time, '') <> ''),
		MIN(`+yearExpr+`), MAX(`+yearExpr+`)
		FROM shows s JOIN items i ON i.show_id = s.id AND i.missing = 0
		WHERE s.missing = 0 AND IFNULL(s.network, '') <> '' GROUP BY s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	by := map[string]*NetworkFact{}
	for rows.Next() {
		var name string
		var id int64
		var eps, slot int
		var y0, y1 sql.NullInt64
		if err := rows.Scan(&name, &id, &eps, &slot, &y0, &y1); err != nil {
			return nil, err
		}
		k := NormalizeNetwork(name)
		n := by[k]
		if n == nil {
			n = &NetworkFact{Name: name, Decades: []int{}}
			by[k] = n
		}
		n.Shows++
		n.Episodes += eps
		n.Slots += slot
		if y0.Valid && (n.FromYear == 0 || int(y0.Int64) < n.FromYear) {
			n.FromYear = int(y0.Int64)
		}
		if y1.Valid && int(y1.Int64) > n.ToYear {
			n.ToYear = int(y1.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]NetworkFact, 0, len(by))
	for _, n := range by {
		if n.FromYear > 0 {
			for d := n.FromYear / 10 * 10; d <= n.ToYear; d += 10 {
				n.Decades = append(n.Decades, d)
			}
		}
		out = append(out, *n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Episodes != out[j].Episodes {
			return out[i].Episodes > out[j].Episodes
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Network channel styles.
const (
	StyleAllDay   = "allday"
	StyleDayParts = "dayparts"
	StyleAsAired  = "asaired"
)

// dayParts are the classic parts of a broadcast day and the genres each was
// known for. A part is used only when the library has enough to fill it.
var dayParts = []struct {
	Name       string
	Start, End string
	Genres     []string
}{
	{"Morning", "06:00", "12:00", []string{"Children", "Kids", "Family", "Animation", "Talk Show", "Talk", "News"}},
	{"Afternoon", "12:00", "17:00", []string{"Soap", "Game Show", "Reality", "Talk Show", "Talk", "Drama"}},
	{"Early evening", "17:00", "20:00", []string{"Comedy", "Family", "Animation", "Game Show"}},
	{"Prime time", "20:00", "23:00", []string{"Drama", "Crime", "Action", "Thriller", "Mystery", "Science-Fiction", "Science Fiction", "Comedy"}},
	{"Late night", "23:00", "02:00", []string{"Horror", "Comedy", "Talk Show", "Talk", "Thriller", "Supernatural"}},
}

// BuildNetwork makes the definition for a network channel in one of the
// styles, limited to some years when asked. It returns what fits the data:
// a style the library cannot support falls back to all day.
func (s *Store) BuildNetwork(ctx context.Context, network, style string, yearFrom, yearTo int) (Definition, error) {
	base := Rules{Networks: []string{network}, YearFrom: yearFrom, YearTo: yearTo}
	def := Definition{Mode: "simple", Rules: base, Order: Order{Mode: "rotate"}, AlignMin: 30}
	switch style {
	case StyleDayParts:
		for _, dp := range dayParts {
			r := base
			r.Genres = dp.Genres
			st, err := s.Stats(ctx, r)
			if err != nil {
				return def, err
			}
			if st.Titles < 3 {
				continue
			}
			def.Blocks = append(def.Blocks, Block{ID: blockID(dp.Name), Name: dp.Name, Start: dp.Start, End: dp.End, Rules: &r})
		}
		if len(def.Blocks) > 0 {
			def.Mode = "rules"
		}
	case StyleAsAired:
		blocks, err := s.asAiredBlocks(ctx, network)
		if err != nil {
			return def, err
		}
		if len(blocks) > 0 {
			def.Mode, def.Blocks = "timetable", blocks
		}
	}
	return def, nil
}

var slotTime = regexp.MustCompile(`^([01]?[0-9]|2[0-3]):([0-5][0-9])$`)

var weekdayNumber = map[string]int{"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3, "thursday": 4, "friday": 5, "saturday": 6}

// asAiredBlocks places each show in the day and time it aired. Shows that
// shared a slot take turns in it.
func (s *Store) asAiredBlocks(ctx context.Context, network string) ([]Block, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT s.id, s.air_days, s.air_time,
		(SELECT AVG(i.runtime_sec) FROM items i WHERE i.show_id = s.id AND i.missing = 0)
		FROM shows s WHERE s.missing = 0 AND s.network = ? COLLATE NOCASE AND IFNULL(s.air_time, '') <> '' AND IFNULL(s.air_days, '') <> ''
		AND EXISTS (SELECT 1 FROM items i WHERE i.show_id = s.id AND i.missing = 0)
		ORDER BY s.sort_title COLLATE NOCASE`, network)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type slot struct {
		days  []int
		start string
		mins  int
		shows []int64
	}
	slots := map[string]*slot{}
	var keys []string
	for rows.Next() {
		var id int64
		var days, at string
		var avg sql.NullFloat64
		if err := rows.Scan(&id, &days, &at, &avg); err != nil {
			return nil, err
		}
		m := slotTime.FindStringSubmatch(at)
		if m == nil {
			continue
		}
		h, _ := strconv.Atoi(m[1])
		start := fmt.Sprintf("%02d:%s", h, m[2])
		var nums []int
		for _, d := range strings.Split(days, "|") {
			if n, ok := weekdayNumber[strings.ToLower(strings.TrimSpace(d))]; ok {
				nums = append(nums, n)
			}
		}
		if len(nums) == 0 {
			continue
		}
		sort.Ints(nums)
		// A half-hour show keeps a half-hour slot, an hour show an hour.
		mins := 30
		if avg.Valid {
			mins = int(avg.Float64/60/30+0.999) * 30
		}
		mins = max(30, min(mins, 240))
		k := fmt.Sprint(nums, start)
		sl := slots[k]
		if sl == nil {
			sl = &slot{days: nums, start: start}
			slots[k] = sl
			keys = append(keys, k)
		}
		sl.mins = max(sl.mins, mins)
		sl.shows = append(sl.shows, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(keys)
	out := []Block{}
	for i, k := range keys {
		sl := slots[k]
		end := (minutes(sl.start) + sl.mins) % (24 * 60)
		out = append(out, Block{ID: "slot" + strconv.Itoa(i+1), Days: sl.days, Start: sl.start, End: fmt.Sprintf("%02d:%02d", end/60, end%60),
			Rules: &Rules{Shows: sl.shows}, Order: &Order{Mode: "rotate"}})
	}
	return out, nil
}

func blockID(name string) string { return strings.ReplaceAll(strings.ToLower(name), " ", "-") }

// Suggest lists channels the library can fill, best first. Nothing here needs
// an owner to know how the editor works.
func (s *Store) Suggest(ctx context.Context) ([]Suggestion, error) {
	var out []Suggestion
	add := func(key, group, name, desc string, def Definition) error {
		st, err := s.Stats(ctx, def.Rules)
		if err != nil {
			return err
		}
		out = append(out, Suggestion{Key: key, Group: group, Name: name, Description: desc, Definition: def, Stats: st})
		return nil
	}
	nets, err := s.Networks(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range nets {
		if n.Episodes < 6 {
			continue
		}
		def, _ := s.BuildNetwork(ctx, n.Name, StyleAllDay, 0, 0)
		desc := fmt.Sprintf("%d %s that aired on %s, all day", n.Shows, plural(n.Shows, "show", "shows"), n.Name)
		if err := add("network:"+n.Name, "network", n.Name, desc, def); err != nil {
			return nil, err
		}
		for _, d := range n.Decades {
			if len(n.Decades) < 2 {
				break
			}
			def, _ := s.BuildNetwork(ctx, n.Name, StyleAllDay, d, d+9)
			st, err := s.Stats(ctx, def.Rules)
			if err != nil {
				return nil, err
			}
			if st.Titles < 6 {
				continue
			}
			if err := add(fmt.Sprintf("network:%s:%d", n.Name, d), "network", fmt.Sprintf("%s %ds", n.Name, d), fmt.Sprintf("What aired on %s in the %ds", n.Name, d), def); err != nil {
				return nil, err
			}
		}
		if n.Shows >= 3 {
			if def, _ := s.BuildNetwork(ctx, n.Name, StyleDayParts, 0, 0); def.Mode == "rules" && len(def.Blocks) >= 2 {
				if err := add("network:"+n.Name+":dayparts", "network", n.Name+" through the day", "Morning, afternoon, early evening, prime time and late night, by genre", def); err != nil {
					return nil, err
				}
			}
		}
		if n.Slots >= 2 {
			if def, _ := s.BuildNetwork(ctx, n.Name, StyleAsAired, 0, 0); def.Mode == "timetable" {
				if err := add("network:"+n.Name+":asaired", "network", n.Name+" as it aired", "Each show in the day and time it aired, where the metadata knows it", def); err != nil {
					return nil, err
				}
			}
		}
	}
	// Decades of everything.
	for d := 1950; d <= 2030; d += 10 {
		def := Definition{Mode: "simple", Rules: Rules{Decades: []int{d}}, Order: Order{Mode: "rotate"}, AlignMin: 30}
		st, err := s.Stats(ctx, def.Rules)
		if err != nil {
			return nil, err
		}
		if st.Titles >= 20 {
			if err := add(fmt.Sprintf("era:%d", d), "era", fmt.Sprintf("The %ds", d), fmt.Sprintf("Everything from the %ds", d), def); err != nil {
				return nil, err
			}
		}
	}
	// Genres, biggest first.
	genres, err := s.genreCounts(ctx)
	if err != nil {
		return nil, err
	}
	for i, g := range genres {
		if i >= 12 || g.n < 15 {
			break
		}
		def := Definition{Mode: "simple", Rules: Rules{Genres: []string{g.name}}, Order: Order{Mode: "rotate"}, AlignMin: 30}
		if err := add("genre:"+g.name, "genre", g.name, g.name+" from every library", def); err != nil {
			return nil, err
		}
	}
	// Films, with evening starts on the half hour.
	films := Definition{Mode: "simple", Rules: Rules{Kinds: []string{"movies"}}, Order: Order{Mode: "shuffle"}, AlignMin: 30}
	if st, err := s.Stats(ctx, films.Rules); err == nil && st.Titles >= 5 {
		if err := add("classic:movies", "classic", "Movie Channel", "Your films, shuffled, starting on the half hour", films); err != nil {
			return nil, err
		}
	}
	// Children's morning at the weekend.
	age := 7
	cartoon := Rules{Genres: []string{"Animation", "Children", "Kids"}, MaxAge: &age, AllowUnrated: true}
	if st, err := s.Stats(ctx, cartoon); err == nil && st.Titles >= 10 {
		def := Definition{Mode: "rules", Rules: cartoon, Order: Order{Mode: "rotate"}, AlignMin: 30, Blocks: []Block{
			{ID: "saturday-morning", Name: "Saturday morning", Days: []int{6}, Start: "07:00", End: "12:00", Rules: &cartoon},
		}}
		if err := add("classic:saturday", "classic", "Saturday Morning", "Cartoons all day, with the big block on Saturday morning", def); err != nil {
			return nil, err
		}
	}
	// Marathons of long-running shows.
	marathons, err := s.longShows(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range marathons {
		def := Definition{Mode: "simple", Rules: Rules{Shows: []int64{m.id}}, Order: Order{Mode: "sequential"}}
		if err := add(fmt.Sprintf("show:%d", m.id), "show", m.title+" Marathon", fmt.Sprintf("%d episodes, in order", m.n), def); err != nil {
			return nil, err
		}
	}
	existing, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, c := range existing {
		names[strings.ToLower(c.Name)] = true
	}
	for i := range out {
		out[i].Exists = names[strings.ToLower(out[i].Name)]
	}
	return out, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

type genreCount struct {
	name string
	n    int
}

func (s *Store) genreCounts(ctx context.Context) ([]genreCount, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT COALESCE(NULLIF(i.genres, ''), s.genres, '') FROM items i LEFT JOIN shows s ON s.id = i.show_id WHERE `+notCommercial+` AND `+playable)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	by := map[string]int{}
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		for _, x := range strings.Split(g, "|") {
			if x = strings.TrimSpace(x); x != "" {
				by[x]++
			}
		}
	}
	out := make([]genreCount, 0, len(by))
	for k, v := range by {
		out = append(out, genreCount{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].name < out[j].name
	})
	return out, rows.Err()
}

type longShow struct {
	id    int64
	title string
	n     int
}

func (s *Store) longShows(ctx context.Context) ([]longShow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT s.id, s.title, COUNT(*) AS n FROM shows s JOIN items i ON i.show_id = s.id AND i.missing = 0
		WHERE s.missing = 0 GROUP BY s.id HAVING n >= 40 ORDER BY n DESC LIMIT 12`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []longShow
	for rows.Next() {
		var m longShow
		if err := rows.Scan(&m.id, &m.title, &m.n); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Options is what the editor offers to choose from.
type Options struct {
	Networks []NetworkFact `json:"networks"`
	Genres   []string      `json:"genres"`
	Studios  []string      `json:"studios"`
	Ratings  []string      `json:"ratings"`
	Decades  []int         `json:"decades"`
	Shows    []ShowOption  `json:"shows"`
}

// ShowOption is a show in the editor's pickers.
type ShowOption struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Year    int    `json:"year,omitempty"`
	Network string `json:"network,omitempty"`
	Seasons int    `json:"seasons"`
}

// EditorOptions gathers the choices for the rule editor.
func (s *Store) EditorOptions(ctx context.Context) (*Options, error) {
	o := &Options{Genres: []string{}, Studios: []string{}, Ratings: []string{}, Decades: []int{}, Shows: []ShowOption{}}
	var err error
	if o.Networks, err = s.Networks(ctx); err != nil {
		return nil, err
	}
	gc, err := s.genreCounts(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range gc {
		o.Genres = append(o.Genres, g.name)
	}
	sort.Strings(o.Genres)
	if o.Studios, err = s.strings(ctx, `SELECT DISTINCT i.studio FROM items i WHERE IFNULL(i.studio, '') <> '' AND i.show_id IS NULL AND i.missing = 0 AND `+notCommercial+` ORDER BY i.studio COLLATE NOCASE LIMIT 500`); err != nil {
		return nil, err
	}
	if o.Ratings, err = s.strings(ctx, `SELECT DISTINCT COALESCE(NULLIF(i.content_rating, ''), s.content_rating) AS r FROM items i LEFT JOIN shows s ON s.id = i.show_id
		WHERE r IS NOT NULL AND r <> '' AND `+notCommercial+` AND `+playable+` ORDER BY r LIMIT 200`); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT `+yearExpr+` / 10 * 10 AS d FROM items i LEFT JOIN shows s ON s.id = i.show_id WHERE d IS NOT NULL AND d > 1800 AND `+notCommercial+` AND `+playable+` ORDER BY d`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d int
		if rows.Scan(&d) == nil {
			o.Decades = append(o.Decades, d)
		}
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT s.id, s.title, IFNULL(s.year, 0), IFNULL(s.network, ''), (SELECT COUNT(DISTINCT i.season) FROM items i WHERE i.show_id = s.id AND i.missing = 0)
		FROM shows s WHERE s.missing = 0 ORDER BY s.sort_title COLLATE NOCASE LIMIT 5000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sh ShowOption
		if err := rows.Scan(&sh.ID, &sh.Title, &sh.Year, &sh.Network, &sh.Seasons); err != nil {
			return nil, err
		}
		if sh.Seasons > 0 {
			o.Shows = append(o.Shows, sh)
		}
	}
	return o, rows.Err()
}

func (s *Store) strings(ctx context.Context, q string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
