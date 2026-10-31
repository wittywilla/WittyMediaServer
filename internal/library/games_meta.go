package library

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittyplay"
)

// Finding out about games, in the background: scans only list files, and this comes
// along behind them. See docs/METADATA.md.
//
// It works through the game_meta table a batch at a time. Each game's question goes to
// each source once and the answer is kept, so a restart loses nothing, changing the
// order of sources costs no requests, and adding a source only asks that one. Sources
// pace themselves, and one that says stop is left alone until it says it can be asked
// again. Pictures come last, in their own pass, because they're the slow part.

// gameMetaBatch is how many games are worked on at once. Bulk sources get a batch's
// worth of questions in one go.
const gameMetaBatch = 50

// sourceSet is the sources in use, kept while the settings they're built from stay the
// same, so their pacing and sign-ins carry over.
type sourceSet struct {
	signature string
	all       map[string]gamemeta.Source
	order     []string // every source, in the owner's order
	on        []string // those switched on and able to run
}

// GameMeta runs the background work.
type GameMeta struct {
	Store     *Store
	Log       *slog.Logger
	ImagesDir string
	CacheDir  string
	// SaveMeta and Save write what was found beside the game, if the owner wants that.
	SaveMeta func() bool
	Save     func(context.Context, *Item) error
	// HTTP asks the sources, and Pictures fetches pictures. Both refuse local addresses
	// unless a test says otherwise.
	HTTP, Pictures *http.Client
	// Built overrides the sources, for tests.
	Built func() []gamemeta.Source

	kick chan struct{}
	id   gamemeta.Identifier

	mu      sync.Mutex
	set     *sourceSet
	running bool
	phase   string
	paused  map[string]gamemeta.RateLimited
	gates   map[string]*time.Time
}

// NewGameMeta makes the background worker. Run starts it.
func NewGameMeta(store *Store, imagesDir, cacheDir string, log *slog.Logger) *GameMeta {
	return &GameMeta{Store: store, ImagesDir: imagesDir, CacheDir: cacheDir, Log: log, kick: make(chan struct{}, 1), paused: map[string]gamemeta.RateLimited{}, gates: map[string]*time.Time{}}
}

// Kick says there may be new work.
func (g *GameMeta) Kick() {
	if g == nil {
		return
	}
	select {
	case g.kick <- struct{}{}:
	default:
	}
}

// Run does the work until ctx ends. It wakes when kicked, and now and then by itself
// for whatever was waiting for a time.
func (g *GameMeta) Run(ctx context.Context) {
	_, _ = g.Store.DB.ExecContext(ctx, `INSERT OR IGNORE INTO game_meta(item_id, library_id) SELECT id, library_id FROM items WHERE kind = 'game' AND missing = 0`)
	// Not straight away: the server has just started, and has other things to do first.
	first := time.NewTimer(10 * time.Second)
	defer first.Stop()
	select {
	case <-first.C:
	case <-g.kick:
	case <-ctx.Done():
		return
	}
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for {
		g.work(ctx)
		select {
		case <-g.kick:
		case <-tick.C:
		case <-ctx.Done():
			return
		}
	}
}

func (g *GameMeta) setPhase(running bool, phase string) {
	g.mu.Lock()
	g.running, g.phase = running, phase
	g.mu.Unlock()
}

// work goes until there's nothing left to do right now.
func (g *GameMeta) work(ctx context.Context) {
	defer g.setPhase(false, "")
	defer g.id.Close()
	for ctx.Err() == nil {
		g.setPhase(true, "details")
		details, err := g.detailsBatch(ctx)
		if err != nil && ctx.Err() == nil {
			g.Log.Warn("game details", "cat", "library", "err", err)
			return
		}
		// Taking turns: covers are what someone looking at the page notices first.
		g.setPhase(true, "pictures")
		pictures, err := g.picturesBatch(ctx)
		if err != nil && ctx.Err() == nil {
			g.Log.Warn("game pictures", "cat", "library", "err", err)
			return
		}
		if details == 0 && pictures == 0 {
			return
		}
	}
}

// ---------------------------------------------------------------- the queue

type gameJob struct {
	it     *Item
	meta   gameRow
	lookup gamemeta.Query
}

// gameRow is one row of game_meta, as far as the worker needs it.
type gameRow struct {
	ItemID                 int64
	Status                 string
	Tries                  int
	Locked                 bool
	Edited, Applied        map[string]bool
	FileSize, FileMTime    int64
	CRCs                   []uint32
	CRCSize                int64
	Serial, QueryTitle     string
	Tags                   string
	PosterRefs, BackRefs   []string
	PosterRef, BackdropRef string
}

func setOf(s string) map[string]bool {
	out := map[string]bool{}
	for _, x := range strings.Split(s, ",") {
		if x != "" {
			out[x] = true
		}
	}
	return out
}

func listOf(m map[string]bool) string {
	var out []string
	for k, on := range m {
		if on {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func parseCRCs(s string) []uint32 {
	var out []uint32
	for _, x := range strings.Split(s, ",") {
		var v uint32
		if _, err := fmt.Sscanf(x, "%08x", &v); err == nil && x != "" {
			out = append(out, v)
		}
	}
	return out
}

func formatCRCs(v []uint32) string {
	parts := make([]string, len(v))
	for i, c := range v {
		parts[i] = fmt.Sprintf("%08x", c)
	}
	return strings.Join(parts, ",")
}

func refList(s string) []string {
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

const gameRowCols = `item_id, status, tries, locked, edited, applied, file_size, file_mtime, crcs, crc_size, serial, query_title, tags, poster_refs, backdrop_refs, poster_ref, backdrop_ref`

func scanGameRow(sc interface{ Scan(...any) error }) (gameRow, error) {
	var r gameRow
	var locked int
	var edited, applied, crcs, posters, backs string
	err := sc.Scan(&r.ItemID, &r.Status, &r.Tries, &locked, &edited, &applied, &r.FileSize, &r.FileMTime, &crcs, &r.CRCSize, &r.Serial, &r.QueryTitle, &r.Tags, &posters, &backs, &r.PosterRef, &r.BackdropRef)
	r.Locked = locked != 0
	r.Edited, r.Applied, r.CRCs = setOf(edited), setOf(applied), parseCRCs(crcs)
	r.PosterRefs, r.BackRefs = refList(posters), refList(backs)
	return r, err
}

func (s *Store) gameRow(ctx context.Context, id int64) (gameRow, error) {
	return scanGameRow(s.DB.QueryRowContext(ctx, `SELECT `+gameRowCols+` FROM game_meta WHERE item_id = ?`, id))
}

// nextGames picks the next games to look up: one console at a time, because that's how
// the bulk sources want asking.
func (s *Store) nextGames(ctx context.Context, n int) ([]gameJob, error) {
	now := db.Now()
	var console string
	err := s.DB.QueryRowContext(ctx, `SELECT IFNULL(i.console, '') FROM game_meta m JOIN items i ON i.id = m.item_id
		WHERE m.status = 'pending' AND m.next_try <= ? AND i.missing = 0 AND i.kind = 'game' AND IFNULL(i.console, '') <> ? ORDER BY i.console LIMIT 1`, now, wittyplay.TestConsole).Scan(&console)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT m.item_id FROM game_meta m JOIN items i ON i.id = m.item_id
		WHERE m.status = 'pending' AND m.next_try <= ? AND i.missing = 0 AND i.kind = 'game' AND IFNULL(i.console, '') = ? ORDER BY m.item_id LIMIT ?`, now, console, n)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	var out []gameJob
	for _, id := range ids {
		it, err := s.Item(ctx, id)
		if err != nil {
			continue
		}
		r, err := s.gameRow(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, gameJob{it: it, meta: r})
	}
	return out, rows.Err()
}

// EnqueueGame puts a game in the queue, because it's new or its file changed. A locked
// match stays as it is; what the file is may still need working out again.
func (s *Store) enqueueGame(ctx context.Context, it *Item, local []string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO game_meta(item_id, library_id, edited) VALUES(?,?,?)
		ON CONFLICT(item_id) DO UPDATE SET status = CASE WHEN locked = 1 THEN status ELSE 'pending' END, next_try = 0, tries = 0,
		  edited = CASE WHEN excluded.edited = '' THEN edited ELSE excluded.edited END`,
		it.ID, it.LibraryID, strings.Join(local, ","))
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM game_meta_sources WHERE item_id = ? AND method NOT IN ('manual')`, it.ID); err != nil {
		return err
	}
	return nil
}

// MarkGameEdited: someone changed these fields by hand, so sources leave them alone.
func (s *Store) MarkGameEdited(ctx context.Context, id int64, fields ...string) error {
	r, err := s.gameRow(ctx, id)
	if err != nil {
		return nil // not queued yet: nothing has been filled in to overwrite
	}
	for _, f := range fields {
		r.Edited[f] = true
		delete(r.Applied, f)
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE game_meta SET edited = ?, applied = ? WHERE item_id = ?`, listOf(r.Edited), listOf(r.Applied), id)
	return err
}

// ---------------------------------------------------------------- asking sources

// fetchOrder is the order questions are asked in, which isn't the order answers are
// trusted in: free, local answers first, so the rest can be asked by the exact name
// the file turned out to have.
func fetchOrder(ids []string) []string {
	rank := map[string]int{"libretro": 0, "libretroart": 0, "wikidata": 1, "screenscraper": 2}
	out := append([]string{}, ids...)
	sort.SliceStable(out, func(a, b int) bool {
		ra, ok := rank[out[a]]
		if !ok {
			ra = 3
		}
		rb, ok := rank[out[b]]
		if !ok {
			rb = 3
		}
		return ra < rb
	})
	return out
}

func (g *GameMeta) detailsBatch(ctx context.Context) (int, error) {
	jobs, err := g.Store.nextGames(ctx, gameMetaBatch)
	if err != nil || len(jobs) == 0 {
		return 0, err
	}
	set := g.sources(ctx)
	order := fetchOrder(set.on)
	console := jobs[0].it.Console
	qs := make([]gamemeta.Query, len(jobs))
	for i := range jobs {
		j := &jobs[i]
		g.identify(ctx, j)
		qs[i] = j.lookup
	}
	// Local answers come first, so that bulk sources are asked by the name a file
	// turned out to have and not the one it was given.
	for i := range jobs {
		if q, ok := g.askLocal(ctx, &jobs[i], set); ok {
			qs[i] = q
		}
	}
	for _, id := range order {
		if p, ok := set.all[id].(gamemeta.Preparer); ok && id != "libretro" && g.pausedUntil(id).IsZero() {
			if err := p.Prepare(ctx, console, qs); err != nil {
				g.notePause(id, err)
				if ctx.Err() != nil {
					return 0, ctx.Err()
				}
				g.Log.Info("game source", "cat", "library", "source", id, "err", err)
			}
		}
	}
	for i := range jobs {
		if ctx.Err() != nil {
			return i, ctx.Err()
		}
		if err := g.lookup(ctx, &jobs[i], set, order); err != nil && ctx.Err() == nil {
			g.Log.Warn("game details", "cat", "library", "item", jobs[i].it.ID, "err", err)
		}
	}
	return len(jobs), nil
}

// askLocal asks the local database about a game, if it hasn't been asked, and says
// what the game should be asked about as from now on.
func (g *GameMeta) askLocal(ctx context.Context, j *gameJob, set *sourceSet) (gamemeta.Query, bool) {
	src := set.all["libretro"]
	if src == nil || !slices.Contains(set.on, "libretro") {
		return j.lookup, false
	}
	answers, done, err := g.Store.gameAnswers(ctx, j.it.ID)
	if err != nil {
		return j.lookup, false
	}
	in := answers["libretro"]
	if !done["libretro"] {
		var lerr error
		in, lerr = src.Lookup(ctx, j.lookup)
		switch {
		case lerr == nil:
			_ = g.Store.saveGameAnswer(ctx, j.it.ID, "libretro", in)
		case errors.Is(lerr, gamemeta.ErrNotFound):
			_ = g.Store.saveGameAnswer(ctx, j.it.ID, "libretro", nil)
			return j.lookup, false
		default:
			return j.lookup, false
		}
	}
	if in != nil && in.Confidence >= 90 && in.Title != "" {
		j.lookup.Title = gamemeta.SearchTitle(gameCleanTitle(in.Title))
	}
	return j.lookup, true
}

// identify works out what's needed to ask about a game, once per version of its file.
func (g *GameMeta) identify(ctx context.Context, j *gameJob) {
	it, r := j.it, &j.meta
	title, tags := gamemeta.CleanName(gameBaseName(filepath.Base(it.Path)))
	if r.FileSize != it.SizeBytes || r.FileMTime != it.MTime {
		id := g.id.Identify(it.Path, it.Console)
		r.CRCs, r.CRCSize, r.Serial = id.CRCs, id.Size, id.Serial
		r.FileSize, r.FileMTime = it.SizeBytes, it.MTime
		_, _ = g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET file_size = ?, file_mtime = ?, crcs = ?, crc_size = ?, serial = ?, tags = ? WHERE item_id = ?`,
			r.FileSize, r.FileMTime, formatCRCs(r.CRCs), r.CRCSize, r.Serial, tags, it.ID)
	}
	if s := gamemeta.SerialInName(filepath.Base(it.Path)); s != "" && r.Serial == "" {
		r.Serial = s
	}
	q := title
	if r.QueryTitle != "" {
		q = r.QueryTitle // someone chose what this game is
	}
	j.lookup = gamemeta.Query{Console: it.Console, Title: gamemeta.SearchTitle(q), Tags: tags, CRCs: r.CRCs, Size: r.CRCSize, Serial: r.Serial}
}

// lookup asks the sources that haven't answered about this game yet, then merges.
func (g *GameMeta) lookup(ctx context.Context, j *gameJob, set *sourceSet, order []string) error {
	answers, done, err := g.Store.gameAnswers(ctx, j.it.ID)
	if err != nil {
		return err
	}
	retry := time.Time{}
	failed, faulted := false, false // something's left to ask; and it went wrong, rather than a source asking to wait
	q := j.lookup
	// Someone chose this game or a source recognised the exact file: use its name.
	for _, id := range order {
		if a := answers[id]; a != nil && a.Confidence >= 90 && a.Title != "" {
			q.Title = gamemeta.SearchTitle(gameCleanTitle(a.Title))
			break
		}
	}
	for _, id := range order {
		if done[id] {
			continue
		}
		src := set.all[id]
		if gf, ok := src.(gamemeta.GapFiller); ok && gf.GapsOnly() {
			m := gamemeta.Merge(answersInOrder(answers, set.on))
			if !m.Gaps() {
				continue
			}
		}
		if until := g.pausedUntil(id); !until.IsZero() {
			retry = laterOf(retry, until)
			failed = true
			continue
		}
		in, err := src.Lookup(ctx, q)
		switch {
		case err == nil:
			answers[id] = in
			if err := g.Store.saveGameAnswer(ctx, j.it.ID, id, in); err != nil {
				return err
			}
			if in.Confidence >= 90 && in.Title != "" {
				q.Title = gamemeta.SearchTitle(gameCleanTitle(in.Title))
			}
		case errors.Is(err, gamemeta.ErrNotFound):
			if err := g.Store.saveGameAnswer(ctx, j.it.ID, id, nil); err != nil {
				return err
			}
		case errors.Is(err, gamemeta.ErrNotSet), errors.Is(err, gamemeta.ErrKey):
			// Nothing to retry until the key changes, which asks everything again anyway.
			g.notePause(id, &gamemeta.RateLimited{Until: time.Now().Add(time.Hour), Why: err.Error()})
			retry = laterOf(retry, time.Now().Add(time.Hour))
			failed = true
		default:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if g.notePause(id, err) {
				retry = laterOf(retry, g.pausedUntil(id))
			} else {
				retry = laterOf(retry, time.Now().Add(backoff(j.meta.Tries)))
				faulted = true
				g.Log.Info("game source", "cat", "library", "source", id, "err", err)
			}
			failed = true
		}
	}
	if err := g.applyAnswers(ctx, j.it.ID, answersInOrder(answers, set.on)); err != nil {
		return err
	}
	if failed && (!faulted || j.meta.Tries < 8) {
		tries := j.meta.Tries
		if faulted {
			tries++
		}
		_, err := g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET status = 'pending', next_try = ?, tries = ? WHERE item_id = ?`, retry.Unix(), tries, j.it.ID)
		return err
	}
	_, err = g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET status = 'done', next_try = 0, tries = 0, fetched_at = ? WHERE item_id = ?`, db.Now(), j.it.ID)
	return err
}

func laterOf(a, b time.Time) time.Time {
	if a.IsZero() || b.After(a) {
		return b
	}
	return a
}

func backoff(tries int) time.Duration {
	return min(5*time.Minute<<min(tries, 8), 24*time.Hour)
}

func gameCleanTitle(name string) string {
	t, _ := gamemeta.CleanName(name)
	return t
}

func (g *GameMeta) pausedUntil(id string) time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	p, ok := g.paused[id]
	if !ok {
		return time.Time{}
	}
	if time.Now().After(p.Until) {
		delete(g.paused, id)
		return time.Time{}
	}
	return p.Until
}

// notePause remembers that a source asked to be left alone, if that's what err means.
func (g *GameMeta) notePause(id string, err error) bool {
	var rl *gamemeta.RateLimited
	if !errors.As(err, &rl) {
		return false
	}
	g.mu.Lock()
	g.paused[id] = *rl
	g.mu.Unlock()
	return true
}

// gameAnswers reads what sources have said about a game. done lists every source that
// has answered, including those that said they don't know it.
func (s *Store) gameAnswers(ctx context.Context, id int64) (map[string]*gamemeta.Info, map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT source, method, data FROM game_meta_sources WHERE item_id = ?`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	answers, done := map[string]*gamemeta.Info{}, map[string]bool{}
	for rows.Next() {
		var source, method, data string
		if err := rows.Scan(&source, &method, &data); err != nil {
			return nil, nil, err
		}
		done[source] = true
		if method == "none" || data == "" {
			continue
		}
		var in gamemeta.Info
		if json.Unmarshal([]byte(data), &in) == nil {
			answers[source] = &in
		}
	}
	return answers, done, rows.Err()
}

func (s *Store) saveGameAnswer(ctx context.Context, id int64, source string, in *gamemeta.Info) error {
	sid, method, conf, data := "", "none", 0, ""
	if in != nil {
		sid, method, conf, data = in.ID, in.Method, in.Confidence, toJSON(in)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO game_meta_sources(item_id, source, source_id, method, confidence, data, fetched_at) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(item_id, source) DO UPDATE SET source_id = excluded.source_id, method = excluded.method, confidence = excluded.confidence, data = excluded.data, fetched_at = excluded.fetched_at`,
		id, source, sid, method, conf, data, db.Now())
	return err
}

func answersInOrder(answers map[string]*gamemeta.Info, order []string) []gamemeta.Answer {
	var out []gamemeta.Answer
	for _, id := range order {
		if in := answers[id]; in != nil {
			out = append(out, gamemeta.Answer{Source: id, Info: in})
		}
	}
	return out
}

// ---------------------------------------------------------------- applying answers

// applyAnswers merges the answers and writes the result to the item. Scans never take
// away what someone chose, and neither does this: a field holding something no source
// put there (from an NFO, a file beside the game, or someone's edit) stays.
func (g *GameMeta) applyAnswers(ctx context.Context, id int64, answers []gamemeta.Answer) error {
	it, err := g.Store.Item(ctx, id)
	if err != nil {
		return err
	}
	r, err := g.Store.gameRow(ctx, id)
	if err != nil {
		return err
	}
	m := gamemeta.Merge(answers)
	fills := func(field, current string) bool {
		return !r.Edited[field] && (current == "" || r.Applied[field])
	}
	mark := func(field string) { r.Applied[field] = true }

	// What a source filled in goes when no source says it any more: one was switched off,
	// or a match changed. What someone typed is never in Applied, so it's safe.
	fileTitle, fileTags := gamemeta.CleanName(gameBaseName(filepath.Base(it.Path)))
	drop := func(field string, clear func()) {
		if r.Applied[field] {
			clear()
			delete(r.Applied, field)
		}
	}
	if m.Title == "" {
		drop("title", func() { it.Title, it.SortTitle = fileTitle, SortTitle(fileTitle) })
	}
	if m.Description == "" {
		drop("overview", func() { it.Overview = "" })
	}
	if gamemeta.Year(m.Released) == 0 {
		drop("year", func() { it.Year = nil })
	}
	if len(m.Genres) == 0 {
		drop("genres", func() { it.Genres = nil })
	}
	if m.Developer == "" {
		drop("studio", func() { it.Studio = "" })
	}
	if m.Rating() == nil {
		drop("rating", func() { it.Rating = nil })
	}
	if m.ContentRating() == "" {
		drop("contentRating", func() { it.ContentRating = "" })
	}
	if len(m.Posters) == 0 {
		drop("poster", func() { it.Poster, r.PosterRef = "", "" })
	}
	if len(m.Backdrops) == 0 {
		drop("backdrop", func() { it.Backdrop, r.BackdropRef = "", "" })
	}

	if m.Title != "" && !r.Edited["title"] && m.Confidence >= gamemeta.AcceptConfidence {
		title, _ := gamemeta.CleanName(m.Title)
		if title != "" {
			it.Title, it.SortTitle = title, SortTitle(title)
			mark("title")
		}
	}
	_, nameTags := gamemeta.CleanName(m.Title)
	if nameTags == "" {
		nameTags = m.Tags
	}
	if nameTags != "" && fills("tagline", it.Tagline) {
		it.Tagline = clip(nameTags, 200)
		mark("tagline")
	} else if nameTags == "" {
		drop("tagline", func() { it.Tagline = fileTags })
	}
	if m.Description != "" && fills("overview", it.Overview) {
		it.Overview = m.Description
		mark("overview")
	}
	if y := gamemeta.Year(m.Released); y != 0 && !r.Edited["year"] && (it.Year == nil || r.Applied["year"]) {
		it.Year = &y
		mark("year")
	}
	if len(m.Genres) > 0 && !r.Edited["genres"] && (len(it.Genres) == 0 || r.Applied["genres"]) {
		it.Genres = m.Genres
		mark("genres")
	}
	if m.Developer != "" && fills("studio", it.Studio) {
		it.Studio = m.Developer
		mark("studio")
	}
	if v := m.Rating(); v != nil && !r.Edited["rating"] && (it.Rating == nil || r.Applied["rating"]) {
		it.Rating = v
		mark("rating")
	}
	if c := m.ContentRating(); c != "" && fills("contentRating", it.ContentRating) {
		it.ContentRating = c
		mark("contentRating")
	}
	if m.MatchSource != "" {
		it.Provider, it.ProviderID = "game:"+m.MatchSource, m.MatchID
	} else if strings.HasPrefix(it.Provider, "game:") {
		it.Provider, it.ProviderID = "", ""
	}
	if err := g.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	// Pictures are fetched later, unless what's wanted is what's already here.
	wantArt := 0
	if len(m.Posters) > 0 && m.Posters[0] != r.PosterRef && !r.Edited["poster"] && (it.Poster == "" || r.Applied["poster"]) {
		wantArt = 1
	}
	if len(m.Backdrops) > 0 && m.Backdrops[0] != r.BackdropRef && !r.Edited["backdrop"] && (it.Backdrop == "" || r.Applied["backdrop"]) {
		wantArt = 1
	}
	credits := make([]gamemeta.Credit, 0, len(m.Used))
	set := g.sources(ctx)
	for _, u := range m.Used {
		src := set.all[u]
		if src == nil {
			continue
		}
		about := src.About()
		c := gamemeta.Credit{Source: u, Name: about.Name, Site: about.Site, Text: about.Credit, License: about.License}
		for _, a := range answers {
			if a.Source == u {
				c.URL = a.Info.URL
			}
		}
		credits = append(credits, c)
	}
	_, err = g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET applied = ?, match_source = ?, match_id = ?, match_method = ?, confidence = ?,
		released = ?, developer = ?, publisher = ?, franchise = ?, players = ?, scores = ?, ages = ?, art = ?, credits = ?,
		poster_refs = ?, backdrop_refs = ?, poster_ref = ?, backdrop_ref = ?, art_wanted = CASE WHEN ? = 1 THEN 1 ELSE art_wanted END, art_tries = CASE WHEN ? = 1 THEN 0 ELSE art_tries END WHERE item_id = ?`,
		listOf(r.Applied), m.MatchSource, m.MatchID, m.MatchMethod, m.Confidence, m.Released, m.Developer, m.Publisher, m.Franchise, m.Players,
		toJSON(m.Scores), toJSON(m.Ages), toJSON(m.Art), toJSON(credits), toJSON(m.Posters), toJSON(m.Backdrops), r.PosterRef, r.BackdropRef, wantArt, wantArt, id)
	return err
}

// ---------------------------------------------------------------- pictures

// gameImageName is what a picture is called in the image cache.
func gameImageName(ref, ext string) string {
	h := sha1.Sum([]byte(ref))
	return "game-" + hex.EncodeToString(h[:12]) + ext
}

var pictureExts = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}

// errNoPicture: this picture doesn't exist, so try another. Anything else is worth
// trying again later.
var errNoPicture = errors.New("no such picture")

// pictureGap is how far apart pictures from one site are asked for.
const pictureGap = 250 * time.Millisecond

func (g *GameMeta) waitForHost(ctx context.Context, host string) error {
	g.mu.Lock()
	next := g.gates[host]
	if next == nil {
		next = new(time.Time)
		g.gates[host] = next
	}
	wait := time.Until(*next)
	*next = time.Now().Add(max(wait, 0) + pictureGap)
	g.mu.Unlock()
	if wait <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// pictureAddress turns a stored reference into something to fetch, and says which
// site it's on, without the address (which may hold a login) ever being kept.
func (g *GameMeta) pictureAddress(ctx context.Context, ref string) (string, string, error) {
	if strings.HasPrefix(ref, "ss:") {
		src, ok := g.sources(ctx).all["screenscraper"].(gamemeta.Referrer)
		if !ok {
			return "", "", errNoPicture
		}
		u, err := src.PictureURL(ref)
		if err != nil {
			return "", "", errNoPicture
		}
		return u, "screenscraper", nil
	}
	if gamemeta.WebAddress(ref) == "" {
		return "", "", errNoPicture
	}
	host := ref
	if _, rest, ok := strings.Cut(ref, "://"); ok {
		host, _, _ = strings.Cut(rest, "/")
	}
	return ref, host, nil
}

// FetchPicture stores a picture in the cache, and returns its "cache:" name. Only real
// pictures are kept, however they were labelled.
func (g *GameMeta) FetchPicture(ctx context.Context, ref string) (string, error) {
	for _, ext := range []string{".jpg", ".png", ".webp", ".gif"} {
		name := gameImageName(ref, ext)
		if st, err := os.Stat(filepath.Join(g.ImagesDir, name)); err == nil && !st.IsDir() {
			return "cache:" + name, nil
		}
	}
	address, host, err := g.pictureAddress(ctx, ref)
	if err != nil {
		return "", err
	}
	if err := g.waitForHost(ctx, host); err != nil {
		return "", err
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, "GET", address, nil)
	if err != nil {
		return "", errNoPicture
	}
	req.Header.Set("User-Agent", gamemeta.UserAgent)
	client := g.Pictures
	if client == nil {
		client = SafeClient
	}
	res, err := client.Do(req)
	if err != nil {
		// The error would name the address, which can hold a login.
		return "", errors.New("the picture's site did not answer")
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == 404 || res.StatusCode == 410:
		return "", errNoPicture
	case res.StatusCode != 200:
		return "", fmt.Errorf("the picture's site answered %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 20<<20+1))
	if err != nil {
		return "", errors.New("the picture couldn't be read")
	}
	if len(b) > 20<<20 {
		return "", errNoPicture
	}
	ext := pictureExts[http.DetectContentType(b)]
	if ext == "" {
		return "", errNoPicture
	}
	if err := os.MkdirAll(g.ImagesDir, 0o755); err != nil {
		return "", err
	}
	name := gameImageName(ref, ext)
	tmp, err := os.CreateTemp(g.ImagesDir, ".game-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(g.ImagesDir, name)); err != nil {
		return "", err
	}
	return "cache:" + name, nil
}

// firstPicture tries a list of references until one works.
func (g *GameMeta) firstPicture(ctx context.Context, refs []string) (cache, ref string, err error) {
	for _, r := range refs {
		c, ferr := g.FetchPicture(ctx, r)
		if ferr == nil {
			return c, r, nil
		}
		if !errors.Is(ferr, errNoPicture) {
			return "", "", ferr
		}
	}
	return "", "", errNoPicture
}

func (g *GameMeta) picturesBatch(ctx context.Context) (int, error) {
	rows, err := g.Store.DB.QueryContext(ctx, `SELECT m.item_id FROM game_meta m JOIN items i ON i.id = m.item_id
		WHERE m.art_wanted = 1 AND m.art_next <= ? AND i.missing = 0 ORDER BY m.item_id LIMIT ?`, db.Now(), gameMetaBatch)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		g.itemPictures(ctx, id)
	}
	return len(ids), nil
}

func (g *GameMeta) itemPictures(ctx context.Context, id int64) {
	it, err := g.Store.Item(ctx, id)
	if err != nil {
		return
	}
	r, err := g.Store.gameRow(ctx, id)
	if err != nil {
		return
	}
	retry := false
	changed := false
	get := func(field, current string, refs []string, was string, set func(cache string)) string {
		if len(refs) == 0 || r.Edited[field] || (current != "" && !r.Applied[field]) || (refs[0] == was && current != "") {
			return was
		}
		cache, ref, err := g.firstPicture(ctx, refs)
		if err != nil {
			if !errors.Is(err, errNoPicture) {
				retry = true
				g.Log.Debug("game picture", "cat", "library", "item", id, "err", err)
			}
			return was
		}
		set(cache)
		r.Applied[field] = true
		changed = true
		return ref
	}
	r.PosterRef = get("poster", it.Poster, r.PosterRefs, r.PosterRef, func(c string) { it.Poster = c })
	r.BackdropRef = get("backdrop", it.Backdrop, r.BackRefs, r.BackdropRef, func(c string) { it.Backdrop = c })
	if changed {
		if err := g.Store.upsertItem(ctx, it); err != nil {
			return
		}
	}
	if retry {
		_, _ = g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET applied = ?, poster_ref = ?, backdrop_ref = ?, art_tries = art_tries + 1,
			art_wanted = CASE WHEN art_tries >= 6 THEN 0 ELSE 1 END, art_next = ? WHERE item_id = ?`,
			listOf(r.Applied), r.PosterRef, r.BackdropRef, time.Now().Add(30*time.Minute).Unix(), id)
		return
	}
	_, _ = g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET applied = ?, poster_ref = ?, backdrop_ref = ?, art_wanted = 0, art_tries = 0 WHERE item_id = ?`,
		listOf(r.Applied), r.PosterRef, r.BackdropRef, id)
	g.saveBeside(ctx, it)
}

// saveBeside writes what was found next to the game, only if the owner has asked for
// that, and never over an NFO that's already there.
func (g *GameMeta) saveBeside(ctx context.Context, it *Item) {
	if g.SaveMeta == nil || g.Save == nil || !g.SaveMeta() {
		return
	}
	if it.Provider == "" {
		return
	}
	_, _, nfo := ArtPaths(it)
	if _, err := os.Stat(nfo); err == nil {
		return
	}
	if err := g.Save(ctx, it); err != nil {
		g.Log.Debug("save game metadata", "cat", "library", "path", it.Path, "err", err)
		return
	}
	_ = g.Store.upsertItem(ctx, it)
}

// ---------------------------------------------------------------- what people ask for

// GameArt is one of a game's pictures.
type GameArt struct {
	Kind string `json:"kind"` // screenshot | title | artwork
	URL  string `json:"url"`  // on this server
}

// GameDetails is everything known about a game beyond what an Item holds.
type GameDetails struct {
	ItemID int64 `json:"itemId"`
	// Status: pending (still being looked up) or done.
	Status string `json:"status"`
	// Locked: the match stays as it is until someone changes it.
	Locked bool `json:"locked"`
	// MatchSource is the source most sure of this match, MatchMethod how it knows
	// (checksum, serial, name or manual), and Confidence how sure, out of 100. 0: no match.
	MatchSource string `json:"matchSource"`
	MatchMethod string `json:"matchMethod"`
	Confidence  int    `json:"confidence"`
	// Tags are what the file's name says beyond the title: regions, revisions.
	Tags      string               `json:"tags"`
	Released  string               `json:"released"` // YYYY, YYYY-MM or YYYY-MM-DD
	Developer string               `json:"developer"`
	Publisher string               `json:"publisher"`
	Franchise string               `json:"franchise"`
	Players   int                  `json:"players"` // 0: unknown
	Scores    []gamemeta.Score     `json:"scores"`
	Ages      []gamemeta.AgeRating `json:"ages"`
	Art       []GameArt            `json:"art"`
	Credits   []gamemeta.Credit    `json:"credits"`
	// Checksums and serial, for administrators, to help fix a wrong match.
	CRCs   string `json:"crcs,omitempty"`
	Serial string `json:"serial,omitempty"`
}

// GameDetails reads what's known about a game.
func (s *Store) GameDetails(ctx context.Context, id int64) (*GameDetails, error) {
	d := &GameDetails{ItemID: id, Status: "pending", Scores: []gamemeta.Score{}, Ages: []gamemeta.AgeRating{}, Art: []GameArt{}, Credits: []gamemeta.Credit{}}
	var locked int
	var scores, ages, art, credits string
	err := s.DB.QueryRowContext(ctx, `SELECT status, locked, match_source, match_method, confidence, tags, released, developer, publisher, franchise, players, scores, ages, art, credits, crcs, serial
		FROM game_meta WHERE item_id = ?`, id).Scan(&d.Status, &locked, &d.MatchSource, &d.MatchMethod, &d.Confidence, &d.Tags, &d.Released, &d.Developer, &d.Publisher, &d.Franchise, &d.Players, &scores, &ages, &art, &credits, &d.CRCs, &d.Serial)
	if errors.Is(err, sql.ErrNoRows) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	d.Locked = locked != 0
	_ = json.Unmarshal([]byte(scores), &d.Scores)
	_ = json.Unmarshal([]byte(ages), &d.Ages)
	_ = json.Unmarshal([]byte(credits), &d.Credits)
	var refs []gamemeta.ArtRef
	_ = json.Unmarshal([]byte(art), &refs)
	for i, r := range refs {
		d.Art = append(d.Art, GameArt{Kind: r.Kind, URL: fmt.Sprintf("/api/games/%d/art/%d", id, i)})
	}
	if d.Scores == nil {
		d.Scores = []gamemeta.Score{}
	}
	if d.Ages == nil {
		d.Ages = []gamemeta.AgeRating{}
	}
	if d.Credits == nil {
		d.Credits = []gamemeta.Credit{}
	}
	return d, nil
}

// GameArtCandidates lists pictures sources have for a game, for someone choosing one.
// A picture whose address would hold a login isn't listed.
func (s *Store) GameArtCandidates(ctx context.Context, id int64, backdrop bool) []string {
	r, err := s.gameRow(ctx, id)
	if err != nil {
		return nil
	}
	refs := r.PosterRefs
	if backdrop {
		refs = r.BackRefs
		var art string
		if s.DB.QueryRowContext(ctx, `SELECT art FROM game_meta WHERE item_id = ?`, id).Scan(&art) == nil {
			var more []gamemeta.ArtRef
			_ = json.Unmarshal([]byte(art), &more)
			for _, a := range more {
				refs = append(refs, a.Ref)
			}
		}
	}
	var out []string
	for _, ref := range refs {
		if gamemeta.WebAddress(ref) != "" {
			out = append(out, ref)
		}
	}
	return out
}

// GameArtPicture gets one of a game's extra pictures, fetching it the first time anyone
// looks.
func (g *GameMeta) GameArtPicture(ctx context.Context, id int64, n int) (string, error) {
	var art string
	if err := g.Store.DB.QueryRowContext(ctx, `SELECT art FROM game_meta WHERE item_id = ?`, id).Scan(&art); err != nil {
		return "", ErrNotFound
	}
	var refs []gamemeta.ArtRef
	_ = json.Unmarshal([]byte(art), &refs)
	if n < 0 || n >= len(refs) {
		return "", ErrNotFound
	}
	return g.FetchPicture(ctx, refs[n].Ref)
}

// Search asks every source in use about a name, for someone choosing a match.
func (g *GameMeta) Search(ctx context.Context, console, title string) ([]gamemeta.Candidate, []string) {
	set := g.sources(ctx)
	var mu sync.Mutex
	var wg sync.WaitGroup
	out := []gamemeta.Candidate{}
	var failed []string
	for _, id := range set.on {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c, err := set.all[id].Search(ctx, console, title)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if !errors.Is(err, gamemeta.ErrNotFound) {
					failed = append(failed, set.all[id].About().Name)
				}
				return
			}
			out = append(out, c...)
		}(id)
	}
	wg.Wait()
	sort.SliceStable(out, func(a, b int) bool { return out[a].Score > out[b].Score })
	if len(out) > 40 {
		out = out[:40]
	}
	if art, ok := set.all["libretroart"].(*gamemeta.LibretroArt); ok && slices.Contains(set.on, "libretroart") {
		art.Decorate(ctx, console, out)
	}
	return out, failed
}

// Choose makes one source's game the answer for this game, with the rest asked again
// about it by that name. lock keeps automatic work away from it from then on.
func (g *GameMeta) Choose(ctx context.Context, it *Item, source, id string, lock bool) error {
	set := g.sources(ctx)
	src := set.all[source]
	if src == nil {
		return ErrNotFound
	}
	in, err := src.Fetch(ctx, it.Console, id)
	if err != nil {
		return err
	}
	in.Method, in.Confidence = gamemeta.MethodManual, 100
	if err := g.ensureRow(ctx, it); err != nil {
		return err
	}
	if _, err := g.Store.DB.ExecContext(ctx, `DELETE FROM game_meta_sources WHERE item_id = ?`, it.ID); err != nil {
		return err
	}
	if err := g.Store.saveGameAnswer(ctx, it.ID, source, in); err != nil {
		return err
	}
	if _, err := g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET poster_ref = '', backdrop_ref = '' WHERE item_id = ?`, it.ID); err != nil {
		return err
	}
	locked := 0
	if lock {
		locked = 1
	}
	if _, err := g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET status = 'pending', next_try = 0, tries = 0, locked = ?, query_title = ? WHERE item_id = ?`, locked, gameCleanTitle(in.Title), it.ID); err != nil {
		return err
	}
	if err := g.applyAnswers(ctx, it.ID, []gamemeta.Answer{{Source: source, Info: in}}); err != nil {
		return err
	}
	g.Kick()
	return nil
}

// SetLocked changes whether a match is locked.
func (g *GameMeta) SetLocked(ctx context.Context, it *Item, on bool) error {
	if err := g.ensureRow(ctx, it); err != nil {
		return err
	}
	v := 0
	if on {
		v = 1
	}
	_, err := g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET locked = ? WHERE item_id = ?`, v, it.ID)
	return err
}

func (g *GameMeta) ensureRow(ctx context.Context, it *Item) error {
	_, err := g.Store.DB.ExecContext(ctx, `INSERT OR IGNORE INTO game_meta(item_id, library_id) VALUES(?, ?)`, it.ID, it.LibraryID)
	return err
}

// ClearMatch forgets a match someone says is wrong: nothing is filled in from sources
// until asked again, or a match is chosen. The game stays locked so it isn't refound.
func (g *GameMeta) ClearMatch(ctx context.Context, it *Item) error {
	if err := g.ensureRow(ctx, it); err != nil {
		return err
	}
	r, err := g.Store.gameRow(ctx, it.ID)
	if err != nil {
		return err
	}
	if _, err := g.Store.DB.ExecContext(ctx, `DELETE FROM game_meta_sources WHERE item_id = ?`, it.ID); err != nil {
		return err
	}
	if r.Applied["title"] {
		it.Title, _ = gamemeta.CleanName(gameBaseName(filepath.Base(it.Path)))
		it.SortTitle = SortTitle(it.Title)
	}
	if r.Applied["tagline"] {
		_, it.Tagline = gamemeta.CleanName(gameBaseName(filepath.Base(it.Path)))
	}
	if r.Applied["overview"] {
		it.Overview = ""
	}
	if r.Applied["year"] {
		it.Year = nil
	}
	if r.Applied["genres"] {
		it.Genres = nil
	}
	if r.Applied["studio"] {
		it.Studio = ""
	}
	if r.Applied["rating"] {
		it.Rating = nil
	}
	if r.Applied["contentRating"] {
		it.ContentRating = ""
	}
	if r.Applied["poster"] {
		it.Poster = ""
	}
	if r.Applied["backdrop"] {
		it.Backdrop = ""
	}
	it.Provider, it.ProviderID = "", ""
	if err := g.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	_, err = g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET status = 'done', locked = 1, applied = '', query_title = '', match_source = '', match_id = '', match_method = '', confidence = 0,
		released = '', developer = '', publisher = '', franchise = '', players = 0, scores = '', ages = '', art = '', credits = '', poster_refs = '', backdrop_refs = '', poster_ref = '', backdrop_ref = '', art_wanted = 0 WHERE item_id = ?`, it.ID)
	return err
}

// Refresh asks every source about a game again. A locked or chosen match keeps its game:
// the source's own id is asked for, rather than the name being searched again.
func (g *GameMeta) Refresh(ctx context.Context, it *Item) error {
	if err := g.ensureRow(ctx, it); err != nil {
		return err
	}
	var source, id string
	var locked int
	_ = g.Store.DB.QueryRowContext(ctx, `SELECT s.source, s.source_id, m.locked FROM game_meta m LEFT JOIN game_meta_sources s ON s.item_id = m.item_id AND s.method = 'manual' WHERE m.item_id = ?`, it.ID).Scan(&source, &id, &locked)
	if source != "" && id != "" {
		if src := g.sources(ctx).all[source]; src != nil {
			in, err := src.Fetch(ctx, it.Console, id)
			if err != nil {
				return err
			}
			in.Method, in.Confidence = gamemeta.MethodManual, 100
			if err := g.Store.saveGameAnswer(ctx, it.ID, source, in); err != nil {
				return err
			}
		}
	}
	if _, err := g.Store.DB.ExecContext(ctx, `DELETE FROM game_meta_sources WHERE item_id = ? AND method <> 'manual'`, it.ID); err != nil {
		return err
	}
	// Refreshing a file means reading it again too.
	if _, err := g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET status = 'pending', next_try = 0, tries = 0, file_size = -1, locked = ? WHERE item_id = ?`, locked, it.ID); err != nil {
		return err
	}
	g.Kick()
	return nil
}

// RequeueAll asks again about whatever hasn't answered yet, after the list of sources
// changed. Sources that have already answered aren't asked again, and order changes
// are merged here and now. Locked games stay as they are.
func (g *GameMeta) RequeueAll(ctx context.Context) error {
	_, err := g.Store.DB.ExecContext(ctx, `UPDATE game_meta SET status = 'pending', next_try = 0, tries = 0 WHERE locked = 0`)
	g.mu.Lock()
	g.paused = map[string]gamemeta.RateLimited{}
	g.mu.Unlock()
	g.Kick()
	return err
}

// RefetchAll forgets every answer and starts again. Choices people made stay.
func (g *GameMeta) RefetchAll(ctx context.Context) error {
	if _, err := g.Store.DB.ExecContext(ctx, `DELETE FROM game_meta_sources WHERE method <> 'manual' AND item_id IN (SELECT item_id FROM game_meta WHERE locked = 0)`); err != nil {
		return err
	}
	return g.RequeueAll(ctx)
}

// GameMetaStatus is how the work is going.
type GameMetaStatus struct {
	Running bool   `json:"running"`
	Phase   string `json:"phase"` // details | pictures | ""
	Total   int    `json:"total"`
	Pending int    `json:"pending"`
	Matched int    `json:"matched"`
	// ByChecksum counts matches by exact file or disc.
	ByChecksum  int                         `json:"byChecksum"`
	Pictures    int                         `json:"pictures"` // games whose pictures are still to fetch
	WithPoster  int                         `json:"withPoster"`
	WithDetails int                         `json:"withDetails"`
	Consoles    map[string]GameConsoleTally `json:"consoles"`
	Paused      []GameSourcePause           `json:"paused"`
	// KeysSet: at least one source that takes a key has one. Pages use this to suggest one.
	KeysSet bool `json:"keysSet"`
}

// GameConsoleTally counts one console's games.
type GameConsoleTally struct {
	Total      int `json:"total"`
	Matched    int `json:"matched"`
	ByChecksum int `json:"byChecksum"`
}

// GameSourcePause is a source that's been asked to wait.
type GameSourcePause struct {
	Source string `json:"source"`
	Until  int64  `json:"until"`
	Why    string `json:"why"`
}

// Status says how the work is going.
func (g *GameMeta) Status(ctx context.Context) GameMetaStatus {
	out := GameMetaStatus{Consoles: map[string]GameConsoleTally{}, Paused: []GameSourcePause{}}
	g.mu.Lock()
	out.Running, out.Phase = g.running, g.phase
	for id, p := range g.paused {
		if time.Now().Before(p.Until) {
			out.Paused = append(out.Paused, GameSourcePause{Source: id, Until: p.Until.Unix(), Why: p.Why})
		}
	}
	g.mu.Unlock()
	sort.Slice(out.Paused, func(a, b int) bool { return out.Paused[a].Source < out.Paused[b].Source })
	rows, err := g.Store.DB.QueryContext(ctx, `SELECT IFNULL(i.console, ''), COUNT(*),
		SUM(m.status = 'pending'), SUM(m.confidence >= ?), SUM(m.match_method IN ('checksum', 'serial')),
		SUM(m.art_wanted = 1), SUM(IFNULL(i.poster, '') <> ''), SUM(IFNULL(i.overview, '') <> '')
		FROM game_meta m JOIN items i ON i.id = m.item_id WHERE i.missing = 0 AND i.kind = 'game' GROUP BY 1`, gamemeta.AcceptConfidence)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		var total, pending, matched, exact, art, posters, details int
		if rows.Scan(&c, &total, &pending, &matched, &exact, &art, &posters, &details) != nil {
			continue
		}
		if c == wittyplay.TestConsole {
			continue
		}
		out.Total += total
		out.Pending += pending
		out.Matched += matched
		out.ByChecksum += exact
		out.Pictures += art
		out.WithPoster += posters
		out.WithDetails += details
		out.Consoles[c] = GameConsoleTally{Total: total, Matched: matched, ByChecksum: exact}
	}
	return out
}

// GameFacets are what a library's games can be filtered by, each with how many there are.
type GameFacets struct {
	Genres  []GameFacet `json:"genres"`
	Decades []GameFacet `json:"decades"`
	Players []GameFacet `json:"players"`
	Rated   int         `json:"rated"` // games with a score
}

// GameFacet is one choice in a filter.
type GameFacet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// GameFacets counts what's in a library, as far as this account can see. One console
// narrows it.
func (s *Store) GameFacets(ctx context.Context, libraryID int64, console string, access *Access) (*GameFacets, error) {
	where := `i.library_id = ? AND i.kind = 'game' AND i.missing = 0`
	args := []any{libraryID}
	if console != "" {
		where += ` AND i.console = ?`
		args = append(args, console)
	}
	if access != nil {
		c, a := access.ItemClause()
		where += " AND " + c
		args = append(args, a...)
	}
	out := &GameFacets{Genres: []GameFacet{}, Decades: []GameFacet{}, Players: []GameFacet{}}
	rows, err := s.DB.QueryContext(ctx, `SELECT IFNULL(i.genres, ''), COUNT(*) FROM items i WHERE `+where+` GROUP BY 1`, args...)
	if err != nil {
		return nil, err
	}
	genres := map[string]int{}
	for rows.Next() {
		var list string
		var n int
		if rows.Scan(&list, &n) == nil {
			for _, g := range splitList(list) {
				genres[g] += n
			}
		}
	}
	rows.Close()
	for g, n := range genres {
		out.Genres = append(out.Genres, GameFacet{g, n})
	}
	sort.Slice(out.Genres, func(a, b int) bool {
		if out.Genres[a].Count != out.Genres[b].Count {
			return out.Genres[a].Count > out.Genres[b].Count
		}
		return out.Genres[a].Value < out.Genres[b].Value
	})
	rows, err = s.DB.QueryContext(ctx, `SELECT i.year / 10 * 10, COUNT(*) FROM items i WHERE `+where+` AND i.year IS NOT NULL GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d, n int
		if rows.Scan(&d, &n) == nil {
			out.Decades = append(out.Decades, GameFacet{fmt.Sprint(d), n})
		}
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT m.players, COUNT(*) FROM items i JOIN game_meta m ON m.item_id = i.id WHERE `+where+` AND m.players > 0 GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p, n int
		if rows.Scan(&p, &n) == nil {
			out.Players = append(out.Players, GameFacet{fmt.Sprint(p), n})
		}
	}
	rows.Close()
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items i WHERE `+where+` AND i.rating IS NOT NULL`, args...).Scan(&out.Rated)
	return out, nil
}
