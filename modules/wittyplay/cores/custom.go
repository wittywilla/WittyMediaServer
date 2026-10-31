package cores

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittyplay"
)

// Custom cores: libretro cores an administrator brings themselves. Everything that
// touches the disk is in this file, and nothing here ever loads one: the server
// test-loads it through stream.Probe, in a runner process, before anyone can use it.
//
// A core moves through two places:
//
//	<dir>/.staging/<token>/      where a file waits while it's tested and looked at
//	<dir>/<id>/<sha12>/          a version someone approved, never changed afterwards
//
// Keeping each version in its own folder means replacing a core can't touch a game
// running on the old file, and rolling back is only a matter of pointing at it again.

// SourceCustom is Core.Source for a custom core.
const SourceCustom = "custom"

// Limits bound what adding a core can cost. They're fields so a test can make a
// "huge file" without writing a gigabyte.
type Limits struct {
	Library int64 // one core file
	Package int64 // one upload or download, zipped or not
	Unpack  int64 // everything a package unpacks to
	Entries int   // how many files a package may hold
}

// DefaultLimits are above the largest cores people really use, which reach several
// hundred megabytes.
var DefaultLimits = Limits{Library: 1 << 30, Package: 3 << 29, Unpack: 4 << 30, Entries: 20000}

// CustomStore is the folder custom cores live in.
type CustomStore struct {
	Dir    string // <data>/gaming/custom-cores
	System string // the BIOS folder a package's system/ files go to
	Placed []string
	Limits Limits
	// Client fetches addresses. It must refuse private addresses when it connects,
	// because a name can resolve to anything. There's deliberately no default.
	Client *http.Client
}

// Staged is a file waiting to be approved.
type Staged struct {
	Token       string
	Dir         string // the whole of this stage
	Library     string // the core file inside it
	OrigName    string
	SHA256      string
	DownloadSHA string // of the file as it arrived, for comparing with a published hash
	Size        int64
	Binary      Binary
	GPUHint     bool // the file mentions graphics APIs, so it might want a card
	Info        *Info
	HasInfo     bool
	Assets      int // files in its assets/ folder
	System      int // files in its system/ folder
	Ignored     []string
	Source      string // upload | url | placed
	SourceURL   string
	Placed      string // the file it came from, when picked from a folder we own
	Created     time.Time
}

func (st *CustomStore) limits() Limits {
	if st.Limits.Library == 0 {
		return DefaultLimits
	}
	return st.Limits
}

func (st *CustomStore) stagingDir() string { return filepath.Join(st.Dir, ".staging") }

// ClearStaging removes whatever was left waiting; a stage doesn't outlive a restart.
func (st *CustomStore) ClearStaging() { _ = os.RemoveAll(st.stagingDir()) }

// Discard removes a stage.
func (st *CustomStore) Discard(s *Staged) {
	if s != nil {
		_ = os.RemoveAll(s.Dir)
	}
}

func (st *CustomStore) newStage() (token, dir string, err error) {
	var b [16]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b[:])
	dir = filepath.Join(st.stagingDir(), token)
	if err = os.MkdirAll(filepath.Join(dir, "v"), 0o700); err != nil {
		return "", "", err
	}
	return token, dir, nil
}

// LibraryExt is what a core file ends in on this server.
func LibraryExt() string { return libraryExt() }

// libraryExts is every extension a core might have, so that a core for another
// system gets told what it is rather than "not a core".
var libraryExts = map[string]bool{".dll": true, ".so": true, ".dylib": true}

var reservedName = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$`)

// CheckFileName refuses a name that tries to mean a path. Names are never used to
// build a path to write to, but one trying this isn't an honest mistake, so it
// doesn't get quietly cleaned up and accepted either.
func CheckFileName(name string) error {
	if name == "" || len(name) > 200 {
		return errors.New("that file name can't be used")
	}
	if strings.ContainsAny(name, "/\\:\x00") || name == "." || name == ".." || strings.Contains(name, "..") {
		return errors.New("a file name can't name folders or climb out of one")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return errors.New("that file name can't be used")
		}
	}
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || reservedName.MatchString(name) {
		return errors.New("that file name can't be used")
	}
	return nil
}

// StageReader takes a core or a zip from an upload, up to the size limit. Nothing is
// held in memory: a low-end server with a large core still has its RAM.
func (st *CustomStore) StageReader(r io.Reader, name string) (*Staged, error) {
	if err := CheckFileName(name); err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !libraryExts[ext] && ext != ".zip" {
		return nil, fmt.Errorf("a core is a file ending in %s, or a .zip holding one; this is neither", libraryExt())
	}
	s, err := st.begin(r, name)
	if err != nil {
		return nil, err
	}
	s.Source = "upload"
	return s, nil
}

// begin writes the incoming file to a new stage, then works out what it is.
func (st *CustomStore) begin(r io.Reader, name string) (*Staged, error) {
	token, dir, err := st.newStage()
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(dir)
		}
	}()
	incoming := filepath.Join(dir, "incoming")
	f, err := os.OpenFile(incoming, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	max := st.limits().Package
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, sum), io.LimitReader(r, max+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	if n > max {
		return nil, tooLarge(max)
	}
	s, err := st.identify(token, dir, incoming, name)
	if err != nil {
		return nil, err
	}
	s.DownloadSHA = hex.EncodeToString(sum.Sum(nil))
	ok = true
	return s, nil
}

func tooLarge(max int64) error {
	return fmt.Errorf("this file is larger than the %d MB a core may be", max>>20)
}

// identify sorts out a stage's incoming file: unpacks it if it's a zip, then reads
// the core's headers and hash.
func (st *CustomStore) identify(token, dir, incoming, name string) (*Staged, error) {
	s := &Staged{Token: token, Dir: dir, OrigName: name, Created: time.Now()}
	head := make([]byte, 4)
	f, err := os.Open(incoming)
	if err != nil {
		return nil, err
	}
	_, _ = io.ReadFull(f, head)
	_ = f.Close()
	version := filepath.Join(dir, "v")
	if bytes.Equal(head, []byte("PK\x03\x04")) {
		if err := st.unpack(incoming, version, s); err != nil {
			return nil, err
		}
		_ = os.Remove(incoming)
	} else {
		if st.fileTooBig(incoming) {
			return nil, tooLarge(st.limits().Library)
		}
		base := filepath.Base(name)
		if !libraryExts[strings.ToLower(filepath.Ext(base))] {
			base = "core" + libraryExt()
		}
		s.OrigName = base
		s.Library = filepath.Join(version, base)
		if err := os.Rename(incoming, s.Library); err != nil {
			return nil, err
		}
	}
	bin, err := ReadBinary(s.Library)
	if err != nil {
		return nil, err
	}
	if why := bin.Refusal(); why != "" {
		return nil, errors.New(why)
	}
	s.Binary = bin
	if s.SHA256, s.Size, s.GPUHint, err = hashAndScan(s.Library); err != nil {
		return nil, err
	}
	return s, nil
}

func (st *CustomStore) fileTooBig(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Size() > st.limits().Library
}

// Words only a program that draws with a graphics card has reason to contain. A hit
// means "might", which is all that can be said without a game running.
var gpuHints = [][]byte{[]byte("vkCreateInstance"), []byte("glCreateShader"), []byte("glCreateProgram")}

func hashAndScan(p string) (sum string, size int64, gpu bool, err error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, false, err
	}
	defer f.Close()
	h := sha256.New()
	const keep = 32
	buf := make([]byte, 1<<20)
	var tail []byte
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
			size += int64(n)
			if !gpu {
				window := append(tail[:len(tail):len(tail)], buf[:n]...)
				for _, w := range gpuHints {
					gpu = gpu || bytes.Contains(window, w)
				}
				tail = append(tail[:0], window[max(0, len(window)-keep):]...)
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return "", 0, false, rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), size, gpu, nil
}

// HashFile is a file's SHA-256.
func HashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// unpack takes a package apart. Its layout is:
//
//	<name>_libretro.<ext>   the core, at the top
//	<name>_libretro.info    its info file, optional
//	assets/...              files only this core sees
//	system/...              files for the BIOS folder
//
// Everything else is ignored, not unpacked. One entry that tries anything odd refuses
// the whole package, because an honest package has no reason to.
func (st *CustomStore) unpack(zipPath, version string, s *Staged) error {
	lim := st.limits()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return errors.New("this .zip can't be read: " + err.Error())
	}
	defer zr.Close()
	if len(zr.File) > lim.Entries {
		return fmt.Errorf("this package holds more than %d files", lim.Entries)
	}
	seen := map[string]bool{}
	var mine, foreign []*zip.File
	var infos []*zip.File
	var keep []*zip.File
	for _, f := range zr.File {
		name := f.Name
		isDir := strings.HasSuffix(name, "/")
		clean := strings.TrimSuffix(name, "/")
		if err := checkEntryName(clean); err != nil {
			return fmt.Errorf("this package has a file called %q, which %v", name, err)
		}
		if !isDir && !f.Mode().IsRegular() {
			return fmt.Errorf("this package has a link or special file (%q), which isn't allowed", name)
		}
		lower := strings.ToLower(clean)
		if seen[lower] {
			return fmt.Errorf("this package names %q twice", name)
		}
		seen[lower] = true
		if isDir {
			continue
		}
		top := !strings.Contains(clean, "/")
		ext := strings.ToLower(path.Ext(clean))
		switch {
		case top && libraryExts[ext] && ext == libraryExt():
			mine = append(mine, f)
		case top && libraryExts[ext]:
			foreign = append(foreign, f)
		case top && ext == ".info":
			infos = append(infos, f)
		case strings.HasPrefix(lower, "assets/"), strings.HasPrefix(lower, "system/"):
			keep = append(keep, f)
		default:
			if len(s.Ignored) < 20 {
				s.Ignored = append(s.Ignored, clean)
			}
		}
	}
	switch {
	case len(mine) > 1:
		return errors.New("this package holds several cores; add them one at a time")
	case len(mine) == 0 && len(foreign) > 0:
		return fmt.Errorf("this package holds a core for another kind of computer (%s). Cores for this server end in %s", foreign[0].Name, libraryExt())
	case len(mine) == 0:
		return fmt.Errorf("there's no core in this package: it should hold a file ending in %s at the top level", libraryExt())
	}
	var used int64
	copyOut := func(f *zip.File, to string, limit int64) error {
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("%q in this package can't be read: %v", f.Name, err)
		}
		defer rc.Close()
		out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		// What's counted is what really comes out, not what the header claims.
		n, err := io.Copy(out, io.LimitReader(rc, limit+1))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("%q in this package can't be unpacked: %v", f.Name, err)
		}
		if n > limit {
			return fmt.Errorf("%q unpacks to more than the %d MB allowed", f.Name, limit>>20)
		}
		used += n
		if used > lim.Unpack {
			return fmt.Errorf("this package unpacks to more than the %d MB allowed", lim.Unpack>>20)
		}
		return nil
	}
	lib := mine[0]
	s.OrigName = lib.Name
	s.Library = filepath.Join(version, lib.Name)
	if err := copyOut(lib, s.Library, lim.Library); err != nil {
		return err
	}
	if len(infos) > 0 {
		chosen := infos[0]
		stem := strings.TrimSuffix(lib.Name, filepath.Ext(lib.Name))
		for _, f := range infos {
			if strings.EqualFold(strings.TrimSuffix(f.Name, ".info"), stem) {
				chosen = f
			}
		}
		if err := copyOut(chosen, filepath.Join(version, infoFile), MaxInfoSize); err != nil {
			return err
		}
		if err := st.readInfo(version, s); err != nil {
			return err
		}
	}
	for _, f := range keep {
		if err := copyOut(f, filepath.Join(version, filepath.FromSlash(f.Name)), lim.Unpack); err != nil {
			return err
		}
		if strings.HasPrefix(strings.ToLower(f.Name), "assets/") {
			s.Assets++
		} else {
			s.System++
		}
	}
	return nil
}

// infoFile is what a core's info file is called inside its version folder.
const infoFile = "core.info"

func (st *CustomStore) readInfo(version string, s *Staged) error {
	f, err := os.Open(filepath.Join(version, infoFile))
	if err != nil {
		return err
	}
	defer f.Close()
	info := ParseInfo(f)
	s.Info, s.HasInfo = &info, true
	return nil
}

// checkEntryName refuses a path in a package that climbs out, or means something
// else to Windows than it looks like.
func checkEntryName(name string) error {
	if name == "" || len(name) > 240 {
		return errors.New("is too long to use")
	}
	if strings.Contains(name, "\\") || strings.ContainsAny(name, ":\x00") || !filepath.IsLocal(filepath.FromSlash(name)) || strings.HasPrefix(name, "/") {
		return errors.New("climbs out of its folder")
	}
	segs := strings.Split(name, "/")
	if len(segs) > 16 {
		return errors.New("is nested too deeply")
	}
	for _, seg := range segs {
		if seg == "" || seg == "." || seg == ".." {
			return errors.New("climbs out of its folder")
		}
		if strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") || reservedName.MatchString(seg) {
			return errors.New("isn't a name that can be used")
		}
		for _, r := range seg {
			if r < 0x20 || r == 0x7f {
				return errors.New("isn't a name that can be used")
			}
		}
	}
	return nil
}

// ---- from an address ----

// CheckFetchURL refuses an address before anything connects. Which machine a name
// really leads to is checked by the client when it connects, because that's the only
// moment the answer means anything; this catches what never needs a lookup.
func CheckFetchURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, errors.New("that isn't a web address")
	}
	if u.Scheme != "https" {
		return nil, errors.New("only https addresses can be used, so the file can't be changed on its way here")
	}
	if u.User != nil {
		return nil, errors.New("a web address with a password in it can't be used")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || host == "localhost" || !strings.Contains(host, ".") && !strings.Contains(host, ":") ||
		strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") ||
		strings.HasSuffix(host, ".lan") || strings.HasSuffix(host, ".home") || strings.HasSuffix(host, ".arpa") {
		return nil, errors.New("only addresses on the public internet can be used, not this server or your local network")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()) {
		return nil, errors.New("only addresses on the public internet can be used, not this server or your local network")
	}
	return u, nil
}

// fetchIdle is how long a download may sit with nothing arriving.
const fetchIdle = time.Minute

// StageURL downloads a core or zip. The client refuses private addresses when it
// connects, redirects stay on https and are re-checked, and the size is capped however
// much the far end says it will send.
func (st *CustomStore) StageURL(ctx context.Context, raw string) (*Staged, error) {
	if st.Client == nil {
		return nil, errors.New("this server has no way to fetch an address")
	}
	u, err := CheckFetchURL(raw)
	if err != nil {
		return nil, err
	}
	client := *st.Client
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("the address redirected too many times")
		}
		if _, err := CheckFetchURL(r.URL.String()); err != nil {
			return fmt.Errorf("the address redirected somewhere that isn't allowed: %w", err)
		}
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, friendlyFetchError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the server at that address answered %s", resp.Status)
	}
	if max := st.limits().Package; resp.ContentLength > max {
		return nil, tooLarge(max)
	}
	name := fetchedName(resp.Request.URL)
	// A stalled connection shouldn't hold a stage open forever.
	idle := time.AfterFunc(fetchIdle, cancel)
	defer idle.Stop()
	s, err := st.begin(&idleReader{r: resp.Body, timer: idle}, name)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("the download stopped arriving")
		}
		return nil, err
	}
	s.Source, s.SourceURL = "url", u.String()
	return s, nil
}

type idleReader struct {
	r     io.Reader
	timer *time.Timer
}

func (i *idleReader) Read(p []byte) (int, error) {
	i.timer.Reset(fetchIdle)
	return i.r.Read(p)
}

// friendlyFetchError keeps the useful part of Go's long error chains.
func friendlyFetchError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not on the internet"):
		return errors.New("only addresses on the public internet can be used, not this server or your local network")
	case strings.Contains(msg, "redirected"):
		var ue *url.Error
		if errors.As(err, &ue) {
			return ue.Err
		}
	}
	return errors.New("that address couldn't be reached: " + msg)
}

// fetchedName is only ever a label and a hint at what the file is.
func fetchedName(u *url.URL) string {
	name := path.Base(u.Path)
	if CheckFileName(name) != nil {
		return "core"
	}
	return name
}

// ---- from a folder ----

// PlacedFile is a file someone put where cores go.
type PlacedFile struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Folder string `json:"folder"`
	Ours   bool   `json:"ours"` // our own cores folder; a copy is moved out of it once approved
	Size   int64  `json:"size"`
	Refuse string `json:"refuse,omitempty"` // why it can't be picked, such as being a link
}

// ListPlaced lists candidate files. A request can only name a Key from this list, so
// no request can name a path: there's nothing to climb out of.
func (st *CustomStore) ListPlaced() []PlacedFile {
	out := []PlacedFile{}
	for i, dir := range st.Placed {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			name := e.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if !libraryExts[ext] && ext != ".zip" {
				continue
			}
			if CheckFileName(name) != nil {
				continue
			}
			p := PlacedFile{Key: fmt.Sprintf("%d/%s", i, name), Name: name, Folder: dir, Ours: i == 0}
			if !e.Type().IsRegular() {
				p.Refuse = "It's a link or a folder. Put the file itself here."
			} else if info, err := e.Info(); err == nil {
				p.Size = info.Size()
				if p.Size > st.limits().Package {
					p.Refuse = tooLarge(st.limits().Package).Error()
				}
			}
			out = append(out, p)
			if len(out) >= 2000 {
				return out
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// StagePlaced copies a listed file in. It's a copy, so a link could only ever have
// shown us its contents, and what was tested is what runs. It's refused anyway: a
// link out of the folder isn't something anyone should need.
func (st *CustomStore) StagePlaced(key string) (*Staged, error) {
	var found *PlacedFile
	for _, p := range st.ListPlaced() {
		if p.Key == key {
			found = &p
			break
		}
	}
	if found == nil {
		return nil, errors.New("that file isn't in the cores folder")
	}
	if found.Refuse != "" {
		return nil, errors.New(found.Refuse)
	}
	src := filepath.Join(found.Folder, found.Name)
	info, err := os.Lstat(src)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("it's a link or a folder. Put the file itself here")
	}
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s, err := st.begin(f, found.Name)
	if err != nil {
		return nil, err
	}
	s.Source = "placed"
	if found.Ours {
		s.Placed = src
	}
	// A matching info file beside it comes along, as it would in a package.
	stem := strings.TrimSuffix(found.Name, filepath.Ext(found.Name))
	if b, err := readRegular(filepath.Join(found.Folder, stem+".info"), MaxInfoSize); err == nil && !s.HasInfo {
		version := filepath.Dir(s.Library)
		if err := os.WriteFile(filepath.Join(version, infoFile), b, 0o644); err == nil {
			_ = st.readInfo(version, s)
		}
	}
	return s, nil
}

func readRegular(p string, limit int64) ([]byte, error) {
	info, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("not usable")
	}
	return os.ReadFile(p)
}

// ---- approved versions ----

// SuggestID turns a file name into a core's name here. A name our own table uses gets
// a suffix: a custom build isn't the core the table vouches for, and must never be
// mistaken for it or change which core a console already uses.
func SuggestID(fileName, reported string) string {
	id := cleanID(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if id == "" || id == "core" {
		if alt := cleanID(reported); alt != "" {
			id = alt
		}
	}
	if id == "" {
		id = "custom-core"
	}
	return AvoidTableID(id)
}

// AvoidTableID gives a name in our table a suffix.
func AvoidTableID(id string) string {
	if _, known := wittyplay.CoreLicense(id); known && !wittyplay.IsNonCommercial(id) {
		suffixed := id + "-custom"
		if len(suffixed) <= 64 {
			return suffixed
		}
	}
	return id
}

func cleanID(s string) string {
	s = strings.ToLower(s)
	for _, cut := range []string{"_libretro", "-libretro", ".libretro"} {
		s = strings.TrimSuffix(s, cut)
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "_"):
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_-")
	if len(out) > 56 {
		out = strings.TrimRight(out[:56], "_-")
	}
	if !validID.MatchString(out) {
		return ""
	}
	return out
}

// ValidCustomID says whether a name can be a custom core's.
func ValidCustomID(id string) bool { return validID.MatchString(id) && id != ".staging" }

// VersionFolder is where one approved version lives.
func (st *CustomStore) VersionFolder(id, sum string) string {
	return filepath.Join(st.Dir, id, sum[:12])
}

// CorePath is the file of one approved version.
func (st *CustomStore) CorePath(id, sum string) string {
	return filepath.Join(st.VersionFolder(id, sum), fileName(id))
}

// AssetsPath is a version's own folder of support files, or "" when it has none.
func (st *CustomStore) AssetsPath(id, sum string) string {
	p := filepath.Join(st.VersionFolder(id, sum), "assets")
	if info, err := os.Lstat(p); err == nil && info.IsDir() {
		return p
	}
	return ""
}

// Commit makes a stage an approved version of a core. It re-reads the hash first: what
// gets approved has to be what was tested. It returns the system files it added to
// the BIOS folder, so removing the core can take back only those, and anything it left
// because the owner already has a file by that name.
func (st *CustomStore) Commit(s *Staged, id string) (added, kept []string, err error) {
	if !ValidCustomID(id) {
		return nil, nil, errors.New("that name can't be used for a core")
	}
	sum, err := HashFile(s.Library)
	if err != nil {
		return nil, nil, err
	}
	if sum != s.SHA256 {
		return nil, nil, errors.New("the file changed after it was tested, so it can't be approved")
	}
	if err := os.MkdirAll(filepath.Join(st.Dir, id), 0o755); err != nil {
		return nil, nil, err
	}
	version := filepath.Dir(s.Library)
	if err := os.Rename(s.Library, filepath.Join(version, fileName(id))); err != nil {
		return nil, nil, err
	}
	if added, kept, err = st.moveSystem(filepath.Join(version, "system")); err != nil {
		return nil, nil, err
	}
	dest := st.VersionFolder(id, sum)
	if err := os.RemoveAll(dest); err != nil {
		return nil, nil, fmt.Errorf("the same version is already here and in use: %w", err)
	}
	if err := os.Rename(version, dest); err != nil {
		return nil, nil, err
	}
	_ = os.RemoveAll(s.Dir)
	if err := os.Chmod(st.CorePath(id, sum), 0o755); err != nil {
		return nil, nil, err
	}
	if s.Placed != "" {
		// It has moved to where approved cores live. Left behind, it would only
		// look like another core waiting to be added.
		_ = os.Remove(s.Placed)
		_ = os.Remove(strings.TrimSuffix(s.Placed, filepath.Ext(s.Placed)) + ".info")
	}
	return added, kept, nil
}

// moveSystem puts a package's system/ files in the BIOS folder. A BIOS file is
// somebody's own, so nothing there is ever overwritten.
func (st *CustomStore) moveSystem(from string) (added, kept []string, err error) {
	if _, serr := os.Lstat(from); serr != nil {
		return nil, nil, nil
	}
	if st.System == "" {
		return nil, nil, errors.New("there is no BIOS folder to put these files in")
	}
	err = filepath.WalkDir(from, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return errors.New("a link or special file appeared while unpacking")
		}
		rel, err := filepath.Rel(from, p)
		if err != nil || !filepath.IsLocal(rel) {
			return errors.New("a file tried to leave the BIOS folder")
		}
		to := filepath.Join(st.System, rel)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		if _, err := os.Lstat(to); err == nil {
			kept = append(kept, filepath.ToSlash(rel))
			return nil
		}
		if err := os.Rename(p, to); err != nil {
			return err
		}
		added = append(added, filepath.ToSlash(rel))
		return nil
	})
	return added, kept, err
}

// AddExtras adds an info file or a package's assets/ and system/ folders to a version
// already approved. The core file itself can't change this way.
func (st *CustomStore) AddExtras(r io.Reader, name, id, sum string) (added, kept []string, err error) {
	if err := CheckFileName(name); err != nil {
		return nil, nil, err
	}
	version := st.VersionFolder(id, sum)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".info":
		b, err := io.ReadAll(io.LimitReader(r, MaxInfoSize+1))
		if err != nil {
			return nil, nil, err
		}
		if len(b) > MaxInfoSize {
			return nil, nil, errors.New("this is too long to be a core's info file")
		}
		return nil, nil, writeFile(filepath.Join(version, infoFile), b, 0o644)
	case ".zip":
	default:
		return nil, nil, errors.New("add a core's .info file, or a .zip holding assets/ or system/ folders")
	}
	token, dir, err := st.newStage()
	if err != nil {
		return nil, nil, err
	}
	_ = token
	defer os.RemoveAll(dir)
	incoming := filepath.Join(dir, "incoming")
	f, err := os.OpenFile(incoming, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	lim := st.limits()
	n, err := io.Copy(f, io.LimitReader(r, lim.Package+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, nil, err
	}
	if n > lim.Package {
		return nil, nil, tooLarge(lim.Package)
	}
	zr, err := zip.OpenReader(incoming)
	if err != nil {
		return nil, nil, errors.New("this .zip can't be read: " + err.Error())
	}
	defer zr.Close()
	if len(zr.File) > lim.Entries {
		return nil, nil, fmt.Errorf("this package holds more than %d files", lim.Entries)
	}
	unpacked := filepath.Join(dir, "v")
	var used int64
	files := 0
	for _, f := range zr.File {
		clean := strings.TrimSuffix(f.Name, "/")
		if err := checkEntryName(clean); err != nil {
			return nil, nil, fmt.Errorf("this package has a file called %q, which %v", f.Name, err)
		}
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		if !f.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("this package has a link or special file (%q), which isn't allowed", f.Name)
		}
		lower := strings.ToLower(clean)
		if !strings.HasPrefix(lower, "assets/") && !strings.HasPrefix(lower, "system/") {
			continue
		}
		to := filepath.Join(unpacked, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return nil, nil, err
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, err
		}
		out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			rc.Close()
			return nil, nil, err
		}
		written, err := io.Copy(out, io.LimitReader(rc, lim.Unpack-used+1))
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, nil, err
		}
		used += written
		if used > lim.Unpack {
			return nil, nil, fmt.Errorf("this package unpacks to more than the %d MB allowed", lim.Unpack>>20)
		}
		files++
	}
	if files == 0 {
		return nil, nil, errors.New("this package holds nothing to add: it needs an assets/ or system/ folder")
	}
	if added, kept, err = st.moveSystem(filepath.Join(unpacked, "system")); err != nil {
		return nil, nil, err
	}
	if from := filepath.Join(unpacked, "assets"); pathExists(from) {
		to := filepath.Join(version, "assets")
		if err := mergeInto(from, to); err != nil {
			return nil, nil, err
		}
	}
	return added, kept, nil
}

func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// mergeInto moves one folder's files into another, replacing files of the same name.
func mergeInto(from, to string) error {
	return filepath.WalkDir(from, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(from, p)
		if err != nil || !filepath.IsLocal(rel) {
			return errors.New("a file tried to leave its folder")
		}
		dest := filepath.Join(to, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.Rename(p, dest)
	})
}

// ReadVersionInfo reads a version's info file, if it has one.
func (st *CustomStore) ReadVersionInfo(id, sum string) *Info {
	b, err := readRegular(filepath.Join(st.VersionFolder(id, sum), infoFile), MaxInfoSize)
	if err != nil {
		return nil
	}
	info := ParseInfo(bytes.NewReader(b))
	return &info
}

// CoreFileState is what a core's file looks like against what was approved.
type CoreFileState struct {
	State string `json:"state"`           // ok | changed | missing | link
	Found string `json:"found,omitempty"` // the hash it has now
}

// Verify reads the whole file and compares it. It can't be a link: everything in this
// folder got here by being written by us.
func (st *CustomStore) Verify(id, sum string) CoreFileState {
	p := st.CorePath(id, sum)
	info, err := os.Lstat(p)
	if err != nil {
		return CoreFileState{State: "missing"}
	}
	if !info.Mode().IsRegular() {
		return CoreFileState{State: "link"}
	}
	got, err := HashFile(p)
	if err != nil {
		return CoreFileState{State: "missing"}
	}
	if got != sum {
		return CoreFileState{State: "changed", Found: got}
	}
	return CoreFileState{State: "ok"}
}

// RemoveVersion deletes one kept version. A file in use on Windows simply stays until
// next time.
func (st *CustomStore) RemoveVersion(id, sum string) { _ = os.RemoveAll(st.VersionFolder(id, sum)) }

// RemoveCore deletes everything of a core, and only the files it added to the BIOS
// folder. Saves live elsewhere and are never touched.
func (st *CustomStore) RemoveCore(id string, systemFiles []string) error {
	if !ValidCustomID(id) {
		return errors.New("bad core name")
	}
	for _, rel := range systemFiles {
		native := filepath.FromSlash(rel)
		if filepath.IsLocal(native) && st.System != "" {
			_ = os.Remove(filepath.Join(st.System, native))
		}
	}
	return os.RemoveAll(filepath.Join(st.Dir, id))
}
