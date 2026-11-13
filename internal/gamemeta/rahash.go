// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gamemeta

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"path/filepath"
	"strings"
)

// RetroAchievements knows a game by a checksum of its file, taken by rules of its own
// for each console (its rcheevos library: see
// https://docs.retroachievements.org/developer-docs/game-identification.html). Only
// cartridge consoles with simple rules are done here; a disc's checksum means reading
// its file system, and those consoles are answered as not covered.

// raConsoles maps wittyplay's console ids to RetroAchievements' own. Game Boy and Game
// Boy Color are one console here and two there, so both of theirs are looked in.
var raConsoles = map[string][]int{
	"md":     {1},
	"n64":    {2},
	"snes":   {3},
	"gb":     {4, 6},
	"gba":    {5},
	"nes":    {7}, // Famicom Disk System games are listed with the NES
	"pce":    {8}, // and SuperGrafx
	"32x":    {10},
	"sms":    {11},
	"lynx":   {13},
	"ngp":    {14},
	"gg":     {15},
	"jaguar": {17},
	"a2600":  {25},
	"arcade": {27},
	"neogeo": {27}, // a Neo Geo cartridge set is an arcade set there
	"vb":     {28},
	"a7800":  {51},
	"ws":     {53},
}

// RAConsoles is RetroAchievements' consoles for one of ours, or none if this server
// can't take checksums the way it needs them for that console.
func RAConsoles(console string) []int { return raConsoles[console] }

// RAByName: on these consoles a game is known by its file's name, not its contents.
func RAByName(console string) bool { return console == "arcade" || console == "neogeo" }

// RAMaxSize is the biggest file read to take a checksum. The largest cartridges
// (Nintendo 64) are 64 MB.
const RAMaxSize = 128 << 20

// ErrRANotCovered: this console isn't one whose checksums are taken here.
var ErrRANotCovered = errors.New("RetroAchievements checksums aren't taken for this console")

// RAHashName is the checksum of an arcade set: of its file's name, without the folder
// or the extension, exactly as written.
func RAHashName(file string) string {
	base := filepath.Base(file)
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		base = base[:i]
	}
	sum := md5.Sum([]byte(base))
	return hex.EncodeToString(sum[:])
}

// RAHash takes the checksum of a cartridge, read from r, which holds size bytes.
func RAHash(console string, r io.Reader, size int64) (string, error) {
	if _, ok := raConsoles[console]; !ok || RAByName(console) {
		return "", ErrRANotCovered
	}
	if size > RAMaxSize {
		return "", errors.New("the file is too big to be a cartridge")
	}
	br := bufio.NewReaderSize(r, 64<<10)
	head, _ := br.Peek(128)
	skip := int64(0)
	switch console {
	case "nes":
		// An iNES header, or a Famicom Disk System one: neither is part of the game.
		if bytes.HasPrefix(head, []byte("NES\x1a")) || bytes.HasPrefix(head, []byte("FDS\x1a")) {
			skip = 16
		}
	case "snes":
		// A copier's header: 512 bytes more than a whole number of 8 KB blocks.
		if size%8192 == 512 {
			skip = 512
		}
	case "pce":
		// The same, in 128 KB blocks.
		if size%(128<<10) == 512 {
			skip = 512
		}
	case "a7800":
		if bytes.HasPrefix(head, []byte("\x01ATARI7800")) {
			skip = 128
		}
	case "lynx":
		if bytes.HasPrefix(head, []byte("LYNX\x00")) {
			skip = 64
		}
	}
	if skip > 0 {
		if _, err := br.Discard(int(skip)); err != nil {
			return "", err
		}
	}
	h := md5.New()
	var err error
	if console == "n64" {
		err = n64BigEndian(h, br, head)
	} else {
		_, err = io.Copy(h, br)
	}
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// n64BigEndian copies a Nintendo 64 cartridge to w in the order its .z64 form has:
// a .v64 file has each pair of bytes swapped, and a .n64 file each group of four
// reversed. The first byte of the header says which it is.
func n64BigEndian(w io.Writer, r io.Reader, head []byte) error {
	mode := 0 // as it is
	if len(head) > 0 {
		switch head[0] {
		case 0x37:
			mode = 2
		case 0x40:
			mode = 4
		}
	}
	if mode == 0 {
		_, err := io.Copy(w, r)
		return err
	}
	buf := make([]byte, 64<<10)
	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			chunk := buf[:n]
			for i := 0; i+mode <= len(chunk); i += mode {
				if mode == 2 {
					chunk[i], chunk[i+1] = chunk[i+1], chunk[i]
				} else {
					chunk[i], chunk[i+1], chunk[i+2], chunk[i+3] = chunk[i+3], chunk[i+2], chunk[i+1], chunk[i]
				}
			}
			if _, werr := w.Write(chunk); werr != nil {
				return werr
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
