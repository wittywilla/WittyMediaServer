package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/libtranscode"
)

// The library transcoding API's own rules. What it does to files is tried on real files in
// internal/libtranscode; sign-in is tried for every route by the contract tests.

func TestLibTranscodeAPI(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	ctx := context.Background()
	lib, err := ts.Store.CreateLibrary(ctx, "Films", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	music, err := ts.Store.CreateLibrary(ctx, "Songs", "music", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path string, body any) (int, string) {
		t.Helper()
		w := ts.do(t, method, "/api/libtranscode"+path, admin, body)
		return w.Code, w.Body.String()
	}
	raw := func(s string) []byte { return []byte(s) }
	libPath := "/libraries/" + strconv.FormatInt(lib.ID, 10)

	// It starts off, in a way that stays out of people's way.
	code, body := do(http.MethodGet, "/settings", nil)
	var set libtranscode.Settings
	if err := json.Unmarshal([]byte(body), &set); err != nil || code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if set.Enabled || set.Acknowledged || !set.LowPriority || !set.PauseWhileStreaming || set.MaxJobs != 1 {
		t.Fatalf("%+v", set)
	}
	// It can't be turned on without the warning having been read.
	if code, body := do(http.MethodPut, "/settings", raw(`{"enabled":true,"acknowledged":false}`)); code != 400 {
		t.Fatalf("turned on without accepting the warning: %d %s", code, body)
	}
	if code, _ := do(http.MethodPut, "/settings", raw(`{"enabled":true,"acknowledged":true,"maxJobs":99}`)); code != 400 {
		t.Fatalf("99 files at once: %d", code)
	}
	// While it's off, nothing but a dry run can start, and the page is told why.
	policy, _ := libtranscode.PresetPolicy(libtranscode.PresetCompatible)
	if code, body := do(http.MethodPut, libPath, map[string]any{"policy": policy, "output": "keep"}); code != 200 || !strings.Contains(body, "off") {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := do(http.MethodPost, libPath+"/runs", raw(`{"dry":false}`)); code != 409 || !strings.Contains(body, "off") {
		t.Fatalf("started while off: %d %s", code, body)
	}
	if code, body := do(http.MethodPost, libPath+"/runs", raw(`{"dry":true}`)); code != 202 {
		t.Fatalf("a dry run: %d %s", code, body)
	}
	// Recipes from outside are checked.
	bad := policy
	bad.Container = "avi -i /etc/passwd"
	if code, _ := do(http.MethodPut, libPath, map[string]any{"policy": bad, "output": "keep"}); code != 400 {
		t.Fatalf("a bad recipe: %d", code)
	}
	if code, _ := do(http.MethodPut, libPath, map[string]any{"policy": policy, "output": "overwrite-everything"}); code != 400 {
		t.Fatalf("a bad output: %d", code)
	}
	// Read-only sticks to the library, and says so.
	if code, body := do(http.MethodPut, libPath, map[string]any{"policy": policy, "output": "keep", "readOnly": true}); code != 200 || !strings.Contains(body, `"readOnly":true`) {
		t.Fatalf("%d %s", code, body)
	}
	if l, err := ts.Store.Library(ctx, lib.ID); err != nil || !l.ReadOnly {
		t.Fatalf("%+v %v", l, err)
	}
	// Only libraries of video, and only those that exist.
	if code, _ := do(http.MethodPost, "/libraries/"+strconv.FormatInt(music.ID, 10)+"/preview", raw(`{"itemId":1}`)); code != 400 {
		t.Fatalf("a music library: %d", code)
	}
	if code, _ := do(http.MethodPost, "/libraries/9999/preview", raw(`{"itemId":1}`)); code != 404 {
		t.Fatalf("no such library: %d", code)
	}
	if code, body := do(http.MethodGet, "/libraries", nil); code != 200 || strings.Contains(body, "Songs") || !strings.Contains(body, "Films") {
		t.Fatalf("%d %s", code, body)
	}
	// The Tdarr key goes in and never comes back.
	code, body = do(http.MethodPut, "/tdarr", raw(`{"enabled":false,"url":"http://127.0.0.1:1","password":"correct-horse-1","apiKey":"tapi_secret_value","mappings":[]}`))
	if code != 200 || strings.Contains(body, "tapi_secret_value") {
		t.Fatalf("%d %s", code, body)
	}
	_, body = do(http.MethodGet, "/tdarr", nil)
	var got libtranscode.TdarrSettings
	if err := json.Unmarshal([]byte(body), &got); err != nil || !got.KeySet || strings.Contains(body, "tapi_secret_value") {
		t.Fatalf("%+v %v", got, err)
	}
	if code, _ := do(http.MethodPut, "/tdarr", raw(`{"enabled":true,"url":"file:///etc/passwd","mappings":[]}`)); code != 400 {
		t.Fatalf("a file address: %d", code)
	}
	// Nothing answers there, and the test says so in words.
	_, body = do(http.MethodPost, "/tdarr/test", raw(`{}`))
	var rep libtranscode.TdarrReport
	if err := json.Unmarshal([]byte(body), &rep); err != nil || rep.OK || rep.Error == "" {
		t.Fatalf("%+v %v", rep, err)
	}
	if code, _ := do(http.MethodPost, "/tdarr/send", raw(`{"libraryId":1}`)); code != 409 {
		t.Fatalf("sent with the connection off: %d", code)
	}
}

func TestReadOnlyLibraryKeepsSavedDetailsOut(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	dir := t.TempDir()
	lib, err := ts.Store.CreateLibrary(ctx, "Films", "movies", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Store.SetLibraryReadOnly(ctx, lib.ID, true); err != nil {
		t.Fatal(err)
	}
	if l, _ := ts.Store.Library(ctx, lib.ID); !l.ReadOnly {
		t.Fatal("not stored")
	}
}

// Replacing originals deletes people's films, so it takes more than a stolen session to start.
func TestReplacingOriginalsAsksForThePassword(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	lib, err := ts.Store.CreateLibrary(context.Background(), "Films", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := libtranscode.PresetPolicy(libtranscode.PresetCompatible)
	path := "/api/libtranscode/libraries/" + strconv.FormatInt(lib.ID, 10)
	body := map[string]any{"policy": policy, "output": libtranscode.OutputReplace}
	if w := ts.do(t, http.MethodPut, path, admin, body); w.Code != 403 {
		t.Fatalf("no password: %d %s", w.Code, w.Body)
	}
	body["current"] = "not the password"
	if w := ts.do(t, http.MethodPut, path, admin, body); w.Code != 403 {
		t.Fatalf("wrong password: %d %s", w.Code, w.Body)
	}
	if got := libtranscode.LoadRule(context.Background(), ts.DB, lib.ID).Output; got == libtranscode.OutputReplace {
		t.Fatal("it was saved")
	}
	body["current"] = "correct-horse-1"
	if w := ts.do(t, http.MethodPut, path, admin, body); w.Code != 200 {
		t.Fatalf("the right password: %d %s", w.Code, w.Body)
	}
	// Saving other changes to a rule that already replaces asks nothing more.
	delete(body, "current")
	body["auto"] = true
	if w := ts.do(t, http.MethodPut, path, admin, body); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
