package dvdremote

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mrand "math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"archive/tar"
	"archive/zip"
	"compress/gzip"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittydvd/wire"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// gitHub stands in for GitHub at the transport.
type gitHub struct {
	mu    sync.Mutex
	files map[string][]byte
	rels  []update.GHRelease
}

func (g *gitHub) RoundTrip(req *http.Request) (*http.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	answer := func(code int, b []byte) (*http.Response, error) {
		return &http.Response{StatusCode: code, Status: fmt.Sprint(code), Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Header: http.Header{}, Request: req}, nil
	}
	if req.URL.Host == "api.github.com" {
		b, _ := json.Marshal(g.rels)
		return answer(200, b)
	}
	if b, ok := g.files[req.URL.Path]; ok {
		return answer(200, b)
	}
	return answer(404, nil)
}

func (g *gitHub) release(tag string, assets map[string][]byte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.files == nil {
		g.files = map[string][]byte{}
	}
	r := update.GHRelease{TagName: tag}
	for name, b := range assets {
		g.files["/r/"+name] = b
		r.Assets = append(r.Assets, struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
			URL  string `json:"browser_download_url"`
		}{name, int64(len(b)), "https://github.com/r/" + name})
	}
	g.rels = append(g.rels, r)
}

func installVersion(t *testing.T, gh *gitHub, pub ed25519.PublicKey, builtin []byte, server string) *modules.Manager {
	t.Helper()
	m := modules.New(modules.Config{Dir: filepath.Join(t.TempDir(), "modules"), Server: server, Builtins: [][]byte{builtin}, Keys: []ed25519.PublicKey{pub},
		Fetch: &update.Fetcher{UserAgent: "test", Transport: gh}, Log: quiet})
	t.Cleanup(m.Close)
	m.Check(context.Background(), 0)
	// An install tries the helper once, and on a machine this busy that can fail. It
	// isn't what these tests are about, so it gets another go.
	for attempt := 1; attempt <= 3; attempt++ {
		if err := m.Install(moduleID); err != nil {
			t.Fatalf("%v: %+v", err, m.Report())
		}
		deadline := time.Now().Add(2 * time.Minute)
		s := status(t, m)
		for s.Phase != modules.PhaseIdle && s.Phase != modules.PhaseFailed && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
			s = status(t, m)
		}
		if s.Phase == modules.PhaseIdle {
			return m
		}
		t.Logf("install %d: %s %s", attempt, s.Phase, s.Error)
	}
	t.Fatal("the install never worked")
	return nil
}

func status(t *testing.T, m *modules.Manager) modules.Status {
	t.Helper()
	for _, s := range m.Report().Modules {
		if s.ID == moduleID {
			return s
		}
	}
	t.Fatal("WittyDVD is missing")
	return modules.Status{}
}

// The whole way, with nothing fake: the real helper, built, packed and signed by the
// real tool, installed, and playing a disc through a session beside the built-in copy.
func TestRealHelperEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the real helper")
	}
	root := repoRoot(t)
	out := t.TempDir()
	key := filepath.Join(t.TempDir(), "test.key")
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := os.WriteFile(key, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := func(args ...string) {
		t.Helper()
		cmd := exec.Command("go", append([]string{"run", "./modules/wittymodule/cmd/wittymodule"}, args...)...)
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("wittymodule %v: %v\n%s", args, err, b)
		}
	}
	tool("check", "modules/wittydvd")
	tool("pack", "-out", out, "-targets", runtime.GOOS+"/"+runtime.GOARCH, "modules/wittydvd")
	raw, err := os.ReadFile(filepath.Join(root, "modules", "wittydvd", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	real, err := wittymodule.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if real.Delivery != wittymodule.DeliveryHelper || real.API != 2 || real.Updates == "" || real.Stays == "" {
		t.Fatalf("%+v", real)
	}
	tool("sign", "-key", key, filepath.Join(out, wittymodule.SumsName(real.ID, real.Version)))

	assets := map[string][]byte{}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if assets[e.Name()], err = os.ReadFile(filepath.Join(out, e.Name())); err != nil {
			t.Fatal(err)
		}
	}
	gh := &gitHub{}
	gh.release("v"+real.Version, assets)
	old := bytes.Replace(raw, []byte(`"version": "`+real.Version+`"`), []byte(`"version": "0.0.1"`), 1)
	m := installVersion(t, gh, pub, old, real.MinServer)
	if s := status(t, m); s.Version != real.Version {
		t.Fatalf("%+v", s)
	}

	opener := Opener{M: m, Log: quiet}
	path := makeDisc(t)
	began := time.Now()
	e, err := opener.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("opening a disc with a helper for the first time took %v", time.Since(began))
	if !isRemote(e) {
		t.Fatal("not the helper")
	}
	began = time.Now()
	again, err := opener.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("and the next: %v", time.Since(began))
	again.NewNav(wittydvd.NavOptions{})
	t.Logf("and started: %v", time.Since(began))
	_ = again.Close()
	if len(e.Disc.VTS) != 1 || e.Disc.VTS[1].PGCs[0].PGC.SubpControl[0] == nil || e.Disc.VTS[1].PGCs[0].PGC.SubpControl[1] != nil {
		t.Fatalf("%+v", e.Disc)
	}
	n := e.NewNav(wittydvd.NavOptions{})
	st := n.Start()
	if st == nil || st.CellN != 1 {
		t.Fatalf("%+v", st)
	}
	pic, err := e.Menu(titleSpace, st.FirstSector, st.LastSector, wittydvd.MenuScanOptions{})
	if err != nil || len(pic.Buttons) != 1 {
		t.Fatalf("%+v %v", pic, err)
	}
	if ext, err := e.RangeExtents(titleSpace, st.FirstSector, st.LastSector); err != nil || len(ext) != 1 {
		t.Fatalf("%+v %v", ext, err)
	}
	c := pic.Buttons[0].Cmd
	if st = n.PressButton(1, &c); st == nil || st.CellN != 2 {
		t.Fatalf("%+v", st)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}

	// And through a session, the server's way.
	dvdsession.SetEngine(opener.Open)
	defer dvdsession.SetEngine(dvdsession.OpenBuiltin)
	sm := sessions(t)
	s, err := sm.Open(1, path, dvdsession.Prefs{})
	if err != nil {
		t.Fatal(err)
	}
	defer sm.CloseSession(s.ID)
	if p := s.Start(context.Background()); p.Kind != "menu" || len(p.Menu.Buttons) != 1 {
		t.Fatalf("%+v", p)
	}
	if p := s.Do(context.Background(), dvdsession.Action{Type: "press", Button: 1}); p.Kind != "title" || p.Title == nil {
		t.Fatalf("%+v", p)
	}
}

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func packFiles(top string, files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		for _, n := range names {
			f, _ := zw.Create(top + "/" + n)
			_, _ = f.Write(files[n])
		}
		_ = zw.Close()
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, n := range names {
		_ = tw.WriteHeader(&tar.Header{Name: top + "/" + n, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(files[n]))})
		_, _ = tw.Write(files[n])
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func manifestFor(t *testing.T, ver string) []byte {
	t.Helper()
	b, err := json.Marshal(wittymodule.Manifest{ID: moduleID, Name: "WittyDVD", Version: ver, Repo: "wittywilla/WittyDVD", Delivery: wittymodule.DeliveryHelper,
		Entry: "wittydvd-module", API: 2, MinServer: "0.4.0", MaxServer: "0.5.x"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A helper that really dies with a session running: it costs the viewer nothing but
// the place on the disc, each death counts against the version, and after three the
// version is out of use and discs open with the built-in copy.
func TestCrashingHelperIsCountedAndRolledBack(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a helper")
	}
	exeDir := t.TempDir()
	exe := filepath.Join(exeDir, "fake")
	if b, err := exec.Command("go", "build", "-o", exe, "./testdata/fakedvd").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	if runtime.GOOS == "windows" {
		exe += ".exe"
		if _, err := os.Stat(exe); err != nil {
			exe = strings.TrimSuffix(exe, ".exe")
		}
	}
	bin, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	const ver = "0.2.0"
	man := manifestFor(t, ver)
	top := wittymodule.TopFolder(moduleID, ver, runtime.GOOS, runtime.GOARCH)
	entry := (&wittymodule.Manifest{Entry: "wittydvd-module"}).EntryFile(runtime.GOOS)
	files := map[string][]byte{wittymodule.ManifestFile: man, entry: bin, "LICENSE": []byte("license")}
	archive := packFiles(top, files)
	archiveName := wittymodule.ArchiveName(moduleID, ver, runtime.GOOS, runtime.GOARCH)
	manName := wittymodule.ManifestName(moduleID, ver)
	var sums strings.Builder
	fmt.Fprintf(&sums, "%s  %s\n%s  %s\n", sha(archive), archiveName, sha(man), manName)
	for n, b := range files {
		fmt.Fprintf(&sums, "%s  %s/%s\n", sha(b), top, n)
	}
	list := []byte(sums.String())
	sumsName := wittymodule.SumsName(moduleID, ver)
	gh := &gitHub{}
	gh.release("v"+ver, map[string][]byte{archiveName: archive, manName: man, sumsName: list, sumsName + ".sig": update.Sign(key, list)})
	m := installVersion(t, gh, pub, manifestFor(t, "0.1.0"), "0.4.0")

	opener := Opener{M: m, Log: quiet}
	path := makeDisc(t)
	for i := range 3 {
		if s := status(t, m); s.Version != ver {
			t.Fatalf("crash %d: %+v", i, s)
		}
		e, err := opener.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if !isRemote(e) {
			// On a machine this busy the helper can miss its time to say hello. That
			// counts against it too, and the viewer has the built-in copy.
			_ = e.Close()
			continue
		}
		n := e.NewNav(wittydvd.NavOptions{})
		if st := n.Start(); st == nil || st.CellN != 1 {
			t.Fatalf("%+v", st)
		}
		began := time.Now()
		c := wittydvd.Command{}
		// The helper dies here: the viewer gets the disc's start, promptly.
		if st := n.PressButton(1, &c); st == nil || st.CellN != 1 {
			t.Fatalf("%+v", st)
		}
		if d := time.Since(began); d > 15*time.Second {
			t.Fatalf("the fall back took %v", d)
		}
		if st := n.CellEnded(); st == nil || st.CellN != 2 {
			t.Fatalf("the built-in copy should carry on: %+v", st)
		}
		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
	}
	s := status(t, m)
	if s.Version != "0.1.0" || !strings.Contains(s.Notice, "kept failing") {
		t.Fatalf("%+v", s)
	}
	if a := m.Attach(moduleID, time.Minute); a != nil {
		t.Fatal("a version out of use still has a helper")
	}
	e, err := opener.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if isRemote(e) {
		t.Fatal("a version out of use is still opening discs")
	}
}

// process runs a built helper with no manager around it, to compare it fairly.
type process struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	conn  *wittymodule.Conn
	once  sync.Once
}

const testSeed = 12345

func (p *process) Call(method string, params, out any) ([]byte, error) {
	if o, ok := params.(wire.NavOptions); ok {
		o.Seed = testSeed
		params = o
	}
	return p.conn.Call(method, params, out)
}

func (p *process) Close() {
	p.once.Do(func() {
		_ = p.stdin.Close()
		_ = p.cmd.Wait()
	})
}

func startHelper(t *testing.T, exe string) *process {
	t.Helper()
	cmd := exec.Command(exe, wittymodule.HelperFlag)
	cmd.Env = []string{"GOMEMLIMIT=256MiB", "GOMAXPROCS=2", "SystemRoot=" + os.Getenv("SystemRoot")}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	conn := wittymodule.NewConn(stdout, stdin)
	if _, err := conn.ReadHello(); err != nil {
		t.Fatal(err)
	}
	return &process{cmd: cmd, stdin: stdin, conn: conn}
}

func gobOf(t *testing.T, v any) string {
	t.Helper()
	b, err := wire.Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func discParts(t *testing.T, d *wittydvd.Disc) map[string]string {
	t.Helper()
	out := map[string]string{}
	b, err := wire.EncodeDisc(&wittydvd.Disc{VMG: d.VMG})
	if err != nil {
		t.Fatal(err)
	}
	out["vmg"] = string(b)
	for n, v := range d.VTS {
		// One title set at a time: a map's order isn't the same twice.
		b, err := wire.EncodeDisc(&wittydvd.Disc{VMG: &wittydvd.VMG{}, VTS: map[int]*wittydvd.VTS{n: v}})
		if err != nil {
			t.Fatal(err)
		}
		out[fmt.Sprint("vts", n)] = string(b)
	}
	return out
}

type timing struct {
	builtin, helper []time.Duration
}

func (x *timing) median(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	slices.Sort(d)
	return d[len(d)/2]
}

func (x *timing) String() string {
	return fmt.Sprintf("built-in %v, helper %v (%d times)", x.median(x.builtin), x.median(x.helper), len(x.helper))
}

// Steps a real disc through the built-in copy and the real helper, side by side, doing
// the same to both and comparing all they say: set WMS_TEST_ISO to a clear DVD image.
func TestRealDiscStepsTheSame(t *testing.T) {
	path := os.Getenv("WMS_TEST_ISO")
	if path == "" {
		t.Skip("set WMS_TEST_ISO to a clear DVD image")
	}
	exe := filepath.Join(t.TempDir(), (&wittymodule.Manifest{Entry: "wittydvd-module"}).EntryFile(runtime.GOOS))
	build := exec.Command("go", "build", "-o", exe, "./modules/wittydvd/cmd/wittydvd-module")
	build.Dir = repoRoot(t)
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	var opening timing
	began := time.Now()
	be, err := dvdsession.OpenBuiltin(path)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()
	opening.builtin = append(opening.builtin, time.Since(began))
	began = time.Now()
	p := startHelper(t, exe)
	he, err := open(p, func(reason string) { t.Errorf("the helper was faulted: %s", reason) }, quiet, path)
	if err != nil {
		t.Fatal(err)
	}
	defer he.Close()
	opening.helper = append(opening.helper, time.Since(began))
	r, ok := he.DiscEngine.(*remote)
	if !ok {
		t.Fatal("not the helper: is the disc clear?")
	}
	want, got := discParts(t, be.Disc), discParts(t, he.Disc)
	if len(want) != len(got) {
		t.Fatalf("%d parts, %d parts", len(want), len(got))
	}
	size := 0
	for k, v := range want {
		size += len(v)
		if got[k] != v {
			t.Errorf("the disc's structure differs in %s", k)
		}
	}
	t.Logf("%s: %d titles, structure about %d KB in the wire form", filepath.Base(path), len(be.Disc.VMG.Titles), size/1024)

	// The same random draws for both, or the two part at the first thing a disc picks at random.
	opts := wittydvd.NavOptions{RegisterOptions: wittydvd.RegisterOptions{MenuLanguage: "en", AudioLanguage: "en", Rand: mrand.New(mrand.NewSource(testSeed)).Float64}}
	bn, hn := be.NewNav(opts), he.NewNav(opts)
	var steps, scans, rescans, extents, pgcs timing
	timed := func(x *timing, f func(n dvdsession.Navigator) *wittydvd.Step) (*wittydvd.Step, *wittydvd.Step) {
		a := time.Now()
		s1 := f(bn)
		x.builtin = append(x.builtin, time.Since(a))
		a = time.Now()
		s2 := f(hn)
		x.helper = append(x.helper, time.Since(a))
		return s1, s2
	}
	compareStep := func(i int, a, b *wittydvd.Step) {
		t.Helper()
		if gobOf(t, &wire.NavAnswer{Step: a}) != gobOf(t, &wire.NavAnswer{Step: b}) {
			t.Fatalf("step %d differs:\n built-in %+v\n helper   %+v", i, a, b)
		}
		ra, rb := bn.Registers(), hn.Registers()
		if ra.SPRM != rb.SPRM || ra.GPRM != rb.GPRM || ra.Counter != rb.Counter {
			t.Fatalf("step %d: registers differ:\n built-in %+v\n helper   %+v", i, ra, rb)
		}
	}
	tally := map[string]int{}
	visited := map[string]bool{}
	sa, sb := timed(&steps, dvdsession.Navigator.Start)
	const limit = 400
	for i := 0; i < limit && sa != nil; i++ {
		compareStep(i, sa, sb)
		tally[string(sa.Kind)]++
		if sa.Kind == wittydvd.StepStop {
			// The end of one way through: start again in another title.
			sa, sb = timed(&steps, func(n dvdsession.Navigator) *wittydvd.Step { return n.PlayTitle(i%len(be.Disc.VMG.Titles)+1, 1) })
			continue
		}
		a := time.Now()
		pa := bn.StepPGC(sa)
		pgcs.builtin = append(pgcs.builtin, time.Since(a))
		a = time.Now()
		pb := hn.StepPGC(sb)
		pgcs.helper = append(pgcs.helper, time.Since(a))
		ea, _ := wire.EncodePGC(pa)
		eb, _ := wire.EncodePGC(pb)
		if !bytes.Equal(ea, eb) {
			t.Fatalf("step %d: program chains differ", i)
		}
		sp := wittydvd.SpaceFor(sa.Domain, sa.VTS)
		opt := wittydvd.MenuScanOptions{Palette: sa.Palette, NavButton: sa.DefaultButton, MaxHops: 48}
		if sa.Domain == wittydvd.DomainVTS {
			opt.Video = be.Disc.VTS[sa.VTS].Video
		}
		a = time.Now()
		ma, errA := be.Menu(sp, sa.FirstSector, sa.LastSector, opt)
		first := time.Since(a)
		a = time.Now()
		mb, errB := he.Menu(sp, sa.FirstSector, sa.LastSector, opt)
		second := time.Since(a)
		key := fmt.Sprint(sp, sa.FirstSector, sa.LastSector)
		if visited[key] {
			rescans.builtin, rescans.helper = append(rescans.builtin, first), append(rescans.helper, second)
		} else {
			scans.builtin, scans.helper = append(scans.builtin, first), append(scans.helper, second)
			visited[key] = true
		}
		if (errA == nil) != (errB == nil) || (errA != nil && errors.Is(errA, wittydvd.ErrNoNav) != errors.Is(errB, wittydvd.ErrNoNav)) {
			t.Fatalf("step %d: scans differ: %v, %v", i, errA, errB)
		}
		if errA == nil {
			if ma.Scrambled {
				t.Fatal("this disc is encrypted")
			}
			x, err := wire.EncodeMenu(ma)
			if err != nil {
				t.Fatal(err)
			}
			y, err := wire.EncodeMenu(mb)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(x, y) {
				t.Fatalf("step %d: menu pictures differ", i)
			}
			tally["buttons"] += len(ma.Buttons)
		}
		if pa != nil && sa.CellN >= 1 && sa.CellN <= len(pa.Cells) {
			c := pa.Cells[sa.CellN-1]
			a = time.Now()
			xa, errA := be.CellExtents(sp, c)
			extents.builtin = append(extents.builtin, time.Since(a))
			a = time.Now()
			xb, errB := he.CellExtents(sp, c)
			extents.helper = append(extents.helper, time.Since(a))
			if errA != nil || errB != nil || gobOf(t, xa) != gobOf(t, xb) {
				t.Fatalf("step %d: extents differ: %v %v", i, errA, errB)
			}
			tally["extents"] += len(xa)
		}
		// On to the next place, the same way on both: a button if there is one, turn
		// and turn about, else what a player does at the end of the cell.
		switch {
		case errA == nil && len(ma.Buttons) > 0 && i%5 != 4:
			b := ma.Buttons[i%len(ma.Buttons)]
			cmd := b.Cmd
			sa, sb = timed(&steps, func(n dvdsession.Navigator) *wittydvd.Step { return n.PressButton(b.N, &cmd) })
			tally["presses"]++
		case i%17 == 16:
			sa, sb = timed(&steps, func(n dvdsession.Navigator) *wittydvd.Step { return n.MenuKey("root") })
		case i%11 == 10:
			sa, sb = timed(&steps, dvdsession.Navigator.NextChapter)
		default:
			sa, sb = timed(&steps, dvdsession.Navigator.CellEnded)
		}
		// A look ahead on each, as a session does for a menu's next clip.
		ca, cb := bn.Clone().CellEnded(), hn.Clone().CellEnded()
		if gobOf(t, &wire.NavAnswer{Step: ca}) != gobOf(t, &wire.NavAnswer{Step: cb}) {
			t.Fatalf("step %d: looks ahead differ:\n built-in %+v\n helper   %+v", i, ca, cb)
		}
	}
	if r.local != nil {
		t.Fatal("the helper gave up")
	}
	t.Logf("compared %v, %d different places scanned", tally, len(visited))
	t.Logf("opening: %v", &opening)
	t.Logf("a button press or step: %v", &steps)
	t.Logf("a program chain: %v", &pgcs)
	t.Logf("a first menu scan: %v", &scans)
	t.Logf("a scan seen before: %v", &rescans)
	t.Logf("where a cell is: %v", &extents)

	if runtime.GOOS == "windows" {
		if b, err := exec.Command("tasklist", "/FI", fmt.Sprint("PID eq ", p.cmd.Process.Pid), "/FO", "CSV", "/NH").Output(); err == nil {
			t.Logf("the helper now: %s", strings.TrimSpace(string(b)))
		}
	}

	// Subtitles of the feature's first long cell.
	if m := be.Disc.MainTitle(); m != nil && len(m.Cells) > 0 {
		sp := wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: m.VTS}
		c := m.Cells[0].Cell
		c.LastSector = min(c.LastSector, c.FirstSector+100000)
		a := time.Now()
		ca, errA := be.CellSubpictures(sp, c)
		first := time.Since(a)
		a = time.Now()
		cb, errB := he.CellSubpictures(sp, c)
		second := time.Since(a)
		if errA != nil || errB != nil {
			t.Fatalf("%v %v", errA, errB)
		}
		if !bytes.Equal(wire.EncodeSubs(ca), wire.EncodeSubs(cb)) {
			t.Fatal("subtitles differ")
		}
		t.Logf("%d sub-pictures of the feature: built-in %v, helper %v", len(ca.Units), first, second)
	}
}
