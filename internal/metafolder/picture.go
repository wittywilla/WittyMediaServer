package metafolder

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// MaxPicture is the largest picture taken from the folder, the same as an upload.
const MaxPicture = 20 << 20

// PictureExt says what a picture is from its first bytes, whatever its name says:
// ".jpg", ".png", ".webp" or ".gif", or "" for anything else. SVG is text that can hold
// scripts, so it's never one.
func PictureExt(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return ".jpg"
	case len(b) >= 8 && bytes.Equal(b[:8], []byte("\x89PNG\r\n\x1a\n")):
		return ".png"
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return ".webp"
	case len(b) >= 6 && (bytes.Equal(b[:6], []byte("GIF87a")) || bytes.Equal(b[:6], []byte("GIF89a"))):
		return ".gif"
	}
	return ""
}

// pictureNames are the extensions that say a file is meant as a picture.
var pictureNames = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}

// PlainFile says whether something on disk is an ordinary file, and not a link, a
// junction or a device that could lead somewhere else.
func PlainFile(fi os.FileInfo) bool {
	return fi.Mode().IsRegular() && !isReparsePoint(fi)
}

// openPlain opens a file only if the name itself is an ordinary file, and checks that
// what was opened is what was looked at.
func openPlain(p string, max int64) (*os.File, os.FileInfo, error) {
	before, err := os.Lstat(p)
	if err != nil {
		return nil, nil, err
	}
	if !PlainFile(before) {
		return nil, nil, errors.New("it's a link or a special file, which isn't followed")
	}
	if max > 0 && before.Size() > max {
		return nil, nil, fmt.Errorf("it's too big: the limit is %s", sizeWords(max))
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, nil, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		f.Close()
		return nil, nil, errors.New("it changed while being read")
	}
	return f, before, nil
}

// ReadPlain reads an ordinary file of at most max bytes. Links aren't followed.
func ReadPlain(p string, max int64) ([]byte, error) {
	f, _, err := openPlain(p, max)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("it's too big: the limit is %s", sizeWords(max))
	}
	return b, nil
}

// ReadPicture reads a picture and proves it is one, by its bytes and not its name.
// It answers the picture and the extension it should have.
func ReadPicture(p string) ([]byte, string, error) {
	if strings.EqualFold(extOf(p), ".svg") {
		return nil, "", errors.New("SVG pictures aren't accepted, because they can hold scripts. Use a JPEG, PNG, WebP or GIF")
	}
	b, err := ReadPlain(p, MaxPicture)
	if err != nil {
		return nil, "", err
	}
	ext := PictureExt(b)
	if ext == "" {
		return nil, "", errors.New("it isn't a picture, whatever its name says. Use a JPEG, PNG, WebP or GIF")
	}
	return b, ext, nil
}

func extOf(p string) string {
	i := strings.LastIndexByte(p, '.')
	if i < 0 || strings.ContainsAny(p[i:], `/\`) {
		return ""
	}
	return strings.ToLower(p[i:])
}

func sizeWords(n int64) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d KB", n>>10)
}
