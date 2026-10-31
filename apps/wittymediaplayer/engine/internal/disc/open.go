package disc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittydisc/isofs"
	"github.com/wittywilla/wittydvd"
)

// Kinds of thing a path can be.
const (
	KindFile   = "file"
	KindDVD    = "dvd"
	KindBluray = "bluray"
	KindData   = "data" // a disc or image with no video structure
)

// imageExt names the extensions treated as disc images.
func imageExt(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".iso", ".img":
		return true
	}
	return false
}

// Classify says what a path holds without reading more than it has to.
func Classify(p string) (string, error) {
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		base := strings.ToUpper(filepath.Base(filepath.Clean(p)))
		if base == "VIDEO_TS" || exists(p, "VIDEO_TS") {
			return KindDVD, nil
		}
		if base == "BDMV" || base == "BDAV" || exists(p, "BDMV") || exists(p, "BDAV") || exists(p, "AVCHD") || exists(p, "PRIVATE/AVCHD") {
			return KindBluray, nil
		}
		return KindData, nil
	}
	if !imageExt(p) {
		return KindFile, nil
	}
	d, err := isofs.Open(p)
	if err != nil {
		return KindData, nil // not a file system this reader knows
	}
	defer d.Close()
	var dvd, bd bool
	for _, f := range d.Files() {
		up := strings.ToUpper(f.Path)
		dvd = dvd || strings.HasPrefix(up, "VIDEO_TS/")
		bd = bd || strings.HasPrefix(up, "BDMV/") || strings.HasPrefix(up, "BDAV/") || strings.Contains(up, "AVCHD/")
	}
	switch {
	case bd:
		return KindBluray, nil
	case dvd:
		return KindDVD, nil
	}
	return KindData, nil
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name)))
	return err == nil
}

// Open opens a disc of a known kind. drive says it is read straight from a disc
// drive, where a read failure means something different than in a file.
func Open(path, kind string, drive bool, prefs Prefs) (Session, error) {
	switch kind {
	case KindDVD:
		return openDVD(path, drive, prefs)
	case KindBluray:
		return openBluray(path, drive, prefs)
	}
	return nil, &RefusalError{Code: CodeNotVideo, Message: MsgNotVideo}
}

// CheckProtected refuses a disc of a known kind that carries copy protection,
// without starting a session. Imaging uses it.
func CheckProtected(path, kind string, drive bool) error {
	switch kind {
	case KindDVD:
		o, err := wittydvd.Open(path)
		if err != nil {
			if drive {
				return &RefusalError{Code: CodeUnreadable, Message: MsgDriveUnreadable}
			}
			return err
		}
		defer o.Close()
		return checkDVDProtection(o, drive)
	case KindBluray:
		s, err := openBluray(path, drive, Prefs{})
		if err != nil {
			return err
		}
		return s.Close()
	}
	return nil
}

// How much of a DVD is looked at to decide whether it is protected: a little
// from several places, because menus are often clear when the film isn't.
const (
	sampleSectors = 32
	samplePlaces  = 5
)

// checkDVDProtection samples the menus and the feature for CSS. Scrambled packs
// say so outright. On a drive, the film failing to read does too: drives refuse
// protected sectors to a player that hasn't unlocked them, and this one never
// does.
func checkDVDProtection(o *wittydvd.Opened, drive bool) error {
	scrambled := func(sp wittydvd.Space, first uint32) (bool, error) {
		rd, err := o.Reader(sp)
		if err != nil {
			return false, err
		}
		buf, err := rd.ReadSectors(first, sampleSectors)
		for s := 0; (s+1)*wittydvd.Sector <= len(buf); s++ {
			if wittydvd.PackScrambled(buf[s*wittydvd.Sector : (s+1)*wittydvd.Sector]) {
				return true, nil
			}
		}
		return false, err
	}
	if bad, _ := scrambled(wittydvd.Space{Domain: wittydvd.DomainVMGM}, 0); bad {
		return &RefusalError{Code: CodeProtected, Message: MsgDVDProtected}
	}
	main := o.Disc.MainTitle()
	if main == nil || len(main.Cells) == 0 {
		return nil
	}
	sp := wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: main.VTS}
	if rd, err := o.Reader(sp); err != nil || rd == nil {
		if drive {
			return &RefusalError{Code: CodeUnreadable, Message: MsgDriveUnreadable}
		}
		return nil
	}
	failed, tried := 0, 0
	step := len(main.Cells)/samplePlaces + 1
	for i := 0; i < len(main.Cells); i += step {
		c := main.Cells[i].Cell
		if c.LastSector < c.FirstSector {
			continue
		}
		// Past the first VOBU, which can be clear on an otherwise scrambled disc.
		at := c.FirstSector + (c.LastSector-c.FirstSector)/2
		bad, err := scrambled(sp, at)
		if bad {
			return &RefusalError{Code: CodeProtected, Message: MsgDVDProtected}
		}
		tried++
		if err != nil {
			failed++
		}
	}
	if drive && tried > 0 && failed == tried {
		return &RefusalError{Code: CodeUnreadable, Message: MsgDriveUnreadable}
	}
	return nil
}

// ErrNotDisc is returned for a path that is no disc at all.
var ErrNotDisc = errors.New("not a disc")
