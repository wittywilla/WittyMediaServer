// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Cast and crew (docs/METADATA.md, "Cast and crew").
//
// A person is stored once however many titles name them (table people) and each thing they did
// on a title is a credit (table credits): who, on what (an item, which is a film or an episode,
// or a show), in which role, and which source said so. Every source's credits are kept apart and
// replaced only when that source is read again, so a scan that doesn't go online never loses
// what the online source said, and an NFO someone wrote never loses to it. A page takes each part
// (the cast, the guest stars, each crew department) from the best source that has that part.

// Credit roles.
const (
	RoleCast  = "cast"  // in the regular cast: a film's, or a show's series regulars
	RoleGuest = "guest" // a guest star in one episode
	RoleCrew  = "crew"  // behind the camera, in a department
)

// Credit sources, best first. nfo is a file beside the media (the owner's, or one this server
// wrote, which holds what an online source said); tags are a file's own tags, which name few.
const (
	CreditsNFO    = "nfo"
	CreditsTMDB   = "tmdb"
	CreditsTVDB   = "tvdb"
	CreditsTVMaze = "tvmaze"
	CreditsTags   = "tags"
)

var creditSourceRank = map[string]int{CreditsNFO: 0, CreditsTMDB: 1, CreditsTVDB: 2, CreditsTVMaze: 3, CreditsTags: 4}

// Department is one crew department: the key the API sends and the name a page shows.
type Department struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Departments are the crew departments in the order a page lists them. The names are TMDB's,
// which Kodi, Jellyfin and TVDB's own people types map onto.
var Departments = []Department{
	{"directing", "Directing"}, {"writing", "Writing"}, {"production", "Production"}, {"camera", "Camera"},
	{"lighting", "Lighting"}, {"editing", "Editing"}, {"sound", "Sound"}, {"art", "Art"},
	{"costume", "Costume & Make-Up"}, {"effects", "Visual Effects"}, {"crew", "Crew"},
}

// DepartmentKeys are the departments' keys in order.
func DepartmentKeys() []string {
	out := make([]string, len(Departments))
	for i, d := range Departments {
		out[i] = d.Key
	}
	return out
}

func departmentLabel(key string) string {
	for _, d := range Departments {
		if d.Key == key {
			return d.Label
		}
	}
	return "Crew"
}

// DepartmentOf turns a department as a source writes it ("Directing", "Costume & Make-Up",
// "Visual Effects") or a job with no department ("Director", "Set Decoration") into a key.
func DepartmentOf(department, job string) string {
	d := strings.ToLower(strings.TrimSpace(department))
	switch {
	case d == "":
	case strings.HasPrefix(d, "direct"):
		return "directing"
	case strings.HasPrefix(d, "writ"):
		return "writing"
	case strings.HasPrefix(d, "produc"):
		return "production"
	case strings.HasPrefix(d, "camera"), strings.HasPrefix(d, "photograph"):
		return "camera"
	case strings.HasPrefix(d, "light"):
		return "lighting"
	case strings.HasPrefix(d, "edit"):
		return "editing"
	case strings.HasPrefix(d, "sound"), strings.HasPrefix(d, "music"):
		return "sound"
	case strings.HasPrefix(d, "art"):
		return "art"
	case strings.HasPrefix(d, "costume"), strings.Contains(d, "make"):
		return "costume"
	case strings.Contains(d, "effect"), d == "vfx":
		return "effects"
	case d == "crew":
		return "crew"
	}
	j := strings.ToLower(job)
	for _, r := range jobDepartments {
		for _, w := range r.words {
			if strings.Contains(j, w) {
				return r.dept
			}
		}
	}
	return "crew"
}

// jobDepartments place a job a source names without its department.
var jobDepartments = []struct {
	dept  string
	words []string
}{
	{"art", []string{"production design", "art direct", "set decor", "set design", "art department", "props", "property master", "construction"}},
	{"costume", []string{"costume", "make-up", "makeup", "hair", "wardrobe"}},
	{"effects", []string{"visual effect", "vfx", "special effect", "animation"}},
	{"camera", []string{"photography", "cinematograph", "camera", "steadicam"}},
	{"lighting", []string{"gaffer", "lighting", "electrician"}},
	{"editing", []string{"editor", "editing", "colorist", "colourist"}},
	{"sound", []string{"composer", "music", "sound", "foley", "mixer"}},
	{"writing", []string{"writer", "screenplay", "story", "novel", "teleplay", "author", "creator"}},
	{"directing", []string{"director", "directed"}},
	{"production", []string{"producer", "casting", "production manager", "line producer"}},
}

// jobRank orders the jobs of a department: the ones a page names first come first.
var jobRank = map[string]int{
	"director": 0, "co-director": 1, "series director": 2,
	"screenplay": 0, "writer": 1, "teleplay": 2, "story": 3, "novel": 4, "characters": 5, "creator": 6, "author": 7,
	"producer": 0, "executive producer": 1, "co-producer": 2, "casting": 3,
	"director of photography": 0, "cinematography": 0,
	"editor":                  0,
	"original music composer": 0, "music": 1, "composer": 1, "sound designer": 2,
	"production design": 0, "production designer": 0, "art direction": 1, "art director": 1, "set decoration": 2, "set decorator": 2,
	"costume design": 0, "costume designer": 0, "makeup artist": 1, "hairstylist": 2,
	"visual effects supervisor": 0,
}

func rankJob(job string) int {
	if r, ok := jobRank[strings.ToLower(job)]; ok {
		return r
	}
	return 50
}

// PersonIDs are a person's numbers at the online sources.
type PersonIDs struct {
	TMDB   string `json:"tmdb,omitempty"`
	IMDB   string `json:"imdb,omitempty"`
	TVDB   string `json:"tvdb,omitempty"`
	TVMaze string `json:"tvmaze,omitempty"`
}

func (p PersonIDs) empty() bool {
	return p.TMDB == "" && p.IMDB == "" && p.TVDB == "" && p.TVMaze == ""
}

// CreditIn is one credit as a source gives it.
type CreditIn struct {
	Name       string
	IDs        PersonIDs
	Photo      string // a web address or a picture beside the media
	Role       string // RoleCast, RoleGuest or RoleCrew
	Department string // crew: a key from Departments, or a source's own name for one
	Job        string // crew: Director, Screenplay, Set Decoration ...
	Character  string // cast and guests: who they played
	Order      int    // cast: billing order, 0 first
}

// Limits on what a source may give one title: a film's full crew is a few hundred.
const (
	maxCreditsPerTitle = 600
	maxPersonName      = 160
	maxCharacter       = 200
	maxJob             = 100
	maxBiography       = 8000
)

// PersonKey is how two spellings of a name are told to be the same person: case, accents,
// punctuation and spacing don't count.
func PersonKey(name string) string {
	var b strings.Builder
	space := false
	for _, r := range norm.NFKD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}

// PersonSortName is a name surname first ("Hanks Tom"), as a list of people is sorted, keeping
// a suffix (Jr., III) with the surname. One word, or a name that already has a comma, stays.
func PersonSortName(name string) string {
	name = strings.TrimSpace(name)
	if strings.Contains(name, ",") {
		return name
	}
	words := strings.Fields(name)
	if len(words) < 2 {
		return name
	}
	last := len(words) - 1
	switch strings.ToLower(strings.TrimSuffix(words[last], ".")) {
	case "jr", "sr", "ii", "iii", "iv":
		if last >= 2 {
			last--
		}
	}
	return strings.Join(words[last:], " ") + " " + strings.Join(words[:last], " ")
}

func cleanCredit(c CreditIn) (CreditIn, bool) {
	c.Name = cleanText(c.Name, maxPersonName, false)
	if PersonKey(c.Name) == "" {
		return c, false
	}
	switch c.Role {
	case RoleCast, RoleGuest:
		c.Department, c.Job = "", ""
		c.Character = cleanText(c.Character, maxCharacter, false)
	case RoleCrew:
		c.Job = cleanText(c.Job, maxJob, false)
		c.Department = DepartmentOf(c.Department, c.Job)
		c.Character = ""
		if c.Job == "" {
			c.Job = departmentLabel(c.Department)
		}
	default:
		return c, false
	}
	c.Photo = cleanText(c.Photo, 1000, false)
	if c.Photo != "" && !isRemote(c.Photo) && !strings.HasPrefix(c.Photo, "/") && !(len(c.Photo) > 2 && c.Photo[1] == ':') {
		c.Photo = "" // a picture is a web address or a whole path, never something relative
	}
	c.IDs.TMDB, c.IDs.IMDB = clip(strings.TrimSpace(c.IDs.TMDB), 20), clip(strings.TrimSpace(c.IDs.IMDB), 20)
	c.IDs.TVDB, c.IDs.TVMaze = clip(strings.TrimSpace(c.IDs.TVDB), 20), clip(strings.TrimSpace(c.IDs.TVMaze), 20)
	if c.Order < 0 {
		c.Order = 0
	}
	return c, true
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// personFor finds (or adds) the person a credit names: by an external id first, then by name
// among people with no id that says they are someone else. What the credit knows that the stored
// person doesn't (an id, a photo) is added to them.
func personFor(ctx context.Context, x execer, c CreditIn) (int64, error) {
	var id int64
	for _, k := range []struct{ col, v string }{{"tmdb_id", c.IDs.TMDB}, {"imdb_id", c.IDs.IMDB}, {"tvdb_id", c.IDs.TVDB}, {"tvmaze_id", c.IDs.TVMaze}} {
		if k.v == "" {
			continue
		}
		if err := x.QueryRowContext(ctx, `SELECT id FROM people WHERE `+k.col+` = ? ORDER BY id LIMIT 1`, k.v).Scan(&id); err == nil {
			break
		}
	}
	key := PersonKey(c.Name)
	if id == 0 {
		// Two people can share a name: one whose id at a source differs from the credit's is
		// someone else.
		err := x.QueryRowContext(ctx, `SELECT id FROM people WHERE key = ?
			AND (? = '' OR tmdb_id = '' OR tmdb_id = ?) AND (? = '' OR imdb_id = '' OR imdb_id = ?)
			AND (? = '' OR tvdb_id = '' OR tvdb_id = ?) AND (? = '' OR tvmaze_id = '' OR tvmaze_id = ?)
			ORDER BY (tmdb_id <> '') + (imdb_id <> '') + (tvdb_id <> '') + (tvmaze_id <> '') DESC, id LIMIT 1`,
			key, c.IDs.TMDB, c.IDs.TMDB, c.IDs.IMDB, c.IDs.IMDB, c.IDs.TVDB, c.IDs.TVDB, c.IDs.TVMaze, c.IDs.TVMaze).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	now := db.Now()
	if id == 0 {
		res, err := x.ExecContext(ctx, `INSERT INTO people(name, sort_name, key, photo, tmdb_id, imdb_id, tvdb_id, tvmaze_id, updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			c.Name, PersonSortName(c.Name), key, c.Photo, c.IDs.TMDB, c.IDs.IMDB, c.IDs.TVDB, c.IDs.TVMaze, now)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	// Fill in what is missing; an id another person already holds stays theirs.
	_, err := x.ExecContext(ctx, `UPDATE people SET
		tmdb_id = CASE WHEN tmdb_id = '' AND ?1 <> '' AND NOT EXISTS (SELECT 1 FROM people o WHERE o.tmdb_id = ?1) THEN ?1 ELSE tmdb_id END,
		imdb_id = CASE WHEN imdb_id = '' AND ?2 <> '' AND NOT EXISTS (SELECT 1 FROM people o WHERE o.imdb_id = ?2) THEN ?2 ELSE imdb_id END,
		tvdb_id = CASE WHEN tvdb_id = '' AND ?3 <> '' AND NOT EXISTS (SELECT 1 FROM people o WHERE o.tvdb_id = ?3) THEN ?3 ELSE tvdb_id END,
		tvmaze_id = CASE WHEN tvmaze_id = '' AND ?4 <> '' AND NOT EXISTS (SELECT 1 FROM people o WHERE o.tvmaze_id = ?4) THEN ?4 ELSE tvmaze_id END,
		photo = CASE WHEN ?5 = '' THEN photo WHEN photo = '' THEN ?5 WHEN ?6 = 1 AND photo LIKE 'http%' THEN ?5 ELSE photo END,
		updated_at = ?7
		WHERE id = ?8`, c.IDs.TMDB, c.IDs.IMDB, c.IDs.TVDB, c.IDs.TVMaze, c.Photo, boolInt(c.Photo != "" && !isRemote(c.Photo)), now, id)
	return id, err
}

// SetCredits replaces what one source says of a title's credits. target is TargetItem (a film or
// an episode) or TargetShow. An empty list says the source names nobody now.
func (s *Store) SetCredits(ctx context.Context, target string, id int64, source string, list []CreditIn) error {
	if target != TargetItem && target != TargetShow {
		return errors.New("credits: unknown target " + target)
	}
	if _, ok := creditSourceRank[source]; !ok {
		return errors.New("credits: unknown source " + source)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM credits WHERE target = ? AND target_id = ? AND source = ?`, target, id, source); err != nil {
		return err
	}
	seen := map[string]bool{}
	n := 0
	for _, raw := range list {
		c, ok := cleanCredit(raw)
		if !ok || n >= maxCreditsPerTitle {
			continue
		}
		pid, err := personFor(ctx, tx, c)
		if err != nil {
			return err
		}
		k := strconv.FormatInt(pid, 10) + "|" + c.Role + "|" + c.Department + "|" + strings.ToLower(c.Job) + "|" + strings.ToLower(c.Character)
		if seen[k] {
			continue
		}
		seen[k] = true
		n++
		if _, err := tx.ExecContext(ctx, `INSERT INTO credits(person_id, target, target_id, source, role, department, job, character, ord) VALUES(?,?,?,?,?,?,?,?,?)`,
			pid, target, id, source, c.Role, c.Department, c.Job, c.Character, c.Order); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PrunePeople forgets people no title names any more.
func (s *Store) PrunePeople(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM people WHERE NOT EXISTS (SELECT 1 FROM credits c WHERE c.person_id = people.id)`)
	return err
}

// Credit is one person's part in a title, as a page shows it.
type Credit struct {
	PersonID   int64  `json:"personId"`
	Name       string `json:"name"`
	HasPhoto   bool   `json:"hasPhoto"`
	Role       string `json:"role"`
	Department string `json:"department,omitempty"`
	Job        string `json:"job,omitempty"`
	Character  string `json:"character,omitempty"`
	Order      int    `json:"order"`
	// Episodes: how many of a season's episodes a guest star is in (a season's guests).
	Episodes int    `json:"episodes,omitempty"`
	Source   string `json:"source"`
}

// CrewGroup is one department's people.
type CrewGroup struct {
	Department string   `json:"department"`
	Label      string   `json:"label"`
	People     []Credit `json:"people"`
}

// Credits are a title's cast, guest stars and crew by department.
type Credits struct {
	Cast   []Credit    `json:"cast"`
	Guests []Credit    `json:"guests"`
	Crew   []CrewGroup `json:"crew"`
}

// Empty: nobody is credited.
func (c *Credits) Empty() bool { return c == nil || len(c.Cast)+len(c.Guests)+len(c.Crew) == 0 }

// Count is how many credits there are in all.
func (c *Credits) Count() int {
	if c == nil {
		return 0
	}
	n := len(c.Cast) + len(c.Guests)
	for _, g := range c.Crew {
		n += len(g.People)
	}
	return n
}

type creditRow struct {
	Credit
	part string // cast, guest or the crew department
}

func (s *Store) creditRows(ctx context.Context, target string, id int64) ([]creditRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.person_id, p.name, p.photo <> '', c.role, c.department, c.job, c.character, c.ord, c.source
		FROM credits c JOIN people p ON p.id = c.person_id WHERE c.target = ? AND c.target_id = ?`, target, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []creditRow
	for rows.Next() {
		var r creditRow
		if err := rows.Scan(&r.PersonID, &r.Name, &r.HasPhoto, &r.Role, &r.Department, &r.Job, &r.Character, &r.Order, &r.Source); err != nil {
			return nil, err
		}
		r.part = r.Role
		if r.Role == RoleCrew {
			r.part = r.Department
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// layered keeps, for each part, the rows of the best source that has that part.
func layered(rows []creditRow) map[string][]Credit {
	best := map[string]int{}
	for _, r := range rows {
		rank := creditSourceRank[r.Source]
		if b, ok := best[r.part]; !ok || rank < b {
			best[r.part] = rank
		}
	}
	out := map[string][]Credit{}
	for _, r := range rows {
		if creditSourceRank[r.Source] == best[r.part] {
			out[r.part] = append(out[r.part], r.Credit)
		}
	}
	return out
}

// merged folds one person's several rows in a part into one: two characters, or two jobs.
func merged(list []Credit, crew bool) []Credit {
	at := map[int64]int{}
	var out []Credit
	for _, c := range list {
		i, ok := at[c.PersonID]
		if !ok {
			at[c.PersonID] = len(out)
			out = append(out, c)
			continue
		}
		o := &out[i]
		switch {
		case crew && c.Job != "" && !containsFold(strings.Split(o.Job, ", "), c.Job):
			if rankJob(c.Job) < rankJob(strings.SplitN(o.Job, ", ", 2)[0]) {
				o.Job = c.Job + ", " + o.Job
			} else {
				o.Job += ", " + c.Job
			}
		case !crew && c.Character != "" && !containsFold(strings.Split(o.Character, " / "), c.Character):
			o.Character += " / " + c.Character
		}
		o.Order = min(o.Order, c.Order)
		o.Episodes += c.Episodes
	}
	return out
}

func sortCast(list []Credit) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Episodes != list[j].Episodes {
			return list[i].Episodes > list[j].Episodes
		}
		if list[i].Order != list[j].Order {
			return list[i].Order < list[j].Order
		}
		return list[i].Name < list[j].Name
	})
}

func sortCrew(list []Credit) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := rankJob(strings.SplitN(list[i].Job, ", ", 2)[0]), rankJob(strings.SplitN(list[j].Job, ", ", 2)[0])
		if a != b {
			return a < b
		}
		if list[i].Job != list[j].Job {
			return list[i].Job < list[j].Job
		}
		if list[i].Order != list[j].Order {
			return list[i].Order < list[j].Order
		}
		return list[i].Name < list[j].Name
	})
}

func assemble(parts map[string][]Credit) *Credits {
	out := &Credits{Cast: merged(parts[RoleCast], false), Guests: merged(parts[RoleGuest], false), Crew: []CrewGroup{}}
	sortCast(out.Cast)
	sortCast(out.Guests)
	if out.Cast == nil {
		out.Cast = []Credit{}
	}
	if out.Guests == nil {
		out.Guests = []Credit{}
	}
	for _, d := range Departments {
		if people := merged(parts[d.Key], true); len(people) > 0 {
			sortCrew(people)
			out.Crew = append(out.Crew, CrewGroup{Department: d.Key, Label: d.Label, People: people})
		}
	}
	return out
}

// ItemCredits are a film's credits, or an episode's: its show's regular cast, its own guest
// stars, and its own crew where it names a department (else the show's).
func (s *Store) ItemCredits(ctx context.Context, it *Item) (*Credits, error) {
	rows, err := s.creditRows(ctx, TargetItem, it.ID)
	if err != nil {
		return nil, err
	}
	parts := layered(rows)
	if it.ShowID != nil {
		showRows, err := s.creditRows(ctx, TargetShow, *it.ShowID)
		if err != nil {
			return nil, err
		}
		show := layered(showRows)
		for part, list := range show {
			if part == RoleGuest {
				continue
			}
			if _, own := parts[part]; !own {
				parts[part] = list
			}
		}
		// A regular named as a guest of this episode is a regular here.
		if regs := parts[RoleCast]; len(regs) > 0 {
			in := map[int64]bool{}
			for _, c := range regs {
				in[c.PersonID] = true
			}
			kept := parts[RoleGuest][:0:0]
			for _, g := range parts[RoleGuest] {
				if !in[g.PersonID] {
					kept = append(kept, g)
				}
			}
			parts[RoleGuest] = kept
		}
	}
	return assemble(parts), nil
}

// ShowCredits are a show's credits. With a season (>= 0) they add the guest stars of that
// season's episodes this account may see, most episodes first.
func (s *Store) ShowCredits(ctx context.Context, showID int64, season int, access *Access) (*Credits, error) {
	rows, err := s.creditRows(ctx, TargetShow, showID)
	if err != nil {
		return nil, err
	}
	parts := layered(rows)
	delete(parts, RoleGuest)
	if season >= 0 {
		guests, err := s.seasonGuests(ctx, showID, season, access)
		if err != nil {
			return nil, err
		}
		regular := map[int64]bool{}
		for _, c := range parts[RoleCast] {
			regular[c.PersonID] = true
		}
		for _, g := range guests {
			if !regular[g.PersonID] {
				parts[RoleGuest] = append(parts[RoleGuest], g)
			}
		}
	}
	return assemble(parts), nil
}

// seasonGuests are the guest stars of a season's episodes, one row per person and episode, from
// each episode's best source of guest stars.
func (s *Store) seasonGuests(ctx context.Context, showID int64, season int, access *Access) ([]Credit, error) {
	ic, ia := access.ItemClause()
	args := append([]any{showID, season}, ia...)
	rows, err := s.DB.QueryContext(ctx, `SELECT c.target_id, c.person_id, p.name, p.photo <> '', c.character, c.ord, c.source
		FROM credits c JOIN people p ON p.id = c.person_id JOIN items i ON i.id = c.target_id
		WHERE c.target = 'item' AND c.role = 'guest' AND i.show_id = ? AND i.season = ? AND i.missing = 0 AND `+ic, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type row struct {
		ep int64
		c  Credit
	}
	var all []row
	best := map[int64]int{}
	for rows.Next() {
		var r row
		r.c.Role = RoleGuest
		if err := rows.Scan(&r.ep, &r.c.PersonID, &r.c.Name, &r.c.HasPhoto, &r.c.Character, &r.c.Order, &r.c.Source); err != nil {
			return nil, err
		}
		if b, ok := best[r.ep]; !ok || creditSourceRank[r.c.Source] < b {
			best[r.ep] = creditSourceRank[r.c.Source]
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []Credit
	counted := map[[2]int64]bool{}
	for _, r := range all {
		if creditSourceRank[r.c.Source] != best[r.ep] || counted[[2]int64{r.ep, r.c.PersonID}] {
			continue
		}
		counted[[2]int64{r.ep, r.c.PersonID}] = true
		r.c.Episodes = 1
		out = append(out, r.c)
	}
	return out, nil
}

// Person is someone credited on this server.
type Person struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	SortName   string    `json:"sortName"`
	HasPhoto   bool      `json:"hasPhoto"`
	Birthday   string    `json:"birthday,omitempty"`
	Deathday   string    `json:"deathday,omitempty"`
	Birthplace string    `json:"birthplace,omitempty"`
	Biography  string    `json:"biography,omitempty"`
	KnownFor   string    `json:"knownFor,omitempty"` // a department key, or "acting"
	IDs        PersonIDs `json:"ids"`
	// Photo is where the picture is: served through /api/people/{id}/photo, never sent.
	Photo     string `json:"-"`
	DetailsAt int64  `json:"-"`
}

// Person loads one person.
func (s *Store) Person(ctx context.Context, id int64) (*Person, error) {
	p := &Person{ID: id}
	err := s.DB.QueryRowContext(ctx, `SELECT name, sort_name, photo, birthday, deathday, birthplace, biography, known_for, tmdb_id, imdb_id, tvdb_id, tvmaze_id, details_at
		FROM people WHERE id = ?`, id).Scan(&p.Name, &p.SortName, &p.Photo, &p.Birthday, &p.Deathday, &p.Birthplace, &p.Biography, &p.KnownFor,
		&p.IDs.TMDB, &p.IDs.IMDB, &p.IDs.TVDB, &p.IDs.TVMaze, &p.DetailsAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.HasPhoto = p.Photo != ""
	return p, nil
}

// PersonDetails is what an online source says about a person beyond their name.
type PersonDetails struct {
	Photo, Birthday, Deathday, Birthplace, Biography, KnownFor string
	IDs                                                        PersonIDs
}

// SetPersonDetails stores what a source said about a person. A photo of the person's own
// beside the media is kept over a web address.
func (s *Store) SetPersonDetails(ctx context.Context, id int64, d PersonDetails) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE people SET
		birthday = CASE WHEN ?1 <> '' THEN ?1 ELSE birthday END, deathday = CASE WHEN ?2 <> '' THEN ?2 ELSE deathday END,
		birthplace = CASE WHEN ?3 <> '' THEN ?3 ELSE birthplace END, biography = CASE WHEN ?4 <> '' THEN ?4 ELSE biography END,
		known_for = CASE WHEN ?5 <> '' THEN ?5 ELSE known_for END,
		photo = CASE WHEN ?6 = '' THEN photo WHEN photo = '' OR photo LIKE 'http%' THEN ?6 ELSE photo END,
		imdb_id = CASE WHEN imdb_id = '' AND ?7 <> '' AND NOT EXISTS (SELECT 1 FROM people o WHERE o.imdb_id = ?7) THEN ?7 ELSE imdb_id END,
		details_at = ?8, updated_at = ?8 WHERE id = ?9`,
		cleanDate(d.Birthday), cleanDate(d.Deathday), cleanText(d.Birthplace, 200, false), cleanText(d.Biography, maxBiography, true),
		cleanText(d.KnownFor, 40, false), cleanText(d.Photo, 1000, false), clip(strings.TrimSpace(d.IDs.IMDB), 20), db.Now(), id)
	return err
}

// MarkPersonLookedUp records that a source was asked about a person, found or not, so it isn't
// asked again on every visit.
func (s *Store) MarkPersonLookedUp(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE people SET details_at = ? WHERE id = ?`, db.Now(), id)
	return err
}

// cleanDate keeps a date in the YYYY-MM-DD (or YYYY) form sources write.
func cleanDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		s = s[:10]
	}
	for i, r := range s {
		if !(unicode.IsDigit(r) || (r == '-' && (i == 4 || i == 7))) {
			return ""
		}
	}
	if len(s) != 4 && len(s) != 7 && len(s) != 10 {
		return ""
	}
	return s
}

// The parts of a person's titles: what they are seen (and heard) in, then each department.
const (
	FilmActing = "acting"
)

// Filmography is a person's titles in this library that an account may see, by part.
type Filmography struct {
	Groups []FilmGroup `json:"groups"`
}

// FilmGroup is one part of a filmography: their acting, or one department's work.
type FilmGroup struct {
	Key    string      `json:"key"`
	Label  string      `json:"label"`
	Titles []FilmTitle `json:"titles"`
}

// FilmTitle is one film or show and what the person did in it. An episode's guest part or crew
// work is its show's, with how many episodes.
type FilmTitle struct {
	Target    string `json:"target"` // item or show
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Year      int    `json:"year,omitempty"`
	Character string `json:"character,omitempty"`
	Job       string `json:"job,omitempty"`
	Episodes  int    `json:"episodes,omitempty"`
	Guest     bool   `json:"guest,omitempty"`
	sortTitle string
}

// PersonVisible: the person is credited on at least one title this account may see. A person
// credited only on hidden titles doesn't exist for that account.
func (s *Store) PersonVisible(ctx context.Context, id int64, access *Access) bool {
	q, args := visibleCreditsQuery("SELECT 1", access)
	var one int
	return s.DB.QueryRowContext(ctx, q+" LIMIT 1", args(id)...).Scan(&one) == nil
}

// visibleCreditsQuery selects a person's credits on titles the account may see: films (not
// episodes, not attached extras), shows, and episodes (named by their show). It returns the
// query and what makes its arguments for one person: the person's ID stands before each part's
// own limits.
func visibleCreditsQuery(sel string, access *Access) (string, func(int64) []any) {
	ic, ia := access.ItemClause()
	sc, sa := access.ShowClause()
	q := sel + ` FROM (
		SELECT c.role, c.department, c.job, c.character, c.ord, c.source, 'item' AS target, i.id AS tid, i.title AS title, i.sort_title AS sort_title, IFNULL(i.year, 0) AS year, 0 AS ep, c.target_id AS src_id
		  FROM credits c JOIN items i ON i.id = c.target_id
		  WHERE c.person_id = ? AND c.target = 'item' AND i.show_id IS NULL AND i.missing = 0 AND ` + hideLinked + ` AND ` + ic + `
		UNION ALL
		SELECT c.role, c.department, c.job, c.character, c.ord, c.source, 'show', s.id, s.title, s.sort_title, IFNULL(s.year, 0), 0, c.target_id
		  FROM credits c JOIN shows s ON s.id = c.target_id
		  WHERE c.person_id = ? AND c.target = 'show' AND s.missing = 0 AND ` + sc + `
		UNION ALL
		SELECT c.role, c.department, c.job, c.character, c.ord, c.source, 'show', s.id, s.title, s.sort_title, IFNULL(s.year, 0), i.id, c.target_id
		  FROM credits c JOIN items i ON i.id = c.target_id JOIN shows s ON s.id = i.show_id
		  WHERE c.person_id = ? AND c.target = 'item' AND i.missing = 0 AND s.missing = 0 AND ` + ic + ` AND ` + sc + `
	) AS v`
	return q, func(person int64) []any {
		args := append([]any{person}, ia...)
		args = append(append(args, person), sa...)
		args = append(append(args, person), ia...)
		return append(args, sa...)
	}
}

// Filmography lists a person's titles that the account may see. Each title is named once per
// part, newest first.
func (s *Store) Filmography(ctx context.Context, personID int64, access *Access) (*Filmography, error) {
	q, args := visibleCreditsQuery("SELECT v.role, v.department, v.job, v.character, v.ord, v.target, v.tid, v.title, v.sort_title, v.year, v.ep", access)
	// The sources' order isn't layered here: a person's part in a title is named if any source
	// names it, which is what a filmography is for.
	rows, err := s.DB.QueryContext(ctx, q, args(personID)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type key struct {
		part, target string
		id           int64
	}
	at := map[key]int{}
	eps := map[key]map[int64]bool{}
	groups := map[string][]FilmTitle{}
	for rows.Next() {
		var role, dept, job, character, target, title, sortTitle string
		var ord, year int
		var tid, ep int64
		if err := rows.Scan(&role, &dept, &job, &character, &ord, &target, &tid, &title, &sortTitle, &year, &ep); err != nil {
			return nil, err
		}
		part := FilmActing
		if role == RoleCrew {
			part = dept
		}
		k := key{part, target, tid}
		i, ok := at[k]
		if !ok {
			i = len(groups[part])
			at[k] = i
			groups[part] = append(groups[part], FilmTitle{Target: target, ID: tid, Title: title, Year: year, sortTitle: sortTitle})
		}
		ft := &groups[part][i]
		switch {
		case role == RoleCrew && job != "" && !containsFold(strings.Split(ft.Job, ", "), job):
			ft.Job = strings.TrimPrefix(ft.Job+", "+job, ", ")
		case role != RoleCrew && character != "" && !containsFold(strings.Split(ft.Character, " / "), character):
			ft.Character = strings.TrimPrefix(ft.Character+" / "+character, " / ")
		}
		if ep != 0 {
			if eps[k] == nil {
				eps[k] = map[int64]bool{}
			}
			eps[k][ep] = true
			ft.Guest = ft.Guest || role == RoleGuest
		} else if role == RoleCast {
			ft.Guest = false
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for k, set := range eps {
		g := groups[k.part]
		g[at[k]].Episodes = len(set)
	}
	out := &Filmography{Groups: []FilmGroup{}}
	order := append([]Department{{FilmActing, "Acting"}}, Departments...)
	for _, d := range order {
		list := groups[d.Key]
		if len(list) == 0 {
			continue
		}
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Year != list[j].Year {
				return list[i].Year > list[j].Year
			}
			return list[i].sortTitle < list[j].sortTitle
		})
		out.Groups = append(out.Groups, FilmGroup{Key: d.Key, Label: d.Label, Titles: list})
	}
	return out, nil
}

// FoundPerson is a person a search found, with how many titles of theirs the account may see.
type FoundPerson struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	HasPhoto bool   `json:"hasPhoto"`
	KnownFor string `json:"knownFor,omitempty"`
	Titles   int    `json:"titles"`
}

// SearchPeople finds people by name among those credited on titles the account may see, the
// best known (most titles) first.
func (s *Store) SearchPeople(ctx context.Context, text string, limit int, access *Access) ([]FoundPerson, error) {
	k := PersonKey(text)
	if len([]rune(k)) < 2 {
		return []FoundPerson{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	like := "%" + strings.NewReplacer("%", "", "_", "").Replace(k) + "%"
	// Candidates by name first (an index walk of a small table), then only those with a title
	// the account may see are counted.
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, photo <> '', known_for FROM people WHERE key LIKE ?
		ORDER BY (key = ?) DESC, (key LIKE ?) DESC, sort_name COLLATE NOCASE LIMIT 200`, like, k, strings.TrimSuffix(like[1:], "%")+"%")
	if err != nil {
		return nil, err
	}
	var cands []FoundPerson
	for rows.Next() {
		var p FoundPerson
		if err := rows.Scan(&p.ID, &p.Name, &p.HasPhoto, &p.KnownFor); err != nil {
			rows.Close()
			return nil, err
		}
		cands = append(cands, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	q, args := visibleCreditsQuery("SELECT COUNT(DISTINCT v.target || v.tid)", access)
	out := []FoundPerson{}
	for _, p := range cands {
		if err := s.DB.QueryRowContext(ctx, q, args(p.ID)...).Scan(&p.Titles); err != nil {
			return nil, err
		}
		if p.Titles > 0 {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Titles > out[j].Titles })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// PeopleNeedingDetails are people with an id at TMDB who were never looked up, or not for a
// month, most credited first: what a background pass asks about.
func (s *Store) PeopleNeedingDetails(ctx context.Context, limit int) ([]*Person, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT p.id FROM people p WHERE p.tmdb_id <> '' AND p.details_at < ?
		ORDER BY (SELECT COUNT(*) FROM credits c WHERE c.person_id = p.id) DESC LIMIT ?`, db.Now()-30*86400, limit)
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
	out := make([]*Person, 0, len(ids))
	for _, id := range ids {
		if p, err := s.Person(ctx, id); err == nil {
			out = append(out, p)
		}
	}
	return out, nil
}
