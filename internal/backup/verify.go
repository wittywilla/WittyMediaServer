package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/metafolder"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// Limits on an archive, none of which a real backup comes near except the totals, which
// the free disk space usually decides first. Exposed as variables so tests can lower them.
var (
	maxEntries          = 2_000_000
	maxManifest   int64 = 512 << 20
	maxDatabase   int64 = 64 << 30
	maxFileSize   int64 = 256 << 30
	maxTotalSize  int64 = 4 << 40
	maxBombRatio  int64 = 400 // how far an entry over 1 MB may expand; SQLite with empty pages gets to 50
	minRatioBytes int64 = 1 << 20
	// maxListedManifest is how much of a manifest is read to list a backup. One with no password
	// names each file, so this is passed at around 100,000 of them, and all that's lost then is
	// the version beside its name in the list. Checking one reads the whole of it.
	maxListedManifest int64 = 16 << 20
	freeBytes               = sysinfo.FreeBytes
)

// Refusal says why an archive won't be used, in words for the person holding it.
type Refusal struct {
	File   string `json:"file,omitempty"`
	Reason string `json:"reason"`
}

func (r *Refusal) Error() string {
	if r.File != "" {
		return "This backup can't be used: " + r.File + ": " + r.Reason + "."
	}
	return "This backup can't be used: " + r.Reason + "."
}

// Refused makes one.
func Refused(file, reason string) error { return &Refusal{File: file, Reason: reason} }

// programExtensions can never come out of an archive, whichever folder they say they're
// for. Nothing in a backup is a program or a web page.
var programExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bat": true, ".cmd": true, ".com": true, ".ps1": true, ".psm1": true, ".sh": true, ".bash": true,
	".py": true, ".pyc": true, ".rb": true, ".pl": true, ".php": true, ".js": true, ".mjs": true, ".cjs": true, ".html": true, ".htm": true, ".xhtml": true, ".svg": true,
	".jar": true, ".class": true, ".app": true, ".msi": true, ".msix": true, ".scr": true, ".lnk": true, ".url": true, ".desktop": true, ".hta": true, ".vbs": true,
	".wsf": true, ".wasm": true, ".pif": true, ".cpl": true, ".sys": true, ".drv": true, ".service": true, ".plist": true, ".deb": true, ".rpm": true, ".pkg": true, ".dmg": true,
}

// checkProgramName refuses a name that is, or could pass for, a program.
func checkProgramName(rel string) error {
	if programExtensions[strings.ToLower(path.Ext(rel))] {
		return errors.New("programs and web pages can't be in a backup")
	}
	return nil
}

func mustBePicture(rel string, head []byte) error {
	if err := checkProgramName(rel); err != nil {
		return err
	}
	if metafolder.PictureExt(head) == "" {
		return errors.New("it should be a picture, and isn't a JPEG, PNG, WebP or GIF")
	}
	return nil
}

func notAProgram(rel string, head []byte) error {
	if err := checkProgramName(rel); err != nil {
		return err
	}
	// Windows, Linux, macOS and script programs, whatever they're called.
	switch {
	case bytes.HasPrefix(head, []byte("MZ\x90\x00")),
		bytes.HasPrefix(head, []byte("\x7fELF")),
		bytes.HasPrefix(head, []byte("\xcf\xfa\xed\xfe")), bytes.HasPrefix(head, []byte("\xce\xfa\xed\xfe")), bytes.HasPrefix(head, []byte("\xca\xfe\xba\xbe")),
		bytes.HasPrefix(head, []byte("#!")):
		return errors.New("it's a program")
	}
	return nil
}

func metadataFile(rel string, head []byte) error {
	if err := checkProgramName(rel); err != nil {
		return err
	}
	base := path.Base(rel)
	switch strings.ToLower(path.Ext(base)) {
	case ".json", ".txt":
		return nil
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		if metafolder.PictureExt(head) == "" {
			return errors.New("it's named as a picture, and isn't one")
		}
		return nil
	}
	return errors.New("only text files and pictures belong in the metadata folder")
}

func mustBeJSONFile(rel string, _ []byte) error {
	if strings.ToLower(path.Ext(rel)) != ".json" {
		return errors.New("only JSON files belong here")
	}
	return nil
}

func mustBeZipName(rel string, head []byte) error {
	if strings.Contains(rel, "/") || strings.ToLower(path.Ext(rel)) != ".zip" {
		return errors.New("a theme is one ZIP file")
	}
	if !bytes.HasPrefix(head, []byte("PK")) {
		return errors.New("it isn't a ZIP file")
	}
	return nil
}

func moduleSettingsFile(rel string, _ []byte) error {
	if !moduleSettings[rel] {
		return errors.New("only a module's settings can be in a backup, never its program")
	}
	return nil
}

var reservedWindowsNames = map[string]bool{"con": true, "prn": true, "aux": true, "nul": true, "com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true, "lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true}

// unsafeName says what's wrong with a name in an archive, on any platform this server runs
// on, whichever one it's on now. Empty means nothing.
func unsafeName(name string) string {
	switch {
	case name == "":
		return "it has no name"
	case len(name) > 1024:
		return "its name is too long"
	case !utf8.ValidString(name):
		return "its name isn't valid text"
	case strings.ContainsAny(name, "\\:*?\"<>|"):
		return "its name has a character that could lead somewhere else"
	case strings.HasPrefix(name, "/"):
		return "it has an absolute path"
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f || unicode.Is(unicode.Cf, r) {
			return "its name has a hidden character"
		}
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "its path tries to leave its folder"
		}
		if len(seg) > 255 {
			return "its name is too long"
		}
		if strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") {
			return "its name ends in a way Windows would change"
		}
		stem, _, _ := strings.Cut(seg, ".")
		if reservedWindowsNames[strings.ToLower(stem)] {
			return "its name is reserved by Windows"
		}
	}
	return ""
}

// peekManifest reads the outer manifest of an archive, with limits, for a listing.
func peekManifest(p string) (*Manifest, error) {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == ManifestName && f.UncompressedSize64 < uint64(maxListedManifest) {
			m, err := readManifest(f, maxListedManifest)
			return m, err
		}
	}
	return nil, errors.New("no manifest")
}

func readManifest(f *zip.File, limit int64) (*Manifest, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, Refused(ManifestName, "it can't be read")
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, Refused(ManifestName, "it can't be read")
	}
	if int64(len(b)) > limit {
		return nil, Refused(ManifestName, "it's far too large")
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&m); err != nil {
		return nil, Refused(ManifestName, "it isn't readable, so this isn't a backup or it's damaged")
	}
	if m.App != "WittyMediaServer" {
		return nil, Refused(ManifestName, "this isn't a WittyMediaServer backup")
	}
	if m.Format < 1 {
		return nil, Refused(ManifestName, "it doesn't say what format it's in")
	}
	if m.Format > FormatVersion {
		return nil, &Refusal{Reason: fmt.Sprintf("it was made by a newer version (%s). Update this server first", cleanWords(m.ServerVersion))}
	}
	return &m, nil
}

// cleanWords keeps text from an archive harmless in a message.
func cleanWords(s string) string { return metafolder.Line(s, 80) }

// checkEntries refuses anything in a ZIP's own directory that shouldn't be there. It
// reads no contents.
func checkEntries(files []*zip.File) error {
	if len(files) > maxEntries {
		return &Refusal{Reason: "it has far too many files"}
	}
	seen := make(map[string]bool, len(files))
	var total int64
	for _, f := range files {
		if why := unsafeName(strings.TrimSuffix(f.Name, "/")); why != "" {
			return Refused(printableName(f.Name), why)
		}
		mode := f.Mode()
		if mode&os.ModeType != 0 && !mode.IsDir() {
			return Refused(printableName(f.Name), "it's a link or a special file, which can't be in a backup")
		}
		if mode&os.ModeSetuid != 0 || mode&os.ModeSetgid != 0 {
			return Refused(printableName(f.Name), "it asks to run with someone else's rights")
		}
		if f.Flags&0x1 != 0 {
			return Refused(printableName(f.Name), "it's encrypted with ZIP's own passwords, which this server doesn't use")
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return Refused(printableName(f.Name), "it's compressed in a way a backup never is")
		}
		key := strings.ToLower(f.Name)
		if seen[key] {
			return Refused(printableName(f.Name), "it's in the archive twice")
		}
		seen[key] = true
		size := int64(f.UncompressedSize64)
		if size < 0 || size > maxFileSize {
			return Refused(printableName(f.Name), "it's larger than any backup holds")
		}
		if total += size; total > maxTotalSize || total < 0 {
			return &Refusal{Reason: "its files add up to more than any backup holds"}
		}
		if size > minRatioBytes {
			if comp := int64(f.CompressedSize64); comp == 0 || size/comp > maxBombRatio {
				return Refused(printableName(f.Name), "it would expand to far more than it should, which is how a ZIP bomb works")
			}
		}
	}
	return nil
}

// printableName keeps a hostile name harmless in a message.
func printableName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() > 120 {
			b.WriteString("…")
			break
		}
		if r < 0x20 || r == 0x7f || unicode.Is(unicode.Cf, r) {
			b.WriteRune('?')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Inspection is what a check of an archive found: all a person needs to decide, and
// what's needed to carry on. Nothing has been touched when it's made.
type Inspection struct {
	Name          string        `json:"name"`
	Size          int64         `json:"size"`
	ServerVersion string        `json:"serverVersion"`
	Created       time.Time     `json:"created"`
	Protected     bool          `json:"protected"`
	Older         bool          `json:"older"` // made with an older database, which is brought up to date
	Parts         []PartContent `json:"parts"`
	Accounts      []AccountInfo `json:"accounts"`
	Libraries     []LibraryInfo `json:"libraries"`
	Addresses     []string      `json:"addresses"`  // places on the network this backup would make the server contact
	Dropped       []string      `json:"dropped"`    // what won't be taken from it, and why
	HandEdited    []string      `json:"handEdited"` // metadata files changed since it was made
	Problems      []string      `json:"problems"`   // file:line, in those files
	Notes         []string      `json:"notes"`
	Needed        int64         `json:"needed"` // disk space to unpack it

	man     *Manifest
	zipPath string // the inner archive, as a plain ZIP
	workDir string
}

// PartContent is one part of what's inside.
type PartContent struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Desc     string   `json:"desc"`
	Files    int      `json:"files"`
	Bytes    int64    `json:"bytes"`
	Rows     int64    `json:"rows"`
	Needs    []string `json:"needs,omitempty"`
	HasFiles bool     `json:"hasFiles"`
}

// AccountInfo says who'd be able to sign in afterwards.
type AccountInfo struct {
	Name  string `json:"name"`
	Admin bool   `json:"admin"`
	Kind  string `json:"kind,omitempty"`
}

// LibraryInfo says which folders on this machine the server would be pointed at.
type LibraryInfo struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Folders []string `json:"folders"`
	Items   int64    `json:"items"`
}

// Close removes what checking left behind.
func (in *Inspection) Close() {
	if in != nil && in.workDir != "" {
		_ = os.RemoveAll(in.workDir)
	}
}

// Inspect checks an archive from end to end. It verifies, and touches nothing outside its
// own work folder. A bad archive returns a *Refusal, a wrong password errWrongPassword.
func Inspect(ctx context.Context, archive, password string, f Folders, progress func(Step)) (in *Inspection, err error) {
	defer func() {
		// The file is someone else's work, and readers of it aren't all ours.
		if p := recover(); p != nil {
			err = &Refusal{Reason: fmt.Sprintf("reading it went wrong in a way it shouldn't (%v)", p)}
		}
		if err != nil && in != nil {
			in.Close()
			in = nil
		}
	}()
	st, err := os.Lstat(archive)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, &Refusal{Reason: "it isn't an ordinary file"}
	}
	if err := os.MkdirAll(f.Dir(), 0o700); err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(f.Dir(), ".restore-check-")
	if err != nil {
		return nil, err
	}
	in = &Inspection{Name: filepath.Base(archive), Size: st.Size(), workDir: work,
		Accounts: []AccountInfo{}, Libraries: []LibraryInfo{}, Addresses: []string{}, Dropped: []string{}, HandEdited: []string{}, Problems: []string{}, Notes: []string{}, Parts: []PartContent{}}
	say := func(phase string, done, total int64) {
		if progress != nil {
			progress(Step{Phase: phase, Done: done, Total: total})
		}
	}

	zr, err := zip.OpenReader(archive)
	if err != nil {
		return in, &Refusal{Reason: "it isn't a ZIP file, or it's damaged"}
	}
	defer zr.Close()
	if err := checkEntries(zr.File); err != nil {
		return in, err
	}
	var manEntry, payload *zip.File
	for _, e := range zr.File {
		switch e.Name {
		case ManifestName:
			manEntry = e
		case PayloadName:
			payload = e
		}
	}
	if manEntry == nil {
		return in, &Refusal{Reason: "it has no manifest, so it isn't a WittyMediaServer backup"}
	}
	man, err := readManifest(manEntry, maxManifest)
	if err != nil {
		return in, err
	}
	in.Protected = man.Crypt != nil
	in.zipPath = archive

	files := zr.File
	if man.Crypt != nil {
		if payload == nil {
			return in, Refused(PayloadName, "it's missing")
		}
		if len(zr.File) > 3 {
			return in, &Refusal{Reason: "a protected backup holds only its sealed payload, and this has more"}
		}
		if err := man.Crypt.check(); err != nil {
			return in, err
		}
		if password == "" {
			return in, ErrPasswordNeeded
		}
		say("Opening with the password", 0, int64(payload.UncompressedSize64))
		rc, err := payload.Open()
		if err != nil {
			return in, Refused(PayloadName, "it can't be read")
		}
		defer rc.Close()
		if free := freeBytes(work); free >= 0 && free < int64(payload.UncompressedSize64) {
			return in, fmt.Errorf("there isn't room to open this backup: it needs about %d MB, and the drive has %d MB free", payload.UncompressedSize64>>20, free>>20)
		}
		innerPath := filepath.Join(work, "inner.zip")
		out, err := os.OpenFile(innerPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return in, err
		}
		var done int64
		openErr := openPayload(out, ctxReader(ctx, rc), man.Crypt, password, maxTotalSize, func(n int64) {
			done += n
			say("Opening with the password", done, int64(payload.UncompressedSize64))
		})
		if cerr := out.Close(); openErr == nil {
			openErr = cerr
		}
		if openErr != nil {
			return in, openErr
		}
		iz, err := zip.OpenReader(innerPath)
		if err != nil {
			return in, Refused(PayloadName, "what's inside isn't a ZIP file")
		}
		defer iz.Close()
		if err := checkEntries(iz.File); err != nil {
			return in, err
		}
		var innerMan *zip.File
		for _, e := range iz.File {
			if e.Name == ManifestName {
				innerMan = e
			}
		}
		if innerMan == nil {
			return in, Refused(PayloadName, "what's inside has no manifest")
		}
		if man, err = readManifest(innerMan, maxManifest); err != nil {
			return in, err
		}
		if man.Crypt != nil {
			return in, Refused(PayloadName, "a backup inside a backup isn't something this server makes")
		}
		files = iz.File
		in.zipPath = innerPath
		man.Protected = true
	} else if man.Protected {
		// A plain archive can say what it likes about itself. Nothing checks this, so
		// nothing is believed: it gets the same treatment as any without a password.
		man.Protected = false
	}
	in.man, in.ServerVersion, in.Created = man, cleanWords(man.ServerVersion), man.Created
	in.Protected = man.Protected

	cur := db.CurrentVersions()
	if man.Schema.Newer(cur) {
		return in, &Refusal{Reason: fmt.Sprintf("its database is from a newer version (%s). Update this server first", cleanWords(man.ServerVersion))}
	}
	in.Older = man.Schema != cur

	if err := verifyContents(ctx, in, files, say); err != nil {
		return in, err
	}
	if err := inspectDatabase(ctx, in, files, f); err != nil {
		return in, err
	}
	sort.Strings(in.HandEdited)
	return in, nil
}

// ErrPasswordNeeded: this archive is protected and no password was given.
var ErrPasswordNeeded = errors.New("this backup is protected: enter its password")

// IsWrongPassword says whether an error is a password that didn't open it.
func IsWrongPassword(err error) bool {
	return errors.Is(err, errWrongPassword) || errors.Is(err, ErrPasswordNeeded)
}

type ctxRead struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxRead) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

func ctxReader(ctx context.Context, r io.Reader) io.Reader { return ctxRead{ctx, r} }

// sourceFor finds which rules a name in an archive falls under.
func sourceFor(name string) (*fileSource, string) {
	for _, s := range sources() {
		if strings.HasPrefix(name, s.Prefix) && len(name) > len(s.Prefix) {
			src := s
			return &src, strings.TrimPrefix(name, s.Prefix)
		}
	}
	return nil, ""
}

// verifyContents reads every file, once: its hash, its real size and whether it's what
// its folder says it should be.
func verifyContents(ctx context.Context, in *Inspection, files []*zip.File, say func(string, int64, int64)) error {
	man := in.man
	known := make(map[string]FileEntry, len(man.Files))
	for _, e := range man.Files {
		if why := unsafeName(e.Path); why != "" {
			return Refused(printableName(e.Path), why)
		}
		known[e.Path] = e
	}
	var total, done int64
	for _, f := range files {
		total += int64(f.UncompressedSize64)
	}
	parts := map[string]*PartContent{}
	found := map[string]bool{}
	buf := make([]byte, 256<<10)
	for _, f := range files {
		name := f.Name
		if strings.HasSuffix(name, "/") || name == ManifestName || name == ReadmeName {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entry, listed := known[name]
		src, rel := sourceFor(name)
		hand := src != nil && src.Part == PartMetaFiles
		switch {
		case name == dbEntry:
			entry.Part = ""
		case src == nil:
			return Refused(printableName(name), "it isn't something a backup holds")
		case src.Skip != nil && src.Skip(rel):
			return Refused(printableName(name), "it isn't something a backup holds")
		case !listed && !hand:
			return Refused(printableName(name), "the manifest doesn't list it, so someone has added it")
		}
		limit := int64(f.UncompressedSize64)
		if listed && !hand && entry.Size != limit {
			return Refused(printableName(name), "it's a different size from what the manifest says")
		}
		maxHere := maxFileSize
		if name == dbEntry {
			maxHere = maxDatabase
		}
		if limit > maxHere {
			return Refused(printableName(name), "it's larger than a backup holds")
		}
		rc, err := f.Open()
		if err != nil {
			return Refused(printableName(name), "it can't be read")
		}
		h := sha256.New()
		head := make([]byte, 16)
		headN, _ := io.ReadFull(rc, head)
		head = head[:headN]
		h.Write(head)
		// The directory's size is a claim. The reader stops one byte past it, to catch
		// more coming out than was promised; the ZIP reader's own checks fail on less.
		lim := io.LimitReader(rc, limit+1-int64(headN))
		var got = int64(headN)
		for {
			n, rerr := lim.Read(buf)
			if n > 0 {
				h.Write(buf[:n])
				got += int64(n)
				done += int64(n)
			}
			if errors.Is(rerr, io.EOF) {
				break
			}
			if rerr != nil {
				rc.Close()
				return Refused(printableName(name), "it's damaged")
			}
			if err := ctx.Err(); err != nil {
				rc.Close()
				return err
			}
			if got > limit {
				break
			}
		}
		rc.Close()
		if got != limit {
			return Refused(printableName(name), "it isn't the size the archive says")
		}
		say("Checking every file", done, total)
		sum := hex.EncodeToString(h.Sum(nil))
		if listed && sum != entry.SHA256 && !hand {
			return Refused(printableName(name), "it's been changed or damaged since the backup was made")
		}
		if hand && (!listed || sum != entry.SHA256) {
			in.HandEdited = append(in.HandEdited, strings.TrimPrefix(name, src.Prefix))
		}
		found[name] = true
		if src != nil {
			if err := src.Check(rel, head); err != nil {
				return Refused(printableName(name), err.Error())
			}
			p := parts[src.Part]
			if p == nil {
				p = &PartContent{ID: src.Part}
				parts[src.Part] = p
			}
			p.Files++
			p.Bytes += got
			p.HasFiles = true
		}
		in.Needed += got
	}
	for _, e := range man.Files {
		if !found[e.Path] {
			src, _ := sourceFor(e.Path)
			if src != nil && src.Part == PartMetaFiles {
				in.HandEdited = append(in.HandEdited, strings.TrimPrefix(e.Path, src.Prefix)+" (removed)")
				continue
			}
			return Refused(printableName(e.Path), "the manifest lists it, and it isn't there")
		}
	}
	if err := checkSpecialFiles(ctx, in, files); err != nil {
		return err
	}
	for _, id := range man.Parts {
		if _, ok := partByID(id.ID); !ok {
			return Refused(ManifestName, "it names a part of a backup that doesn't exist")
		}
	}
	for _, def := range Parts() {
		listed := false
		for _, m := range man.Parts {
			if m.ID == def.ID {
				listed = true
			}
		}
		if p := parts[def.ID]; p != nil || (listed && def.HasTables) {
			pc := PartContent{ID: def.ID}
			if p != nil {
				pc = *p
			}
			pc.Label, pc.Desc, pc.Needs = def.Label, def.Desc, def.Needs
			in.Parts = append(in.Parts, pc)
		}
	}
	return nil
}

// checkSpecialFiles looks inside what can be looked inside, in a form that's safe to use:
// themes through the same check as an upload, metadata files through their parser, and
// module settings by what they say.
func checkSpecialFiles(ctx context.Context, in *Inspection, files []*zip.File) error {
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(f.Name, "themes/"):
			b, err := readEntry(f, 64<<20)
			if err != nil {
				return Refused(printableName(f.Name), err.Error())
			}
			if err := themes.CheckZip(b); err != nil {
				return Refused(printableName(f.Name), "this theme isn't valid: "+err.Error())
			}
		case strings.HasPrefix(f.Name, "metadata/") && strings.EqualFold(path.Ext(f.Name), ".json"):
			rel := strings.TrimPrefix(f.Name, "metadata/")
			if path.Base(rel) == metafolder.StateFile {
				continue
			}
			b, err := readEntry(f, metafolder.MaxJSON)
			if err != nil {
				in.Problems = append(in.Problems, rel+": "+err.Error())
				continue
			}
			if _, probs := metafolder.ParseJSON(rel, b); len(probs) > 0 {
				for _, p := range probs {
					in.Problems = append(in.Problems, p.String())
				}
			}
		case strings.HasPrefix(f.Name, "lyrics-corrections/"):
			b, err := readEntry(f, 1<<20)
			if err != nil || !json.Valid(b) {
				return Refused(printableName(f.Name), "it isn't valid JSON")
			}
		case f.Name == "modules/state.json" || f.Name == "modules/library-names.json":
			b, err := readEntry(f, 8<<20)
			if err != nil || !json.Valid(b) {
				return Refused(printableName(f.Name), "it isn't valid JSON")
			}
		}
	}
	return nil
}

func readEntry(f *zip.File, limit int64) ([]byte, error) {
	if int64(f.UncompressedSize64) > limit {
		return nil, errors.New("it's too large")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, errors.New("it can't be read")
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, errors.New("it's damaged")
	}
	if int64(len(b)) > limit {
		return nil, errors.New("it's too large")
	}
	return b, nil
}
