// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package bdsession

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wittywilla/wittydisc/isofs"
)

// The server's own refusal of encrypted discs. A helper may be a fork, and its word that a disc
// is clear means nothing, so this reads the disc's streams itself before a helper hears of it.

// How AACS shows in a stream. Blu-ray streams are written in aligned units of 32 source packets
// (6144 bytes), and only the first 16 bytes of an encrypted unit are in the clear: its first
// packet header survives, with the copy-permission bits at its very start set, and every other
// packet is noise with no sync byte. Damaged rips keep their sync bytes, and clear streams have
// those bits at zero, so it takes both to call a unit encrypted.
const (
	sourcePacket = 192
	alignedUnit  = 32 * sourcePacket
)

func unitEncrypted(u []byte) bool {
	if len(u) < alignedUnit || u[0]&0xc0 == 0 {
		return false
	}
	lost := 0
	for i := sourcePacket; i+sourcePacket <= alignedUnit; i += sourcePacket {
		if u[i+4] != 0x47 {
			lost++
		}
	}
	return lost >= 16
}

// streamScrambled samples a stream at its start and in the middle, as a clear intro can front
// an encrypted film. sampled is false if nothing could be read.
func streamScrambled(r io.ReaderAt, size int64) (scrambled, sampled bool) {
	for _, at := range []int64{0, size / 2 / alignedUnit * alignedUnit} {
		buf := make([]byte, alignedUnit*8)
		n, _ := r.ReadAt(buf, at)
		if n < alignedUnit {
			continue
		}
		sampled = true
		units, tsScrambled := 0, 0
		for u := 0; u+alignedUnit <= n; u += alignedUnit {
			if unitEncrypted(buf[u : u+alignedUnit]) {
				units++
			}
		}
		for i := 0; i+sourcePacket <= n; i += sourcePacket {
			if buf[i+4] == 0x47 && buf[i+7]&0xc0 != 0 {
				tsScrambled++
			}
		}
		if units >= 2 || tsScrambled > 16 {
			return true, true
		}
	}
	return false, sampled
}

// stream is one of a disc's streams, however it's reached.
type stream struct {
	size int64
	open func() (io.ReaderAt, func(), error)
}

// maxSampled is how many of a disc's largest streams are looked at.
const maxSampled = 3

// Scrambled says whether a Blu-ray folder or image is encrypted, from its streams. A disc with
// an AACS folder and no stream that could be read counts as encrypted: nothing could be checked.
func Scrambled(path string) bool {
	streams, aacs, closer := discStreams(path)
	defer closer()
	sort.Slice(streams, func(i, j int) bool { return streams[i].size > streams[j].size })
	if len(streams) > maxSampled {
		streams = streams[:maxSampled]
	}
	sampled := false
	for _, s := range streams {
		r, done, err := s.open()
		if err != nil {
			continue
		}
		bad, read := streamScrambled(r, s.size)
		done()
		sampled = sampled || read
		if bad {
			return true
		}
	}
	return !sampled && aacs
}

// discStreams finds a disc's streams, and whether it has an AACS folder, with the case of its
// names whatever it is: a disc's own folder names aren't always in capitals.
func discStreams(path string) (streams []stream, aacs bool, closer func()) {
	closer = func() {}
	st, err := os.Stat(path)
	if err != nil {
		return nil, false, closer
	}
	if !st.IsDir() {
		d, err := isofs.Open(path)
		if err != nil {
			return nil, false, closer
		}
		closer = func() { _ = d.Close() }
		for _, f := range d.Files() {
			up := strings.ToUpper(f.Path)
			if strings.HasPrefix(up, "AACS/") || strings.HasPrefix(up, "/AACS/") {
				aacs = true
			}
			if isStreamPath(up) {
				file := f
				streams = append(streams, stream{size: file.Size, open: func() (io.ReaderAt, func(), error) { return d.Reader(file), func() {}, nil }})
			}
		}
		return streams, aacs, closer
	}
	root := path
	if b := filepath.Base(path); strings.EqualFold(b, "BDMV") || strings.EqualFold(b, "BDAV") {
		root = filepath.Dir(path)
	}
	tops, _ := os.ReadDir(root)
	for _, t := range tops {
		if !t.IsDir() {
			continue
		}
		if strings.EqualFold(t.Name(), "AACS") {
			aacs = true
		}
		if !strings.EqualFold(t.Name(), "BDMV") && !strings.EqualFold(t.Name(), "BDAV") {
			continue
		}
		subs, _ := os.ReadDir(filepath.Join(root, t.Name()))
		for _, sub := range subs {
			if !sub.IsDir() || !strings.EqualFold(sub.Name(), "STREAM") {
				continue
			}
			dir := filepath.Join(root, t.Name(), sub.Name())
			files, _ := os.ReadDir(dir)
			for _, f := range files {
				if !isStreamName(f.Name()) {
					continue
				}
				info, err := f.Info()
				if err != nil {
					continue
				}
				p := filepath.Join(dir, f.Name())
				streams = append(streams, stream{size: info.Size(), open: func() (io.ReaderAt, func(), error) {
					h, err := os.Open(p)
					if err != nil {
						return nil, nil, err
					}
					return h, func() { _ = h.Close() }, nil
				}})
			}
		}
	}
	return streams, aacs, closer
}

func isStreamPath(up string) bool {
	return isStreamName(up) && strings.Contains(up, "STREAM/")
}

func isStreamName(name string) bool {
	up := strings.ToUpper(name)
	return strings.HasSuffix(up, ".M2TS") || strings.HasSuffix(up, ".MTS")
}
