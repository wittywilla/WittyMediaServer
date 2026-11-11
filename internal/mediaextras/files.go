package mediaextras

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/metafolder"
)

// DirName is the folder, inside the server's images folder, that uploads are kept in.
const DirName = "media-extras"

// ErrTooBig and ErrBadType are what an upload can be refused for.
var (
	ErrTooBig   = errors.New("that file is too big")
	ErrBadType  = errors.New("that isn't a kind of file this takes")
	ErrBadName  = errors.New("that name isn't one this takes")
	ErrNotPlain = errors.New("it's a link or a special file, which isn't followed")
)

// Key names an item's folder of uploads: its number, and a hash of where its file is. A
// number comes back when the newest item is deleted, so the number alone would hand one
// item's uploads to whatever got that number next.
func Key(itemID int64, mediaPath string) string {
	h := sha256.Sum256([]byte(filepath.Clean(mediaPath)))
	return strconv.FormatInt(itemID, 10) + "-" + hex.EncodeToString(h[:6])
}

// OpenPlain opens a file only if the name itself is an ordinary file, and checks that
// what was opened is what was looked at, so a link left in its place leads nowhere.
func OpenPlain(p string) (*os.File, os.FileInfo, error) {
	before, err := os.Lstat(p)
	if err != nil {
		return nil, nil, err
	}
	if !metafolder.PlainFile(before) {
		return nil, nil, ErrNotPlain
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, nil, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		f.Close()
		return nil, nil, errors.New("it changed while being opened")
	}
	return f, before, nil
}

// plainExists says whether an ordinary file (not a link) is at p.
func plainExists(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && metafolder.PlainFile(fi)
}

// relOK: a relative path with forward slashes that stays where it is put, with no empty,
// "." or ".." part and no backslash.
func relOK(rel string) bool {
	if rel == "" || strings.ContainsAny(rel, "\\\x00:") {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return path.Clean(rel) == rel
}

// SaveUpload streams r into root/rel+ext, where ext is what check says the first bytes
// are. It's written to a temporary file beside it and renamed over, so a reader never
// sees half a file, and a link planted where it goes is replaced rather than written
// through. Nothing is kept if the file is too big or of the wrong kind. rel uses forward
// slashes and must stay inside root.
func SaveUpload(root, rel string, r io.Reader, max int64, check func(head []byte) string) (string, int64, error) {
	if !relOK(rel) {
		return "", 0, ErrBadName
	}
	head := make([]byte, 64)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", 0, err
	}
	head = head[:n]
	ext := check(head)
	if ext == "" {
		return "", 0, ErrBadType
	}
	dir, err := metafolder.SafeDir(root, path.Dir(rel))
	if err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp(dir, ".wms-*.tmp")
	if err != nil {
		return "", 0, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(head); err != nil {
		tmp.Close()
		return "", 0, err
	}
	copied, err := io.Copy(tmp, io.LimitReader(r, max-int64(n)+1))
	total := int64(n) + copied
	if err != nil {
		tmp.Close()
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			return "", 0, ErrTooBig
		}
		return "", 0, err
	}
	if total > max {
		tmp.Close()
		return "", 0, ErrTooBig
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	dst := filepath.Join(dir, path.Base(rel)+ext)
	if err := os.Rename(name, dst); err != nil {
		return "", 0, fmt.Errorf("couldn't put the file in place: %w", err)
	}
	return dst, total, nil
}

// RemoveMatching deletes root/rel with any of the given extensions, and says whether
// there was one. Only ordinary files are removed.
func RemoveMatching(root, rel string, exts []string) (bool, error) {
	if !relOK(rel) {
		return false, ErrBadName
	}
	found := false
	for _, ext := range exts {
		p := filepath.Join(root, filepath.FromSlash(rel)+ext)
		if !plainExists(p) {
			continue
		}
		if err := os.Remove(p); err != nil {
			return found, err
		}
		found = true
	}
	return found, nil
}

// FirstPlain is the first of root/rel+ext that is an ordinary file, or "".
func FirstPlain(root, rel string, exts []string) string {
	if !relOK(rel) {
		return ""
	}
	for _, ext := range exts {
		if p := filepath.Join(root, filepath.FromSlash(rel)+ext); plainExists(p) {
			return p
		}
	}
	return ""
}
