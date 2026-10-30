package channels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
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
	CreatedAt     int64      `json:"createdAt"`
	UpdatedAt     int64      `json:"updatedAt"`
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

const channelCols = `id, number, name, description, enabled, definition, seed, rev, anchor_day, logo, visibility, count_progress, created_at, updated_at`

func scanChannel(sc interface{ Scan(...any) error }) (*Channel, error) {
	var c Channel
	var def, vis string
	var seed int64
	var enabled, count int
	if err := sc.Scan(&c.ID, &c.Number, &c.Name, &c.Description, &enabled, &def, &seed, &c.Rev, &c.AnchorDay, &c.Logo, &vis, &count, &c.CreatedAt, &c.UpdatedAt); err != nil {
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
	return c.Visibility.validate()
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
	def, _ := json.Marshal(c.Definition)
	vis, _ := json.Marshal(c.Visibility)
	c.Seed = rand.Uint64() >> 1 // keep it in SQLite's signed integers
	c.Rev = 1
	c.Enabled = true
	now := db.Now()
	c.AnchorDay = DayNumber(time.Now(), s.loc())
	res, err := s.DB.ExecContext(ctx, `INSERT INTO channels(number, name, description, enabled, definition, seed, rev, anchor_day, visibility, count_progress, created_by, created_at, updated_at)
		VALUES(?,?,?,1,?,?,1,?,?,?,?,?,?)`, c.Number, c.Name, c.Description, string(def), int64(c.Seed), c.AnchorDay, string(vis), boolInt(c.CountProgress), userID, now, now)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	c.CreatedAt, c.UpdatedAt = now, now
	return nil
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
	if _, err := s.DB.ExecContext(ctx, `UPDATE channels SET number=?, name=?, description=?, enabled=?, definition=?, visibility=?, count_progress=?, updated_at=? WHERE id=?`,
		c.Number, c.Name, c.Description, boolInt(c.Enabled), string(newDef), string(vis), boolInt(c.CountProgress), db.Now(), c.ID); err != nil {
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
		for _, p := range row.Programmes {
			if p.Start <= now.Unix() {
				kept = append(kept, p)
			}
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

// Now is what is on at t, how far into it, and what follows. Between
// programmes (a moment left by a schedule edit) cur is a slate up to the next.
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
			next = p
			return cur, next, nil
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
