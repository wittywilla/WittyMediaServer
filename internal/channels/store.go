package channels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Channel is one library channel.
type Channel struct {
	ID            int64      `json:"id"`
	Number        string     `json:"number"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Enabled       bool       `json:"enabled"`
	Definition    Definition `json:"definition"`
	Seed          uint64     `json:"seed,string"`
	Rev           int        `json:"rev"`
	AnchorDay     int        `json:"-"`
	Logo          string     `json:"logo"` // version of the owner's logo, '' = none
	Visibility    Visibility `json:"visibility"`
	CountProgress bool       `json:"countProgress"`
	// Categories are the owner's own filters in the guide. None chosen: the
	// guide works some out (see AutoCategories).
	Categories []string `json:"categories"`
	Look       Look     `json:"look"`
	CreatedAt  int64    `json:"createdAt"`
	UpdatedAt  int64    `json:"updatedAt"`
}

// Store reads and writes channels and their days.
type Store struct {
	DB  *db.DB
	Loc *time.Location // the guide's time zone; nil = the server's
	mu  sync.Mutex     // one schedule generated at a time; each takes milliseconds
}

// ErrNotFound: no such channel.
var ErrNotFound = errors.New("channel not found")

func (s *Store) loc() *time.Location {
	if s.Loc != nil {
		return s.Loc
	}
	return time.Local
}

// Look is how one channel stands out in the guide and on its banner.
type Look struct {
	Accent string `json:"accent,omitempty"` // #rrggbb
	// LogoKind and LogoName choose its logo from the logo service. Empty: it is
	// worked out from what the channel plays. LogoKind "none": never a logo.
	LogoKind string `json:"logoKind,omitempty"` // network | studio | none
	LogoName string `json:"logoName,omitempty"`
}

// Brand is the name a channel's logo goes by, in the logo service: chosen by the
// owner, or else the one network or studio that's all it plays.
func (c *Channel) Brand() (kind, name string) {
	switch {
	case c.Look.LogoKind == "none":
		return "", ""
	case c.Look.LogoName != "":
		return c.Look.LogoKind, c.Look.LogoName
	}
	r := &c.Definition.Rules
	switch {
	case len(r.Networks) == 1 && len(r.Studios) == 0:
		return "network", r.Networks[0]
	case len(r.Studios) == 1 && len(r.Networks) == 0:
		return "studio", r.Studios[0]
	}
	return "", ""
}

const channelCols = `id, number, name, description, enabled, definition, seed, rev, anchor_day, logo, visibility, count_progress, categories, look, created_at, updated_at`

func scanChannel(sc interface{ Scan(...any) error }) (*Channel, error) {
	var c Channel
	var def, vis, cats, look string
	var seed int64
	var enabled, count int
	if err := sc.Scan(&c.ID, &c.Number, &c.Name, &c.Description, &enabled, &def, &seed, &c.Rev, &c.AnchorDay, &c.Logo, &vis, &count, &cats, &look, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.Seed = uint64(seed)
	c.Enabled, c.CountProgress = enabled != 0, count != 0
	if err := json.Unmarshal([]byte(def), &c.Definition); err != nil {
		return nil, err
	}
	if vis != "" {
		_ = json.Unmarshal([]byte(vis), &c.Visibility)
	}
	c.Categories = []string{}
	if cats != "" {
		_ = json.Unmarshal([]byte(cats), &c.Categories)
	}
	if look != "" {
		_ = json.Unmarshal([]byte(look), &c.Look)
	}
	return &c, nil
}

// List returns every channel in number order.
func (s *Store) List(ctx context.Context) ([]*Channel, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+channelCols+` FROM channels`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Channel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return numberLess(out[i].Number, out[j].Number) })
	return out, rows.Err()
}

// numberLess orders channel numbers the way a viewer expects: 5, 5.1, 12, 101.
func numberLess(a, b string) bool {
	na, ea := strconv.ParseFloat(a, 64)
	nb, eb := strconv.ParseFloat(b, 64)
	switch {
	case ea == nil && eb == nil:
		return na < nb
	case ea == nil:
		return true
	case eb == nil:
		return false
	}
	return a < b
}

// NumberLess is the guide's ordering of channel numbers.
func NumberLess(a, b string) bool { return numberLess(a, b) }

// Get returns one channel.
func (s *Store) Get(ctx context.Context, id int64) (*Channel, error) {
	c, err := scanChannel(s.DB.QueryRowContext(ctx, `SELECT `+channelCols+` FROM channels WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// NextNumber is the first free number from 101 up: library channels sit
// together, above the over-the-air numbers.
func (s *Store) NextNumber(ctx context.Context) (string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT number FROM channels`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	used := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return "", err
		}
		used[n] = true
	}
	for n := 101; ; n++ {
		if v := strconv.Itoa(n); !used[v] {
			return v, rows.Err()
		}
	}
}

// Check cleans what an owner typed and refuses what cannot be saved.
func (c *Channel) Check() error {
	c.Name = strings.TrimSpace(c.Name)
	c.Number = strings.TrimSpace(c.Number)
	c.Description = strings.TrimSpace(c.Description)
	if c.Name == "" {
		return errors.New("give the channel a name")
	}
	if len(c.Name) > maxTextLen || len(c.Description) > 2000 {
		return errors.New("the name or description is too long")
	}
	if c.Number != "" {
		if len(c.Number) > 12 || strings.Trim(c.Number, "0123456789.-") != "" {
			return errors.New("channel numbers look like 101 or 5.1")
		}
	}
	if err := c.Definition.Validate(); err != nil {
		return err
	}
	if len(c.Categories) > 20 {
		return errors.New("a channel can be in at most 20 categories")
	}
	seen := map[string]bool{}
	cats := []string{}
	for _, v := range c.Categories {
		v = strings.TrimSpace(v)
		if len(v) > 40 {
			return errors.New("a category's name is too long")
		}
		if k := strings.ToLower(v); v != "" && !seen[k] {
			seen[k] = true
			cats = append(cats, v)
		}
	}
	c.Categories = cats
	if c.Look.Accent != "" && !hexColour.MatchString(c.Look.Accent) {
		return errors.New("the channel's colour must look like #336699")
	}
	switch c.Look.LogoKind {
	case "", "none":
		if c.Look.LogoName != "" && c.Look.LogoKind == "" {
			return errors.New("say whether the logo is a network's or a studio's")
		}
	case "network", "studio":
		if strings.TrimSpace(c.Look.LogoName) == "" {
			return errors.New("name the network or studio whose logo to use")
		}
	default:
		return errors.New("a channel's logo is a network's or a studio's")
	}
	if len(c.Look.LogoName) > maxTextLen {
		return errors.New("the logo's name is too long")
	}
	return c.Visibility.validate()
}

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Kids' genres, for working out the guide's Kids filter.
var kidsGenres = []string{"Children", "Kids", "Family", "Animation"}

// AutoCategories are the guide filters a channel falls under on its own: the
// films-only ones under Movies, the child-friendly under Kids.
func (c *Channel) AutoCategories() []string {
	r := c.Definition.Rules
	var out []string
	films := len(r.Kinds) > 0
	for _, k := range r.Kinds {
		films = films && (k == "movies" || k == "discs")
	}
	if films || len(r.Studios) > 0 || r.onlyClips() {
		out = append(out, "Movies")
	}
	kids := r.MaxAge != nil && *r.MaxAge <= 12
	for _, g := range r.Genres {
		for _, k := range kidsGenres {
			kids = kids || strings.EqualFold(g, k)
		}
	}
	if kids {
		out = append(out, "Kids")
	}
	return out
}

// GuideCategories are the filters a channel shows under in the guide.
func (c *Channel) GuideCategories() []string {
	if len(c.Categories) > 0 {
		return c.Categories
	}
	return c.AutoCategories()
}

// Create saves a new channel.
func (s *Store) Create(ctx context.Context, c *Channel, userID int64) error {
	if err := c.Check(); err != nil {
		return err
	}
	if c.Number == "" {
		n, err := s.NextNumber(ctx)
		if err != nil {
			return err
		}
		c.Number = n
	}
	if c.Definition.Mode == "" {
		c.Definition.Mode = "simple"
	}
	if err := s.checkClips(ctx, &c.Definition); err != nil {
		return err
	}
	def, _ := json.Marshal(c.Definition)
	vis, _ := json.Marshal(c.Visibility)
	cats, look := marshalExtras(c)
	c.Seed = rand.Uint64() >> 1 // keep it in SQLite's signed integers
	c.Rev = 1
	c.Enabled = true
	now := db.Now()
	c.AnchorDay = DayNumber(time.Now(), s.loc())
	res, err := s.DB.ExecContext(ctx, `INSERT INTO channels(number, name, description, enabled, definition, seed, rev, anchor_day, visibility, count_progress, categories, look, created_by, created_at, updated_at)
		VALUES(?,?,?,1,?,?,1,?,?,?,?,?,?,?,?)`, c.Number, c.Name, c.Description, string(def), int64(c.Seed), c.AnchorDay, string(vis), boolInt(c.CountProgress), cats, look, userID, now, now)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	c.CreatedAt, c.UpdatedAt = now, now
	return nil
}

func marshalExtras(c *Channel) (cats, look string) {
	if c.Categories == nil {
		c.Categories = []string{}
	}
	a, _ := json.Marshal(c.Categories)
	b, _ := json.Marshal(c.Look)
	return string(a), string(b)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

const keepHistory = 20

// Update saves changes. The definition a save replaces goes to the history,
// and a changed definition takes effect after whatever is on now.
func (s *Store) Update(ctx context.Context, c *Channel) error {
	if err := c.Check(); err != nil {
		return err
	}
	old, err := s.Get(ctx, c.ID)
	if err != nil {
		return err
	}
	if err := s.checkClips(ctx, &c.Definition); err != nil {
		return err
	}
	oldDef, _ := json.Marshal(old.Definition)
	newDef, _ := json.Marshal(c.Definition)
	vis, _ := json.Marshal(c.Visibility)
	changed := string(oldDef) != string(newDef)
	if changed {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO channel_history(channel_id, saved_at, name, definition) VALUES(?,?,?,?)`, c.ID, old.UpdatedAt, old.Name, string(oldDef)); err != nil {
			return err
		}
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM channel_history WHERE channel_id = ? AND id NOT IN
			(SELECT id FROM channel_history WHERE channel_id = ? ORDER BY id DESC LIMIT ?)`, c.ID, c.ID, keepHistory)
	}
	cats, look := marshalExtras(c)
	if _, err := s.DB.ExecContext(ctx, `UPDATE channels SET number=?, name=?, description=?, enabled=?, definition=?, visibility=?, count_progress=?, categories=?, look=?, updated_at=? WHERE id=?`,
		c.Number, c.Name, c.Description, boolInt(c.Enabled), string(newDef), string(vis), boolInt(c.CountProgress), cats, look, db.Now(), c.ID); err != nil {
		return err
	}
	if changed {
		return s.Rebuild(ctx, c.ID, false)
	}
	return nil
}

// Delete removes a channel, its schedule and its history.
func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetLogo records the logo's version.
func (s *Store) SetLogo(ctx context.Context, id int64, version string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE channels SET logo = ?, updated_at = ? WHERE id = ?`, version, db.Now(), id)
	return err
}

// HistoryEntry is an earlier version of a channel.
type HistoryEntry struct {
	ID         int64      `json:"id"`
	SavedAt    int64      `json:"savedAt"`
	Name       string     `json:"name"`
	Definition Definition `json:"definition"`
}

// History lists earlier versions, newest first.
func (s *Store) History(ctx context.Context, id int64) ([]HistoryEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, saved_at, name, definition FROM channel_history WHERE channel_id = ? ORDER BY id DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var h HistoryEntry
		var def string
		if err := rows.Scan(&h.ID, &h.SavedAt, &h.Name, &def); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(def), &h.Definition) == nil {
			out = append(out, h)
		}
	}
	return out, rows.Err()
}

// Restore makes an earlier version current; the version it replaces goes to
// the history, so a restore can be undone too.
func (s *Store) Restore(ctx context.Context, id, historyID int64) (*Channel, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	var def string
	if err := s.DB.QueryRowContext(ctx, `SELECT definition FROM channel_history WHERE id = ? AND channel_id = ?`, historyID, id).Scan(&def); err != nil {
		return nil, ErrNotFound
	}
	if err := json.Unmarshal([]byte(def), &c.Definition); err != nil {
		return nil, err
	}
	if err := s.Update(ctx, c); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// ---------------------------------------------------------------------------
// Days
// ---------------------------------------------------------------------------

// maxSimulate bounds how far back a schedule is replayed when nothing is
// stored: a little over a year, which is unnoticeable and takes milliseconds.
const maxSimulate = 400

func (s *Store) loader(ctx context.Context) PoolLoader {
	return func(r Rules, minSec int64) ([]Entry, error) { return s.LoadPool(ctx, r, minSec) }
}

// Rebuild throws away the schedule after what is on now and starts again from
// there. reseed also picks a new shuffle.
func (s *Store) Rebuild(ctx context.Context, id int64, reseed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now()
	today := DayNumber(now, s.loc())
	var kept []Programme
	var state State
	if row, ok := s.readDay(ctx, id, today); ok {
		var onNow int64
		for _, p := range row.Programmes {
			// The rest of a programme that is on now stays too, breaks included.
			inBreak := p.Kind == KindFiller && (p.Role == RoleBreak || p.Role == RoleAd || p.Role == RoleBetween)
			goesOn := onNow != 0 && (inBreak || (p.Kind == KindShow && p.Offset > 0 && p.ItemID == onNow))
			if p.Start > now.Unix() && !goesOn {
				break
			}
			if p.Kind == KindShow {
				onNow = p.ItemID
			}
			kept = append(kept, p)
		}
		if len(kept) > 0 {
			state.Carry = kept[len(kept)-1].End
		}
		state.Cursors = row.BeforeCursors()
	}
	if reseed {
		c.Seed = rand.Uint64() >> 1
	}
	rev := c.Rev + 1
	if _, err := s.DB.ExecContext(ctx, `UPDATE channels SET rev = ?, seed = ?, anchor_day = ? WHERE id = ?`, rev, int64(c.Seed), today, id); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM channel_days WHERE channel_id = ?`, id); err != nil {
		return err
	}
	c.Rev, c.AnchorDay = rev, today
	if state.Cursors == nil {
		state.Cursors = map[string]*cursor{}
	}
	before := State{Cursors: cloneCursors(state.Cursors)}
	progs, next, err := GenerateDay(&c.Definition, c.Seed, today, s.loc(), state, s.loader(ctx), nil)
	if err != nil {
		return err
	}
	return s.writeDay(ctx, c, today, append(kept, progs...), before, next)
}

type dayRow struct {
	Rev        int
	Programmes []Programme
	State      dayState
}

// dayState is stored with a day: what it began from, and what it ended with.
type dayState struct {
	Before map[string]*cursor `json:"before"`
	After  State              `json:"after"`
}

// BeforeCursors are the positions at the start of the day. Editing a channel
// keeps them, so shows carry on rather than start over.
func (r *dayRow) BeforeCursors() map[string]*cursor { return r.State.Before }

func (s *Store) readDay(ctx context.Context, id int64, day int) (*dayRow, bool) {
	var progs, state string
	var rev int
	if err := s.DB.QueryRowContext(ctx, `SELECT rev, programmes, state FROM channel_days WHERE channel_id = ? AND day = ?`, id, day).Scan(&rev, &progs, &state); err != nil {
		return nil, false
	}
	r := &dayRow{Rev: rev}
	if json.Unmarshal([]byte(progs), &r.Programmes) != nil || json.Unmarshal([]byte(state), &r.State) != nil {
		return nil, false
	}
	return r, true
}

func (s *Store) writeDay(ctx context.Context, c *Channel, day int, progs []Programme, before, after State) error {
	if progs == nil {
		progs = []Programme{}
	}
	p, _ := json.Marshal(progs)
	st, _ := json.Marshal(dayState{Before: before.Cursors, After: after})
	_, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO channel_days(channel_id, day, rev, programmes, state) VALUES(?,?,?,?,?)`, c.ID, day, c.Rev, string(p), string(st))
	return err
}

func cloneCursors(m map[string]*cursor) map[string]*cursor {
	b, _ := json.Marshal(m)
	out := map[string]*cursor{}
	_ = json.Unmarshal(b, &out)
	return out
}

// Day returns one day's programmes, working it out if nobody has asked yet.
// Days from today on are stored, so the guide never changes under a viewer.
func (s *Store) Day(ctx context.Context, c *Channel, day int) ([]Programme, error) {
	if r, ok := s.readDay(ctx, c.ID, day); ok && r.Rev == c.Rev {
		return r.Programmes, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.readDay(ctx, c.ID, day); ok && r.Rev == c.Rev {
		return r.Programmes, nil
	}
	today := DayNumber(time.Now(), s.loc())
	// Start from the nearest earlier stored day, else replay from the anchor.
	state := State{Cursors: map[string]*cursor{}}
	from := max(c.AnchorDay, day-maxSimulate)
	var near int
	var raw string
	err := s.DB.QueryRowContext(ctx, `SELECT day, state FROM channel_days WHERE channel_id = ? AND rev = ? AND day < ? ORDER BY day DESC LIMIT 1`, c.ID, c.Rev, day).Scan(&near, &raw)
	if err == nil {
		var st dayState
		if json.Unmarshal([]byte(raw), &st) == nil && near >= from-1 {
			state = st.After
			from = near + 1
			if state.Cursors == nil {
				state.Cursors = map[string]*cursor{}
			}
		}
	}
	if from > day {
		from = day
	}
	cache := map[string][]Entry{}
	load := s.loader(ctx)
	var progs []Programme
	for d := from; d <= day; d++ {
		before := State{Cursors: cloneCursors(state.Cursors)}
		out, next, err := GenerateDay(&c.Definition, c.Seed, d, s.loc(), state, load, cache)
		if err != nil {
			return nil, err
		}
		state = next
		progs = out
		if d >= today {
			if err := s.writeDay(ctx, c, d, out, before, next); err != nil {
				return nil, err
			}
		}
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM channel_days WHERE channel_id = ? AND day < ?`, c.ID, today-1)
	if progs == nil {
		progs = []Programme{}
	}
	return progs, nil
}

// Window lists the programmes overlapping [from, to).
func (s *Store) Window(ctx context.Context, c *Channel, from, to time.Time) ([]Programme, error) {
	out := []Programme{}
	// A programme can run over from the day before.
	for d := DayNumber(from, s.loc()) - 1; d <= DayNumber(to, s.loc()); d++ {
		if d < c.AnchorDay {
			continue
		}
		day, err := s.Day(ctx, c, d)
		if err != nil {
			return nil, err
		}
		for _, p := range day {
			if p.End > from.Unix() && p.Start < to.Unix() {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

// Now is what is on at t, and the next programme after it. Between programmes
// (a moment left by a schedule edit) cur is a slate up to the next.
func (s *Store) Now(ctx context.Context, c *Channel, t time.Time) (cur, next *Programme, err error) {
	list, err := s.Window(ctx, c, t, t.Add(26*time.Hour))
	if err != nil {
		return nil, nil, err
	}
	u := t.Unix()
	for i := range list {
		p := &list[i]
		if p.Start > u {
			if cur == nil {
				cur = &Programme{Start: u, End: p.Start, Kind: KindSlate}
			}
			// Bumpers and slates are not what a viewer means by "next".
			if p.Kind == KindShow {
				return cur, p, nil
			}
			continue
		}
		if p.End > u {
			cur = p
		}
	}
	if cur == nil {
		cur = &Programme{Start: u, End: u + 3600, Kind: KindSlate}
	}
	return cur, nil, nil
}

// slateMerge: a slate or filler shorter than this joins the programme it
// follows in the guide; a long stretch of nothing shows as what it is.
const slateMerge = 900

// GuideCells turns the schedule into what a viewer means by a programme. A
// bumper before a show starts its slot, breaks and the parts of a programme are
// one programme, and what follows it belongs to it until the next one starts.
// Every start and end in the result is when that actually happens, so the guide
// is as true as the schedule.
func GuideCells(list []Programme) []Programme {
	out := make([]Programme, 0, len(list))
	var lead int64 // when a bumper leading into the next programme began
	for _, p := range list {
		if p.Kind == KindFiller && p.Role == RolePre {
			if lead == 0 {
				lead = p.Start
			}
			continue
		}
		n := len(out)
		if p.Kind == KindShow {
			if n > 0 && p.Offset > 0 && out[n-1].Kind == KindShow && out[n-1].ItemID == p.ItemID && lead == 0 {
				out[n-1].End = p.End // the next part of the same programme
				continue
			}
			if lead != 0 {
				p.Start, lead = lead, 0
			}
			out = append(out, p)
			continue
		}
		if lead != 0 {
			// A bumper leading into nothing: keep it where it is.
			if n > 0 {
				out[n-1].End = p.Start
			}
			lead = 0
		}
		if n > 0 && p.End-p.Start < slateMerge && out[n-1].End >= p.Start {
			out[n-1].End = p.End
			continue
		}
		out = append(out, p)
	}
	return out
}

// GuideNow is the programme on at t as the guide tells it, and the one after.
func (s *Store) GuideNow(ctx context.Context, c *Channel, t time.Time) (cur, next *Programme, err error) {
	list, err := s.Window(ctx, c, t.Add(-12*time.Hour), t.Add(26*time.Hour))
	if err != nil {
		return nil, nil, err
	}
	cells := GuideCells(list)
	u := t.Unix()
	for i := range cells {
		p := &cells[i]
		if p.Start <= u && u < p.End {
			cur = p
			continue
		}
		if p.Start > u {
			if p.Kind == KindShow {
				return cur, p, nil
			}
		}
	}
	return cur, nil, nil
}

// Find looks through what a channel will play in a window.
func (s *Store) Find(ctx context.Context, c *Channel, from, to time.Time, needle string, limit int) ([]Programme, error) {
	list, err := s.Window(ctx, c, from.Add(-12*time.Hour), to)
	if err != nil {
		return nil, err
	}
	needle = strings.ToLower(needle)
	out := []Programme{}
	for _, p := range GuideCells(list) {
		if p.Kind != KindShow || p.End <= from.Unix() || p.Start >= to.Unix() {
			continue
		}
		if strings.Contains(strings.ToLower(p.Title), needle) || strings.Contains(strings.ToLower(p.Sub), needle) {
			out = append(out, p)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Who may see a channel
// ---------------------------------------------------------------------------

// allRules lists every set of rules a channel can play.
func (d *Definition) allRules() []Rules {
	out := []Rules{d.Rules}
	if d.Mode != "simple" && d.Mode != "" {
		for i := range d.Blocks {
			if r := d.Blocks[i].Rules; r != nil {
				out = append(out, *r)
			}
		}
	}
	if d.Filler.Rules != nil {
		out = append(out, *d.Filler.Rules)
	}
	for _, c := range d.Filler.Commercials {
		if r, ok := c.rules(); ok {
			out = append(out, r)
		}
	}
	for i := range d.Bumpers.Rules {
		if b := &d.Bumpers.Rules[i]; !b.Off {
			out = append(out, b.Source)
		}
	}
	// A disc a scene comes from counts just as if the whole disc could play.
	for _, r := range out[:len(out):len(out)] {
		if ids := r.clipItems(); len(ids) > 0 && !r.onlyClips() {
			out = append(out, Rules{Items: ids})
		}
	}
	return out
}

// Visible says whether an account may see a channel. Viewers of a channel
// share one stream, so an account with limits sees it only when everything
// it could play is within those limits.
func (s *Store) Visible(ctx context.Context, c *Channel, u *auth.User, a *library.Access) bool {
	if u == nil {
		return false
	}
	if u.IsAdmin {
		return true
	}
	if !c.Enabled || !a.CanWatchChannels("library") {
		return false
	}
	switch c.Visibility.Mode {
	case "adults":
		if u.Kind != auth.KindAdult && u.Kind != "" {
			return false
		}
	case "only":
		found := false
		for _, id := range c.Visibility.Users {
			found = found || id == u.ID
		}
		if !found {
			return false
		}
	}
	if a == nil {
		return true
	}
	seen := map[string]bool{}
	for _, r := range c.Definition.allRules() {
		k := hashKey(r)
		if seen[k] {
			continue
		}
		seen[k] = true
		if bad, err := s.excludes(ctx, r, a); bad || err != nil {
			return false
		}
	}
	return true
}
