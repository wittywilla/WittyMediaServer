package wittyplay

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BIOS files are never bundled or downloaded. People put their own either in the
// server's system folder or in a folder named BIOS beside their games, and those
// are copied to the system folder the first time a game needs them. The games
// folder may be read-only, and a core only reads the system folder.

// BIOSFolderName is what the server looks for beside the games. Any capitalisation
// and any folders inside it are fine.
const BIOSFolderName = "BIOS"

// MissingBIOS says what a core needs and doesn't have, "" when it has what it needs.
func MissingBIOS(coreID, systemDir string) string {
	rule := Traits(coreID).RequiredBIOS
	if rule == nil || rule.satisfied(filepath.Join(systemDir, filepath.FromSlash(rule.Folder))) {
		return ""
	}
	return rule.Example
}

// BIOSHelp tells a person what to do about a missing BIOS.
func BIOSHelp(coreID string, consoleName string) string {
	rule := Traits(coreID).RequiredBIOS
	if rule == nil {
		return ""
	}
	return consoleName + " games need a BIOS file, such as " + rule.Example + ", from " + rule.For +
		". Put it in a folder named " + BIOSFolderName + " beside your " + consoleName + " games, and it will be found when you play. BIOS files are never downloaded for you."
}

func (r *BIOSRule) satisfied(dir string) bool {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if !e.IsDir() && r.matches(dir, e) {
			return true
		}
	}
	return false
}

func (r *BIOSRule) matches(dir string, e os.DirEntry) bool {
	if !r.Match.MatchString(strings.ToLower(e.Name())) {
		return false
	}
	st, err := e.Info()
	return err == nil && st.Size() >= r.MinSize && st.Size() <= r.MaxSize
}

// How far to look, and how much to take: this runs when a game starts, on a folder
// someone else may have filled.
const (
	biosMaxDepth = 3
	biosMaxFiles = 256
	biosMaxBytes = 32 << 20 // one file
)

// CollectBIOS copies BIOS files the core wants from BIOS folders near a game into
// the system folder, leaving the originals alone. A file already there stays.
// It looks in the game's own folder and the one above it.
func CollectBIOS(console *Console, coreID, gamePath, systemDir string) error {
	rule := Traits(coreID).RequiredBIOS
	var wanted []BIOS
	for _, b := range console.BIOS {
		if !b.Required {
			wanted = append(wanted, b)
		}
	}
	if rule == nil && len(wanted) == 0 {
		return nil
	}
	gameDir := filepath.Dir(gamePath)
	var errs []error
	for _, dir := range []string{gameDir, filepath.Dir(gameDir)} {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() && strings.EqualFold(e.Name(), BIOSFolderName) {
				count := 0
				if err := walkBIOS(filepath.Join(dir, e.Name()), 0, &count, rule, wanted, systemDir); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}

func walkBIOS(dir string, depth int, count *int, rule *BIOSRule, wanted []BIOS, systemDir string) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	// Dumps come with small companion files named after the image. The image says
	// that the whole folder is worth taking.
	imageStems := map[string]bool{}
	if rule != nil {
		for _, e := range ents {
			if !e.IsDir() && rule.matches(dir, e) {
				imageStems[strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))] = true
			}
		}
	}
	var errs []error
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() {
			if depth < biosMaxDepth && !strings.HasPrefix(name, ".") {
				if err := walkBIOS(filepath.Join(dir, name), depth+1, count, rule, wanted, systemDir); err != nil {
					errs = append(errs, err)
				}
			}
			continue
		}
		if *count >= biosMaxFiles {
			break
		}
		*count++
		dest := ""
		if rule != nil && imageStems[strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))] {
			dest = filepath.Join(systemDir, filepath.FromSlash(rule.Folder), name)
		}
		for _, b := range wanted {
			if strings.EqualFold(filepath.Base(filepath.FromSlash(b.File)), name) {
				dest = filepath.Join(systemDir, filepath.FromSlash(b.File))
			}
		}
		if dest != "" {
			if err := copyBIOS(filepath.Join(dir, name), dest); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func copyBIOS(from, to string) error {
	if _, err := os.Stat(to); err == nil {
		return nil
	}
	st, err := os.Stat(from)
	if err != nil || !st.Mode().IsRegular() || st.Size() > biosMaxBytes {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(to), ".bios-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(in, biosMaxBytes)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), to)
}
