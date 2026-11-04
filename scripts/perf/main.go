// Command perf measures a server build the same way each time, so that releases can be
// compared: start-up, memory and processor use while idle, library scans, and the API's
// answers. docs/PERFORMANCE.md says how to run it and holds the numbers so far.
//
//	go build -tags wmsprofile -o .build/wms-prof.exe ./cmd/wittymediaserver
//	go run ./scripts/perf -exe .build/wms-prof.exe -ffmpeg <ffmpeg> -ffprobe <ffprobe> \
//	    -lib movies=D:\WittyDemoMedia\Movies -lib music=D:\WittyDemoMedia\Music -out <folder>
//
// Every run starts on a new, empty data folder. Settings that would reach the internet
// (update checks, online details) are turned off so that runs compare, and saveMetadata
// is off so that nothing is written into the media folders.
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type libFlag []string

func (l *libFlag) String() string     { return strings.Join(*l, ",") }
func (l *libFlag) Set(v string) error { *l = append(*l, v); return nil }

var (
	exe      = flag.String("exe", "", "server built with -tags wmsprofile")
	ffmpeg   = flag.String("ffmpeg", "", "ffmpeg for the server")
	ffprobe  = flag.String("ffprobe", "", "ffprobe for the server")
	outDir   = flag.String("out", "", "folder for the results (default: a new temporary folder)")
	port     = flag.Int("port", 19890, "port for the server; the profiling port is one above")
	starts   = flag.Int("starts", 5, "how many times to time start-up")
	settle   = flag.Duration("settle", 30*time.Second, "wait before an idle reading")
	idleFor  = flag.Duration("idle", 2*time.Minute, "length of each idle reading")
	repeat   = flag.Int("repeat", 30, "how many times each API route is asked")
	scanWait = flag.Duration("scan-timeout", 60*time.Minute, "longest wait for one library's scan")
	again    = flag.Bool("report", false, "report the run in -out again, with the query plans made again, instead of running")
	libs     libFlag
)

func main() {
	flag.Var(&libs, "lib", "type=folder of a library to add and scan (repeatable), e.g. movies=D:\\Media\\Movies")
	flag.Parse()
	if *exe == "" && !*again {
		fmt.Fprintln(os.Stderr, "-exe is needed: a server built with -tags wmsprofile")
		os.Exit(2)
	}
	if *outDir == "" {
		d, err := os.MkdirTemp("", "wms-perf-")
		check(err)
		*outDir = d
	}
	check(os.MkdirAll(*outDir, 0o755))
	if *again {
		// The report of an earlier run, with the plans made again (after adding an index, say).
		b, err := os.ReadFile(filepath.Join(*outDir, "results.json"))
		check(err)
		var res Results
		check(json.Unmarshal(b, &res))
		res.SQL = plan(res.SQL, filepath.Join(*outDir, "data", "wms.db"))
		report(&res, os.Stdout)
		return
	}
	res := Results{When: time.Now().UTC().Format(time.RFC3339), Exe: *exe}
	res.Startup = timeStarts()
	runMain(&res)
	b, _ := json.MarshalIndent(res, "", "  ")
	check(os.WriteFile(filepath.Join(*outDir, "results.json"), b, 0o644))
	report(&res, os.Stdout)
	fmt.Println("\nresults and profiles are in", *outDir)
}

// Results is everything one run measured.
type Results struct {
	When      string
	Exe       string
	Startup   Startup
	IdleFresh Idle // after first-time setup, with no libraries
	Scans     []Scan
	Rescans   []Scan // the same again, with nothing changed
	IdleAfter Idle   // after every scan has finished
	Routes    []Route
	SQL       []Statement // every statement run, the costliest first
}

type Statement struct {
	Query string
	N     int64
	Sec   float64
	Plan  []string
	Scans bool // the plan reads a whole table, or sorts without an index
}

type Startup struct {
	FirstMs []float64 // a new data folder: the database is made
	WarmMs  []float64 // the same folder again
}

type Idle struct {
	Seconds     float64
	CPUPercent  float64 // of one processor
	RSSMiB      float64
	HeapLiveMiB float64
	GoMemMiB    float64 // all the Go runtime holds from the system
	Goroutines  int
	Threads     int
	GCCycles    uint64 // during the reading
	// WakeupsPerMin is how often the process's threads were switched in, counted by
	// Linux (every thread's context switches); 0 elsewhere.
	WakeupsPerMin float64
	// FreedRSSMiB is RSS after asking Go to return everything it can at once: what's
	// left above it was only waiting to go back to the system.
	FreedRSSMiB float64
	TopStacks   []string
}

type Scan struct {
	ID         int64
	Type, Path string
	AllocMiB   float64 // allocated by the server meanwhile
	Seconds    float64
	CPUSec     float64
	PeakRSSMiB float64
	Items      int
	Error      string `json:",omitempty"`
}

type Route struct {
	Path                string
	Status              int
	Bytes               int
	P50Ms, P95Ms, MaxMs float64
	AllocKiB            float64 // per request, everything the process allocated meanwhile
	AllocObjects        float64
	Statements          float64 // database statements per request
}

// --- the server ---

type server struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	base  string
	prof  string
	token string
	data  string
}

func start(data string) (*server, time.Duration) {
	args := []string{"serve", "-listen", "127.0.0.1:" + strconv.Itoa(*port), "-data", data, "-open-browser=false", "-watch-stdin", "-log", "warn"}
	if *ffmpeg != "" {
		args = append(args, "-ffmpeg", *ffmpeg)
	}
	if *ffprobe != "" {
		args = append(args, "-ffprobe", *ffprobe)
	}
	cmd := exec.Command(*exe, args...)
	cmd.Env = append(os.Environ(), "WMS_PPROF=127.0.0.1:"+strconv.Itoa(*port+1), "WMS_NO_BROWSER=1")
	logf, err := os.OpenFile(filepath.Join(*outDir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	check(err)
	cmd.Stdout, cmd.Stderr = logf, logf
	stdin, err := cmd.StdinPipe()
	check(err)
	s := &server{cmd: cmd, stdin: stdin, base: fmt.Sprintf("http://127.0.0.1:%d", *port), prof: fmt.Sprintf("http://127.0.0.1:%d", *port+1), data: data}
	t0 := time.Now()
	check(cmd.Start())
	c := http.Client{Timeout: time.Second}
	for time.Since(t0) < 2*time.Minute {
		if r, err := c.Get(s.base + "/api/health"); err == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				return s, time.Since(t0)
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.stop()
	panic("the server didn't answer within 2 minutes; see server.log")
}

// stop asks the server to stop as the tray app does, by closing its input.
func (s *server) stop() {
	_ = s.stdin.Close()
	done := make(chan struct{})
	go func() { _ = s.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = s.cmd.Process.Kill()
		<-done
	}
	// The port is free again before the next start.
	for i := 0; i < 100; i++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
		if err == nil {
			ln.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (s *server) call(method, path string, body any) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, s.base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, []byte(err.Error())
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, b
}

type vars struct {
	Goroutines int
	RSS        float64
	CPUSec     float64
	Threads    int
	Runtime    map[string]float64
}

func (s *server) vars() vars {
	r, err := http.Get(s.prof + "/debug/wms")
	check(err)
	defer r.Body.Close()
	var v vars
	check(json.NewDecoder(r.Body).Decode(&v))
	return v
}

func (s *server) save(path, name string) []byte {
	r, err := http.Get(s.prof + path)
	if err != nil {
		return nil
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	_ = os.WriteFile(filepath.Join(*outDir, name), b, 0o644)
	return b
}

// --- the steps ---

func timeStarts() Startup {
	var st Startup
	for i := 0; i < *starts; i++ {
		data := filepath.Join(*outDir, "start-"+strconv.Itoa(i))
		_ = os.RemoveAll(data)
		s, d := start(data)
		st.FirstMs = append(st.FirstMs, ms(d))
		s.stop()
		s, d = start(data)
		st.WarmMs = append(st.WarmMs, ms(d))
		s.stop()
		_ = os.RemoveAll(data)
		fmt.Printf("start-up %d: first %.0f ms, again %.0f ms\n", i+1, st.FirstMs[i], st.WarmMs[i])
	}
	return st
}

func runMain(res *Results) {
	data := filepath.Join(*outDir, "data")
	_ = os.RemoveAll(data)
	s, _ := start(data)
	defer s.stop()
	code, b := s.call("POST", "/api/setup", map[string]any{"name": "perf", "password": "perf-only-password-" + strconv.Itoa(*port), "acknowledgedLegal": true})
	if code != 200 {
		panic(fmt.Sprintf("setup: %d %s", code, b))
	}
	var tok struct{ Token string }
	check(json.Unmarshal(b, &tok))
	s.token = tok.Token
	code, b = s.call("PUT", "/api/settings", map[string]string{
		"saveMetadata": "0", "updateCheck": "0", "moduleUpdateCheck": "0",
		"tvmaze": "0", "wikidata": "0", "musicbrainz": "0",
	})
	if code != 200 {
		panic(fmt.Sprintf("settings: %d %s", code, b))
	}
	fmt.Println("idle, no libraries...")
	res.IdleFresh = s.idle("fresh")
	for _, l := range libs {
		typ, path, ok := strings.Cut(l, "=")
		if !ok {
			panic("-lib wants type=folder: " + l)
		}
		sc := s.scan(typ, path)
		fmt.Printf("scan %s %s: %.1f s, %d items, %.1f CPU s, peak %.0f MiB %s\n", typ, path, sc.Seconds, sc.Items, sc.CPUSec, sc.PeakRSSMiB, sc.Error)
		res.Scans = append(res.Scans, sc)
	}
	for _, was := range res.Scans {
		if was.Error != "" {
			continue
		}
		sc := s.rescan(was)
		fmt.Printf("rescan %s %s: %.1f s, %.1f CPU s, %.0f MiB allocated %s\n", sc.Type, sc.Path, sc.Seconds, sc.CPUSec, sc.AllocMiB, sc.Error)
		res.Rescans = append(res.Rescans, sc)
	}
	fmt.Println("idle, after the scans...")
	res.IdleAfter = s.idle("after")
	res.Routes = s.routes()
	res.SQL = s.statements()
}

type sqlStats struct {
	Count      int64
	Statements []Statement
}

func (s *server) sql() sqlStats {
	r, err := http.Get(s.prof + "/debug/wms/sql")
	check(err)
	defer r.Body.Close()
	var v sqlStats
	check(json.NewDecoder(r.Body).Decode(&v))
	return v
}

// statements is every statement the run made, with SQLite's plan for each. A plan
// is made without the values, as for a prepared statement.
func (s *server) statements() []Statement {
	return plan(s.sql().Statements, filepath.Join(s.data, "wms.db"))
}

func plan(all []Statement, dbPath string) []Statement {
	sort.Slice(all, func(i, j int) bool { return all[i].Sec > all[j].Sec })
	d, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	check(err)
	defer d.Close()
	for i := range all {
		q := all[i].Query
		all[i].Plan, all[i].Scans = nil, false
		head := strings.ToUpper(strings.TrimSpace(q))
		if !strings.HasPrefix(head, "SELECT") && !strings.HasPrefix(head, "WITH") && !strings.HasPrefix(head, "UPDATE") && !strings.HasPrefix(head, "DELETE") && !strings.HasPrefix(head, "INSERT") {
			continue
		}
		// The driver wants a value for each placeholder; NULL plans the same as any.
		args := make([]any, strings.Count(q, "?"))
		rows, err := d.Query("EXPLAIN QUERY PLAN "+q, args...)
		if err != nil {
			all[i].Plan = []string{"(no plan: " + err.Error() + ")"}
			continue
		}
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if rows.Scan(&id, &parent, &unused, &detail) == nil {
				all[i].Plan = append(all[i].Plan, detail)
				// "SCAN t" reads all of t; "SCAN t USING INDEX" walks an index in order, and
				// a covering index is as good as a search. A temporary B-tree is a sort.
				if strings.HasPrefix(detail, "SCAN ") && !strings.Contains(detail, "COVERING INDEX") && !strings.Contains(detail, "CONSTANT ROW") || strings.Contains(detail, "TEMP B-TREE") {
					all[i].Scans = true
				}
			}
		}
		rows.Close()
	}
	return all
}

func (s *server) idle(name string) Idle {
	time.Sleep(*settle)
	a := s.vars()
	wa := switches(s.cmd.Process.Pid)
	t0 := time.Now()
	time.Sleep(*idleFor)
	wb := switches(s.cmd.Process.Pid)
	b := s.vars()
	dt := time.Since(t0).Seconds()
	gor := s.save("/debug/pprof/goroutine?debug=1", "goroutines-"+name+".txt")
	s.save("/debug/pprof/heap", "heap-"+name+".pprof")
	s.save("/debug/wms/free", "free-"+name+".txt")
	time.Sleep(time.Second)
	freed := s.vars()
	return Idle{
		WakeupsPerMin: float64(wb-wa) * 60 / dt, FreedRSSMiB: freed.RSS / (1 << 20),
		Seconds: dt, CPUPercent: (b.CPUSec - a.CPUSec) * 100 / dt, RSSMiB: b.RSS / (1 << 20),
		HeapLiveMiB: b.Runtime["/gc/heap/live:bytes"] / (1 << 20), GoMemMiB: b.Runtime["/memory/classes/total:bytes"] / (1 << 20),
		Goroutines: b.Goroutines, Threads: b.Threads, GCCycles: uint64(b.Runtime["/gc/cycles/total:gc-cycles"] - a.Runtime["/gc/cycles/total:gc-cycles"]),
		TopStacks: topStacks(gor),
	}
}

// switches is how many times the threads of a process were switched in, on Linux.
func switches(pid int) int64 {
	tasks, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	if err != nil {
		return 0
	}
	var n int64
	for _, t := range tasks {
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%s/status", pid, t.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.HasSuffix(k, "ctxt_switches") {
				x, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
				n += x
			}
		}
	}
	return n
}

// topStacks is the goroutine profile in short: how many wait in each function of ours.
func topStacks(dump []byte) []string {
	var out []string
	for _, block := range strings.Split(string(dump), "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 2 || !strings.Contains(lines[0], " @ ") {
			continue
		}
		n, _, _ := strings.Cut(lines[0], " @")
		where := ""
		for _, l := range lines[1:] {
			// #  0x7ff6...  function+0x65  file:line
			f := strings.Fields(l)
			if len(f) < 4 {
				continue
			}
			fn, _, _ := strings.Cut(f[2], "+")
			if where == "" {
				where = fn // the frame it waits in, if none of ours is found
			}
			ours := strings.HasPrefix(fn, "main.") || strings.HasPrefix(fn, "github.com/wittywilla/wittymediaserver/")
			if ours && !strings.Contains(fn, "/logging.Go") && !strings.Contains(fn, "/logging.Loop") {
				where = strings.TrimPrefix(fn, "github.com/wittywilla/wittymediaserver/") + " " + filepath.Base(f[3])
				break
			}
		}
		out = append(out, n+" "+where)
	}
	return out
}

func (s *server) scan(typ, path string) Scan {
	sc := Scan{Type: typ, Path: path}
	before := s.vars()
	t0 := time.Now()
	code, b := s.call("POST", "/api/libraries", map[string]any{"name": typ + " " + filepath.Base(path), "type": typ, "paths": []string{path}})
	if code != 201 {
		sc.Error = fmt.Sprintf("%d %s", code, bytes.TrimSpace(b))
		return sc
	}
	var lib struct{ ID int64 }
	check(json.Unmarshal(b, &lib))
	sc.ID = lib.ID
	s.waitScan(&sc, before, t0)
	return sc
}

// rescan scans a library again with nothing changed, as the six-hourly scan does.
func (s *server) rescan(was Scan) Scan {
	sc := Scan{Type: was.Type, Path: was.Path, ID: was.ID}
	before := s.vars()
	t0 := time.Now()
	if code, b := s.call("POST", fmt.Sprintf("/api/libraries/%d/scan", was.ID), nil); code != 202 {
		sc.Error = fmt.Sprintf("%d %s", code, bytes.TrimSpace(b))
		return sc
	}
	s.waitScan(&sc, before, t0)
	return sc
}

func (s *server) waitScan(sc *Scan, before vars, t0 time.Time) {
	lib := struct{ ID int64 }{sc.ID}
	peak := before.RSS
	for time.Since(t0) < *scanWait {
		time.Sleep(250 * time.Millisecond)
		v := s.vars()
		peak = max(peak, v.RSS)
		_, b := s.call("GET", fmt.Sprintf("/api/libraries/%d/scan", lib.ID), nil)
		var p struct{ Scanning bool }
		_ = json.Unmarshal(b, &p)
		if !p.Scanning {
			break
		}
	}
	sc.Seconds = time.Since(t0).Seconds()
	after := s.vars()
	sc.CPUSec = after.CPUSec - before.CPUSec
	sc.AllocMiB = (after.Runtime["/gc/heap/allocs:bytes"] - before.Runtime["/gc/heap/allocs:bytes"]) / (1 << 20)
	sc.PeakRSSMiB = peak / (1 << 20)
	_, b := s.call("GET", fmt.Sprintf("/api/items?library=%d&limit=1", lib.ID), nil)
	var items struct{ Total int }
	_ = json.Unmarshal(b, &items)
	sc.Items = items.Total
}

// routes asks each route in turn, one request at a time, so that what the process
// allocates meanwhile is that route's.
func (s *server) routes() []Route {
	paths := []string{"/api/status", "/api/me", "/api/libraries", "/api/home", "/api/settings", "/api/modules", "/api/system", "/api/media-types", "/api/views", "/api/prefs"}
	_, b := s.call("GET", "/api/libraries", nil)
	var ls []struct {
		ID   int64
		Type string
	}
	_ = json.Unmarshal(b, &ls)
	for _, l := range ls {
		q := "?library=" + strconv.FormatInt(l.ID, 10)
		paths = append(paths, "/api/items"+q+"&limit=60", "/api/items"+q+"&limit=60&sort=added&desc=1", "/api/items"+q+"&search=a&limit=60", "/api/genres"+q)
		switch l.Type {
		case "movies", "anime", "videos", "musicvideos", "commercials":
			paths = append(paths, "/api/browse"+q, "/api/browse/shelves"+q, "/api/browse/facets"+q)
		case "shows":
			paths = append(paths, "/api/shows"+q, "/api/shows/genres"+q)
		case "music":
			paths = append(paths, "/api/music/home"+q, "/api/music/albums"+q, "/api/music/artists"+q, "/api/music/songs"+q+"&limit=100")
		case "photos":
			paths = append(paths, "/api/photos/months"+q, "/api/photos/list"+q+"&limit=100", "/api/photos/folders"+q)
		case "channels", "shorts":
			paths = append(paths, "/api/creators"+q, "/api/videos/feed"+q)
		}
		_, b := s.call("GET", "/api/items"+q+"&limit=1", nil)
		var it struct{ Items []struct{ ID int64 } }
		_ = json.Unmarshal(b, &it)
		if len(it.Items) > 0 {
			id := strconv.FormatInt(it.Items[0].ID, 10)
			paths = append(paths, "/api/items/"+id, "/api/items/"+id+"/poster?w=300", "/api/items/"+id+"/image?w=400")
		}
	}
	var out []Route
	for _, p := range paths {
		out = append(out, s.route(p))
	}
	return out
}

func (s *server) route(path string) Route {
	rt := Route{Path: path}
	var times []float64
	qa := s.sql().Count
	a := s.vars()
	for i := 0; i < *repeat; i++ {
		t0 := time.Now()
		code, b := s.call("GET", path, nil)
		times = append(times, ms(time.Since(t0)))
		rt.Status, rt.Bytes = code, len(b)
	}
	z := s.vars()
	n := float64(*repeat)
	rt.Statements = float64(s.sql().Count-qa) / n
	// The two readings of /debug/wms allocate too; that's small, and the same for every route.
	rt.AllocKiB = (z.Runtime["/gc/heap/allocs:bytes"] - a.Runtime["/gc/heap/allocs:bytes"]) / n / 1024
	rt.AllocObjects = (z.Runtime["/gc/heap/allocs:objects"] - a.Runtime["/gc/heap/allocs:objects"]) / n
	sort.Float64s(times)
	rt.P50Ms, rt.P95Ms, rt.MaxMs = times[len(times)/2], times[min(len(times)-1, len(times)*95/100)], times[len(times)-1]
	return rt
}

// --- the report ---

func report(r *Results, w io.Writer) {
	fmt.Fprintf(w, "\n## %s\n\n", r.When)
	fmt.Fprintf(w, "Start-up, new data folder: %s ms. Again: %s ms.\n\n", spread(r.Startup.FirstMs), spread(r.Startup.WarmMs))
	fmt.Fprintln(w, "| Idle | CPU % | RSS MiB | RSS after returning memory | Go heap live MiB | Go total MiB | Goroutines | Threads | GCs | Wake-ups a minute |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|")
	for _, x := range []struct {
		n string
		i Idle
	}{{"no libraries", r.IdleFresh}, {"after the scans", r.IdleAfter}} {
		fmt.Fprintf(w, "| %s | %.3f | %.1f | %.1f | %.1f | %.1f | %d | %d | %d | %.0f |\n", x.n, x.i.CPUPercent, x.i.RSSMiB, x.i.FreedRSSMiB, x.i.HeapLiveMiB, x.i.GoMemMiB, x.i.Goroutines, x.i.Threads, x.i.GCCycles, x.i.WakeupsPerMin)
	}
	if len(r.Scans) > 0 {
		fmt.Fprintln(w, "\n| Scan | Folder | Items | Seconds | CPU s | MiB allocated | Peak RSS MiB | Again: seconds | CPU s | MiB allocated |")
		fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|")
		for _, s := range r.Scans {
			if s.Error != "" {
				fmt.Fprintf(w, "| %s | %s | not scanned: %s | | | | | | | |\n", s.Type, filepath.Base(s.Path), cut(s.Error, 120))
				continue
			}
			var again Scan
			for _, x := range r.Rescans {
				if x.ID == s.ID {
					again = x
				}
			}
			fmt.Fprintf(w, "| %s | %s | %d | %.1f | %.1f | %.0f | %.0f | %.1f | %.2f | %.1f |\n", s.Type, filepath.Base(s.Path), s.Items, s.Seconds, s.CPUSec, s.AllocMiB, s.PeakRSSMiB, again.Seconds, again.CPUSec, again.AllocMiB)
		}
	}
	// Ordered by what each answer allocates, which a busy machine doesn't change; times do.
	fmt.Fprintln(w, "\n| Route | Status | KiB sent | KiB allocated | Objects | Statements | p50 ms | p95 ms |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|")
	rs := slices.Clone(r.Routes)
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].AllocKiB > rs[j].AllocKiB })
	for _, x := range rs {
		fmt.Fprintf(w, "| `%s` | %d | %.1f | %.0f | %.0f | %.1f | %.2f | %.2f |\n", x.Path, x.Status, float64(x.Bytes)/1024, x.AllocKiB, x.AllocObjects, x.Statements, x.P50Ms, x.P95Ms)
	}
	var n int64
	var sec float64
	for _, q := range r.SQL {
		n, sec = n+q.N, sec+q.Sec
	}
	fmt.Fprintf(w, "\n%d statements of %d kinds took %.1f s in all. The costliest:\n\n", n, len(r.SQL), sec)
	fmt.Fprintln(w, "| Runs | Seconds | Whole table or sort | Statement |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for i, q := range r.SQL {
		if i == 30 {
			break
		}
		fmt.Fprintf(w, "| %d | %.3f | %s | `%s` |\n", q.N, q.Sec, yes(q.Scans), cut(q.Query, 160))
	}
	fmt.Fprintln(w, "\nEvery statement whose plan reads a whole table or sorts without an index, the costliest first:")
	for _, q := range r.SQL {
		if q.Scans {
			fmt.Fprintf(w, "\n- %d runs, %.3f s: `%s`\n  - %s\n", q.N, q.Sec, cut(q.Query, 300), strings.Join(q.Plan, "\n  - "))
		}
	}
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

func spread(v []float64) string {
	if len(v) == 0 {
		return "-"
	}
	s := slices.Clone(v)
	sort.Float64s(s)
	return fmt.Sprintf("median %.0f (%.0f to %.0f)", s[len(s)/2], s[0], s[len(s)-1])
}

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func check(err error) {
	if err != nil && !errors.Is(err, os.ErrExist) {
		panic(err)
	}
}
