// Command wittyplay-check plays real games headlessly and says what happened. It
// starts the real runner with its sandbox on, exactly as the server does, loads
// each game with a real core, presses buttons, and records whether the game
// loaded, stayed up, drew moving pictures, made sound, kept up with its frame
// rate, and could save and restore a state, including across a restart.
//
// The report goes in a folder of your choosing, which must be outside the
// repository: it names your games. Output on the terminal uses numbers only.
// docs/GAMING.md explains how to use it.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittyplay/cores"
)

type config struct {
	console, core, coresDir, systemDir, out, runner, sevenZip string
	jobs, memoryMB, limit, every                              int
	seconds                                                   float64
	loadTimeout, hang                                         time.Duration
	install, smoke, shots, keepGoing, trace, quick            bool
	options                                                   map[string]string
	log                                                       *slog.Logger
}

func main() {
	var c config
	var opts string
	flag.StringVar(&c.console, "console", "", "console id such as nes, or all (with -install or -smoke)")
	flag.StringVar(&c.core, "core", "all", "core id, a path to a core file, or all for every core offered for the console")
	flag.StringVar(&c.coresDir, "cores-dir", "", "folder holding the cores; installs go here")
	flag.StringVar(&c.systemDir, "system", "", "BIOS folder (default: an empty folder in the report folder)")
	flag.StringVar(&c.out, "out", "", "report folder, outside the repository")
	flag.StringVar(&c.runner, "runner", "", "runner program (default: wittyplay-host beside this program, then on the PATH)")
	flag.StringVar(&c.sevenZip, "7z", "", "7-Zip program for .7z games (default: 7z or 7zz on the PATH)")
	flag.IntVar(&c.jobs, "jobs", max(1, runtime.NumCPU()/2), "games to run at once")
	flag.IntVar(&c.memoryMB, "memory-mb", 2048, "the runner's memory limit, as on a server")
	flag.IntVar(&c.limit, "limit", 0, "stop after this many games per core")
	flag.IntVar(&c.every, "every", 1, "test only every Nth game")
	flag.Float64Var(&c.seconds, "seconds", 30, "how much of each game to play, in seconds of game time; a busy machine takes longer")
	flag.DurationVar(&c.loadTimeout, "load-timeout", 2*time.Minute, "how long a game may take to load; a server allows 30s, and a busy machine needs more")
	flag.DurationVar(&c.hang, "hang", time.Minute, "no frame for this long is a hang")
	flag.BoolVar(&c.install, "install", false, "install the console's offered cores with the app's own installer first")
	flag.BoolVar(&c.smoke, "smoke", false, "with no games: check that each core opens in the sandboxed runner and refuses junk without crashing")
	flag.BoolVar(&c.shots, "shots", true, "write each game's last picture, and contact sheets, to the report folder")
	flag.BoolVar(&c.keepGoing, "continue", false, "skip games already in the report folder's results.jsonl, except those too slow to judge")
	flag.BoolVar(&c.quick, "quick", false, "only load and play: no state, input or resume checks, for going over a very large library")
	flag.BoolVar(&c.trace, "trace", false, "write everything each core says, and what it was refused, to trace-*.log in the report folder")
	flag.StringVar(&opts, "options", "", "core options: key=value,key=value")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: wittyplay-check -console nes -cores-dir DIR -out DIR [flags] folder | file | @list ...\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	c.log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if opts != "" {
		c.options = map[string]string{}
		for _, kv := range strings.Split(opts, ",") {
			k, v, _ := strings.Cut(kv, "=")
			c.options[k] = v
		}
	}
	if err := run(&c, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "wittyplay-check:", err)
		os.Exit(1)
	}
}

func run(c *config, args []string) error {
	if c.console == "" || c.coresDir == "" || c.out == "" {
		flag.Usage()
		return errors.New("-console, -cores-dir and -out are needed")
	}
	out, err := outsideRepo(c.out)
	if err != nil {
		return err
	}
	c.out = out
	if c.coresDir, err = filepath.Abs(c.coresDir); err != nil {
		return err
	}
	if c.runner == "" {
		if c.runner, err = findRunner(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(c.out, 0o755); err != nil {
		return err
	}
	if c.systemDir == "" {
		c.systemDir = filepath.Join(c.out, "system")
	}
	if c.systemDir, err = filepath.Abs(c.systemDir); err != nil {
		return err
	}
	if err := os.MkdirAll(c.systemDir, 0o755); err != nil {
		return err
	}

	consoles := []*wittyplay.Console{}
	if c.console == "all" {
		for i := range wittyplay.Consoles {
			if !wittyplay.Consoles[i].Hidden {
				consoles = append(consoles, &wittyplay.Consoles[i])
			}
		}
	} else {
		cn := wittyplay.ByID(c.console)
		if cn == nil {
			return fmt.Errorf("no console %q", c.console)
		}
		consoles = append(consoles, cn)
	}
	mgr := &cores.Manager{Dir: c.coresDir, Extra: []string{}}
	if c.install {
		installAll(c, mgr, consoles)
	}

	var games []string
	if len(args) > 0 {
		if c.console == "all" {
			return errors.New("a game list needs one console")
		}
		if games, err = findGames(args, consoles[0]); err != nil {
			return err
		}
		if c.every > 1 {
			var some []string
			for i := 0; i < len(games); i += c.every {
				some = append(some, games[i])
			}
			games = some
		}
		if c.limit > 0 && len(games) > c.limit {
			games = games[:c.limit]
		}
	}

	report, done, err := openReport(c)
	if err != nil {
		return err
	}
	defer report.Close()
	var all []Result
	all = append(all, done...)
	seen := map[string]bool{}
	for _, r := range done {
		if r.Outcome != OutcomeTooSlow {
			seen[r.key()] = true
		}
	}

	var work []job
	for _, cn := range consoles {
		list, err := coresToTry(c, mgr, cn)
		if err != nil {
			fmt.Printf("%s: %v\n", cn.ID, err)
			continue
		}
		if len(list) == 0 && (c.smoke || c.install) {
			fmt.Printf("%s: no cores are offered for this console\n", cn.ID)
		}
		for _, core := range list {
			if c.smoke {
				work = append(work, job{console: cn, core: core, smoke: true})
				continue
			}
			for i, g := range games {
				j := job{console: cn, core: core, game: g, index: i + 1}
				if seen[Result{Core: core.id, File: g}.key()] {
					continue
				}
				work = append(work, j)
			}
		}
	}
	if len(work) == 0 && len(done) == 0 {
		if c.install {
			return nil
		}
		return errors.New("nothing to do: give a folder of games, or -smoke")
	}

	// A hard limit on what runs at once, and one writer for the report.
	jobs := make(chan job)
	results := make(chan Result)
	var wg sync.WaitGroup
	for i := 0; i < max(1, c.jobs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				results <- safely(c, j)
			}
		}()
	}
	go func() {
		for _, j := range work {
			jobs <- j
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	start := time.Now()
	n := 0
	enc := json.NewEncoder(report)
	for r := range results {
		n++
		all = append(all, r)
		_ = enc.Encode(r)
		_ = report.Sync()
		fmt.Printf("[%d/%d] %s %s #%d %s%s  %.1f/%.1f fps\n", n, len(work), r.Console, r.Core, r.Index, r.Outcome, warnText(r), r.FPS, r.TargetFPS)
	}
	fmt.Printf("%d checks in %s\n", n, time.Since(start).Round(time.Second))
	return writeSummary(c, all)
}

type job struct {
	console *wittyplay.Console
	core    coreRef
	game    string
	index   int
	smoke   bool
}

type coreRef struct{ id, path string }

// safely turns a panic in the tool itself into one bad result, so a run survives.
func safely(c *config, j job) (r Result) {
	defer func() {
		if p := recover(); p != nil {
			r = Result{Console: j.console.ID, Core: j.core.id, File: j.game, Index: j.index, Outcome: OutcomeToolError, Reason: fmt.Sprint(p)}
		}
	}()
	if j.smoke {
		return smokeCheck(c, j)
	}
	return checkGame(c, j)
}

func warnText(r Result) string {
	if len(r.Warnings) == 0 {
		return ""
	}
	return " [" + strings.Join(r.Warnings, ",") + "]"
}

// installAll installs every core offered for these consoles, with the same code
// an administrator's click runs.
func installAll(c *config, mgr *cores.Manager, consoles []*wittyplay.Console) {
	seen := map[string]bool{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	for _, cn := range consoles {
		for _, core := range cn.Cores {
			if seen[core.ID] {
				continue
			}
			seen[core.ID] = true
			if mgr.Find(core.ID) != nil {
				fmt.Printf("install %s: already here\n", core.ID)
				continue
			}
			t := time.Now()
			got, err := mgr.Install(ctx, core.ID)
			if err != nil {
				fmt.Printf("install %s: FAILED: %v\n", core.ID, err)
				continue
			}
			st, _ := os.Stat(got.Path)
			var size int64
			if st != nil {
				size = st.Size()
			}
			fmt.Printf("install %s: ok, %.1f MB in %s\n", core.ID, float64(size)/(1<<20), time.Since(t).Round(100*time.Millisecond))
		}
	}
}

func coresToTry(c *config, mgr *cores.Manager, cn *wittyplay.Console) ([]coreRef, error) {
	if c.core != "all" && c.core != "" {
		if strings.ContainsAny(c.core, `/\`) {
			abs, err := filepath.Abs(c.core)
			if err != nil {
				return nil, err
			}
			id := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs)), "_libretro")
			return []coreRef{{id, abs}}, nil
		}
		got := mgr.Find(c.core)
		if got == nil {
			return nil, fmt.Errorf("core %s is not installed; use -install", c.core)
		}
		return []coreRef{{got.ID, got.Path}}, nil
	}
	var out []coreRef
	for _, want := range cn.Cores {
		got := mgr.Find(want.ID)
		if got == nil {
			fmt.Printf("%s: core %s is not installed\n", cn.ID, want.ID)
			continue
		}
		out = append(out, coreRef{got.ID, got.Path})
	}
	return out, nil
}

// findGames expands folders and @lists into game files for this console.
func findGames(args []string, cn *wittyplay.Console) ([]string, error) {
	var out []string
	add := func(p string) error {
		abs, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return err
		}
		if !st.IsDir() {
			out = append(out, abs)
			return nil
		}
		hidden := map[string]bool{}
		var found []string
		err = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				n := strings.ToLower(d.Name())
				if n == "bios" || n == "system" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			// An installer is no use to a core, but people have them, and the report
			// should say so rather than pass over them.
			if !cn.Accepts(ext) && !isArchive(ext) && (cn.ID != "3ds" || ext != ".cia") {
				return nil
			}
			if ext == ".cue" || ext == ".m3u" {
				for _, n := range listedFiles(path) {
					hidden[strings.ToLower(filepath.Join(filepath.Dir(path), n))] = true
				}
			}
			found = append(found, path)
			return nil
		})
		for _, f := range found {
			if !hidden[strings.ToLower(f)] {
				out = append(out, f)
			}
		}
		return err
	}
	for _, a := range args {
		if list, ok := strings.CutPrefix(a, "@"); ok {
			b, err := os.ReadFile(list)
			if err != nil {
				return nil, err
			}
			for _, line := range strings.Split(string(b), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					if err := add(line); err != nil {
						return nil, err
					}
				}
			}
			continue
		}
		if err := add(a); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// outsideRepo refuses a report folder inside a git checkout, because the report
// names games and they must not be committed.
func outsideRepo(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for p := abs; ; p = filepath.Dir(p) {
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			return "", fmt.Errorf("%s is inside a git checkout (%s); the report names your games, so keep it elsewhere", abs, p)
		}
		if filepath.Dir(p) == p {
			return abs, nil
		}
	}
}

func findRunner() (string, error) {
	name := "wittyplay-host"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if self, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s was not found beside this program; build it there or use -runner", name)
}
