package server

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittyplay/cores"
	"github.com/wittywilla/wittyplay/testcore/fakecore"
)

const testPassword = "a long enough password"

var (
	customRunnerOnce sync.Once
	customRunner     string
	customRunnerErr  error
)

// A runner built once for all these tests, because the build takes a while.
func buildCustomRunner(t *testing.T) string {
	t.Helper()
	customRunnerOnce.Do(func() {
		dir, err := os.MkdirTemp("", "wittyplay-runner")
		if err != nil {
			customRunnerErr = err
			return
		}
		customRunner = filepath.Join(dir, "wittyplay-host")
		if runtime.GOOS == "windows" {
			customRunner += ".exe"
		}
		b := exec.Command("go", "build", "-o", customRunner, "./cmd/wittyplay-host")
		b.Dir = filepath.Join("..", "..", "modules", "wittyplay")
		b.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := b.CombinedOutput(); err != nil {
			customRunnerErr = fmt.Errorf("%v: %s", err, out)
		}
	})
	if customRunnerErr != nil {
		t.Fatal(customRunnerErr)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", customRunner)
	return customRunner
}

func fakeCoreFile(t *testing.T, name string, v fakecore.Variant) []byte {
	t.Helper()
	p, err := fakecore.Build(t.TempDir(), name, v)
	if errors.Is(err, fakecore.ErrNoCompiler) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func coreName(id string) string { return id + "_libretro" + cores.LibraryExt() }

// uploadForm builds the multipart body. The password comes first unless asked not to.
func uploadForm(t *testing.T, password, fileName string, body []byte, extra map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	if password != "" {
		_ = w.WriteField("password", password)
	}
	for k, v := range extra {
		_ = w.WriteField(k, v)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, fileName))
	h.Set("Content-Type", "application/octet-stream")
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(body)
	_ = w.Close()
	return &b, w.FormDataContentType()
}

func (g *gameServer) uploadCore(t *testing.T, path, tok, password, fileName string, body []byte, extra map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	b, ct := uploadForm(t, password, fileName, body, extra)
	req := httptest.NewRequest("POST", path, b)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.RemoteAddr = "192.168.1.50:4000"
	w := httptest.NewRecorder()
	g.h.ServeHTTP(w, req)
	return w
}

func (g *gameServer) enableCustom(t *testing.T) {
	t.Helper()
	if w := g.do("PUT", "/api/gaming/custom/enabled", g.admin, map[string]any{"enabled": true, "password": testPassword}); w.Code != 200 {
		t.Fatalf("turning on: %d %s", w.Code, w.Body)
	}
}

type stagedAnswer struct {
	Token  string     `json:"token"`
	Report CoreReport `json:"report"`
	Error  string     `json:"error"`
	Kind   string     `json:"kind"`
}

// addCore takes a file through upload and approval, the way the settings page does.
func (g *gameServer) addCore(t *testing.T, id string, body []byte, consoles []string, extra map[string]any) *CustomCore {
	t.Helper()
	w := g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, testPassword, coreName(id), body, nil)
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	st := decode[stagedAnswer](t, w)
	req := map[string]any{"acknowledged": true, "consoles": consoles}
	for k, v := range extra {
		req[k] = v
	}
	w = g.do("POST", "/api/gaming/custom/staged/"+st.Token+"/approve", g.admin, req)
	if w.Code != 200 {
		t.Fatalf("approve: %d %s", w.Code, w.Body)
	}
	c := decode[CustomCore](t, w)
	return &c
}

func (g *gameServer) customAudit(t *testing.T) []CoreAudit {
	t.Helper()
	return g.s.coreAudit(context.Background(), 200)
}

func auditHas(list []CoreAudit, action, core string) bool {
	for _, a := range list {
		if a.Action == action && (core == "" || a.Core == core) {
			return true
		}
	}
	return false
}

// addGame makes a library of one game for a console.
func (g *gameServer) addGame(t *testing.T, console string) int64 {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "A Game.wittytest"), []byte("rom bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "Games " + console, "type": "games", "paths": []string{root}, "console": console})
	if w.Code >= 300 {
		t.Fatalf("making a library: %d %s", w.Code, w.Body)
	}
	lib := decode[library.Library](t, w)
	g.s.Scanner.Start(lib.ID)
	deadline := time.Now().Add(20 * time.Second)
	for {
		var id int64
		err := g.s.DB.QueryRowContext(ctx, `SELECT id FROM items WHERE library_id = ? LIMIT 1`, lib.ID).Scan(&id)
		if err == nil {
			return id
		}
		if time.Now().After(deadline) {
			t.Fatalf("a scan didn't find the game: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestCustomCoresAreOffByDefault(t *testing.T) {
	buildCustomRunner(t)
	g := newGameServer(t)
	good := fakeCoreFile(t, "x", fakecore.Good)
	st := decode[CustomStatus](t, g.do("GET", "/api/gaming/custom", g.admin, nil))
	if st.Enabled {
		t.Fatal("custom cores must start off")
	}
	if w := g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, testPassword, coreName("a"), good, nil); w.Code != 409 {
		t.Errorf("adding while off: %d %s", w.Code, w.Body)
	}
	if w := g.do("PUT", "/api/gaming/custom/enabled", g.admin, map[string]any{"enabled": true, "password": "wrong"}); w.Code != 403 {
		t.Errorf("turning on with the wrong password: %d", w.Code)
	}
	g.enableCustom(t)
	if !decode[CustomStatus](t, g.do("GET", "/api/gaming/custom", g.admin, nil)).Enabled {
		t.Fatal("it should be on")
	}

	// The general settings route can't turn it on, or change it at all.
	if w := g.do("PUT", "/api/gaming/custom/enabled", g.admin, map[string]any{"enabled": false}); w.Code != 200 {
		t.Fatalf("turning off needs no password: %d", w.Code)
	}
	if w := g.do("PUT", "/api/settings", g.admin, map[string]any{settingCustomCores: "1"}); w.Code == 200 {
		if decode[CustomStatus](t, g.do("GET", "/api/gaming/custom", g.admin, nil)).Enabled {
			t.Fatal("the general settings route turned on custom cores, skipping the password")
		}
	}

	t.Setenv(lockCustomCores, "1")
	if w := g.do("PUT", "/api/gaming/custom/enabled", g.admin, map[string]any{"enabled": true, "password": testPassword}); w.Code != 403 {
		t.Errorf("the machine's owner said no: %d", w.Code)
	}
}

func TestCustomCoreRoutesAreForAdminsOnly(t *testing.T) {
	buildCustomRunner(t)
	g := newGameServer(t)
	g.enableCustom(t)
	good := fakeCoreFile(t, "x", fakecore.Good)
	for _, tok := range []string{g.other, g.kid} {
		for _, c := range []struct{ method, path string }{
			{"GET", "/api/gaming/custom"}, {"PUT", "/api/gaming/custom/enabled"}, {"POST", "/api/gaming/custom/staged/url"}, {"POST", "/api/gaming/custom/staged/placed"},
			{"POST", "/api/gaming/custom/staged/abc/approve"}, {"DELETE", "/api/gaming/custom/staged/abc"}, {"PUT", "/api/gaming/custom/cores/abc"}, {"DELETE", "/api/gaming/custom/cores/abc"},
			{"POST", "/api/gaming/custom/cores/abc/reapprove"}, {"POST", "/api/gaming/custom/cores/abc/rollback"}, {"POST", "/api/gaming/custom/consoles"},
			{"PUT", "/api/gaming/custom/consoles/custom-x"}, {"DELETE", "/api/gaming/custom/consoles/custom-x"}, {"PUT", "/api/gaming/consoles/nes/core"},
			{"GET", "/api/gaming/games/1/core"}, {"PUT", "/api/gaming/games/1/core"},
		} {
			if w := g.do(c.method, c.path, tok, map[string]any{"password": testPassword}); w.Code != 403 {
				t.Errorf("%s %s as a non-admin: %d", c.method, c.path, w.Code)
			}
		}
		if w := g.uploadCore(t, "/api/gaming/custom/staged/upload", tok, testPassword, coreName("a"), good, nil); w.Code != 403 {
			t.Errorf("an upload by a non-admin: %d %s", w.Code, w.Body)
		}
		if w := g.uploadCore(t, "/api/gaming/custom/cores/abc/files", tok, testPassword, "a.zip", good, nil); w.Code != 403 {
			t.Errorf("files from a non-admin: %d", w.Code)
		}
	}
	if st, _ := os.ReadDir(g.s.games().custom.store.Dir); len(st) > 0 {
		t.Errorf("a refused request left %d things behind", len(st))
	}
	if w := g.do("GET", "/api/gaming/custom", "", nil); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
}

func TestKidsProfilesCantReachAnyOfIt(t *testing.T) {
	buildCustomRunner(t)
	g := newGameServer(t)
	g.enableCustom(t)
	ctx := context.Background()
	if _, err := g.s.Auth.CreateProfile(ctx, "Kid", "1234", auth.KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	tok, _, err := g.s.Auth.Login(ctx, "Kid", "1234", "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if w := g.do("GET", "/api/gaming/custom", tok, nil); w.Code != 403 {
		t.Errorf("reading: %d", w.Code)
	}
	if w := g.uploadCore(t, "/api/gaming/custom/staged/upload", tok, "1234", coreName("a"), []byte("x"), nil); w.Code != 403 {
		t.Errorf("uploading: %d", w.Code)
	}
	if w := g.do("PUT", "/api/gaming/custom/enabled", tok, map[string]any{"enabled": false}); w.Code != 403 {
		t.Errorf("turning it off: %d", w.Code)
	}
	for _, v := range decode[[]consoleView](t, g.do("GET", "/api/games/consoles", tok, nil)) {
		if len(v.Custom) != 0 || v.DefaultCore != "" {
			t.Errorf("a kid can see %+v", v)
		}
	}
}

func TestAddingACoreChecksThePasswordFirst(t *testing.T) {
	buildCustomRunner(t)
	g := newGameServer(t)
	g.enableCustom(t)
	good := fakeCoreFile(t, "x", fakecore.Good)
	if w := g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, "not the password", coreName("a"), good, nil); w.Code != 403 {
		t.Errorf("a wrong password: %d %s", w.Code, w.Body)
	}
	if w := g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, "", coreName("a"), good, nil); w.Code != 400 {
		t.Errorf("no password: %d %s", w.Code, w.Body)
	}
	if w := g.do("POST", "/api/gaming/custom/staged/url", g.admin, map[string]any{"password": "nope", "url": "https://example.com/x"}); w.Code != 403 {
		t.Errorf("a wrong password for an address: %d", w.Code)
	}
	if w := g.do("POST", "/api/gaming/custom/staged/placed", g.admin, map[string]any{"password": "nope", "key": "0/x"}); w.Code != 403 {
		t.Errorf("a wrong password for a placed file: %d", w.Code)
	}
	if !auditHas(g.customAudit(t), "password-refused", "") {
		t.Error("a refused password should be written down")
	}
}

// Every hostile file, through the whole route: refused plainly, written down, nothing
// left behind, and the server carries on.
func TestHostileFilesThroughTheAPI(t *testing.T) {
	buildCustomRunner(t)
	g := newGameServer(t)
	g.enableCustom(t)
	g.s.games().custom.probeTime = 3 * time.Second
	store := g.s.games().custom.store
	foreign, err := fakecore.Foreign(t.TempDir(), "other_libretro")
	if err != nil {
		t.Fatal(err)
	}
	foreignBytes, _ := os.ReadFile(foreign)

	for _, c := range []struct {
		name, file string
		body       []byte
		kind, says string
	}{
		{"not a library", coreName("text"), []byte("just some text pretending"), "", "isn't a program library"},
		{"a web page saved by mistake", coreName("page"), []byte("<!doctype html><title>Downloads</title>"), "", "web page"},
		{"wrong architecture", coreName("other"), foreignBytes, "", "built for"},
		{"climbs out of the folder", "../../evil" + cores.LibraryExt(), fakeCoreFile(t, "x", fakecore.Good), "", "climb"},
		{"climbs out, Windows style", `..\..\evil.dll`, fakeCoreFile(t, "x", fakecore.Good), "", "climb"},
		{"crashes at load", coreName("crash"), fakeCoreFile(t, "x", fakecore.CrashAtInit), "crashed", "crashed"},
		{"hangs at load", coreName("hang"), fakeCoreFile(t, "x", fakecore.HangAtInit), "hung", "stopped"},
		{"isn't a core", coreName("nope"), fakeCoreFile(t, "x", fakecore.MissingSymbols), "refused", "isn't a libretro core"},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, testPassword, c.file, c.body, nil)
			if w.Code != 422 {
				t.Fatalf("got %d: %s", w.Code, w.Body)
			}
			got := decode[stagedAnswer](t, w)
			if !strings.Contains(got.Error, c.says) || (c.kind != "" && got.Kind != c.kind) {
				t.Errorf("got %+v", got)
			}
			if ents, _ := os.ReadDir(filepath.Join(store.Dir, ".staging")); len(ents) != 0 {
				t.Errorf("%d things were left behind", len(ents))
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(g.s.Cfg.DataDir), "evil"+cores.LibraryExt())); err == nil {
				t.Error("a file escaped")
			}
		})
	}
	audit := g.customAudit(t)
	if !auditHas(audit, "test-load-failed", "") || !auditHas(audit, "add-refused", "") {
		t.Errorf("failures should be in the audit log: %+v", audit)
	}
	if len(decode[CustomStatus](t, g.do("GET", "/api/gaming/custom", g.admin, nil)).Cores) != 0 {
		t.Error("none of those should have been added")
	}

	t.Run("a huge file", func(t *testing.T) {
		g.s.games().custom.store.Limits = cores.Limits{Library: 1 << 20, Package: 1 << 20, Unpack: 1 << 20, Entries: 10}
		defer func() { g.s.games().custom.store.Limits = cores.Limits{} }()
		w := g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, testPassword, coreName("big"), make([]byte, 3<<20), nil)
		if w.Code != 413 && w.Code != 422 {
			t.Fatalf("got %d: %s", w.Code, w.Body)
		}
		if ents, _ := os.ReadDir(filepath.Join(store.Dir, ".staging")); len(ents) != 0 {
			t.Errorf("%d things were left behind", len(ents))
		}
	})
	t.Run("addresses the server won't fetch", func(t *testing.T) {
		for _, u := range []string{"http://example.com/core.so", "https://127.0.0.1/core.so", "https://192.168.1.2/core.so", "https://169.254.169.254/latest", "https://localhost/core.so", "file:///etc/passwd", "https://u:p@example.com/c.so"} {
			w := g.do("POST", "/api/gaming/custom/staged/url", g.admin, map[string]any{"password": testPassword, "url": u})
			if w.Code != 422 {
				t.Errorf("%s: %d %s", u, w.Code, w.Body)
			}
		}
	})
	t.Run("a name that resolves somewhere private", func(t *testing.T) {
		// Only the connection can catch this one, so the client must really be the
		// safe one. A TLS server on this machine stands in for a private address.
		ts := httptest.NewTLSServer(http.NotFoundHandler())
		defer ts.Close()
		c := customFetchClient()
		if tr := c.Transport.(*http.Transport); tr.Proxy != nil {
			t.Error("a proxy would hide the address being connected to")
		}
		if _, err := c.Get(ts.URL); err == nil || !strings.Contains(err.Error(), "not on the internet") {
			t.Errorf("it connected: %v", err)
		}
	})
	t.Run("files climbing out of the folder by key", func(t *testing.T) {
		for _, key := range []string{"../../evil", "0/../../../etc/passwd", "0/" + strings.Repeat("../", 20) + "x", "/etc/passwd", `0\..\..\evil`} {
			w := g.do("POST", "/api/gaming/custom/staged/placed", g.admin, map[string]any{"password": testPassword, "key": key})
			if w.Code != 422 {
				t.Errorf("%q: %d %s", key, w.Code, w.Body)
			}
		}
	})
	t.Run("the wrong SHA-256", func(t *testing.T) {
		w := g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, testPassword, coreName("a"), fakeCoreFile(t, "x", fakecore.Good), map[string]string{"sha256": strings.Repeat("0", 64)})
		if w.Code != 422 || !strings.Contains(w.Body.String(), "isn't the file") {
			t.Errorf("got %d: %s", w.Code, w.Body)
		}
	})
}

func TestHostileConsolesThroughTheAPI(t *testing.T) {
	buildCustomRunner(t)
	g := newGameServer(t)
	t.Cleanup(func() { wittyplay.SetCustomConsoles(nil) })
	valid := map[string]any{"name": "Fantasy Box", "short": "FBox", "extensions": []string{".fbx"}}
	if w := g.do("POST", "/api/gaming/custom/consoles", g.admin, valid); w.Code != 409 {
		t.Errorf("while off: %d", w.Code)
	}
	g.enableCustom(t)
	for _, tok := range []string{g.other, g.kid} {
		if w := g.do("POST", "/api/gaming/custom/consoles", tok, valid); w.Code != 403 {
			t.Errorf("a non-admin: %d", w.Code)
		}
	}
	for name, body := range map[string]any{
		"a script as its name": map[string]any{"name": "<script>alert(1)</script>", "short": "x", "extensions": []string{".x"}},
		"climbing BIOS":        map[string]any{"name": "A", "short": "A", "extensions": []string{".a"}, "bios": []map[string]any{{"file": "../../secrets"}}},
		"no extension":         map[string]any{"name": "A", "short": "A", "extensions": []string{}},
		"an unknown core":      map[string]any{"name": "A", "short": "A", "extensions": []string{".a"}, "cores": []string{"nothing-called-this"}},
		"a surprise field":     map[string]any{"name": "A", "short": "A", "extensions": []string{".a"}, "hidden": true},
	} {
		if w := g.do("POST", "/api/gaming/custom/consoles", g.admin, body); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	w := g.do("POST", "/api/gaming/custom/consoles", g.admin, map[string]any{"name": "Fantasy Box", "short": "FBox", "extensions": []string{".fbx", ".wittytest"}, "padLike": "snes", "aliases": []string{"Box 3000"}})
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	id := decode[wittyplay.Console](t, w).ID
	if !strings.HasPrefix(id, wittyplay.CustomPrefix) {
		t.Errorf("id = %q", id)
	}
	if w := g.do("PUT", "/api/gaming/custom/consoles/nes", g.admin, valid); w.Code != 404 {
		t.Errorf("editing one of ours: %d", w.Code)
	}
	if w := g.do("DELETE", "/api/gaming/custom/consoles/snes", g.admin, nil); w.Code != 404 {
		t.Errorf("removing one of ours: %d", w.Code)
	}

	// It's in the lists people choose from, for everyone, and it works in a library.
	views := decode[[]consoleView](t, g.do("GET", "/api/games/consoles", g.other, nil))
	found := false
	for _, v := range views {
		found = found || v.ID == id
	}
	if !found {
		t.Error("it should be listed")
	}
	if w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "Boxes", "type": "games", "paths": []string{t.TempDir()}, "console": id}); w.Code >= 300 {
		t.Errorf("a library for it: %d %s", w.Code, w.Body)
	}
	if w := g.do("DELETE", "/api/gaming/custom/consoles/"+id, g.admin, nil); w.Code != 409 {
		t.Errorf("removing one a library uses: %d %s", w.Code, w.Body)
	}
	got := decode[map[string]any](t, g.do("GET", "/api/consoles/"+id+"/controls", g.other, nil))
	pad, _ := got["pad"].(map[string]any)
	if face, _ := pad["face"].([]any); len(face) != 4 {
		t.Errorf("it asked for a controller like the SNES's: %+v", got["pad"])
	}
}

func TestCustomCoreLifecycle(t *testing.T) {
	buildCustomRunner(t)
	g := newGameServer(t)
	g.enableCustom(t)
	t.Cleanup(func() { wittyplay.SetCustomConsoles(nil) })
	ctx := context.Background()
	store := g.s.games().custom.store
	good := fakeCoreFile(t, "x", fakecore.Good)

	// A custom console for it, then the core, then a game.
	w := g.do("POST", "/api/gaming/custom/consoles", g.admin, map[string]any{"name": "Fantasy Box", "short": "FBox", "extensions": []string{".wittytest", ".fake"}})
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	consoleID := decode[wittyplay.Console](t, w).ID
	item := g.addGame(t, consoleID)

	if w := g.do("POST", fmt.Sprintf("/api/games/%d/play", item), g.other, map[string]any{}); w.Code != 409 || !strings.Contains(w.Body.String(), "needCore") {
		t.Errorf("no core yet: %d %s", w.Code, w.Body)
	}

	// The report says what's in the file.
	w = g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, testPassword, coreName("mine"), good, nil)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	st := decode[stagedAnswer](t, w)
	r := st.Report
	if r.ID != "mine" || r.Name != "WittyPlay fake core" || r.Version != "1.2.3" || !r.Binary.Matches || r.SHA256 == "" || r.NeedsGPU != "no" {
		t.Errorf("report = %+v", r)
	}
	if r.License != "" || r.NonCommercial {
		t.Errorf("nobody knows this core's licence: %+v", r)
	}
	if !slices.Contains(r.Consoles, consoleID) {
		t.Errorf("its .wittytest files should point at our console: %v", r.Consoles)
	}
	if w := g.do("POST", "/api/gaming/custom/staged/"+st.Token+"/approve", g.admin, map[string]any{"consoles": []string{consoleID}}); w.Code != 400 {
		t.Errorf("approving without acknowledging: %d", w.Code)
	}
	if w := g.do("POST", "/api/gaming/custom/staged/"+st.Token+"/approve", g.admin, map[string]any{"acknowledged": true, "id": "mgba"}); w.Code != 409 {
		t.Errorf("taking a name from our own table: %d %s", w.Code, w.Body)
	}
	if w := g.do("POST", "/api/gaming/custom/staged/"+st.Token+"/approve", g.admin, map[string]any{"acknowledged": true, "consoles": []string{"nothing"}}); w.Code != 400 {
		t.Errorf("a console that doesn't exist: %d", w.Code)
	}
	w = g.do("POST", "/api/gaming/custom/staged/"+st.Token+"/approve", g.admin, map[string]any{"acknowledged": true, "consoles": []string{consoleID}})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	core := decode[CustomCore](t, w)
	if core.SHA256 != r.SHA256 {
		t.Errorf("approved %s after testing %s", core.SHA256, r.SHA256)
	}
	if w := g.do("POST", "/api/gaming/custom/staged/"+st.Token+"/approve", g.admin, map[string]any{"acknowledged": true}); w.Code != 404 {
		t.Errorf("approving twice: %d", w.Code)
	}
	list := decode[CustomStatus](t, g.do("GET", "/api/gaming/custom", g.admin, nil))
	if len(list.Cores) != 1 || list.Cores[0].State.State != "ok" {
		t.Fatalf("status = %+v", list.Cores)
	}
	if cores.Offered("mine") {
		t.Error("a custom core must never become one we offer")
	}
	if w := g.do("POST", "/api/gaming/cores/mine/install", g.admin, nil); w.Code != 404 {
		t.Errorf("custom cores aren't downloaded by us: %d", w.Code)
	}

	// Non-admins can't see what it is or set it, but they can play with it.
	views := decode[[]consoleView](t, g.do("GET", "/api/games/consoles", g.other, nil))
	for _, v := range views {
		if v.ID == consoleID && (len(v.Custom) != 0 || !v.Ready) {
			t.Errorf("as a non-admin: %+v", v)
		}
	}

	// A game keeps its own save whatever happens to the core.
	saveDir := g.s.games().saveDir(1, item)
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	savePath := filepath.Join(saveDir, "save.srm")
	if err := os.WriteFile(savePath, []byte("battery save 16b"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Plain plays work through the runner: the session's core is what was uploaded.
	playOnce := func(t *testing.T) int {
		w := g.do("POST", fmt.Sprintf("/api/games/%d/play", item), g.admin, map[string]any{})
		if w.Code == 201 {
			g.s.StopGames()
		}
		return w.Code
	}
	if code := playOnce(t); code != 201 && code != 500 {
		t.Errorf("playing: %d", code)
	}

	// It changes on disk: refused, shown, written down, then approved again.
	path := store.CorePath(core.ID, core.SHA256)
	tampered := append(append([]byte{}, good...), 1, 2, 3)
	if err := os.WriteFile(path, tampered, 0o755); err != nil {
		t.Fatal(err)
	}
	w = g.do("POST", fmt.Sprintf("/api/games/%d/play", item), g.other, map[string]any{})
	if w.Code != 409 || !strings.Contains(w.Body.String(), "changed since it was approved") {
		t.Errorf("playing a changed core: %d %s", w.Code, w.Body)
	}
	list = decode[CustomStatus](t, g.do("GET", "/api/gaming/custom", g.admin, nil))
	if list.Cores[0].State.State != "changed" || list.Cores[0].State.Found == "" {
		t.Errorf("state = %+v", list.Cores[0].State)
	}
	if !auditHas(g.customAudit(t), "file-changed", core.ID) {
		t.Error("a changed file should be written down")
	}
	if w := g.do("POST", "/api/gaming/custom/cores/mine/reapprove", g.admin, map[string]any{"password": "wrong"}); w.Code != 403 {
		t.Errorf("approving again takes the password: %d", w.Code)
	}
	if w := g.do("POST", "/api/gaming/custom/cores/mine/reapprove", g.admin, map[string]any{"password": testPassword}); w.Code != 200 {
		t.Fatalf("approving again: %d %s", w.Code, w.Body)
	}
	if got, _ := os.ReadFile(savePath); string(got) != "battery save 16b" {
		t.Errorf("the save became %q", got)
	}
	list = decode[CustomStatus](t, g.do("GET", "/api/gaming/custom", g.admin, nil))
	if list.Cores[0].State.State != "ok" || list.Cores[0].SHA256 == core.SHA256 {
		t.Errorf("after approving again: %+v", list.Cores[0])
	}
	firstSum := list.Cores[0].SHA256

	// Replacing it keeps what it's assigned to, and the old file, and the saves.
	w = g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, testPassword, coreName("mine"), good, map[string]string{"replaces": "mine"})
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	st = decode[stagedAnswer](t, w)
	w = g.do("POST", "/api/gaming/custom/staged/"+st.Token+"/approve", g.admin, map[string]any{"acknowledged": true})
	if w.Code != 200 {
		t.Fatalf("replacing: %d %s", w.Code, w.Body)
	}
	replaced := decode[CustomCore](t, w)
	if len(replaced.Consoles) != 1 || replaced.Consoles[0] != consoleID || replaced.SHA256 == firstSum {
		t.Errorf("replaced = %+v", replaced)
	}
	if got, _ := os.ReadFile(savePath); string(got) != "battery save 16b" {
		t.Errorf("the save became %q", got)
	}
	// Adding another core under the same name without saying so is refused.
	w = g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, testPassword, coreName("mine"), tampered, nil)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	st = decode[stagedAnswer](t, w)
	if w := g.do("POST", "/api/gaming/custom/staged/"+st.Token+"/approve", g.admin, map[string]any{"acknowledged": true}); w.Code != 409 {
		t.Errorf("taking the name of a core already here: %d %s", w.Code, w.Body)
	}
	if w := g.do("DELETE", "/api/gaming/custom/staged/"+st.Token, g.admin, nil); w.Code != 200 {
		t.Errorf("cancelling: %d", w.Code)
	}

	// Roll back, with the password, and forward again.
	if w := g.do("POST", "/api/gaming/custom/cores/mine/rollback", g.admin, map[string]any{"password": testPassword, "sha256": firstSum}); w.Code != 200 {
		t.Fatalf("rolling back: %d %s", w.Code, w.Body)
	}
	if got := decode[CustomStatus](t, g.do("GET", "/api/gaming/custom", g.admin, nil)).Cores[0]; got.SHA256 != firstSum || len(got.Versions) < 2 {
		t.Errorf("rolled back to %+v", got)
	}
	if w := g.do("POST", "/api/gaming/custom/cores/mine/rollback", g.admin, map[string]any{"password": testPassword, "sha256": strings.Repeat("a", 64)}); w.Code != 404 {
		t.Errorf("a version that was never here: %d", w.Code)
	}
	if w := g.do("POST", "/api/gaming/custom/cores/mine/rollback", g.admin, map[string]any{"password": testPassword, "sha256": "../../etc"}); w.Code != 400 {
		t.Errorf("a path as a version: %d", w.Code)
	}

	// A game can have its own core, and a console its default.
	w = g.do("GET", fmt.Sprintf("/api/gaming/games/%d/core", item), g.admin, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"mine"`) {
		t.Errorf("game core: %d %s", w.Code, w.Body)
	}
	if w := g.do("PUT", fmt.Sprintf("/api/gaming/games/%d/core", item), g.admin, map[string]any{"core": "nothing"}); w.Code != 400 {
		t.Errorf("a core that isn't for it: %d", w.Code)
	}
	if w := g.do("PUT", fmt.Sprintf("/api/gaming/games/%d/core", item), g.admin, map[string]any{"core": "mine"}); w.Code != 200 {
		t.Errorf("choosing: %d %s", w.Code, w.Body)
	}
	if w := g.do("PUT", "/api/gaming/consoles/"+consoleID+"/core", g.admin, map[string]any{"core": "mine"}); w.Code != 200 {
		t.Errorf("console default: %d %s", w.Code, w.Body)
	}
	if w := g.do("PUT", "/api/gaming/consoles/snes/core", g.admin, map[string]any{"core": "mine"}); w.Code != 400 {
		t.Errorf("a console it isn't assigned to: %d", w.Code)
	}

	// Editing what it's for; a licence typed in; a non-commercial one labelled.
	w = g.do("PUT", "/api/gaming/custom/cores/mine", g.admin, map[string]any{"license": "CC BY-NC 4.0", "consoles": []string{consoleID, "snes"}, "defaultFor": []string{"snes"}})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	edited := decode[CustomCore](t, w)
	if !edited.NonCommercial || edited.LicenseSource != "admin" {
		t.Errorf("edited = %+v", edited)
	}
	if id := g.s.preferredCores(ctx)["snes"]; id != "mine" {
		t.Errorf("the SNES's default is %q", id)
	}

	// Turned off, nothing plays it, and nothing is lost.
	if w := g.do("PUT", "/api/gaming/custom/enabled", g.admin, map[string]any{"enabled": false}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := g.do("POST", fmt.Sprintf("/api/games/%d/play", item), g.other, map[string]any{}); w.Code != 409 {
		t.Errorf("playing with custom cores off: %d %s", w.Code, w.Body)
	}
	if got := g.s.games().cores.List(); len(got) != 0 {
		t.Errorf("with it off, the list holds %+v", got)
	}
	g.enableCustom(t)

	// Removing it takes the core, and nothing else.
	path = store.CorePath(core.ID, firstSum)
	if w := g.do("DELETE", "/api/gaming/custom/cores/mine", g.admin, nil); w.Code != 200 {
		t.Fatalf("removing: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("its file is still there: %v", err)
	}
	if got, _ := os.ReadFile(savePath); string(got) != "battery save 16b" {
		t.Errorf("the save became %q", got)
	}
	if id := g.s.preferredCores(ctx)["snes"]; id != "" {
		t.Errorf("the SNES still points at it: %q", id)
	}
	if id := g.s.gameCoreChoice(ctx, item); id != "" {
		t.Errorf("the game still points at it: %q", id)
	}
	audit := g.customAudit(t)
	for _, a := range []string{"added", "replaced", "changed", "reapproved", "rolled-back", "removed"} {
		if !auditHas(audit, a, "mine") {
			t.Errorf("%q isn't in the audit log", a)
		}
	}
}

func TestAFilePlacedInTheFolderCantBeChosenUntilAdded(t *testing.T) {
	buildCustomRunner(t)
	g := newGameServer(t)
	g.enableCustom(t)
	ctx := context.Background()
	good := fakeCoreFile(t, "x", fakecore.Good)
	dir := g.s.games().cores.Dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, coreName("placed")), good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "placed_libretro.info"), []byte(`license = "MIT"`+"\n"+`display_name = "Placed"`), 0o644); err != nil {
		t.Fatal(err)
	}
	console := wittyplay.ByID("nes")
	if err := g.s.DB.SetSetting(ctx, "gamingCores", `{"nes":"placed"}`); err != nil {
		t.Fatal(err)
	}
	if got := g.s.preferredCore(ctx, console, 0); got != "" {
		t.Errorf("a setting reached a file nobody approved: %q", got)
	}
	if core := g.s.games().cores.ForConsole(console, g.s.preferredCore(ctx, console, 0)); core != nil {
		t.Errorf("it would play with %+v", core)
	}

	st := decode[CustomStatus](t, g.do("GET", "/api/gaming/custom", g.admin, nil))
	key := ""
	for _, p := range st.Placed {
		if p.Name == coreName("placed") {
			key = p.Key
		}
	}
	if key == "" {
		t.Fatalf("it should be offered: %+v", st.Placed)
	}
	w := g.do("POST", "/api/gaming/custom/staged/placed", g.admin, map[string]any{"password": testPassword, "key": key})
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := decode[stagedAnswer](t, w)
	if got.Report.License != "MIT" || got.Report.LicenseSource != "info" || !got.Report.HasInfo {
		t.Errorf("report = %+v", got.Report)
	}
	w = g.do("POST", "/api/gaming/custom/staged/"+got.Token+"/approve", g.admin, map[string]any{"acknowledged": true, "consoles": []string{"nes"}, "defaultFor": []string{"nes"}})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if id := g.s.preferredCore(ctx, console, 0); id != "placed" {
		t.Errorf("once added it should be used: %q", id)
	}
	if _, err := os.Stat(filepath.Join(dir, coreName("placed"))); !os.IsNotExist(err) {
		t.Error("it should have moved out of the folder")
	}
}

func TestPackagesAndExtrasThroughTheAPI(t *testing.T) {
	buildCustomRunner(t)
	g := newGameServer(t)
	g.enableCustom(t)
	good := fakeCoreFile(t, "x", fakecore.Good)
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	add := func(name string, body []byte) {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write(body)
	}
	add(coreName("pack"), good)
	add("pack_libretro.info", []byte("license = \"Non-commercial\"\nfirmware_count = 1\nfirmware0_path = \"needs.bin\"\nfirmware0_opt = \"false\"\nsupported_extensions = \"zzz\"\ncorename = \"Something Else Entirely\"\n"))
	add("assets/data.txt", []byte("asset"))
	add("system/support.bin", []byte("support"))
	_ = zw.Close()
	w := g.uploadCore(t, "/api/gaming/custom/staged/upload", g.admin, testPassword, "pack.zip", b.Bytes(), nil)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	st := decode[stagedAnswer](t, w)
	r := st.Report
	if !r.NonCommercial || r.LicenseSource != "info" || !r.BIOSKnown || len(r.BIOS) != 1 || !r.BIOS[0].Required || r.BIOS[0].Present || r.Assets != 1 || r.SystemFiles != 1 {
		t.Errorf("report = %+v", r)
	}
	if len(r.Warnings) < 2 {
		t.Errorf("an info file that doesn't match should be warned about: %v", r.Warnings)
	}
	w = g.do("POST", "/api/gaming/custom/staged/"+st.Token+"/approve", g.admin, map[string]any{"acknowledged": true, "consoles": []string{"nes"}})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	store := g.s.games().custom.store
	core := decode[CustomCore](t, w)
	if store.AssetsPath(core.ID, core.SHA256) == "" {
		t.Error("the assets folder should be there")
	}
	sys := filepath.Join(g.s.games().systemDir(), "support.bin")
	if _, err := os.Stat(sys); err != nil {
		t.Errorf("the system file: %v", err)
	}
	// Non-commercial cores are never in anything we offer.
	if cores.Offered(core.ID) {
		t.Error("it must not be offered")
	}

	// Adding files later.
	var more bytes.Buffer
	zw = zip.NewWriter(&more)
	f, _ := zw.Create("assets/more.txt")
	_, _ = f.Write([]byte("more"))
	_ = zw.Close()
	if w := g.uploadCore(t, "/api/gaming/custom/cores/pack/files", g.admin, "wrong", "more.zip", more.Bytes(), nil); w.Code != 403 {
		t.Errorf("wrong password: %d", w.Code)
	}
	if w := g.uploadCore(t, "/api/gaming/custom/cores/pack/files", g.admin, testPassword, "more.zip", more.Bytes(), nil); w.Code != 200 {
		t.Errorf("adding files: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(store.AssetsPath(core.ID, core.SHA256), "more.txt")); err != nil {
		t.Error(err)
	}
	if w := g.uploadCore(t, "/api/gaming/custom/cores/pack/files", g.admin, testPassword, "core.exe", good, nil); w.Code != 422 {
		t.Errorf("a program as an extra: %d %s", w.Code, w.Body)
	}
	if w := g.uploadCore(t, "/api/gaming/custom/cores/pack/files", g.admin, testPassword, "../../evil.zip", more.Bytes(), nil); w.Code != 422 {
		t.Errorf("a climbing name: %d %s", w.Code, w.Body)
	}

	// Removing it takes back what it added to the BIOS folder, and only that.
	mine := filepath.Join(g.s.games().systemDir(), "mine.bin")
	if err := os.WriteFile(mine, []byte("my BIOS"), 0o644); err != nil {
		t.Fatal(err)
	}
	if w := g.do("DELETE", "/api/gaming/custom/cores/pack", g.admin, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(sys); !os.IsNotExist(err) {
		t.Errorf("its support file should go with it: %v", err)
	}
	if _, err := os.Stat(mine); err != nil {
		t.Errorf("someone's own BIOS should stay: %v", err)
	}
}

// A real game through the whole path, with a real library that the runner loads and a
// real stream of events: only with a core that can draw, so with a compiler.
func TestPlayingWithACustomCore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hand-written Windows core has no frames to play; docs/GAMING.md says what was run by hand")
	}
	buildCustomRunner(t)
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no FFmpeg on PATH")
	}
	g := newGameServer(t)
	defer g.s.StopGames()
	g.enableCustom(t)
	g.s.Tools.FFmpeg, _ = exec.LookPath("ffmpeg")
	out, _ := exec.Command(g.s.Tools.FFmpeg, "-hide_banner", "-encoders").Output()
	for _, n := range []string{"libx264", "libvpx", "libopus"} {
		g.s.Tools.Encoders[n] = strings.Contains(string(out), " "+n+" ")
	}
	t.Cleanup(func() { wittyplay.SetCustomConsoles(nil) })
	w := g.do("POST", "/api/gaming/custom/consoles", g.admin, map[string]any{"name": "Fantasy Box", "short": "FBox", "extensions": []string{".wittytest"}})
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	console := decode[wittyplay.Console](t, w).ID
	item := g.addGame(t, console)
	g.addCore(t, "mine", fakeCoreFile(t, "x", fakecore.Good), []string{console}, nil)
	w = g.do("POST", fmt.Sprintf("/api/games/%d/play", item), g.other, map[string]any{"client": "browser"})
	if w.Code != 201 {
		t.Fatalf("playing: %d %s", w.Code, w.Body)
	}
	if info := decode[map[string]any](t, w); info["core"] != "WittyPlay fake core" {
		t.Errorf("played with %v", info["core"])
	}
	g.s.StopGames()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p := g.s.corePlayTotals(context.Background())["mine"]; p.Starts == 1 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("the play wasn't counted")
}

func TestBestCoreComesFromRealPlay(t *testing.T) {
	g := newGameServer(t)
	ctx := context.Background()
	lib, err := g.s.Store.CreateLibrary(ctx, "G", "games", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	res, err := g.s.DB.ExecContext(ctx, `INSERT INTO items(library_id, path, title, sort_title, kind, added_at, updated_at, console) VALUES(?, 'x.nes', 'x', 'x', 'game', 0, 0, 'nes')`, lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	item, _ := res.LastInsertId()
	g.s.noteCorePlay(ctx, "brief", item, 1, 0, 5, "")
	options := []GameCoreOption{{ID: "steady"}, {ID: "crashy"}, {ID: "brief"}}
	g.s.markBestCore(ctx, item, options)
	for _, o := range options {
		if o.Best {
			t.Errorf("five seconds isn't evidence: %+v", o)
		}
	}
	g.s.noteCorePlay(ctx, "steady", item, 3, 0, 3000, "")
	g.s.noteCorePlay(ctx, "crashy", item, 3, 2, 3600, "it crashed")
	g.s.markBestCore(ctx, item, options)
	if !options[0].Best || options[1].Best {
		t.Errorf("options = %+v", options)
	}
	if options[1].LastError != "it crashed" || options[1].Failures != 2 {
		t.Errorf("options = %+v", options)
	}
}
