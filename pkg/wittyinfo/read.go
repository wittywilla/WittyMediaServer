// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // pictures are checked by decoding their header
	_ "image/png"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	_ "golang.org/x/image/webp"
)

// Limits. A file past any of them is refused whole: it is a sidecar anyone could have left.
const (
	MaxFile      = 64 << 20 // the whole .wittyinfo
	MaxManifest  = 1 << 20  // manifest.json, unpacked
	MaxMembers   = 512      // members of the archive
	MaxImage     = 16 << 20 // one picture, unpacked
	MaxPixels    = 8192     // a picture's width and height
	MaxSubtitle  = 2 << 20  // one subtitle file, unpacked
	MaxOther     = 4 << 20  // a member this version doesn't use, unpacked
	MaxUnpacked  = 128 << 20
	MaxRatio     = 100 // unpacked size over packed size, for anything over 1 MB
	MaxSegments  = 1000
	MaxScenes    = 1000
	MaxFilters   = 5000
	MaxListItems = 50    // genres, tags, studios, trivia...
	MaxShortText = 200   // titles, labels, names
	MaxLongText  = 20000 // overview, notes
	MaxTime      = 7 * 24 * 3600
)

// ManifestName is the archive member that holds the manifest.
const ManifestName = "manifest.json"

// Problem is one thing wrong with a file: where (a JSON path such as "segments[2].end",
// or an archive member) and what.
type Problem struct {
	Where string `json:"where"`
	What  string `json:"what"`
}

func (p Problem) String() string {
	if p.Where == "" {
		return p.What
	}
	return p.Where + ": " + p.What
}

// Error lists everything wrong with a file that was refused.
type Error struct {
	Problems []Problem `json:"problems"`
}

func (e *Error) Error() string {
	if len(e.Problems) == 1 {
		return "not a usable .wittyinfo: " + e.Problems[0].String()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "not a usable .wittyinfo (%d problems): ", len(e.Problems))
	for i, p := range e.Problems {
		if i == 5 {
			fmt.Fprintf(&b, "; and %d more", len(e.Problems)-5)
			break
		}
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(p.String())
	}
	return b.String()
}

func refuse(where, format string, args ...any) *Error {
	return &Error{Problems: []Problem{{Where: where, What: fmt.Sprintf(format, args...)}}}
}

// IsArchive says whether data looks like a ZIP (rather than a bare manifest).
func IsArchive(data []byte) bool { return bytes.HasPrefix(data, []byte("PK\x03\x04")) }

// Parse reads a .wittyinfo, archive or bare manifest, and checks all of it. A file with
// anything wrong gives an *Error listing the problems and no Info.
func Parse(data []byte) (*Info, error) {
	if len(data) > MaxFile {
		return nil, refuse("", "the file is larger than %d MB", MaxFile>>20)
	}
	var manifest []byte
	files := map[string][]byte{}
	if IsArchive(data) {
		var err error
		if manifest, files, err = readArchive(data); err != nil {
			return nil, err
		}
	} else {
		manifest = data
		if len(manifest) > MaxManifest {
			return nil, refuse(ManifestName, "the manifest is larger than %d KB", MaxManifest>>10)
		}
	}
	info, err := parseManifest(manifest)
	if err != nil {
		return nil, err
	}
	info.Files = files
	if err := info.Check(); err != nil {
		return nil, err
	}
	return info, nil
}

func parseManifest(b []byte) (*Info, error) {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(b) {
		return nil, refuse(ManifestName, "the manifest is not UTF-8 text")
	}
	t := bytes.TrimSpace(b)
	if len(t) == 0 || t[0] != '{' {
		return nil, refuse(ManifestName, "the manifest has to be a JSON object")
	}
	if err := checkDepth(t, 16); err != nil {
		return nil, refuse(ManifestName, "%s", err.Error())
	}
	var info Info
	dec := json.NewDecoder(bytes.NewReader(t))
	if err := dec.Decode(&info); err != nil {
		return nil, refuse(ManifestName, "%s", jsonError(t, err))
	}
	if dec.More() {
		return nil, refuse(ManifestName, "there is more after the JSON object")
	}
	return &info, nil
}

// jsonError says where in the manifest decoding failed, as a line number.
func jsonError(b []byte, err error) string {
	var off int64 = -1
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	switch {
	case errors.As(err, &se):
		off = se.Offset
	case errors.As(err, &te):
		off = te.Offset
		if te.Field != "" {
			return fmt.Sprintf("%s has the wrong kind of value (line %d)", te.Field, lineAt(b, off))
		}
	}
	if off >= 0 {
		return fmt.Sprintf("%s (line %d)", err.Error(), lineAt(b, off))
	}
	return err.Error()
}

func lineAt(b []byte, off int64) int {
	if off > int64(len(b)) {
		off = int64(len(b))
	}
	return bytes.Count(b[:off], []byte("\n")) + 1
}

// checkDepth refuses JSON nested deeper than max, before decoding it.
func checkDepth(b []byte, max int) error {
	depth, inStr, esc := 0, false, false
	for _, c := range b {
		switch {
		case inStr:
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
		case c == '{' || c == '[':
			if depth++; depth > max {
				return fmt.Errorf("the manifest is nested more than %d deep", max)
			}
		case c == '}' || c == ']':
			depth--
		}
	}
	return nil
}

// ------------------------------------------------------------------ the archive

// memberRe: what a member may be called: letters, digits, dot,
// dash, underscore and space, in at most three folders; nothing that climbs out.
var memberRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9 ._-]{0,99}(/[A-Za-z0-9_][A-Za-z0-9 ._-]{0,99}){0,3}$`)

// ValidMemberName says whether name is a safe archive member name.
func ValidMemberName(name string) bool {
	if !memberRe.MatchString(name) || strings.Contains(name, "..") {
		return false
	}
	return true
}

func memberKind(name string) string {
	low := strings.ToLower(name)
	switch {
	case low == ManifestName:
		return "manifest"
	case strings.HasPrefix(low, "images/"):
		return "image"
	case strings.HasPrefix(low, "subtitles/"):
		return "subtitle"
	}
	return "other"
}

func readArchive(data []byte) ([]byte, map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, refuse("", "the archive can't be read: %v", err)
	}
	if len(zr.File) > MaxMembers {
		return nil, nil, refuse("", "the archive has more than %d members", MaxMembers)
	}
	var manifest []byte
	files := map[string][]byte{}
	seen := map[string]bool{}
	var total int64
	for _, f := range zr.File {
		name := f.Name
		if strings.HasSuffix(name, "/") && f.UncompressedSize64 == 0 {
			continue // a folder
		}
		if !ValidMemberName(name) {
			return nil, nil, refuse(name, "a member's name has to be a plain relative name (letters, digits, . _ - and space)")
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, nil, refuse(name, "the archive holds this name twice")
		}
		seen[key] = true
		if f.Flags&0x1 != 0 {
			return nil, nil, refuse(name, "encrypted members are not allowed")
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return nil, nil, refuse(name, "only stored and deflated members are allowed")
		}
		if mode := f.Mode(); !mode.IsRegular() {
			return nil, nil, refuse(name, "only ordinary files are allowed (no links)")
		}
		kind := memberKind(name)
		limit := map[string]int64{"manifest": MaxManifest, "image": MaxImage, "subtitle": MaxSubtitle, "other": MaxOther}[kind]
		if f.UncompressedSize64 > uint64(limit) {
			return nil, nil, refuse(name, "larger than %d KB unpacked", limit>>10)
		}
		if f.UncompressedSize64 > 1<<20 && f.CompressedSize64 > 0 && f.UncompressedSize64/f.CompressedSize64 > MaxRatio {
			return nil, nil, refuse(name, "packed too tightly to be ordinary content")
		}
		b, err := readMember(f, limit)
		if err != nil {
			return nil, nil, refuse(name, "%v", err)
		}
		if total += int64(len(b)); total > MaxUnpacked {
			return nil, nil, refuse("", "the archive unpacks to more than %d MB", MaxUnpacked>>20)
		}
		switch kind {
		case "manifest":
			manifest = b
		case "image":
			if err := checkImage(name, b); err != nil {
				return nil, nil, refuse(name, "%v", err)
			}
			files[name] = b
		case "subtitle":
			if err := checkSubtitle(name, b); err != nil {
				return nil, nil, refuse(name, "%v", err)
			}
			files[name] = b
		default:
			files[name] = b // kept, never used
		}
	}
	if manifest == nil {
		return nil, nil, refuse("", "the archive has no %s", ManifestName)
	}
	return manifest, files, nil
}

// readMember unpacks one member, never more than limit bytes whatever its header says.
func readMember(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, fmt.Errorf("can't be unpacked: %v", err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("larger than %d KB unpacked", limit>>10)
	}
	return b, nil
}

// ImageExts are the picture types a .wittyinfo may hold, and what each really is.
var ImageExts = map[string]string{".jpg": "jpeg", ".jpeg": "jpeg", ".png": "png", ".webp": "webp"}

// SubtitleExts are the subtitle types a .wittyinfo may hold.
var SubtitleExts = map[string]bool{".srt": true, ".vtt": true, ".ass": true, ".ssa": true}

func extOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return ""
	}
	return strings.ToLower(name[i:])
}

// CheckImage checks that b is a picture of the type its name says, of a sensible size.
func CheckImage(name string, b []byte) error { return checkImage(name, b) }

func checkImage(name string, b []byte) error {
	want, ok := ImageExts[extOf(name)]
	if !ok {
		return errors.New("pictures have to be .jpg, .png or .webp")
	}
	if len(b) > MaxImage {
		return fmt.Errorf("pictures can be %d MB at most", MaxImage>>20)
	}
	cfg, got, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return errors.New("not a picture that can be read")
	}
	if got != want {
		return fmt.Errorf("the name says %s but it is %s", want, got)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxPixels || cfg.Height > MaxPixels {
		return fmt.Errorf("pictures can be %d pixels across at most", MaxPixels)
	}
	return nil
}

func checkSubtitle(name string, b []byte) error {
	if !SubtitleExts[extOf(name)] {
		return errors.New("subtitles have to be .srt, .vtt, .ass or .ssa")
	}
	if len(b) > MaxSubtitle {
		return fmt.Errorf("subtitles can be %d MB at most", MaxSubtitle>>20)
	}
	if !utf8.Valid(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))) {
		return errors.New("subtitles have to be UTF-8 text")
	}
	return nil
}

// ------------------------------------------------------------------ checking

type checker struct{ problems []Problem }

func (c *checker) bad(where, format string, args ...any) {
	if len(c.problems) < 100 {
		c.problems = append(c.problems, Problem{Where: where, What: fmt.Sprintf(format, args...)})
	}
}

func (c *checker) text(where, s string, max int) {
	if utf8.RuneCountInString(s) > max {
		c.bad(where, "longer than %d characters", max)
	}
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			c.bad(where, "holds control characters")
			return
		}
	}
}

func (c *checker) list(where string, l []string, max int) {
	if len(l) > MaxListItems {
		c.bad(where, "more than %d entries", MaxListItems)
	}
	for i, s := range l {
		c.text(fmt.Sprintf("%s[%d]", where, i), s, max)
	}
}

func (c *checker) time(where string, v Seconds) {
	f := float64(v)
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > MaxTime {
		c.bad(where, "a time has to be between 0 and 7 days")
	}
}

func (c *checker) oneOf(where, v string, allowed []string) {
	for _, a := range allowed {
		if v == a {
			return
		}
	}
	c.bad(where, "%q is not one of %s", v, strings.Join(allowed, ", "))
}

// member checks a reference to another member of the archive: it has to be a plain name
// of a member that is there, of the kind wanted. A URL or a path never is.
func (c *checker) member(where, name, prefix string, files map[string][]byte) {
	if name == "" {
		return
	}
	if strings.Contains(name, ":") || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || !ValidMemberName(name) {
		c.bad(where, "has to name a file inside the .wittyinfo, such as %sposter.jpg", prefix)
		return
	}
	if !strings.HasPrefix(strings.ToLower(name), prefix) {
		c.bad(where, "has to be in the %s folder of the .wittyinfo", strings.TrimSuffix(prefix, "/"))
		return
	}
	if _, ok := files[name]; !ok {
		c.bad(where, "names %s, which isn't in the .wittyinfo", name)
	}
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

// Check checks everything in the manifest, and that each name it gives is in Files.
func (x *Info) Check() error {
	c := &checker{}
	if x.Format != Format {
		c.bad("format", "has to be %q", Format)
	}
	if x.Version < 1 {
		c.bad("version", "has to be 1 or more")
	}
	scope := x.ScopeOr()
	c.oneOf("scope", scope, []string{ScopeItem, ScopeSeason, ScopeShow})
	if scope == ScopeSeason && (x.Season == nil || *x.Season < 0 || *x.Season > 9999) {
		c.bad("season", "a season file says which season it is for")
	}
	if scope != ScopeItem {
		if len(x.Scenes) > 0 {
			c.bad("scenes", "scenes belong to one video, not to a %s", scope)
		}
		if len(x.Filters) > 0 {
			c.bad("filters", "content filters belong to one video, not to a %s", scope)
		}
		if x.Identity != nil {
			c.bad("identity", "only a file for one video says which video it is")
		}
	}
	if id := x.Identity; id != nil {
		c.time("identity.durationSec", id.DurationSec)
		if id.DurationSec <= 0 {
			c.bad("identity.durationSec", "has to be the video's length")
		}
		if t := id.ToleranceSec; t != nil && (math.IsNaN(*t) || *t < 0 || *t > 600) {
			c.bad("identity.toleranceSec", "has to be between 0 and 600 seconds")
		}
		c.text("identity.fileName", id.FileName, 255)
		c.text("identity.edition", id.Edition, MaxShortText)
		if id.FileSize < 0 {
			c.bad("identity.fileSize", "can't be negative")
		}
		if fp := id.Fingerprint; fp != nil {
			c.text("identity.fingerprint.kind", fp.Kind, 64)
			c.text("identity.fingerprint.value", fp.Value, 512)
		}
	}
	if a := x.Authorship; a != nil {
		c.text("authorship.author", a.Author, MaxShortText)
		c.text("authorship.license", a.License, 64)
		c.text("authorship.source", a.Source, 2000)
		for where, v := range map[string]string{"authorship.created": a.Created, "authorship.updated": a.Updated} {
			if v != "" {
				if _, err := time.Parse(time.RFC3339, v); err != nil {
					c.bad(where, "has to be a date and time such as 2026-11-18T20:15:00Z")
				}
			}
		}
		if a.Tool != nil {
			c.text("authorship.tool.name", a.Tool.Name, MaxShortText)
			c.text("authorship.tool.version", a.Tool.Version, 64)
		}
	}
	if len(x.Segments) > MaxSegments {
		c.bad("segments", "more than %d segments", MaxSegments)
	}
	ids := map[string]bool{}
	checkID := func(where, id string) {
		if id == "" {
			return
		}
		if !idRe.MatchString(id) {
			c.bad(where, "an id is up to 64 letters, digits and _ . : -")
		} else if ids[id] {
			c.bad(where, "the id %q is used twice", id)
		}
		ids[id] = true
	}
	for i := range x.Segments {
		s, w := &x.Segments[i], fmt.Sprintf("segments[%d]", i)
		checkID(w+".id", s.ID)
		c.oneOf(w+".type", s.Type, SegmentTypes)
		if s.Action != "" {
			c.oneOf(w+".action", s.Action, Actions)
		}
		c.time(w+".start", s.Start)
		if s.End != nil {
			c.time(w+".end", *s.End)
			if *s.End <= s.Start {
				c.bad(w+".end", "has to come after the start")
			}
		}
		c.text(w+".label", s.Label, MaxShortText)
		c.text(w+".note", s.Note, 2000)
	}
	for i, t := range x.Absent {
		c.oneOf(fmt.Sprintf("absent[%d]", i), t, SegmentTypes)
	}
	if len(x.Scenes) > MaxScenes {
		c.bad("scenes", "more than %d scenes", MaxScenes)
	}
	for i := range x.Scenes {
		s, w := &x.Scenes[i], fmt.Sprintf("scenes[%d]", i)
		checkID(w+".id", s.ID)
		c.time(w+".start", s.Start)
		if s.ThumbnailAt != nil {
			c.time(w+".thumbnailAt", *s.ThumbnailAt)
		}
		c.text(w+".title", s.Title, MaxShortText)
		c.text(w+".description", s.Description, 2000)
		c.member(w+".image", s.Image, "images/", x.Files)
	}
	if len(x.Filters) > MaxFilters {
		c.bad("filters", "more than %d filters", MaxFilters)
	}
	for i := range x.Filters {
		f, w := &x.Filters[i], fmt.Sprintf("filters[%d]", i)
		checkID(w+".id", f.ID)
		c.oneOf(w+".category", f.Category, FilterCategories)
		if f.Severity < 0 || f.Severity > 3 {
			c.bad(w+".severity", "has to be 1 (mild), 2 (moderate) or 3 (strong)")
		}
		if f.Action != "" {
			c.oneOf(w+".action", f.Action, FilterActions)
		}
		c.time(w+".start", f.Start)
		c.time(w+".end", f.End)
		if f.End <= f.Start {
			c.bad(w+".end", "has to come after the start")
		}
		c.text(w+".label", f.Label, MaxShortText)
	}
	if m := x.Metadata; m != nil {
		for where, v := range map[string]string{"title": m.Title, "originalTitle": m.OriginalTitle, "sortTitle": m.SortTitle, "edition": m.Edition,
			"tagline": m.Tagline, "contentRating": m.ContentRating, "collection": m.Collection} {
			c.text("metadata."+where, v, MaxShortText)
		}
		c.text("metadata.overview", m.Overview, MaxLongText)
		if m.Year != nil && (*m.Year < 1800 || *m.Year > 3000) {
			c.bad("metadata.year", "has to be a year")
		}
		if m.ReleaseDate != "" {
			if _, err := time.Parse("2006-01-02", m.ReleaseDate); err != nil {
				c.bad("metadata.releaseDate", "has to be a date such as 2023-05-19")
			}
		}
		if m.EpisodeKind != "" {
			c.oneOf("metadata.episodeKind", m.EpisodeKind, EpisodeKinds)
		}
		c.list("metadata.genres", m.Genres, 64)
		c.list("metadata.tags", m.Tags, 64)
		c.list("metadata.studios", m.Studios, MaxShortText)
		c.list("metadata.countries", m.Countries, 64)
		c.list("metadata.languages", m.Languages, 64)
		c.list("metadata.trivia", m.Trivia, 2000)
		if len(m.ExternalIDs) > MaxListItems {
			c.bad("metadata.externalIds", "more than %d ids", MaxListItems)
		}
		for k, v := range m.ExternalIDs {
			if !idRe.MatchString(k) {
				c.bad("metadata.externalIds", "%q isn't a plain name", k)
			}
			c.text("metadata.externalIds."+k, v, 128)
		}
	}
	for _, role := range sortedKeys(x.Artwork) {
		c.oneOf("artwork."+role, role, ArtworkRoles)
		c.member("artwork."+role, x.Artwork[role], "images/", x.Files)
	}
	if p := x.Playback; p != nil {
		if p.StartAt != nil {
			c.time("playback.startAt", *p.StartAt)
		}
		c.text("playback.audioLanguage", p.AudioLanguage, 35)
		c.text("playback.subtitleLanguage", p.SubtitleLanguage, 35)
		if p.SubtitleMode != "" {
			c.oneOf("playback.subtitleMode", p.SubtitleMode, []string{"off", "forced", "always"})
		}
		if v := p.VolumeDB; v != nil && (math.IsNaN(*v) || *v < -30 || *v > 30) {
			c.bad("playback.volumeDb", "has to be between -30 and 30")
		}
		if p.AspectRatio != "" && !regexp.MustCompile(`^\d{1,2}(\.\d{1,3})?:\d{1,2}(\.\d{1,3})?$`).MatchString(p.AspectRatio) {
			c.bad("playback.aspectRatio", "has to be written as 16:9 or 2.39:1")
		}
	}
	if len(x.Subtitles) > 100 {
		c.bad("subtitles", "more than 100 subtitle files")
	}
	for i := range x.Subtitles {
		s, w := &x.Subtitles[i], fmt.Sprintf("subtitles[%d]", i)
		c.member(w+".file", s.File, "subtitles/", x.Files)
		c.text(w+".language", s.Language, 35)
		c.text(w+".title", s.Title, MaxShortText)
	}
	c.text("notes", x.Notes, MaxLongText)
	if len(x.Extensions) > 100 {
		c.bad("extensions", "more than 100 extensions")
	}
	if len(c.problems) > 0 {
		return &Error{Problems: c.problems}
	}
	return nil
}

// ScopeOr is the file's scope, "item" when it doesn't say.
func (x *Info) ScopeOr() string {
	if x.Scope == "" {
		return ScopeItem
	}
	return x.Scope
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
