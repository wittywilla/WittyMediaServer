package uninstall

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/config"
)

// DataEntries is everything the server creates at the top of its data folder, and only what's
// on this list is removed. Add a new folder here when the server starts using one: a test
// reads the source and fails otherwise.
var DataEntries = map[string]string{
	"wms.db":                "library, accounts and settings",
	"wms.db-wal":            "library, accounts and settings",
	"wms.db-shm":            "library, accounts and settings",
	"cache":                 "artwork and other caches",
	"transcode":             "temporary video",
	"logs":                  "logs",
	"wms.log":               "logs",
	"crash":                 "crash reports",
	"diagnostics":           "saved diagnostic reports",
	"certs":                 "HTTPS certificates",
	"branding":              "your server's logo",
	"channel-logos":         "TV channel logos",
	"bumpers":               "TV bumpers",
	"lyrics-corrections":    "lyrics you corrected",
	"modules":               "downloaded modules",
	"themes":                "themes",
	"gaming":                "game saves, cores and settings",
	"gamerigs":              "game PC pairings",
	"wittycouch":            "game streaming pairings",
	"metadata":              "text files for fixing titles by hand",
	"setup-code.txt":        "the code for setting up from another network",
	"settings.ini":          "the port, as Settings or you set it",
	"update-withdrawn.json": "which releases were withdrawn",
	"reset-request.json":    "a waiting reset",
	"reset-result.json":     "the last reset's result",
	"restore-staged":        "a backup on its way back",
	"restore-request.json":  "a waiting restore",
	"restore-result.json":   "the last restore's result",
	"repair-request.json":   "a waiting repair",
	"repair-result.json":    "the last repair's result",
	"instance.lock":         "what keeps a second server off this folder",
	"instance.json":         "where the running server is",
}

// These have a question each. Recorded TV is media, and backups are the copy someone keeps
// for when everything else has gone.
const (
	RecordingsEntry = "recordings"
	BackupsEntry    = "backups"
)

// resetCopy is a copy kept by an earlier reset of the server, and repairStage a repaired
// database on its way in.
var (
	resetCopy   = regexp.MustCompile(`^before-reset-`)
	repairStage = regexp.MustCompile(`^repair-stage-`)
)

func dataEntry(name string) (string, bool) {
	if what, ok := DataEntries[name]; ok {
		return what, true
	}
	if resetCopy.MatchString(name) {
		return "a copy kept by an earlier reset", true
	}
	if repairStage.MatchString(name) {
		return "a repair that didn't finish", true
	}
	return "", false
}

// planDataFolder adds what's ours in a data folder. named says the folder is called what
// the server calls it: any other folder must hold a database to be taken for one.
func planDataFolder(e *Env, pl *Plan, dir string, named, admin bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if !named && !exists(filepath.Join(dir, "wms.db")) {
		return
	}
	if !pl.first(dir) {
		return
	}
	var facts DataFacts
	if exists(filepath.Join(dir, "wms.db")) && e.Libraries != nil {
		f, err := e.Libraries(dir)
		if err != nil {
			pl.keep("Your library folders", "the database in "+dir+" couldn't be read, so check nothing of yours was kept in that folder")
		} else {
			facts = f
		}
	}
	for _, c := range facts.Chosen {
		pl.keep(c.Name, c.Reason)
	}
	holdsLibrary := func(p string) string {
		for _, l := range facts.Libraries {
			if inside(l, p) || inside(p, l) {
				return l
			}
		}
		return ""
	}
	for _, ent := range entries {
		name, p := ent.Name(), filepath.Join(dir, ent.Name())
		if l := holdsLibrary(p); l != "" {
			pl.keep(p, "your library "+l+" is in it")
			continue
		}
		if name == RecordingsEntry || name == BackupsEntry {
			switch {
			case sizeOf(p) == 0:
				pl.add(fileItem("folder", p, GroupData, admin))
			case name == RecordingsEntry:
				pl.add(fileItem("recorded TV", p, GroupRecordings, admin))
			default:
				pl.add(fileItem("backups", p, GroupBackups, admin))
			}
			continue
		}
		what, ours := dataEntry(name)
		if !ours {
			pl.keep(p, "it wasn't made by WittyMediaServer")
			continue
		}
		pl.add(fileItem(what, p, GroupData, admin))
	}
	pl.add(emptyFolderItem(dir, GroupData, admin))
}

// planConfigFile adds a config file, and follows what it says: the data folder in it, and
// any other folder it chose, which is reported and left.
func planConfigFile(e *Env, pl *Plan, file string, admin bool) {
	if !exists(file) || !pl.first(file) {
		return
	}
	pl.add(fileItem("settings file", file, GroupData, admin))
	pl.add(emptyFolderItem(filepath.Dir(file), GroupData, admin))
	f, err := config.LoadFile(file)
	if err != nil {
		return
	}
	rel := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(filepath.Dir(file), p)
	}
	if d := rel(f.DataDir); d != "" {
		planDataFolder(e, pl, d, false, admin)
	}
	for _, c := range []struct{ path, what string }{
		{f.CacheDir, "a cache folder"}, {f.MetadataDir, "an artwork folder"}, {f.TranscodeDir, "a temporary video folder"}, {f.LogDir, "a log folder"},
	} {
		if d := rel(c.path); d != "" && isDir(d) {
			pl.keep(d, "you chose it as "+c.what+": delete it yourself if you no longer want it")
		}
	}
}

var dataFlag = regexp.MustCompile(`(?:-data[ =]|WMS_DATA_DIR=)(?:"([^"]+)"|(\S+))`)

// dataDirFromCommand finds a data folder in a service's command line or unit file.
func dataDirFromCommand(s string) string {
	m := dataFlag.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return strings.Trim(m[2], `"'`)
}
