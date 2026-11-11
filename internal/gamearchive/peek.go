// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gamearchive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
)

// Reading the start of one file inside an archive, to see what it really is: its
// header, or a disc's first sectors. It stops as soon as it has enough, and it happens
// in the same boxed-in worker as listing, for the same reasons.

// PeekSize is how much of a file a scan looks at. A disc's SYSTEM.CNF is within this
// nearly always, and nothing else needs more than the first few kilobytes.
const PeekSize = 2 << 20

// peekBudget is how much of a solid archive may be read through to reach a file. Past
// that, the question isn't worth what the answer costs.
const peekBudget = int64(1) << 30

func readHead(r io.Reader, n int) ([]byte, error) {
	b := make([]byte, n)
	got, err := io.ReadFull(r, b)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b[:got], nil
}

// peekFile reads the first n bytes of one file in an archive.
func peekFile(p, name string, n int) (_ []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = ErrDamaged
		}
	}()
	format, err := sniff(p)
	if err != nil {
		return nil, plainError(err)
	}
	var b []byte
	switch format {
	case format7z:
		b, err = peek7z(p, name, n)
	case formatZip:
		b, err = peekZip(p, name, n)
	case formatRar:
		b, err = peekRar(p, name, n)
	case formatTar:
		b, err = peekTar(p, "", name, n)
	case formatGz, formatBz2, formatXz, formatZst:
		b, err = peekSingle(p, format, n)
	default:
		how, isTar := strings.CutPrefix(format, tarPrefix)
		if !isTar {
			return nil, ErrDamaged
		}
		b, err = peekTar(p, how, name, n)
	}
	if err != nil {
		return nil, plainError(err)
	}
	return b, nil
}

func peek7z(p, name string, n int) ([]byte, error) {
	r, err := sevenzip.OpenReader(p)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	stream, found := -1, false
	for _, f := range r.File {
		if clean, err := cleanName(f.Name); err == nil && clean == name && regular(f.FileInfo().Mode()) {
			stream, found = f.Stream, true
			break
		}
	}
	if !found {
		return nil, ErrDamaged
	}
	// Files in one solid block can only be reached by decoding what's before them.
	spent := int64(0)
	for _, f := range r.File {
		if f.Stream != stream || !regular(f.FileInfo().Mode()) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		clean, _ := cleanName(f.Name)
		if clean == name {
			defer rc.Close()
			return readHead(rc, n)
		}
		spent += int64(f.UncompressedSize)
		if spent > peekBudget {
			rc.Close()
			return nil, ErrUnavailable
		}
		_, err = io.Copy(io.Discard, rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
	}
	return nil, ErrDamaged
}

func peekZip(p, name string, n int) ([]byte, error) {
	z, err := openZip(p)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	for _, f := range z.File {
		clean, err := cleanName(f.Name)
		if err != nil || clean != name || !regular(f.Mode()) {
			continue
		}
		if f.Flags&zipEncrypted != 0 {
			return nil, ErrEncrypted
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return readHead(rc, n)
	}
	return nil, ErrDamaged
}

func peekRar(p, name string, n int) ([]byte, error) {
	r, err := rardecode.OpenReader(p, rarOptions()...)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for {
		h, err := r.Next()
		if err != nil {
			return nil, err
		}
		if h.Encrypted || h.HeaderEncrypted {
			return nil, ErrEncrypted
		}
		if clean, cerr := cleanName(h.Name); cerr == nil && clean == name && !h.IsDir {
			return readHead(r, n)
		}
	}
}

func peekTar(p, how, name string, n int) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var src io.Reader = f
	if how != "" {
		z, done, err := decompress(how, f)
		defer done()
		if err != nil {
			return nil, err
		}
		src = z
	}
	tr := tar.NewReader(src)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if clean, cerr := cleanName(h.Name); cerr == nil && clean == name {
			return readHead(tr, n)
		}
	}
}

func peekSingle(p, how string, n int) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, done, err := decompress(how, f)
	defer done()
	if err != nil {
		return nil, err
	}
	if _, ok := src.(*gzip.Reader); ok {
		return readHead(src, n)
	}
	return readHead(src, n)
}

// Peek reads the start of one file in an archive. A worker that hangs on it is killed,
// and the file counts as not read.
func (l *Lister) Peek(path, name string) ([]byte, error) {
	if l.cmd == nil {
		if err := l.start(); err != nil {
			return nil, ErrUnavailable
		}
	}
	return l.ask(listRequest{Path: path, Peek: name, Bytes: PeekSize})
}
