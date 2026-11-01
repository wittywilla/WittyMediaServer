package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/diagnose"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/web"
)

// Exit codes of wms doctor: nothing needing attention, something does, or it couldn't run.
const (
	doctorClean    = 0
	doctorProblems = 1
	doctorFailed   = 2
)

// cmdDoctor finds and fixes problems, with the server stopped or running. It doesn't start
// a server, open a port or need FFmpeg, so it works when nothing else does.
//
//	wms doctor [-data dir] [--fix] [--only id,id|area] [--safe-only] [--json] [--yes] [--try-port]
//	wms doctor remap --from X --to Y [--fix] [-data dir]
//	wms doctor reinstall <module> [-data dir]
func cmdDoctor(args []string) int {
	// Nothing the database driver says should mix with the report.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if len(args) > 0 {
		switch args[0] {
		case "remap":
			return doctorRemap(args[1:])
		case "reinstall":
			return doctorReinstall(args[1:])
		case "list":
			for _, c := range diagnose.AllChecks() {
				fmt.Printf("%-20s %-9s %s\n", c.ID, c.Area, c.Title)
			}
			return doctorClean
		}
	}
	var fix, safeOnly, asJSON, yes, tryPort bool
	var only string
	var rest []string
	for i := 0; i < len(args); i++ {
		switch strings.TrimLeft(args[i], "-") {
		case "fix":
			fix = true
		case "safe-only":
			safeOnly = true
		case "json":
			asJSON = true
		case "yes":
			yes = true
		case "try-port":
			tryPort = true
		case "only":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--only needs a list: wms doctor list shows the checks")
				return doctorFailed
			}
			i++
			only = args[i]
		case "help", "h":
			fmt.Println(doctorUsage)
			return doctorClean
		default:
			rest = append(rest, args[i])
		}
	}
	checks, err := diagnose.Selected(strings.Split(only, ","))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return doctorFailed
	}
	env, err := doctorEnv(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return doctorFailed
	}
	env.TryPort = tryPort
	ctx := context.Background()
	rep := diagnose.RunDoctor(ctx, env, checks, nil)
	if asJSON {
		b, err := rep.JSON()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return doctorFailed
		}
		fmt.Println(string(b))
		if !fix {
			return doctorExit(rep)
		}
	} else {
		fmt.Print(rep.Text())
	}
	if !fix {
		if rep.Fixable > 0 && !asJSON {
			fmt.Println("\nThis was only a look: nothing was changed. To fix what can be fixed: wms doctor --fix")
		}
		return doctorExit(rep)
	}
	in := bufio.NewReader(os.Stdin)
	carefulOK := yes
	// A fix can bring what was hidden behind it into view, such as the rows of a database
	// that couldn't be read until now, so it looks again a few times.
	var copyAt string
	for pass := 0; pass < 3; pass++ {
		var todo []*diagnose.Issue
		for i := range rep.Issues {
			iss := &rep.Issues[i]
			if !iss.CanFix() || (safeOnly && iss.Safety != diagnose.SafetySafe) {
				continue
			}
			if iss.Safety == diagnose.SafetyCareful && !carefulOK {
				fmt.Printf("\n%s\n  %s\nA safety copy is made first. Go ahead? Type yes: ", iss.Title, iss.Fix)
				line, _ := in.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(line)) != "yes" {
					fmt.Println("  Left as it is.")
					continue
				}
			}
			todo = append(todo, iss)
		}
		if len(todo) == 0 {
			break
		}
		fmt.Printf("\nFixing %d...\n", len(todo))
		failed := false
		for _, r := range diagnose.ApplyFixes(ctx, env, todo, "") {
			if r.Copy != "" {
				copyAt = r.Copy
			}
			if r.OK {
				fmt.Printf("  Fixed: %s\n    %s\n", r.Title, r.Done)
			} else {
				failed = true
				fmt.Printf("  Not fixed: %s\n    %s\n", r.Title, r.Error)
			}
		}
		rep = diagnose.RunDoctor(ctx, env, checks, nil)
		if failed {
			break // a fix that failed would only fail again
		}
	}
	if copyAt != "" {
		fmt.Printf("\nSafety copy: %s\n", copyAt)
	}
	fmt.Print("\nAfterwards:\n" + rep.Text())
	return doctorExit(rep)
}

func doctorExit(r *diagnose.DoctorReport) int {
	if r.Healthy() {
		return doctorClean
	}
	return doctorProblems
}

const doctorUsage = `wms doctor: finds and fixes problems with this server's data and install. It works with
the server stopped, when nothing else will. Without --fix it only looks, and changes nothing.

  wms doctor [-data <dir>]        look, and say what's wrong and what a fix would do
    --fix                         fix what can be fixed. A safety copy is made first
    --safe-only                   with --fix, only fixes that can't lose anything
    --yes                         with --fix, don't ask before the ones that change data
    --only <id,id or area>        only these checks. Areas: data, metadata, modules, install
    --json                        a report for a program
    --try-port                    also try the port by listening on it for a moment, on this machine only
  wms doctor list                 every check
  wms doctor remap --from <old> --to <new> [--fix]
                                  a library's drive or folder moved: rewrite the paths. Only with --fix,
                                  and only if most of its files are in the new place
  wms doctor reinstall <module>   download a module again from its publisher. Needs the internet

Exit code: 0 nothing needs attention, 1 something does, 2 it couldn't run.`

// doctorEnv works out the folders. A config file that can't be used doesn't stop the doctor:
// that's one of the things it's for. It says so, and carries on with the defaults, or -data.
func doctorEnv(args []string) (*diagnose.DoctorEnv, error) {
	cfg, err := config.Parse(args)
	if err == nil {
		return envFromConfig(cfg), nil
	}
	fs := flag.NewFlagSet("wms doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	data := fs.String("data", os.Getenv("WMS_DATA_DIR"), "")
	cfgFile := fs.String("config", os.Getenv("WMS_CONFIG"), "")
	if perr := fs.Parse(args); perr != nil {
		return nil, perr
	}
	if *data == "" {
		*data = config.DefaultDataDir()
	}
	if _, serr := os.Stat(*data); serr != nil {
		return nil, fmt.Errorf("the data folder %s can't be used: %v, and the settings couldn't be read: %v", *data, serr, err)
	}
	return &diagnose.DoctorEnv{Data: *data, Cache: filepath.Join(*data, "cache"), Images: filepath.Join(*data, "cache", "images"), Transcode: filepath.Join(*data, "transcode"),
		LogDir: filepath.Join(*data, "logs"), ConfigFile: *cfgFile, Program: config.ProgramDir(), Version: version, ConfigError: err.Error()}, nil
}

func envFromConfig(c *config.Config) *diagnose.DoctorEnv {
	return &diagnose.DoctorEnv{Data: c.DataDir, Cache: c.CacheDir, Images: c.ImagesDir, Transcode: c.TranscodeDir, LogDir: c.LogDir, ConfigFile: c.ConfigFile,
		Program: config.ProgramDir(), Portable: c.Portable, Listen: c.Listen, FFmpeg: c.FFmpegPath, FFprobe: c.FFprobePath, Version: version}
}

func doctorRemap(args []string) int {
	var from, to string
	var fix bool
	var rest []string
	for i := 0; i < len(args); i++ {
		switch strings.TrimLeft(args[i], "-") {
		case "from":
			i++
			if i < len(args) {
				from = args[i]
			}
		case "to":
			i++
			if i < len(args) {
				to = args[i]
			}
		case "fix":
			fix = true
		default:
			rest = append(rest, args[i])
		}
	}
	if from == "" || to == "" {
		fmt.Fprintln(os.Stderr, "usage: wms doctor remap --from <where it was> --to <where it is now> [--fix] [-data <dir>]")
		return doctorFailed
	}
	env, err := doctorEnv(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return doctorFailed
	}
	ctx := context.Background()
	plan, err := diagnose.PlanRemap(ctx, env, from, to)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return doctorFailed
	}
	printRemap(plan)
	if !plan.CanApply {
		return doctorProblems
	}
	if !fix {
		fmt.Println("\nThis was only a look: nothing was changed. Add --fix to do it. A safety copy is made first.")
		return doctorClean
	}
	done, err := diagnose.ApplyRemap(ctx, env, from, to)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Nothing was changed:", err)
		return doctorFailed
	}
	if !done.Applied {
		fmt.Println(done.Why)
		return doctorProblems
	}
	fmt.Printf("\nDone. Scan the library to pick up anything new.\nSafety copy: %s\n", done.Copy)
	return doctorClean
}

func printRemap(p *diagnose.RemapPlan) {
	fmt.Printf("From: %s\nTo:   %s\n", p.From, p.To)
	if p.Why != "" {
		fmt.Println(p.Why)
	}
	if p.Roots > 0 {
		fmt.Printf("%d library folder(s) would change, and about %d values in all.\n", p.Roots, p.Rows)
	}
	if p.Checked > 0 {
		fmt.Printf("Looked for %d files, and %d are in the new place.\n", p.Checked, p.Found)
	}
	for _, s := range p.Sample {
		fmt.Println("  found:    ", s)
	}
	for _, s := range p.NotFound {
		fmt.Println("  not found:", s)
	}
}

// doctorReinstall is the one thing here that goes online, and only because someone asked.
func doctorReinstall(args []string) int {
	id := ""
	if n := len(args); n > 0 {
		id, args = args[n-1], args[:n-1]
	}
	if id == "" || strings.HasPrefix(id, "-") {
		fmt.Fprintln(os.Stderr, "usage: wms doctor reinstall [-data <dir>] <module>")
		return doctorFailed
	}
	env, err := doctorEnv(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return doctorFailed
	}
	if env.ServerUp() {
		fmt.Fprintln(os.Stderr, "A server is running on this folder. Use Settings, Modules, or stop the server first.")
		return doctorFailed
	}
	m := modules.New(modules.Config{Dir: filepath.Join(env.Data, "modules"), Server: version, Builtins: append(append(modules.GoManifests(), web.ModuleManifests()...), modules.ServerManifests()...), Transport: library.SafeClient.Transport})
	defer m.Close()
	fmt.Println("Looking for the newest compatible version of", id, "...")
	if err := m.ReinstallNow(context.Background(), id); err != nil {
		fmt.Fprintln(os.Stderr, "That didn't work:", err)
		return doctorProblems
	}
	fmt.Println("Done.", id, "was installed.")
	return doctorClean
}
