// Package gamearchive reads the archives game collections come in, and unpacks
// them to play. See docs/GAMING.md.
//
// Archives are treated as hostile. Everything that decodes one runs in a short-lived
// child process (worker.go), because the decoders can be made to panic or to ask for
// enormous amounts of memory by a crafted header, and neither of those may take the
// server down. Nothing here ever writes beside the archive.
package gamearchive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/bodgit/sevenzip"
	"github.com/klauspost/compress/zstd"
	"github.com/nwaples/rardecode/v2"
)

const (
	// MaxEntries is far above any game: a disc with every track is under a hundred files.
	MaxEntries = 10000
	// MaxUnpacked is above several dual-layer discs; the cache's own limit is usually lower.
	MaxUnpacked = int64(32) << 30
	// MaxRatio applies above ratioFloor. Blank space in a disc image compresses
	// well, but not this well. A small file may be mostly padding.
	MaxRatio   = 2000
	ratioFloor = int64(1) << 30
	maxNameLen = 1024
	// maxDictionary is the most memory a RAR file may ask for to decode.
	maxDictionary = int64(2) << 30
)

// An Error is why an archive can't be used, in words for the owner. The Code stays
// the same across the pipe from the worker, and into the scan's report.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

var (
	ErrEncrypted   = &Error{"encrypted", "This archive is protected by a password, and this server doesn't open those. Unpack it yourself and put the game in the folder."}
	ErrUnsafe      = &Error{"unsafe", "This archive tries to put files outside its own folder, so it was refused."}
	ErrTooBig      = &Error{"toobig", "This archive unpacks to far more than a game should, so it was refused."}
	ErrTooMany     = &Error{"toomany", "This archive holds too many files to be a game, so it was refused."}
	ErrDamaged     = &Error{"damaged", "This archive is damaged or isn't the kind of file its name says."}
	ErrMethod      = &Error{"method", "This archive uses a kind of compression this server can't read. Unpack it yourself and put the game in the folder."}
	ErrNoSpace     = &Error{"nospace", "There isn't enough free disk space to unpack this game."}
	ErrUnavailable = &Error{"unavailable", "This archive couldn't be read."}
)

var errorsByCode = map[string]*Error{ErrEncrypted.Code: ErrEncrypted, ErrUnsafe.Code: ErrUnsafe, ErrTooBig.Code: ErrTooBig, ErrTooMany.Code: ErrTooMany,
	ErrDamaged.Code: ErrDamaged, ErrMethod.Code: ErrMethod, ErrNoSpace.Code: ErrNoSpace, ErrUnavailable.Code: ErrUnavailable}

// CodeOf says which Error an error is, or "" for any other.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

const (
	format7z  = "7z"
	formatZip = "zip"
	formatRar = "rar"
	formatTar = "tar"
	formatTgz = "tgz"
	formatGz  = "gz"
)

// IsArchive says whether a file's name is one of the kinds of archive a game library
// can read.
func IsArchive(name string) bool { return nameFormat(name) != "" }

func nameFormat(name string) string {
	switch n := strings.ToLower(name); {
	case strings.HasSuffix(n, ".7z"):
		return format7z
	case strings.HasSuffix(n, ".zip"):
		return formatZip
	case strings.HasSuffix(n, ".rar"):
		return formatRar
	case strings.HasSuffix(n, ".tar"):
		return formatTar
	case strings.HasSuffix(n, ".tgz"), strings.HasSuffix(n, ".tar.gz"):
		return formatTgz
	case strings.HasSuffix(n, ".gz"):
		return formatGz
	}
	return ""
}

var (
	laterVolumeRe = regexp.MustCompile(`(?i)\.part0*([2-9]|[1-9][0-9]+)\.rar$`)
	firstVolumeRe = regexp.MustCompile(`(?i)\.part0*1$`)
)

// IsLaterVolume: the second and later parts of a split RAR. They are read through
// the first part, and aren't games by themselves.
func IsLaterVolume(name string) bool { return laterVolumeRe.MatchString(name) }

// BaseName is an archive's name without what makes it an archive: "Game (USA).tar.gz"
// and "Game (USA).part1.rar" are both "Game (USA)". Region and dump tags stay.
func BaseName(name string) string {
	switch nameFormat(name) {
	case formatTgz:
		if s := strings.ToLower(name); strings.HasSuffix(s, ".tar.gz") {
			return name[:len(name)-len(".tar.gz")]
		}
	case formatRar:
		return firstVolumeRe.ReplaceAllString(strings.TrimSuffix(name, filepath.Ext(name)), "")
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// sniff looks at the first bytes, since files get renamed: a .zip that is really a
// .7z should work rather than fail. The name decides anything without a signature.
func sniff(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var b [8]byte
	n, _ := io.ReadFull(f, b[:])
	head := b[:n]
	byName := nameFormat(p)
	switch {
	case bytes.HasPrefix(head, []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}):
		return format7z, nil
	case bytes.HasPrefix(head, []byte("PK\x03\x04")), bytes.HasPrefix(head, []byte("PK\x05\x06")):
		return formatZip, nil
	case bytes.HasPrefix(head, []byte("Rar!\x1a\x07")):
		return formatRar, nil
	case bytes.HasPrefix(head, []byte{0x1f, 0x8b}):
		if byName == formatTgz {
			return formatTgz, nil
		}
		return formatGz, nil
	}
	return byName, nil
}

// cleanName checks a name from inside an archive and makes it a plain relative
// path with forward slashes. Anything that climbs out, is absolute, names a drive or
// a device, or can't be a file name here, is ErrUnsafe.
func cleanName(raw string) (string, error) {
	if raw == "" || len(raw) > maxNameLen {
		return "", ErrUnsafe
	}
	n := strings.ReplaceAll(raw, `\`, "/")
	if strings.HasPrefix(n, "/") || (len(n) >= 2 && n[1] == ':') {
		return "", ErrUnsafe
	}
	var parts []string
	for _, seg := range strings.Split(n, "/") {
		switch seg {
		case "", ".":
			continue
		case "..":
			return "", ErrUnsafe
		}
		for _, r := range seg {
			if r < 0x20 || r == 0x7f {
				return "", ErrUnsafe
			}
		}
		if runtime.GOOS == "windows" && (strings.ContainsAny(seg, `<>:"|?*`) || strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ")) {
			return "", ErrUnsafe
		}
		parts = append(parts, seg)
	}
	out := strings.Join(parts, "/")
	if out == "" || !filepath.IsLocal(filepath.FromSlash(out)) {
		return "", ErrUnsafe
	}
	return out, nil
}

// clutter is what archivers and desktops leave behind: never part of a game.
func clutter(name string) bool {
	for _, seg := range strings.Split(strings.ToLower(name), "/") {
		if seg == "__macosx" || seg == "@eadir" || seg == ".ds_store" || seg == "thumbs.db" || seg == "desktop.ini" || strings.HasPrefix(seg, "._") {
			return true
		}
	}
	return false
}

// An Entry is one regular file in an archive. Size is -1 when the archive doesn't say.
type Entry struct {
	Name string `json:"n"`
	Size int64  `json:"s"`
	// CRC is the file's CRC-32 as the archive records it, or 0 if it doesn't. Game
	// databases are keyed on these, so a game can be recognised without unpacking it.
	CRC uint32 `json:"c,omitempty"`
}

// A Listing is what an archive holds, read from its headers. Partial: a .tar.gz has
// no index, so a scan looks only so far into one.
type Listing struct {
	Entries []Entry `json:"e"`
	Partial bool    `json:"p,omitempty"`
}

// Exts are the lower-case extensions in a listing, once each, in archive order.
func (l *Listing) Exts() []string {
	var out []string
	seen := map[string]bool{}
	for _, e := range l.Entries {
		x := strings.ToLower(path.Ext(e.Name))
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// lister collects entries and keeps the rules in one place.
type lister struct {
	out   Listing
	seen  map[string]bool
	total int64
}

func (l *lister) add(raw string, size int64) error { return l.addCRC(raw, size, 0) }

func (l *lister) addCRC(raw string, size int64, crc uint32) error {
	n, err := cleanName(raw)
	if err != nil {
		return err
	}
	if clutter(n) {
		return nil
	}
	key := strings.ToLower(n)
	if l.seen == nil {
		l.seen = map[string]bool{}
	}
	if l.seen[key] {
		return nil // the same name twice, or differing only in case: the first stands
	}
	l.seen[key] = true
	if len(l.out.Entries) >= MaxEntries {
		return ErrTooMany
	}
	if size > MaxUnpacked {
		return ErrTooBig
	}
	if size > 0 {
		l.total += size
	}
	if l.total > MaxUnpacked {
		return ErrTooBig
	}
	l.out.Entries = append(l.out.Entries, Entry{Name: n, Size: size, CRC: crc})
	return nil
}

func (l *lister) done(p string) (*Listing, error) {
	if st, err := os.Stat(p); err == nil && l.total > ratioFloor && l.total/max(st.Size(), 1) > MaxRatio {
		return nil, ErrTooBig
	}
	return &l.out, nil
}

// listFile reads an archive's headers. budget, above zero, is how many bytes of a
// compressed tar to read through before giving a partial answer.
func listFile(p string, budget int64) (_ *Listing, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = ErrDamaged
		}
	}()
	format, err := sniff(p)
	if err != nil {
		return nil, err
	}
	l := &lister{}
	switch format {
	case format7z:
		err = list7z(p, l)
	case formatZip:
		err = listZip(p, l)
	case formatRar:
		err = listRar(p, l)
	case formatTar:
		err = listTar(p, false, budget, l)
	case formatTgz:
		err = listTar(p, true, budget, l)
	case formatGz:
		err = listGz(p, l)
	default:
		return nil, ErrDamaged
	}
	if err != nil {
		return nil, plainError(err)
	}
	return l.done(p)
}

// plainError turns a decoder's error into one of ours. Its own words mean nothing
// to the owner, and may quote the hostile file.
func plainError(err error) error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	var re *sevenzip.ReadError
	switch {
	case errors.As(err, &re) && re.Encrypted, errors.Is(err, rardecode.ErrArchiveEncrypted), errors.Is(err, rardecode.ErrArchivedFileEncrypted),
		errors.Is(err, rardecode.ErrBadPassword):
		return ErrEncrypted
	case errors.Is(err, zip.ErrAlgorithm), errors.Is(err, rardecode.ErrUnknownDecoder), errors.Is(err, rardecode.ErrUnsupportedDecoder):
		return ErrMethod
	case errors.Is(err, rardecode.ErrDictionaryTooLarge):
		return ErrTooBig
	case isNoSpace(err):
		return ErrNoSpace
	case errors.Is(err, os.ErrPermission), errors.Is(err, os.ErrNotExist):
		return ErrUnavailable
	}
	return ErrDamaged
}

func list7z(p string, l *lister) error {
	r, err := sevenzip.OpenReader(p)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if !regular(f.FileInfo().Mode()) {
			continue
		}
		if err := l.addCRC(f.Name, int64(f.UncompressedSize), f.CRC32); err != nil {
			return err
		}
	}
	return nil
}

// regular leaves out folders, links and devices: an archive's links are never made.
func regular(m os.FileMode) bool { return m.IsRegular() }

// checkZipDirectory refuses a zip before anything allocates for it: the record at
// its end says how many entries follow, and how much room they take.
func checkZipDirectory(p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	tail := min(st.Size(), 65557)
	buf := make([]byte, tail)
	if _, err := f.ReadAt(buf, st.Size()-tail); err != nil {
		return err
	}
	i := bytes.LastIndex(buf, []byte("PK\x05\x06"))
	if i < 0 || i+22 > len(buf) {
		return ErrDamaged
	}
	if n := binary.LittleEndian.Uint16(buf[i+10:]); n != 0xffff && int(n) > MaxEntries {
		return ErrTooMany
	}
	if size := binary.LittleEndian.Uint32(buf[i+12:]); size != 0xffffffff && int64(size) > MaxEntries*(46+maxNameLen+1024) {
		return ErrTooMany
	}
	return nil
}

func openZip(p string) (*zip.ReadCloser, error) {
	if err := checkZipDirectory(p); err != nil {
		return nil, err
	}
	z, err := zip.OpenReader(p)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return nil, err
	}
	z.RegisterDecompressor(12, func(r io.Reader) io.ReadCloser { return io.NopCloser(bzip2.NewReader(r)) })
	z.RegisterDecompressor(93, func(r io.Reader) io.ReadCloser {
		d, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxWindow(uint64(maxDictionary)))
		if err != nil {
			return io.NopCloser(errReader{err})
		}
		return d.IOReadCloser()
	})
	return z, nil
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

const zipEncrypted = 0x1

func listZip(p string, l *lister) error {
	z, err := openZip(p)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if strings.HasSuffix(f.Name, "/") || !regular(f.Mode()) {
			continue
		}
		if f.Flags&zipEncrypted != 0 {
			return ErrEncrypted
		}
		if err := l.addCRC(f.Name, int64(f.UncompressedSize64), f.CRC32); err != nil {
			return err
		}
	}
	return nil
}

func rarOptions() []rardecode.Option {
	return []rardecode.Option{rardecode.MaxDictionarySize(maxDictionary)}
}

func listRar(p string, l *lister) error {
	files, err := rardecode.List(p, rarOptions()...)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.Encrypted || f.HeaderEncrypted {
			return ErrEncrypted
		}
		if f.IsDir || f.LinkType != 0 || !regular(f.Mode()) {
			continue
		}
		size := f.UnPackedSize
		if f.UnKnownSize {
			size = -1
		}
		if err := l.add(f.Name, size); err != nil {
			return err
		}
	}
	return nil
}

// countingReader tells how far a stream has been read, in decompressed bytes.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n += int64(n)
	return n, err
}

func listTar(p string, gz bool, budget int64, l *lister) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	if !gz {
		// A plain tar seeks from header to header without reading what's between.
		tr := tar.NewReader(f)
		return walkTar(tr, l, nil, 0)
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	c := &countingReader{r: zr}
	return walkTar(tar.NewReader(c), l, c, budget)
}

func walkTar(tr *tar.Reader, l *lister, c *countingReader, budget int64) error {
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeReg {
			if err := l.add(h.Name, h.Size); err != nil {
				return err
			}
		}
		// Headers sit between the files in a compressed tar, so finding the next
		// means reading through this one. A scan stops once that's more than a glance.
		if c != nil && budget > 0 && c.n+h.Size > budget {
			l.out.Partial = true
			return nil
		}
	}
}

// listGz: a gzip holds one file. Its name is this file's name without .gz, or else
// the name in its header. Its size is in the trailer, modulo 4 GB, so it's a hint.
func listGz(p string, l *lister) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	size, crc := int64(-1), uint32(0)
	var tail [8]byte
	if _, err := f.ReadAt(tail[:], st.Size()-8); err == nil {
		crc = binary.LittleEndian.Uint32(tail[:4])
		size = int64(binary.LittleEndian.Uint32(tail[4:]))
	}
	return l.addCRC(gzName(p, zr.Name), size, crc)
}

func gzName(p, inHeader string) string {
	base := filepath.Base(p)
	stem := base[:len(base)-len(filepath.Ext(base))]
	if filepath.Ext(stem) == "" && filepath.Ext(path.Base(strings.ReplaceAll(inHeader, `\`, "/"))) != "" {
		return stem + filepath.Ext(path.Base(strings.ReplaceAll(inHeader, `\`, "/")))
	}
	return stem
}

// A Want is one file to unpack: Name is its name in the archive, Out its name in the
// folder (relative, already checked by the caller).
type Want struct {
	Name string `json:"n"`
	Out  string `json:"o"`
}

// A Written is one file that arrived.
type Written struct {
	Out  string `json:"o"`
	Size int64  `json:"s"`
}

// unpacker writes wanted files into a folder it can't climb out of. os.Root refuses
// any path that leaves it, whatever the name checks missed.
type unpacker struct {
	root     *os.Root
	want     map[string]string
	limit    int64
	used     int64
	written  []Written
	progress func(int64)
	last     time.Time
	buf      []byte
}

// wanted says whether a file in the archive is one to unpack: its checked name, and
// where it goes.
func (u *unpacker) wanted(raw string) (name, out string, ok bool, err error) {
	if name, err = cleanName(raw); err != nil {
		return "", "", false, err
	}
	out, ok = u.want[name]
	return name, out, ok, nil
}

// copyEntry writes one file. size, when known, is what the headers promised: more
// than that is a bomb and less is damage. want is the checksum, if there is one.
func (u *unpacker) copyEntry(name, out string, r io.Reader, size int64, want uint32) error {
	if d := path.Dir(out); d != "." {
		if err := u.root.MkdirAll(filepath.FromSlash(d), 0o755); err != nil {
			return err
		}
	}
	f, err := u.root.OpenFile(filepath.FromSlash(out), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	var h hash.Hash32
	var w io.Writer = f
	if want != 0 {
		h = crc32.NewIEEE()
		w = io.MultiWriter(f, h)
	}
	limit := u.limit - u.used
	if size >= 0 && size < limit {
		limit = size
	}
	n, err := u.copyLimited(w, r, limit+1)
	if err != nil {
		return err
	}
	if n > limit {
		return ErrTooBig
	}
	if size >= 0 && n != size {
		return ErrDamaged
	}
	if h != nil && h.Sum32() != want {
		return ErrDamaged
	}
	// What a crash can leave behind is never promoted to the cache, but what was
	// promoted has to really be on disk.
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	u.written = append(u.written, Written{Out: out, Size: n})
	delete(u.want, name)
	return nil
}

func (u *unpacker) copyLimited(w io.Writer, r io.Reader, limit int64) (int64, error) {
	if u.buf == nil {
		u.buf = make([]byte, 1<<20)
	}
	var total int64
	for total < limit {
		b := u.buf
		if rest := limit - total; rest < int64(len(b)) {
			b = b[:rest]
		}
		n, err := r.Read(b)
		if n > 0 {
			if _, werr := w.Write(b[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
			u.used += int64(n)
			if u.progress != nil && time.Since(u.last) > 200*time.Millisecond {
				u.last = time.Now()
				u.progress(u.used)
			}
		}
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// unpackFile is the worker's job: every wanted file, into dst.
func unpackFile(p, dst string, want []Want, limit int64, progress func(int64)) (_ []Written, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = ErrDamaged
		}
	}()
	root, err := os.OpenRoot(dst)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	u := &unpacker{root: root, want: map[string]string{}, limit: min(limit, MaxUnpacked), progress: progress}
	for _, w := range want {
		u.want[w.Name] = w.Out
	}
	format, err := sniff(p)
	if err != nil {
		return nil, plainError(err)
	}
	switch format {
	case format7z:
		err = unpack7z(p, u)
	case formatZip:
		err = unpackZip(p, u)
	case formatRar:
		err = unpackRar(p, u)
	case formatTar:
		err = unpackTar(p, false, u)
	case formatTgz:
		err = unpackTar(p, true, u)
	case formatGz:
		err = unpackGz(p, u)
	default:
		err = ErrDamaged
	}
	if err != nil {
		return nil, plainError(err)
	}
	if len(u.want) > 0 {
		return nil, ErrDamaged
	}
	return u.written, nil
}

func unpack7z(p string, u *unpacker) error {
	r, err := sevenzip.OpenReader(p)
	if err != nil {
		return err
	}
	defer r.Close()
	// Files in one solid block can only be reached by decoding what's before them,
	// and the reader can only carry on from where the last file ended. So in a block
	// that still has something wanted, read everything in order.
	last := map[int]int{}
	for i, f := range r.File {
		if !regular(f.FileInfo().Mode()) {
			continue
		}
		_, _, ok, err := u.wanted(f.Name)
		if err != nil {
			return err
		}
		if ok {
			last[f.Stream] = i
		}
	}
	for i, f := range r.File {
		if len(u.want) == 0 {
			break
		}
		if !regular(f.FileInfo().Mode()) {
			continue
		}
		n, out, ok, err := u.wanted(f.Name)
		if err != nil {
			return err
		}
		if !ok {
			if at, more := last[f.Stream]; !more || i > at {
				continue
			}
		}
		if err := u.copy7z(f, n, out, ok); err != nil {
			return err
		}
	}
	return nil
}

func (u *unpacker) copy7z(f *sevenzip.File, name, out string, ok bool) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if ok {
		return u.copyEntry(name, out, rc, int64(f.UncompressedSize), f.CRC32)
	}
	if _, err := u.copyLimited(io.Discard, rc, u.limit-u.used+1); err != nil {
		return err
	}
	if u.used > u.limit {
		return ErrTooBig
	}
	return nil
}

func unpackZip(p string, u *unpacker) error {
	z, err := openZip(p)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if strings.HasSuffix(f.Name, "/") || !regular(f.Mode()) {
			continue
		}
		n, out, ok, err := u.wanted(f.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if f.Flags&zipEncrypted != 0 {
			return ErrEncrypted
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = u.copyEntry(n, out, rc, int64(f.UncompressedSize64), 0)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func unpackRar(p string, u *unpacker) error {
	r, err := rardecode.OpenReader(p, rarOptions()...)
	if err != nil {
		return err
	}
	defer r.Close()
	for len(u.want) > 0 {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Encrypted || h.HeaderEncrypted {
			return ErrEncrypted
		}
		if h.IsDir || h.LinkType != 0 || !regular(h.Mode()) {
			continue
		}
		n, out, ok, err := u.wanted(h.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		size := h.UnPackedSize
		if h.UnKnownSize {
			size = -1
		}
		if err := u.copyEntry(n, out, r, size, 0); err != nil {
			return err
		}
	}
	return nil
}

func unpackTar(p string, gz bool, u *unpacker) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	var src io.Reader = f
	if gz {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer zr.Close()
		src = zr
	}
	tr := tar.NewReader(src)
	for len(u.want) > 0 {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		n, out, ok, err := u.wanted(h.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := u.copyEntry(n, out, tr, h.Size, 0); err != nil {
			return err
		}
	}
	return nil
}

func unpackGz(p string, u *unpacker) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	n, out, ok, err := u.wanted(gzName(p, zr.Name))
	if err != nil {
		return err
	}
	if !ok {
		return ErrDamaged
	}
	return u.copyEntry(n, out, zr, -1, 0)
}
