// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // what an upload or a source may be
	_ "image/jpeg" // what an upload or a source may be
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // what an upload or a source may be

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// The logo service: logos of studios, networks, consoles, game studios and
// streaming services, for every part of the app (docs/METADATA.md#the-logo-service).
//
// Logos are never bundled. A logo is either downloaded after the owner agreed
// (setting logoConsent), from a source whose files say what may be done with
// them, or uploaded by the owner. Either way it is decoded and written again as a
// plain PNG, kept in the image cache, and served only by this server, so a browser or
// an app never talks to the source. Anything without a logo is shown as its name.

// The kinds of thing that can have a logo.
const (
	LogoStudio     = "studio"
	LogoNetwork    = "network"
	LogoConsole    = "console"
	LogoGameStudio = "gamestudio"
	LogoPlatform   = "platform"
)

// What's known about a logo.
const (
	LogoPending = "pending" // not looked for yet
	LogoFound   = "found"   // there is a file
	LogoNone    = "none"    // looked, and no source has one
	LogoText    = "text"    // someone chose the name alone
)

// BrandSeries is the kind of row in title_brands for a film series. It has no logo.
const BrandSeries = "series"

// LogoConsentVersion goes up if what the owner agreed to changes enough to ask again.
const LogoConsentVersion = "1"

// Logo is one row of the service. Has says there's a picture to show; anything else
// is shown as Name.
type Logo struct {
	ID      int64  `json:"id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Has     bool   `json:"has"`
	Version int    `json:"version"`
	// Opaque: the picture has a background of its own that couldn't be taken off,
	// so all three variants are the same picture, to be shown on its own plate.
	Opaque     bool   `json:"opaque"`
	Source     string `json:"source,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
	License    string `json:"license,omitempty"`
	LicenseURL string `json:"licenseUrl,omitempty"`
	Credit     string `json:"credit,omitempty"`
	file       string
}

// LogoCandidate is a picture a source offers for something.
type LogoCandidate struct {
	Source     string `json:"source"`
	SourceID   string `json:"sourceId"`
	Name       string `json:"name"`
	Note       string `json:"note,omitempty"`
	ImageURL   string `json:"imageUrl"`
	PageURL    string `json:"pageUrl"`
	License    string `json:"license"`
	LicenseURL string `json:"licenseUrl,omitempty"`
	Credit     string `json:"credit,omitempty"`
	Exact      bool   `json:"exact"`
}

// LogoSource finds logos online.
type LogoSource interface {
	Find(ctx context.Context, kind, name string) ([]LogoCandidate, error)
	Fetch(ctx context.Context, c LogoCandidate) ([]byte, error)
}

// Logos runs the service.
type Logos struct {
	Tasks *tasks.Registry // where this shows in the activity panel; may be nil
	Store *Store
	Dir   string // where the pictures are kept
	// Source is where downloads come from. Nil means nowhere.
	Source LogoSource
	Log    *slog.Logger
	// Gap between lookups, so a first run over a big library asks gently.
	Gap time.Duration

	mu      sync.Mutex
	running bool
}

// ValidLogoKind reports whether kind is one the service knows.
func ValidLogoKind(kind string) bool {
	switch kind {
	case LogoStudio, LogoNetwork, LogoConsole, LogoGameStudio, LogoPlatform:
		return true
	}
	return false
}

var logoNoise = regexp.MustCompile(`[^\p{L}\p{N}]+`)
var logoSuffix = regexp.MustCompile(`\s+(inc|llc|ltd|limited|corp|corporation|co|gmbh|sa|plc)$`)

// BrandKey is how two spellings of a name are told to be the same.
func BrandKey(name string) string {
	k := strings.TrimSpace(logoNoise.ReplaceAllString(strings.ToLower(name), " "))
	return logoSuffix.ReplaceAllString(k, "")
}

var (
	// What a channel list puts around a station's name: a country in front, and a picture quality,
	// a country or a timeshift behind.
	channelNamePrefix  = regexp.MustCompile(`(?i)^\s*(?:\[[a-z]{2,3}\]|\([a-z]{2,3}\)|[a-z]{2,3}\s*[:|]|[a-z]{2,3}\s+-)\s*`)
	channelNameSuffix  = regexp.MustCompile(`(?i)(?:\s+|\s*[-.@|]\s*)(?:hd|fhd|uhd|sd|hevc|h\.?26[45]|4k|8k|\d{3,4}[pi]|\+\s*\d{1,2}|\.?[a-z]{2}@\w+)\s*$|\s*(?:\([^()]*\)|\[[^\[\]]*\])\s*$|\.[a-z]{2}\s*$`)
	channelCallSign    = regexp.MustCompile(`(?i)^[kw][a-z]{2,3}(?:-(?:tv|dt|hd|cd|ld))?$`)
	channelGenericOnly = regexp.MustCompile(`(?i)^(?:(?:channel|ch|tv|television|station|stream|live|local|public|access|community|test|news|sports?|movies?|films?|music|kids|weather|shopping|radio|info|events?|adult|ppv|hd|sd|the|and|of|all|free|\d+)[\s/&+.-]*)+$`)
)

// ChannelBrandName is the name a tuner or playlist channel would have its logo under, or "" if
// it isn't one worth asking about. What comes out goes to the logo service, which only takes a
// logo by itself when the name is exact, so this keeps to removing what's plainly not part of
// the name, and leaves out what could only ever match the wrong thing.
func ChannelBrandName(name string) string {
	n := cleanBrandName(name)
	n = channelNamePrefix.ReplaceAllString(n, "")
	for range 4 {
		next := channelNameSuffix.ReplaceAllString(n, "")
		if next == n {
			break
		}
		n = strings.TrimSpace(next)
	}
	// A station's number after its call sign: WABC-TV 7.
	if f := strings.Fields(n); len(f) == 2 && channelCallSign.MatchString(f[0]) && strings.Trim(f[1], "0123456789.") == "" {
		n = f[0]
	}
	if strings.ContainsAny(n, "/\\@:") || len(n) > 60 || channelGenericOnly.MatchString(n) {
		return ""
	}
	if len([]rune(BrandKey(n))) < 2 {
		return ""
	}
	return n
}

// cleanBrandName trims and bounds a name that came from a source or a person.
func cleanBrandName(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name))
	if rs := []rune(name); len(rs) > 120 {
		name = string(rs[:120])
	}
	return name
}

// ---------------------------------------------------------------------------
// Consent
// ---------------------------------------------------------------------------

// LogoConsent says what the owner decided: "yes", "no" or "" (not asked yet).
func (s *Store) LogoConsent(ctx context.Context) string {
	switch s.DB.GetSetting(ctx, "logoConsent", "") {
	case LogoConsentVersion:
		return "yes"
	case "0":
		return "no"
	}
	return ""
}

// SetLogoConsent records the owner's answer. Agreeing starts the downloads;
// declining stops them, and drops what was downloaded when asked to. A logo the
// owner uploaded is theirs, and stays.
func (l *Logos) SetLogoConsent(ctx context.Context, accepted, removeDownloaded bool) error {
	v := "0"
	if accepted {
		v = LogoConsentVersion
	}
	if err := l.Store.DB.SetSetting(ctx, "logoConsent", v); err != nil {
		return err
	}
	if err := l.Store.DB.SetSetting(ctx, "logoConsentAt", strconv.FormatInt(db.Now(), 10)); err != nil {
		return err
	}
	if accepted {
		// What was given up on while declined can be looked for now.
		if _, err := l.Store.DB.ExecContext(ctx, `UPDATE logos SET status = ?, tried_at = 0 WHERE status = ?`, LogoPending, LogoNone); err != nil {
			return err
		}
		l.Kick()
		return nil
	}
	if !removeDownloaded {
		return nil
	}
	rows, err := l.Store.DB.QueryContext(ctx, `SELECT id, file FROM logos WHERE status = ? AND source <> 'you'`, LogoFound)
	if err != nil {
		return err
	}
	var gone []Logo
	for rows.Next() {
		var g Logo
		if err := rows.Scan(&g.ID, &g.file); err != nil {
			rows.Close()
			return err
		}
		gone = append(gone, g)
	}
	rows.Close()
	for _, g := range gone {
		l.removeFiles(g.file)
	}
	_, err = l.Store.DB.ExecContext(ctx, `UPDATE logos SET status = ?, file = '', source = '', source_id = '', source_url = '', license = '', license_url = '', credit = '', opaque = 0, version = version + 1
		WHERE status = ? AND source <> 'you'`, LogoPending, LogoFound)
	return err
}

// ---------------------------------------------------------------------------
// Rows
// ---------------------------------------------------------------------------

const logoCols = `id, kind, name, status, source, source_url, license, license_url, credit, file, opaque, version`

func scanLogo(sc interface{ Scan(...any) error }) (*Logo, error) {
	var g Logo
	var opaque int
	if err := sc.Scan(&g.ID, &g.Kind, &g.Name, &g.Status, &g.Source, &g.SourceURL, &g.License, &g.LicenseURL, &g.Credit, &g.file, &opaque, &g.Version); err != nil {
		return nil, err
	}
	g.Opaque = opaque != 0
	g.Has = g.Status == LogoFound && g.file != ""
	return &g, nil
}

// LogoByID reads one row.
func (s *Store) LogoByID(ctx context.Context, id int64) (*Logo, error) {
	g, err := scanLogo(s.DB.QueryRowContext(ctx, `SELECT `+logoCols+` FROM logos WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return g, err
}

// LogoFor reads the row for a name, or nil if nobody has asked about it.
func (s *Store) LogoFor(ctx context.Context, kind, name string) (*Logo, error) {
	g, err := scanLogo(s.DB.QueryRowContext(ctx, `SELECT `+logoCols+` FROM logos WHERE kind = ? AND key = ?`, kind, BrandKey(name)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return g, err
}

// EnsureLogo makes sure a name has a row, so the service looks for it. This is
// what other parts of the server call when they find a console or a game studio.
// It does nothing for an empty or unknown name, and contacts nobody by itself.
func (s *Store) EnsureLogo(ctx context.Context, kind, name string) (*Logo, error) {
	name = cleanBrandName(name)
	key := BrandKey(name)
	if !ValidLogoKind(kind) || key == "" {
		return nil, errors.New("not a name")
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO logos(kind, key, name) VALUES(?,?,?) ON CONFLICT(kind, key) DO NOTHING`, kind, key, name); err != nil {
		return nil, err
	}
	return s.LogoFor(ctx, kind, name)
}

// LogoRef is one thing asked about in a lookup.
type LogoRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// LookupLogos answers for many names at once, in the order asked. create says the
// asker may add names nobody has asked about yet. A name without a row still gets
// an answer, with no id and no picture.
func (l *Logos) LookupLogos(ctx context.Context, refs []LogoRef, create bool) ([]Logo, error) {
	out := make([]Logo, 0, len(refs))
	added := false
	for _, r := range refs {
		name := cleanBrandName(r.Name)
		if !ValidLogoKind(r.Kind) || BrandKey(name) == "" {
			continue
		}
		var g *Logo
		var err error
		if create {
			g, err = l.Store.LogoFor(ctx, r.Kind, name)
			if err == nil && g == nil {
				g, err = l.Store.EnsureLogo(ctx, r.Kind, name)
				added = true
			}
		} else {
			g, err = l.Store.LogoFor(ctx, r.Kind, name)
		}
		if err != nil {
			return nil, err
		}
		if g == nil {
			out = append(out, Logo{Kind: r.Kind, Name: name, Status: LogoPending})
			continue
		}
		g.Name = name // as this screen wrote it
		out = append(out, *g)
	}
	if added {
		l.Kick()
	}
	return out, nil
}

// ListLogos is a page of the names the service knows, for an administrator to find one.
func (s *Store) ListLogos(ctx context.Context, kind, query string, limit, offset int) ([]Logo, int, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	where, args := "1 = 1", []any{}
	if kind != "" {
		where += " AND kind = ?"
		args = append(args, kind)
	}
	if key := BrandKey(query); key != "" {
		where += ` AND key LIKE ? ESCAPE '\'`
		args = append(args, "%"+strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(key)+"%")
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM logos WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+logoCols+` FROM logos WHERE `+where+` ORDER BY name COLLATE NOCASE LIMIT ? OFFSET ?`, append(args, limit, max(offset, 0))...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Logo{}
	for rows.Next() {
		g, err := scanLogo(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *g)
	}
	return out, total, rows.Err()
}

// LogoCredits lists where every downloaded logo came from: its source, licence and
// author, which is what the licences of those files ask for.
func (s *Store) LogoCredits(ctx context.Context) ([]Logo, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+logoCols+` FROM logos WHERE status = ? ORDER BY name COLLATE NOCASE`, LogoFound)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Logo{}
	for rows.Next() {
		g, err := scanLogo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

// LogoCounts says how far the service has got.
func (s *Store) LogoCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT status, COUNT(*) FROM logos GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{LogoPending: 0, LogoFound: 0, LogoNone: 0, LogoText: 0}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Looking for logos
// ---------------------------------------------------------------------------

// Kick starts looking for what's pending, if the owner agreed. It returns at once.
func (l *Logos) Kick() {
	if l == nil || l.Source == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return
	}
	l.running = true
	logging.Go("logo service", func() {
		defer func() {
			l.mu.Lock()
			l.running = false
			l.mu.Unlock()
		}()
		l.work(context.Background())
	}, "cat", "library")
}

func (l *Logos) work(ctx context.Context) {
	gap := l.Gap
	if gap == 0 {
		gap = time.Second
	}
	var t *tasks.Handle // only once there's something to do: this wakes after every scan
	defer func() { t.Finish(nil) }()
	for l.Store.LogoConsent(ctx) == "yes" {
		g, err := scanLogo(l.Store.DB.QueryRowContext(ctx, `SELECT `+logoCols+` FROM logos WHERE status = ? ORDER BY kind <> 'console', id LIMIT 1`, LogoPending))
		if err != nil {
			return
		}
		if t == nil {
			t = l.Tasks.Start(tasks.Spec{Type: tasks.TypeMetadata, Title: "Looking for logos", Unit: "logos"})
		}
		t.Item(g.Name, "")
		if err := l.lookFor(ctx, g); err != nil {
			l.log().Warn("could not look for a logo", "kind", g.Kind, "name", g.Name, "err", err)
			// The source may be away. Stop here: the next scan or lookup starts again.
			_, _ = l.Store.DB.ExecContext(ctx, `UPDATE logos SET tried_at = ? WHERE id = ?`, db.Now(), g.ID)
			words, fix := tasks.Explain(err)
			t.Problem(g.Name, "Couldn't look for this logo. "+words+" "+fix)
			return
		}
		t.Add(1)
		time.Sleep(gap)
	}
}

func (l *Logos) log() *slog.Logger {
	if l.Log != nil {
		return l.Log
	}
	return slog.Default().With("cat", "library")
}

func (l *Logos) lookFor(ctx context.Context, g *Logo) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cands, err := l.Source.Find(ctx, g.Kind, g.Name)
	if err != nil {
		return err
	}
	for _, c := range cands {
		if !c.Exact {
			continue // close isn't good enough to put on a title by itself: Change offers these
		}
		if err := l.use(ctx, g, c); err != nil {
			l.log().Debug("a logo couldn't be used", "name", g.Name, "file", c.SourceID, "err", err)
			continue
		}
		return nil
	}
	_, err = l.Store.DB.ExecContext(ctx, `UPDATE logos SET status = ?, tried_at = ? WHERE id = ?`, LogoNone, db.Now(), g.ID)
	return err
}

// Candidates lists what a source has for a logo, for someone choosing by hand.
func (l *Logos) Candidates(ctx context.Context, g *Logo, query string) ([]LogoCandidate, error) {
	if l.Store.LogoConsent(ctx) != "yes" {
		return nil, ErrLogosNotAgreed
	}
	if l.Source == nil {
		return []LogoCandidate{}, nil
	}
	if query = cleanBrandName(query); query == "" {
		query = g.Name
	}
	c, err := l.Source.Find(ctx, g.Kind, query)
	if c == nil {
		c = []LogoCandidate{}
	}
	return c, err
}

// ErrLogosNotAgreed: downloading logos needs the owner's agreement first.
var ErrLogosNotAgreed = errors.New("downloading logos isn't turned on. Turn it on in Settings, Metadata agents, after reading what it means")

// Pick downloads a candidate and makes it this logo. The candidate comes from
// the browser, so only a fresh answer from the source is believed: a page must not
// be able to make the server fetch an address of its choosing, or claim a licence.
func (l *Logos) Pick(ctx context.Context, g *Logo, query, sourceID string) (*Logo, error) {
	cands, err := l.Candidates(ctx, g, query)
	if err != nil {
		return nil, err
	}
	for _, c := range cands {
		if c.SourceID == sourceID {
			if err := l.use(ctx, g, c); err != nil {
				return nil, err
			}
			return l.Store.LogoByID(ctx, g.ID)
		}
	}
	return nil, ErrNotFound
}

func (l *Logos) use(ctx context.Context, g *Logo, c LogoCandidate) error {
	b, err := l.Source.Fetch(ctx, c)
	if err != nil {
		return err
	}
	return l.store(ctx, g, b, c)
}

// Upload makes a picture from the owner this logo.
func (l *Logos) Upload(ctx context.Context, g *Logo, b []byte) (*Logo, error) {
	if err := l.store(ctx, g, b, LogoCandidate{Source: "you", Credit: "Uploaded by this server's owner"}); err != nil {
		return nil, err
	}
	return l.Store.LogoByID(ctx, g.ID)
}

// SetLogoStatus handles "use the name alone" and "look again".
func (l *Logos) SetLogoStatus(ctx context.Context, g *Logo, status string) (*Logo, error) {
	if status != LogoText && status != LogoPending {
		return nil, errors.New("status must be text or pending")
	}
	l.removeFiles(g.file)
	if _, err := l.Store.DB.ExecContext(ctx, `UPDATE logos SET status = ?, file = '', source = '', source_id = '', source_url = '', license = '', license_url = '', credit = '', opaque = 0, tried_at = 0, version = version + 1 WHERE id = ?`, status, g.ID); err != nil {
		return nil, err
	}
	if status == LogoPending {
		l.Kick()
	}
	return l.Store.LogoByID(ctx, g.ID)
}

func (l *Logos) store(ctx context.Context, g *Logo, b []byte, c LogoCandidate) error {
	set, err := makeLogoVariants(b)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return err
	}
	version := g.Version + 1
	file := fmt.Sprintf("%d-%d", g.ID, version) // the name is ours alone: nothing from a source is in it
	for v, data := range map[string][]byte{"original": set.original, "white": set.white, "dark": set.dark} {
		tmp, err := os.CreateTemp(l.Dir, "tmp-*")
		if err != nil {
			return err
		}
		_, werr := tmp.Write(data)
		cerr := tmp.Close()
		if werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = os.Rename(tmp.Name(), l.pathFor(file, v))
		}
		if werr != nil {
			os.Remove(tmp.Name())
			return werr
		}
	}
	old := g.file
	opaque := 0
	if set.opaque {
		opaque = 1
	}
	if _, err := l.Store.DB.ExecContext(ctx, `UPDATE logos SET status = ?, source = ?, source_id = ?, source_url = ?, license = ?, license_url = ?, credit = ?, file = ?, opaque = ?, version = ?, tried_at = ? WHERE id = ?`,
		LogoFound, c.Source, c.SourceID, c.PageURL, c.License, c.LicenseURL, c.Credit, file, opaque, version, db.Now(), g.ID); err != nil {
		return err
	}
	l.removeFiles(old)
	g.Version, g.file = version, file
	return nil
}

func (l *Logos) pathFor(file, variant string) string {
	return filepath.Join(l.Dir, file+"-"+variant+".png")
}

func (l *Logos) removeFiles(file string) {
	if file == "" {
		return
	}
	for _, v := range []string{"original", "white", "dark"} {
		_ = os.Remove(l.pathFor(file, v))
	}
}

// ImagePath is where a variant of a logo is, or "".
func (l *Logos) ImagePath(g *Logo, variant string) string {
	if !g.Has {
		return ""
	}
	switch variant {
	case "white", "dark", "original":
	default:
		variant = "original"
	}
	return l.pathFor(g.file, variant)
}

// ---------------------------------------------------------------------------
// Making a picture safe, and white and dark versions of it
// ---------------------------------------------------------------------------

const (
	maxLogoBytes  = 8 << 20
	maxLogoPixels = 25_000_000
	logoWidth     = 640
)

type logoVariants struct {
	original, white, dark []byte
	opaque                bool
}

// makeLogoVariants reads a picture from anywhere and writes it again from its pixels
// as PNG, so nothing but pixels survives: no metadata, no trailing data, no script.
func makeLogoVariants(b []byte) (set *logoVariants, err error) {
	defer func() {
		if p := recover(); p != nil { // image decoders see whatever was downloaded
			set, err = nil, fmt.Errorf("could not read this picture (%v)", p)
		}
	}()
	if len(b) > maxLogoBytes {
		return nil, errors.New("the picture is too big: the limit is 8 MB")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("use a PNG, JPEG, WebP or GIF picture")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxLogoPixels {
		return nil, errors.New("the picture is too big")
	}
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("this picture couldn't be read")
	}
	full := toNRGBA(src)
	matte, opaque := logoMatte(full)
	if !opaque {
		// Trim the empty margin, so logos line up on a card by what can be seen.
		if box := alphaBounds(matte); !box.Empty() {
			full = cropNRGBA(full, box)
			matte = cropAlpha(matte, box)
		}
	}
	full, matte = fitLogo(full, matte)
	white, dark := full, full
	if !opaque {
		white = paintMatte(matte, color.NRGBA{255, 255, 255, 255})
		dark = paintMatte(matte, color.NRGBA{16, 18, 24, 255})
	}
	out := &logoVariants{opaque: opaque}
	if out.original, err = encodePNG(full); err != nil {
		return nil, err
	}
	if out.white, err = encodePNG(white); err != nil {
		return nil, err
	}
	if out.dark, err = encodePNG(dark); err != nil {
		return nil, err
	}
	return out, nil
}

func encodePNG(m image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func toNRGBA(src image.Image) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.SetNRGBA(x, y, color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA))
		}
	}
	return out
}

func lum(c color.NRGBA) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// logoMatte works out which pixels are the logo, for painting it in one colour.
//
//   - A transparent background goes by its transparency. If what's left is in two
//     clearly different tones (a dark shield with light letters), the larger tone
//     becomes the logo and the smaller is cut out of it, so the letters stay readable
//     once everything is one colour.
//   - A plain background (all four corners alike) goes by how far each pixel is from it.
//   - Anything else, such as a photo, can't be cut out: opaque is true and the
//     picture stays as it is, to sit on a plate of its own.
func logoMatte(m *image.NRGBA) (matte *image.Alpha, opaque bool) {
	b := m.Bounds()
	w, h := b.Dx(), b.Dy()
	matte = image.NewAlpha(b)
	see := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if m.NRGBAAt(x, y).A < 250 {
				see++
			}
		}
	}
	if see*50 >= w*h { // at least 2% can be seen through
		var dark, light int
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if c := m.NRGBAAt(x, y); c.A > 128 {
					if lum(c) < 0.5 {
						dark++
					} else {
						light++
					}
				}
			}
		}
		solid := dark + light
		twoTone := solid > 0 && min(dark, light)*100 >= solid*8
		major, minor := 0.0, 1.0
		if light > dark {
			major, minor = 1, 0
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := m.NRGBAAt(x, y)
				a := float64(c.A)
				if twoTone {
					d := (lum(c) - minor) / (major - minor)
					a *= clamp01(d*1.6 - 0.1)
				}
				matte.SetAlpha(x, y, color.Alpha{uint8(a)})
			}
		}
		return matte, false
	}
	corners := []color.NRGBA{m.NRGBAAt(0, 0), m.NRGBAAt(w-1, 0), m.NRGBAAt(0, h-1), m.NRGBAAt(w-1, h-1)}
	bg := corners[0]
	for _, c := range corners[1:] {
		if colorDist(bg, c) > 0.08 {
			return matte, true
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			matte.SetAlpha(x, y, color.Alpha{uint8(clamp01((colorDist(m.NRGBAAt(x, y), bg)-0.06)*3) * 255)})
		}
	}
	return matte, false
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

func colorDist(a, b color.NRGBA) float64 {
	dr, dg, db := float64(a.R)-float64(b.R), float64(a.G)-float64(b.G), float64(a.B)-float64(b.B)
	return (abs64(dr) + abs64(dg) + abs64(db)) / (3 * 255)
}

func abs64(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func alphaBounds(m *image.Alpha) image.Rectangle {
	b := m.Bounds()
	box := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if m.AlphaAt(x, y).A > 8 {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return box
}

func cropNRGBA(m *image.NRGBA, box image.Rectangle) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	xdraw.Draw(out, out.Bounds(), m, box.Min, xdraw.Src)
	return out
}

func cropAlpha(m *image.Alpha, box image.Rectangle) *image.Alpha {
	out := image.NewAlpha(image.Rect(0, 0, box.Dx(), box.Dy()))
	xdraw.Draw(out, out.Bounds(), m, box.Min, xdraw.Src)
	return out
}

func fitLogo(m *image.NRGBA, matte *image.Alpha) (*image.NRGBA, *image.Alpha) {
	b := m.Bounds()
	if b.Dx() <= logoWidth {
		return m, matte
	}
	h := max(1, b.Dy()*logoWidth/b.Dx())
	r := image.Rect(0, 0, logoWidth, h)
	small := image.NewNRGBA(r)
	xdraw.CatmullRom.Scale(small, r, m, b, xdraw.Over, nil)
	sm := image.NewAlpha(r)
	xdraw.CatmullRom.Scale(sm, r, matte, b, xdraw.Over, nil)
	return small, sm
}

func paintMatte(m *image.Alpha, c color.NRGBA) *image.NRGBA {
	b := m.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, m.AlphaAt(b.Min.X+x, b.Min.Y+y).A})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Brands: the studios and networks of titles
// ---------------------------------------------------------------------------

// Brand is a studio or a network with what's on this server from it.
type Brand struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// setTitleBrands replaces a title's studios or networks.
func (s *Store) setTitleBrands(ctx context.Context, target string, id, libraryID int64, kind string, names []string) error {
	col := "item_id"
	if target == "show" {
		col = "show_id"
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM title_brands WHERE kind = ? AND `+col+` = ?`, kind, id); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, n := range names {
		n = cleanBrandName(n)
		k := BrandKey(n)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO title_brands(kind, key, name, library_id, `+col+`) VALUES(?,?,?,?,?)`, kind, k, n, libraryID, id); err != nil {
			return err
		}
		if ValidLogoKind(kind) {
			if _, err := s.EnsureLogo(ctx, kind, n); err != nil {
				return err
			}
		}
	}
	return nil
}

// TitleBrands lists the studios (or networks) of one title.
func (s *Store) TitleBrands(ctx context.Context, target string, id int64, kind string) []string {
	col := "item_id"
	if target == "show" {
		col = "show_id"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT name FROM title_brands WHERE kind = ? AND `+col+` = ? ORDER BY rowid`, kind, id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			out = append(out, n)
		}
	}
	return out
}

// SyncBrands links titles to the studio or network they name, for anything saved
// before this existed or changed by a scan since. It's cheap when nothing changed.
func (s *Store) SyncBrands(ctx context.Context) error {
	gallery := `(SELECT id FROM libraries WHERE type IN ('photos', 'shorts', 'channels', 'musicvideos', 'radio', 'music', 'audiobooks', 'podcasts', 'books', 'games'))`
	type row struct {
		id, lib int64
		name    string
	}
	read := func(q string) ([]row, error) {
		rows, err := s.DB.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.lib, &r.name); err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, rows.Err()
	}
	items, err := read(`SELECT i.id, i.library_id, i.studio FROM items i WHERE IFNULL(i.studio, '') <> '' AND i.show_id IS NULL AND i.library_id NOT IN ` + gallery + `
		AND NOT EXISTS (SELECT 1 FROM title_brands b WHERE b.kind = 'studio' AND b.item_id = i.id)`)
	if err != nil {
		return err
	}
	for _, r := range items {
		if err := s.setTitleBrands(ctx, "item", r.id, r.lib, LogoStudio, []string{r.name}); err != nil {
			return err
		}
	}
	shows, err := read(`SELECT s.id, s.library_id, s.network FROM shows s WHERE IFNULL(s.network, '') <> ''
		AND NOT EXISTS (SELECT 1 FROM title_brands b WHERE b.kind = 'network' AND b.show_id = s.id)`)
	if err != nil {
		return err
	}
	for _, r := range shows {
		if err := s.setTitleBrands(ctx, "show", r.id, r.lib, LogoNetwork, []string{r.name}); err != nil {
			return err
		}
	}
	return nil
}

// Brands lists the studios or networks in a library (0 = all), most titles first,
// counting only what this viewer may see.
func (s *Store) Brands(ctx context.Context, libraryID int64, kind string, access *Access) ([]Brand, error) {
	ic, ia := access.ItemClause()
	sc, sa := access.ShowClause()
	args := []any{kind, libraryID, libraryID}
	args = append(args, ia...)
	args = append(args, sa...)
	rows, err := s.DB.QueryContext(ctx, `SELECT b.key, MIN(b.name), COUNT(*) FROM title_brands b
		LEFT JOIN items i ON i.id = b.item_id
		LEFT JOIN shows s ON s.id = b.show_id
		WHERE b.kind = ? AND (? = 0 OR b.library_id = ?)
		  AND ((b.item_id IS NOT NULL AND i.missing = 0 AND `+hideLinked+` AND `+ic+`)
		    OR (b.show_id IS NOT NULL AND s.missing = 0 AND `+sc+`))
		GROUP BY b.key`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Brand{}
	for rows.Next() {
		b := Brand{Kind: kind}
		if err := rows.Scan(&b.Key, &b.Name, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, rows.Err()
}
