package uninstall

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Args is how the uninstallers are told what to do.
type Args struct {
	Options
	KeepData      bool   // the answer to the data question, given up front
	Yes           bool   // don't ask anything
	DryRun        bool   // say what would go, and stop
	JSON          bool   // the result as JSON
	ProgramFolder string // the folder a copy in the temp folder was started from
	Worker        bool   // the questions were asked: do the work
	Report        string // write the result here as well
	Unattended    bool   // nobody is there: a worker shows no dialog when it's done
}

// AskedByPerson says someone is waiting to see the result.
func (a Args) AskedByPerson() bool { return !a.Unattended }

// ParseArgs reads the flags both uninstallers share: --name for the command line, and
// /Name, as Windows installers have them, for the Windows one.
func ParseArgs(argv []string) (Args, error) {
	var a Args
	for i := 0; i < len(argv); i++ {
		name := strings.ToLower(strings.TrimLeft(argv[i], "-/"))
		next := func() (string, error) {
			if i+1 >= len(argv) {
				return "", fmt.Errorf("%s needs a value", argv[i])
			}
			i++
			return argv[i], nil
		}
		var err error
		switch name {
		case "uninstall":
		case "remove-data", "removedata":
			a.RemoveData = true
		case "keep-data", "keepdata":
			a.KeepData = true
		case "remove-recordings", "removerecordings":
			a.RemoveRecordings = true
		case "yes", "y", "s", "silent", "quiet", "q":
			a.Yes = true
		case "dry-run", "dryrun":
			a.DryRun = true
		case "json":
			a.JSON = true
		case "worker":
			a.Worker = true
		case "unattended":
			a.Unattended = true
		case "program-folder":
			a.ProgramFolder, err = next()
		case "report":
			a.Report, err = next()
		default:
			return a, fmt.Errorf("unknown option %s", argv[i])
		}
		if err != nil {
			return a, err
		}
	}
	if a.RemoveData && a.KeepData {
		return a, fmt.Errorf("--remove-data and --keep-data can't go together")
	}
	return a, nil
}

// Flags is the worker's command line for these answers.
func (a Args) Flags() []string {
	f := []string{"--worker", "--yes"}
	if a.RemoveData {
		f = append(f, "--remove-data")
	} else {
		f = append(f, "--keep-data")
	}
	if a.RemoveRecordings {
		f = append(f, "--remove-recordings")
	}
	if a.ProgramFolder != "" {
		f = append(f, "--program-folder", a.ProgramFolder)
	}
	if a.Report != "" {
		f = append(f, "--report", a.Report)
	}
	return f
}

// ReportPath is a place for the list of what was removed that outlives everything removed.
func ReportPath() string {
	return filepath.Join(os.TempDir(), "WittyMediaServer-uninstall-"+time.Now().Format("20060102-150405")+".txt")
}

const noMedia = "Your movies, shows, music, photos, books and games are never touched."

// RunCLI is `wittymediaserver --uninstall`, for people whose installer has gone missing as
// much as for scripts. It asks in plain words unless told the answers.
func RunCLI(argv []string, in io.Reader, out io.Writer) int {
	a, err := ParseArgs(argv)
	if err != nil {
		fmt.Fprintln(out, err)
		fmt.Fprintln(out, "usage: wittymediaserver --uninstall [--keep-data | --remove-data] [--remove-recordings] [--yes] [--dry-run] [--json]")
		return 2
	}
	e := Here()
	if a.ProgramFolder != "" {
		e.SelfRoot = a.ProgramFolder
	}
	return runCLI(e, a, in, out)
}

func runCLI(e *Env, a Args, in io.Reader, out io.Writer) int {
	pl := MakePlan(e)
	if len(pl.Items) == 0 {
		fmt.Fprintln(out, "Nothing of WittyMediaServer was found on this computer.")
		for _, k := range pl.Kept {
			fmt.Fprintf(out, "  %s: %s\n", k.Name, k.Reason)
		}
		return 0
	}
	r := bufio.NewReader(in)
	if !a.JSON {
		fmt.Fprintln(out, "This removes WittyMediaServer from this computer:")
		fmt.Fprint(out, pl.Describe(GroupProgram))
		fmt.Fprintln(out, noMedia)
	}
	if !a.Yes && !a.DryRun && pl.Has(GroupData) && !a.KeepData && !a.RemoveData {
		fmt.Fprintln(out, "\nYour library, accounts and settings:")
		fmt.Fprint(out, pl.Describe(GroupData))
		a.RemoveData = askYes(r, out, "Delete these too? If you keep them, they're there if you install it again.")
	}
	if !a.Yes && !a.DryRun && pl.Has(GroupRecordings) && !a.RemoveRecordings {
		fmt.Fprintln(out, "\nRecorded TV:")
		fmt.Fprint(out, pl.Describe(GroupRecordings))
		a.RemoveRecordings = askYes(r, out, "Delete your recordings too?")
	}
	if a.DryRun {
		if a.JSON {
			_ = json.NewEncoder(out).Encode(pl)
			return 0
		}
		fmt.Fprintln(out, "\nWith --remove-data it would also remove:")
		fmt.Fprint(out, pl.Describe(GroupData))
		if pl.Has(GroupRecordings) {
			fmt.Fprintln(out, "\nWith --remove-recordings it would also remove:")
			fmt.Fprint(out, pl.Describe(GroupRecordings))
		}
		if len(pl.Kept) > 0 {
			fmt.Fprintln(out, "\nIt would leave in place:")
			for _, k := range pl.Kept {
				fmt.Fprintf(out, "  %s: %s\n", k.Name, k.Reason)
			}
		}
		fmt.Fprintln(out, "\nNothing was changed.")
		return 0
	}
	if !a.Yes && !askYes(r, out, "\nGo ahead?") {
		fmt.Fprintln(out, "Nothing was changed.")
		return 1
	}
	if pl.NeedsAdmin(a.Options) && !e.IsAdmin {
		if e.GOOS == "windows" {
			fmt.Fprintln(out, "Some of this needs an administrator. Open a terminal with Run as administrator and try again, or use Uninstall WittyMediaServer.")
		} else {
			fmt.Fprintln(out, "Some of this needs root. Try again with sudo.")
		}
		return 1
	}
	if runtime.GOOS == "windows" && e.GOOS == "windows" && !a.Worker && e.SelfRoot != "" {
		// A running program can't be deleted here. A copy in the temp folder does the work.
		a.ProgramFolder = e.SelfRoot
		if a.Report == "" {
			a.Report = ReportPath()
		}
		if err := StartWorker(a.Flags(), false); err != nil {
			fmt.Fprintln(out, "Could not start:", err)
			return 1
		}
		fmt.Fprintln(out, "Removing it now, in the background. The list of what was removed will be in", a.Report)
		return 0
	}
	res := Execute(pl, a.Options)
	if a.JSON {
		_ = json.NewEncoder(out).Encode(res)
	} else {
		fmt.Fprintln(out)
		fmt.Fprint(out, res.Report())
	}
	WriteReport(a.Report, res)
	if !res.OK() {
		return 1
	}
	return 0
}

// WriteReport keeps the result where it was asked for.
func WriteReport(path string, res Result) {
	if path != "" {
		_ = os.WriteFile(path, []byte(res.Report()), 0o644)
	}
}

func askYes(r *bufio.Reader, out io.Writer, q string) bool {
	fmt.Fprint(out, q, " [y/N] ")
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
