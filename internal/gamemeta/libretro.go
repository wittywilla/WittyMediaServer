package gamemeta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// libretro's game database: published files, one per console, listing every known
// cartridge and disc with its checksums, who made it and how it was rated. A console's
// file is downloaded once, and matching happens here on this server, so nothing about
// the owner's games is sent anywhere. Nothing from it is in this repository: its
// licence (CC BY-SA 4.0) allows bundling, but its size and its updates argue against.
//
// The pictures are a separate source, LibretroArt, with its own switch: they state no
// licence, as the box art of commercial games wouldn't be theirs to license anyway.

// rdbNames lists each console's files, which are also its folders of pictures.
var rdbNames = map[string][]string{
	"nes":    {"Nintendo - Nintendo Entertainment System", "Nintendo - Family Computer Disk System"},
	"snes":   {"Nintendo - Super Nintendo Entertainment System", "Nintendo - Satellaview", "Nintendo - Sufami Turbo"},
	"n64":    {"Nintendo - Nintendo 64"},
	"gc":     {"Nintendo - GameCube"},
	"wii":    {"Nintendo - Wii"},
	"wiiu":   {"Nintendo - Wii U"},
	"gb":     {"Nintendo - Game Boy", "Nintendo - Game Boy Color"},
	"gba":    {"Nintendo - Game Boy Advance"},
	"nds":    {"Nintendo - Nintendo DS"},
	"3ds":    {"Nintendo - Nintendo 3DS"},
	"vb":     {"Nintendo - Virtual Boy"},
	"md":     {"Sega - Mega Drive - Genesis"},
	"sms":    {"Sega - Master System - Mark III"},
	"gg":     {"Sega - Game Gear"},
	"segacd": {"Sega - Mega-CD - Sega CD"},
	"32x":    {"Sega - 32X"},
	"saturn": {"Sega - Saturn"},
	"dc":     {"Sega - Dreamcast"},
	"ps1":    {"Sony - PlayStation"},
	"ps2":    {"Sony - PlayStation 2"},
	"psp":    {"Sony - PlayStation Portable"},
	"pce":    {"NEC - PC Engine - TurboGrafx 16", "NEC - PC Engine CD - TurboGrafx-CD", "NEC - PC Engine SuperGrafx"},
	"a2600":  {"Atari - 2600"},
	"a5200":  {"Atari - 5200"},
	"a7800":  {"Atari - 7800"},
	"lynx":   {"Atari - Lynx"},
	"jaguar": {"Atari - Jaguar"},
	"ws":     {"Bandai - WonderSwan", "Bandai - WonderSwan Color"},
	"ngp":    {"SNK - Neo Geo Pocket", "SNK - Neo Geo Pocket Color"},
	"neogeo": {"SNK - Neo Geo"},
	"arcade": {"MAME"},
}

// rdbFresh is how long a downloaded list is used before asking whether there's a newer
// one. The lists don't change much, and asking costs its host next to nothing.
const rdbFresh = 30 * 24 * time.Hour

// Libretro is the local database.
type Libretro struct {
	HTTP *http.Client
	Dir  string
	// Base is only changed by tests.
	Base string

	once sync.Once
	c    *Client

	mu      sync.Mutex
	console string
	db      *libDB
}

func (s *Libretro) init() {
	s.once.Do(func() {
		s.c = NewClient("libretro", s.HTTP, 500*time.Millisecond)
		if s.Base == "" {
			s.Base = "https://raw.githubusercontent.com/libretro/libretro-database/master/rdb"
		}
	})
}

// About implements Source.
func (s *Libretro) About() About {
	return About{ID: "libretro", Name: "libretro database", Site: "https://github.com/libretro/libretro-database",
		Why:     "Recognises your exact files by checksum, on this server, for no requests at all.",
		Gives:   []string{"exact file match", "dates", "developer", "publisher", "genre", "players", "critic score", "age ratings"},
		Fields:  []KeyField{},
		Cost:    "Free, no account",
		Limit:   "Each console's list is downloaded once and checked for updates monthly.",
		Sends:   "Nothing about your games. It downloads a list for each console you have, and matches here",
		Credit:  "Game data from the libretro database",
		License: "CC BY-SA 4.0",
	}
}

// libDB is one console's list, indexed.
type libDB struct {
	recs     []rdbRecord
	system   []string // which file each record came from, by recSystem
	recFile  []uint8
	byCRC    map[uint32]int32
	bySerial map[string]int32
	byKey    map[string][]int32
	keys     []string
	byWord   map[string][]int32 // word -> index into keys
	allFiles []string
}

func (d *libDB) add(r *rdbRecord, file uint8) {
	if r.Name == "" {
		return
	}
	i := int32(len(d.recs))
	d.recs = append(d.recs, *r)
	d.recFile = append(d.recFile, file)
	if r.HasCRC {
		if _, dup := d.byCRC[r.CRC]; !dup {
			d.byCRC[r.CRC] = i
		}
	}
	if r.Serial != "" {
		if _, dup := d.bySerial[normalSerial(r.Serial)]; !dup {
			d.bySerial[normalSerial(r.Serial)] = i
		}
	}
	k := NameKey(r.Name)
	if _, ok := d.byKey[k]; !ok {
		ki := int32(len(d.keys))
		d.keys = append(d.keys, k)
		for _, w := range strings.Fields(k) {
			d.byWord[w] = append(d.byWord[w], ki)
		}
	}
	d.byKey[k] = append(d.byKey[k], i)
}

// gcRegions name the regions in a Redump-style GameCube or Wii serial, from the last
// letter of the game's id.
var gcRegions = map[byte]string{'E': "USA", 'J': "JPN", 'P': "EUR", 'X': "EUR", 'Y': "FAH", 'D': "NOE", 'S': "ESP", 'F': "FRA", 'I': "ITA", 'H': "HOL"}

// serialForms lists how a list might write a disc's product code. Discs of a set
// add which disc they are, counting from 0.
func serialForms(console, serial string, disc int) []string {
	if serial == "" {
		return nil
	}
	out := []string{serial}
	if disc > 0 {
		out = append(out, serial+"-"+strconv.Itoa(disc-1))
	} else {
		out = append(out, serial+"-0")
	}
	if len(serial) == 6 && (console == "gc" || console == "wii") {
		if r, ok := gcRegions[serial[3]]; ok {
			prefix := "DL-DOL-"
			if console == "wii" {
				prefix = "RVL-"
			}
			out = append(out, prefix+serial[:4]+"-"+r)
		}
	}
	return out
}

// normalSerial lets SLUS-00594, SLUS_005.94 and slus00594 all be the same.
func normalSerial(s string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(s))
}

// fetchFile gets a console's list, reusing what's on disk when it's recent, and when
// the host can't be reached. Lists aren't secret: a plain file in the cache folder.
func (s *Libretro) fetchFile(ctx context.Context, name string) ([]byte, error) {
	s.init()
	if s.Dir == "" {
		return nil, errors.New("no cache folder")
	}
	p := filepath.Join(s.Dir, name+".rdb")
	st, statErr := os.Stat(p)
	if statErr == nil && time.Since(st.ModTime()) < rdbFresh {
		return os.ReadFile(p)
	}
	// Not every console's extras have a list. There's no need to keep asking.
	if m, err := os.Stat(p + ".missing"); err == nil && time.Since(m.ModTime()) < rdbFresh {
		return nil, ErrNotFound
	}
	req, err := http.NewRequestWithContext(ctx, "GET", s.Base+"/"+url.PathEscape(name)+".rdb", nil)
	if err != nil {
		return nil, err
	}
	etag, _ := os.ReadFile(p + ".etag")
	if statErr == nil && len(etag) > 0 && len(etag) < 200 {
		req.Header.Set("If-None-Match", strings.TrimSpace(string(etag)))
	}
	res, err := s.c.Do(req)
	if err != nil {
		if statErr == nil {
			return os.ReadFile(p)
		}
		return nil, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case 200:
	case 304:
		now := time.Now()
		_ = os.Chtimes(p, now, now)
		return os.ReadFile(p)
	default:
		if statErr == nil {
			return os.ReadFile(p)
		}
		if res.StatusCode == 404 && os.MkdirAll(s.Dir, 0o755) == nil {
			_ = os.WriteFile(p+".missing", nil, 0o644)
		}
		return nil, s.c.statusError(res)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, rdbMaxFile+1))
	if err != nil {
		if statErr == nil {
			return os.ReadFile(p)
		}
		return nil, s.c.scrub(err)
	}
	if len(b) > rdbMaxFile || parseRDB(b, func(*rdbRecord) {}) != nil {
		if statErr == nil {
			return os.ReadFile(p)
		}
		return nil, errBadRDB
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	if e := res.Header.Get("ETag"); e != "" {
		_ = os.WriteFile(p+".etag", []byte(e), 0o644)
	}
	return b, nil
}

// load gets a console's list ready. One console stays in memory at a time.
func (s *Libretro) load(ctx context.Context, console string) (*libDB, error) {
	names, ok := rdbNames[console]
	if !ok {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.console == console && s.db != nil {
		return s.db, nil
	}
	d := &libDB{byCRC: map[uint32]int32{}, bySerial: map[string]int32{}, byKey: map[string][]int32{}, byWord: map[string][]int32{}, allFiles: names}
	var firstErr error
	got := 0
	for i, name := range names {
		b, err := s.fetchFile(ctx, name)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := parseRDB(b, func(r *rdbRecord) { d.add(r, uint8(i)) }); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		got++
	}
	if got == 0 && firstErr != nil {
		return nil, firstErr
	}
	s.console, s.db = console, d
	return d, nil
}

// Prepare implements Preparer: the download can wait for the first game that needs it,
// but a failure is better found here, once, than by every game in the batch.
func (s *Libretro) Prepare(ctx context.Context, console string, qs []Query) error {
	_, err := s.load(ctx, console)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// best picks among records of one game: the region the file says it's from if there's
// a match, otherwise whichever is most useful to an English speaker.
func (d *libDB) best(ids []int32, tags string) int32 {
	want := strings.ToLower(tags)
	best, bestRank := ids[0], 99
	for _, i := range ids {
		_, t := CleanName(d.recs[i].Name)
		rank := regionRank(t)
		if want != "" && regionRank(want) == rank {
			rank -= 20
		}
		// A record with more known about it beats one with less.
		if d.recs[i].Developer == "" {
			rank++
		}
		if rank < bestRank {
			best, bestRank = i, rank
		}
	}
	return best
}

// closest finds names that might be meant: those sharing the most words, scored. It
// doesn't look at every name in the list, which an old machine would feel.
func (d *libDB) closest(title string, want int) []struct {
	key   string
	score int
} {
	shared := map[int32]int{}
	for _, w := range words(title) {
		list := d.byWord[w]
		if len(list) > 4000 {
			continue // "super", "the": they don't tell games apart
		}
		for _, k := range list {
			shared[k]++
		}
	}
	type pair struct{ k, n int32 }
	pairs := make([]pair, 0, len(shared))
	for k, n := range shared {
		pairs = append(pairs, pair{k, int32(n)})
	}
	sort.Slice(pairs, func(a, b int) bool {
		if pairs[a].n != pairs[b].n {
			return pairs[a].n > pairs[b].n
		}
		return pairs[a].k < pairs[b].k
	})
	if len(pairs) > 150 {
		pairs = pairs[:150]
	}
	out := make([]struct {
		key   string
		score int
	}, 0, len(pairs))
	for _, p := range pairs {
		k := d.keys[p.k]
		out = append(out, struct {
			key   string
			score int
		}{k, NameScore(title, k)})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].score > out[b].score })
	if len(out) > want {
		out = out[:want]
	}
	return out
}

// find is what Lookup and LibretroArt share: which record a file is.
func (s *Libretro) find(ctx context.Context, q Query) (*libDB, int32, string, int, error) {
	d, err := s.load(ctx, q.Console)
	if err != nil {
		return nil, 0, "", 0, err
	}
	for _, crc := range q.CRCs {
		if i, ok := d.byCRC[crc]; ok {
			return d, i, MethodChecksum, 100, nil
		}
	}
	for _, form := range serialForms(q.Console, q.Serial, DiscNumber(q.Tags)) {
		if i, ok := d.bySerial[normalSerial(form)]; ok {
			return d, i, MethodSerial, 98, nil
		}
	}
	if ids := d.byKey[NameKey(q.Title)]; len(ids) > 0 {
		return d, d.best(ids, q.Tags), MethodName, nameConfidence(100), nil
	}
	if c := d.closest(q.Title, 1); len(c) > 0 && c[0].score >= AcceptScore {
		return d, d.best(d.byKey[c[0].key], q.Tags), MethodName, nameConfidence(c[0].score), nil
	}
	return nil, 0, "", 0, ErrNotFound
}

func (d *libDB) info(i int32) *Info {
	r := &d.recs[i]
	in := &Info{ID: r.Name, Title: Line(r.Name), Developer: Line(r.Developer), Publisher: Line(r.Publisher), Franchise: Line(r.Franchise)}
	in.Genres = lines(strings.Split(r.Genre, ","), 8)
	if r.Users > 0 && r.Users <= 64 {
		in.Players = r.Users
	}
	if r.ReleaseYear > 1950 && r.ReleaseYear < 2100 {
		in.Released = strconv.Itoa(r.ReleaseYear)
		if r.ReleaseMonth >= 1 && r.ReleaseMonth <= 12 {
			in.Released = fmt.Sprintf("%d-%02d", r.ReleaseYear, r.ReleaseMonth)
		}
	}
	if r.Edge > 0 && r.Edge <= 10 {
		in.Scores = append(in.Scores, Score{Source: "Edge", Kind: "critic", Value: float64(r.Edge) * 10})
	}
	if r.Famitsu > 0 && r.Famitsu <= 40 {
		in.Scores = append(in.Scores, Score{Source: "Famitsu", Kind: "critic", Value: float64(r.Famitsu) * 2.5})
	}
	for _, a := range [][2]string{{"ESRB", r.ESRB}, {"PEGI", r.PEGI}, {"CERO", r.CERO}, {"ELSPA", r.ELSPA}} {
		if x, ok := NewAge(a[0], a[1]); ok {
			in.Ages = append(in.Ages, x)
		}
	}
	// Records of the same game fill each other's gaps: the metadata files behind these
	// don't cover every revision.
	for _, j := range d.byKey[NameKey(r.Name)] {
		o := &d.recs[j]
		if j == i {
			continue
		}
		if in.Developer == "" {
			in.Developer = Line(o.Developer)
		}
		if in.Publisher == "" {
			in.Publisher = Line(o.Publisher)
		}
		if len(in.Genres) == 0 {
			in.Genres = lines(strings.Split(o.Genre, ","), 8)
		}
		if in.Players == 0 && o.Users > 0 && o.Users <= 64 {
			in.Players = o.Users
		}
		if in.Released == "" && o.ReleaseYear > 1950 && o.ReleaseYear < 2100 {
			in.Released = strconv.Itoa(o.ReleaseYear)
		}
		if len(in.Ages) == 0 {
			for _, a := range [][2]string{{"ESRB", o.ESRB}, {"PEGI", o.PEGI}, {"CERO", o.CERO}, {"ELSPA", o.ELSPA}} {
				if x, ok := NewAge(a[0], a[1]); ok {
					in.Ages = append(in.Ages, x)
				}
			}
		}
	}
	return in
}

// Lookup implements Source.
func (s *Libretro) Lookup(ctx context.Context, q Query) (*Info, error) {
	d, i, method, conf, err := s.find(ctx, q)
	if err != nil {
		return nil, err
	}
	in := d.info(i)
	in.Method, in.Confidence = method, conf
	return in, nil
}

// Search implements Source.
func (s *Libretro) Search(ctx context.Context, console, title string) ([]Candidate, error) {
	d, err := s.load(ctx, console)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, c := range d.closest(title, 20) {
		for _, i := range d.byKey[c.key] {
			r := &d.recs[i]
			out = append(out, Candidate{Source: "libretro", ID: r.Name, Title: Line(r.Name), Year: r.ReleaseYear, Score: c.score})
			if len(out) >= 30 {
				return out, nil
			}
		}
	}
	return out, nil
}

// Fetch implements Source. An id here is the game's full name in the list.
func (s *Libretro) Fetch(ctx context.Context, console, id string) (*Info, error) {
	d, err := s.load(ctx, console)
	if err != nil {
		return nil, err
	}
	for _, i := range d.byKey[NameKey(id)] {
		if d.recs[i].Name == id {
			in := d.info(i)
			in.Method, in.Confidence = MethodManual, 100
			return in, nil
		}
	}
	return nil, ErrNotFound
}

// Test implements Source.
func (s *Libretro) Test(ctx context.Context) (string, error) {
	if _, err := s.load(ctx, "vb"); err != nil {
		return "", err
	}
	return "The database can be reached.", nil
}

// ---------------------------------------------------------------- pictures

// LibretroArt adds libretro's pictures to the database's matches. It's a source of its
// own so that it can be switched off by itself.
type LibretroArt struct {
	DB *Libretro
	// Base is only changed by tests.
	Base string
}

// About implements Source.
func (s *LibretroArt) About() About {
	return About{ID: "libretroart", Name: "libretro thumbnails", Site: "https://github.com/libretro-thumbnails/libretro-thumbnails",
		Why:     "Box art, title screens and in-game pictures, with no account needed.",
		Gives:   []string{"cover", "screenshots", "title screen"},
		Fields:  []KeyField{},
		Cost:    "Free, no account",
		Limit:   "No stated limit. This server asks for four pictures a second at most, once per picture, and keeps them.",
		Sends:   "The names of your games, as the database lists them",
		Credit:  "Pictures from the libretro thumbnails project",
		License: "None stated; the pictures themselves belong to their publishers. Kept on this server only",
	}
}

func (s *LibretroArt) base() string {
	if s.Base != "" {
		return s.Base
	}
	return "https://thumbnails.libretro.com"
}

// thumbName makes a game's name into the name of its picture.
func thumbName(name string) string {
	return strings.NewReplacer("&", "_", "*", "_", "/", "_", ":", "_", "`", "_", `"`, "_", "<", "_", ">", "_", "?", "_", `\`, "_", "|", "_").Replace(name)
}

func (s *LibretroArt) picture(system, kind, name string) string {
	return s.base() + "/" + url.PathEscape(system) + "/" + kind + "/" + url.PathEscape(thumbName(name)) + ".png"
}

func (s *LibretroArt) pictures(d *libDB, i int32) *Info {
	r := &d.recs[i]
	sys := d.allFiles[d.recFile[i]]
	in := &Info{ID: r.Name, Poster: s.picture(sys, "Named_Boxarts", r.Name), Backdrop: s.picture(sys, "Named_Snaps", r.Name)}
	in.Art = []ArtRef{{Ref: s.picture(sys, "Named_Snaps", r.Name), Kind: "screenshot"}, {Ref: s.picture(sys, "Named_Titles", r.Name), Kind: "title"}}
	// If this exact release has no picture, another release of the game might.
	seen := 0
	for _, j := range d.byKey[NameKey(r.Name)] {
		if j != i && seen < 3 && d.recs[j].Name != r.Name {
			seen++
			name := d.recs[j].Name
			osys := d.allFiles[d.recFile[j]]
			in.MorePosters = append(in.MorePosters, s.picture(osys, "Named_Boxarts", name))
			in.MoreBackdrops = append(in.MoreBackdrops, s.picture(osys, "Named_Snaps", name))
		}
	}
	// And some pictures are named for the game alone.
	if short, _, ok := strings.Cut(r.Name, " ("); ok {
		in.MorePosters = append(in.MorePosters, s.picture(sys, "Named_Boxarts", short))
		in.MoreBackdrops = append(in.MoreBackdrops, s.picture(sys, "Named_Snaps", short))
	}
	return in
}

// Lookup implements Source.
func (s *LibretroArt) Lookup(ctx context.Context, q Query) (*Info, error) {
	d, i, method, conf, err := s.DB.find(ctx, q)
	if err != nil {
		return nil, err
	}
	in := s.pictures(d, i)
	in.Method, in.Confidence = method, conf
	return in, nil
}

// Search implements Source: the database's own search lists the same games.
func (s *LibretroArt) Search(ctx context.Context, console, title string) ([]Candidate, error) {
	return nil, ErrNotFound
}

// Fetch implements Source.
func (s *LibretroArt) Fetch(ctx context.Context, console, id string) (*Info, error) {
	d, err := s.DB.load(ctx, console)
	if err != nil {
		return nil, err
	}
	for _, i := range d.byKey[NameKey(id)] {
		if d.recs[i].Name == id {
			in := s.pictures(d, i)
			in.Method, in.Confidence = MethodManual, 100
			return in, nil
		}
	}
	return nil, ErrNotFound
}

// Test implements Source.
func (s *LibretroArt) Test(ctx context.Context) (string, error) {
	return "This source needs no key.", nil
}
