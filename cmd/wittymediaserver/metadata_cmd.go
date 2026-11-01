package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/metafolder"
)

const metadataUsage = `usage:
  wittymediaserver metadata export [-data <dir>] [-library <id>]
                       write a plain text file for each title into <data>/metadata
  wittymediaserver metadata check  [-data <dir>]
                       say what's wrong with any file there, by file and line. Needs no database
  wittymediaserver metadata apply  [-data <dir>] [--dry-run | --apply]
                       say what the files would change, and with --apply change it
  wittymediaserver metadata undo   [-data <dir>] <batch>
                       take back everything one apply changed

See Help in the app: Fixing wrong details, and The metadata folder.`

// wittymediaserver metadata: the editable metadata folder, for when the server can't start or
// someone would rather use a terminal. It works on the same files as Settings.
func cmdMetadata(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(metadataUsage)
		return 2
	}
	what := args[0]
	apply, libraryID := false, int64(0)
	var rest, plain []string
	a := args[1:]
	for i := 0; i < len(a); i++ {
		switch strings.TrimLeft(a[i], "-") {
		case "apply":
			apply = true
		case "dry-run":
			apply = false
		case "library":
			if i+1 >= len(a) {
				fmt.Fprintln(os.Stderr, metadataUsage)
				return 2
			}
			i++
			n, err := strconv.ParseInt(a[i], 10, 64)
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "-library needs a library's number")
				return 2
			}
			libraryID = n
		case "data", "config", "listen", "log-dir", "ffmpeg", "ffprobe", "log", "public-url", "trusted-proxies", "max-transcodes":
			if i+1 < len(a) {
				rest = append(rest, a[i], a[i+1])
				i++
			}
		default:
			if strings.HasPrefix(a[i], "-") {
				rest = append(rest, a[i])
			} else {
				plain = append(plain, a[i])
			}
		}
	}
	cfg, err := config.Parse(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	dir := filepath.Join(cfg.DataDir, metafolder.DirName)
	if what == "check" {
		return metadataCheck(dir)
	}
	if what != "export" && what != "apply" && what != "undo" {
		fmt.Fprintln(os.Stderr, metadataUsage)
		return 2
	}
	if what == "undo" && len(plain) != 1 {
		fmt.Fprintln(os.Stderr, metadataUsage)
		return 2
	}
	if pid, running := logging.ServerRunning(filepath.Join(cfg.DataDir, "crash")); running && (what == "undo" || (what == "apply" && apply)) {
		fmt.Fprintf(os.Stderr, "A server is running on this data folder (process %d). Use Settings > Metadata files instead, or stop the server first.\n", pid)
		return 1
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "wms.db")); err != nil {
		fmt.Fprintf(os.Stderr, "There's no database in %s, so there's nothing to compare the files with. Is that the right folder? -data sets it.\n", cfg.DataDir)
		return 1
	}
	d, err := db.Open(cfg.DataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "The database can't be opened:", err)
		return 1
	}
	defer d.Close()
	store := &library.Store{DB: d}
	ctx := context.Background()
	switch what {
	case "export":
		return metadataExport(ctx, store, dir, libraryID)
	case "undo":
		n, err := store.Undo(ctx, plain[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("Undone: %s back as before.\n", count(n, "title"))
		return 0
	}
	return metadataApply(ctx, store, dir, cfg.ImagesDir, apply)
}

func metadataExport(ctx context.Context, store *library.Store, dir string, libraryID int64) int {
	res, err := store.ExportMetadata(ctx, dir, libraryID, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "The export stopped:", err)
		return 1
	}
	printProblems(res.Problems)
	fmt.Printf("%s: %d written, %d already up to date, %d left alone because they have edits you haven't applied.\n", count(res.Total, "title"), res.Written, res.Unchanged, res.Kept)
	fmt.Println("The files are in", dir)
	return 0
}

func metadataCheck(dir string) int {
	ps, files := metafolder.CheckDirCount(dir)
	printProblems(ps)
	if files == 0 && len(ps) == 0 {
		fmt.Printf("%s has no files in it yet. Make them with: wittymediaserver metadata export\n", dir)
		return 0
	}
	if metafolder.HasErrors(ps) {
		fmt.Printf("%s looked at: there's something to fix. The other files are fine, and apply without these.\n", count(files, "file"))
		return 1
	}
	fmt.Printf("%s looked at, and all can be used.\n", count(files, "file"))
	return 0
}

func metadataApply(ctx context.Context, store *library.Store, dir, images string, apply bool) int {
	if !apply {
		p, err := store.PreviewMetadataFolder(ctx, dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		printProblems(p.Problems)
		printChanges(p.Changes)
		if p.Total == 0 {
			fmt.Println("Nothing would change.")
		} else {
			fmt.Printf("%s would change. Nothing has yet: run this with --apply to change them.\n", count(p.Total, "title"))
		}
		if metafolder.HasErrors(p.Problems) {
			return 1
		}
		return 0
	}
	res, err := store.ApplyMetadataFolder(ctx, dir, library.FolderApply{User: "command line", ImagesDir: images})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	printProblems(res.Problems)
	printChanges(res.Changes)
	fmt.Printf("%s changed, %s left out.\n", count(res.Applied, "title"), count(res.Skipped, "file"))
	if res.Batch != "" {
		fmt.Println("To take it all back: wittymediaserver metadata undo", res.Batch)
	}
	if metafolder.HasErrors(res.Problems) {
		return 1
	}
	return 0
}

func count(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %ss", n, one)
}

func printProblems(ps []metafolder.Problem) {
	for _, p := range ps {
		word := "problem"
		if p.Severity == metafolder.SevWarning {
			word = "note"
		}
		fmt.Printf("%s: %s\n", word, p)
	}
}

func printChanges(list []library.FileChange) {
	for _, c := range list {
		fmt.Printf("%s\n", c.File)
		for _, f := range c.Fields {
			fmt.Printf("  %s: %q -> %q\n", f.Label, f.Old, f.New)
		}
	}
}
