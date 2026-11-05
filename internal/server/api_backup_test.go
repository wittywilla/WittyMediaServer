package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/backup"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

const backupPassword = "correct horse battery"

type backupServer struct {
	*contractServer
	admin, kid string
}

func newBackupServer(t *testing.T) *backupServer {
	t.Helper()
	ts := newContractServer(t)
	data := t.TempDir()
	ts.Cfg.DataDir = data
	ts.Cfg.ImagesDir = filepath.Join(data, "cache", "images")
	ctx := t.Context()
	if _, err := ts.Auth.CreateUser(ctx, "kim", backupPassword, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Auth.CreateProfile(ctx, "small", backupPassword, auth.KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	admin, _, err := ts.Auth.Login(ctx, "kim", backupPassword, "", false)
	if err != nil {
		t.Fatal(err)
	}
	kid, _, err := ts.Auth.Login(ctx, "small", backupPassword, "", false)
	if err != nil {
		t.Fatal(err)
	}
	return &backupServer{contractServer: ts, admin: admin, kid: kid}
}

func (b *backupServer) call(method, path, token string, body any) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if raw, ok := body.([]byte); ok {
		rd = bytes.NewReader(raw)
	} else if body != nil {
		js, _ := json.Marshal(body)
		rd = bytes.NewReader(js)
	} else {
		rd = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, rd)
	r.RemoteAddr = "192.168.1.50:4000"
	r.Host = "localhost:9797"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	b.h.ServeHTTP(w, r)
	return w
}

func (b *backupServer) waitJob(t *testing.T) backupJob {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		b.backup.mu.Lock()
		j := b.backup.job
		b.backup.mu.Unlock()
		if !j.Running {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the job didn't finish")
	return backupJob{}
}

// Everything here is an administrator's, and the one public route needs a ticket.
func TestBackupRoutesCheckWhoIsAsking(t *testing.T) {
	b := newBackupServer(t)
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/backup"}, {"GET", "/api/backup/estimate"}, {"POST", "/api/backup/create"}, {"POST", "/api/backup/cancel"}, {"PUT", "/api/backup/schedule"},
		{"DELETE", "/api/backup/files/wms-backup-x.zip"}, {"POST", "/api/backup/files/wms-backup-x.zip/link"}, {"POST", "/api/backup/restore/upload"},
		{"POST", "/api/backup/restore/inspect"}, {"POST", "/api/backup/restore/start"}, {"DELETE", "/api/backup/restore/request"}, {"POST", "/api/backup/undo"},
	} {
		if w := b.call(c.method, c.path, "", nil); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", c.method, c.path, w.Code)
		}
		if w := b.call(c.method, c.path, b.kid, nil); w.Code != 403 {
			t.Errorf("%s %s as a kid: %d", c.method, c.path, w.Code)
		}
	}
	for _, q := range []string{"", "?st=", "?st=not-a-ticket", "?st=../../etc/passwd"} {
		if w := b.call("GET", "/api/backup/download"+q, "", nil); w.Code != 401 {
			t.Errorf("a download with %q: %d", q, w.Code)
		}
	}
}

// Names go through one check, so a path can't be passed as one.
func TestBackupNamesAreChecked(t *testing.T) {
	b := newBackupServer(t)
	for _, name := range []string{"..%2f..%2fwms.db", "wms-backup-..%2f..%2fwms.db.zip", "x.zip"} {
		if w := b.call("POST", "/api/backup/files/"+name+"/link", b.admin, nil); w.Code != 404 {
			t.Errorf("link %s: %d", name, w.Code)
		}
		if w := b.call("DELETE", "/api/backup/files/"+name, b.admin, nil); w.Code != 404 {
			t.Errorf("delete %s: %d", name, w.Code)
		}
	}
	if w := b.call("POST", "/api/backup/restore/inspect", b.admin, map[string]any{"file": "../../wms.db"}); w.Code != 404 {
		t.Errorf("inspect a path: %d", w.Code)
	}
}

// The whole thing, through the routes: make one, download it with a ticket, upload it, check
// it, restore it. A restart is what's left for the next start, and this ends there.
func TestBackupThroughTheRoutes(t *testing.T) {
	b := newBackupServer(t)
	restarted := make(chan struct{}, 1)
	b.Restart = func() { restarted <- struct{}{} }
	if err := b.DB.SetSetting(t.Context(), "tmdbKey", "APIKEYPLANTED"); err != nil {
		t.Fatal(err)
	}

	// A password that's too short is refused before anything starts.
	if w := b.call("POST", "/api/backup/create", b.admin, map[string]any{"parts": []string{"settings"}, "password": "short"}); w.Code != 400 {
		t.Fatalf("a short password: %d", w.Code)
	}
	if w := b.call("POST", "/api/backup/create", b.admin, map[string]any{"parts": []string{"nonsense"}}); w.Code != 400 {
		t.Fatalf("a part that doesn't exist: %d", w.Code)
	}
	// Watch history can't go without what its numbers refer to.
	if w := b.call("POST", "/api/backup/create", b.admin, map[string]any{"parts": []string{"history"}}); w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if j := b.waitJob(t); j.Error == "" {
		t.Fatal("history alone was allowed")
	}

	parts := []string{"library", "settings", "users", "history", "channels", "artwork"}
	if w := b.call("POST", "/api/backup/create", b.admin, map[string]any{"parts": parts, "password": backupPassword}); w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	j := b.waitJob(t)
	if j.Error != "" || j.File == "" {
		t.Fatalf("%+v", j)
	}

	// The view lists it, and says nothing of the secret.
	w := b.call("GET", "/api/backup", b.admin, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), j.File) || strings.Contains(w.Body.String(), "APIKEYPLANTED") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := b.call("GET", "/api/backup/estimate", b.admin, nil); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}

	// A backup holds every account's password hash, and with a password every secret too. So a
	// stolen session can make one, and can't take it away.
	if w := b.call("POST", "/api/backup/files/"+j.File+"/link", b.admin, nil); w.Code == 200 {
		t.Fatalf("a download link with no password: %s", w.Body)
	}
	if w := b.call("POST", "/api/backup/files/"+j.File+"/link", b.admin, map[string]any{"password": "not the password"}); w.Code != 403 {
		t.Fatalf("a download link with a wrong password: %d %s", w.Code, w.Body)
	}

	// A ticket opens that one file.
	w = b.call("POST", "/api/backup/files/"+j.File+"/link", b.admin, map[string]any{"password": backupPassword})
	var link struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &link); err != nil || link.URL == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = b.call("GET", link.URL, "", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), j.File) || w.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	archive := w.Body.Bytes()
	if bytes.Contains(archive, []byte("APIKEYPLANTED")) {
		t.Fatal("a protected archive shows a secret")
	}

	// Uploaded under a new name, checked with a wrong password, then the right one.
	w = b.call("POST", "/api/backup/restore/upload", b.admin, archive)
	var up struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &up); err != nil || up.File == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := b.call("POST", "/api/backup/restore/inspect", b.admin, map[string]any{"file": up.File, "password": "wrong wrong wrong"}); w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := b.waitJob(t); !got.NeedsPassword || got.Inspection != nil {
		t.Fatalf("%+v", got)
	}
	if w := b.call("POST", "/api/backup/restore/inspect", b.admin, map[string]any{"file": up.File, "password": backupPassword}); w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := b.waitJob(t)
	if got.Inspection == nil || !got.Inspection.Protected || len(got.Inspection.Accounts) != 2 {
		t.Fatalf("%+v", got)
	}

	// Restoring takes the word, and the administrator's password again.
	parts = nil
	for _, p := range got.Inspection.Parts {
		parts = append(parts, p.ID)
	}
	if w := b.call("POST", "/api/backup/restore/start", b.admin, map[string]any{"parts": parts, "password": backupPassword}); w.Code != 400 {
		t.Fatalf("no word: %d", w.Code)
	}
	if w := b.call("POST", "/api/backup/restore/start", b.admin, map[string]any{"parts": parts, "confirm": "RESTORE", "password": "not it"}); w.Code != 403 {
		t.Fatalf("wrong password: %d", w.Code)
	}
	if w := b.call("POST", "/api/backup/restore/start", b.admin, map[string]any{"parts": parts, "confirm": "RESTORE", "password": backupPassword}); w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	done := b.waitJob(t)
	if done.Error != "" || !done.Restarting {
		t.Fatalf("%+v", done)
	}
	select {
	case <-restarted:
	case <-time.After(5 * time.Second):
		t.Fatal("no restart")
	}
	f := b.backupFolders()
	if backup.Pending(f) == nil {
		t.Fatal("nothing's waiting for the next start")
	}
	if w := b.call("DELETE", "/api/backup/restore/request", b.admin, nil); w.Code != 200 || backup.Pending(f) != nil {
		t.Fatalf("taking it back: %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(f.Data, "restore-staged")); err == nil {
		t.Error("what was unpacked is still there")
	}

	// A stolen session can't take away the way back.
	if w := b.call("DELETE", "/api/backup/files/"+j.File, b.admin, nil); w.Code != 403 {
		t.Fatalf("deleted with no password: %d", w.Code)
	}
	if w := b.call("DELETE", "/api/backup/files/"+j.File, b.admin, map[string]any{"password": backupPassword}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := b.call("GET", link.URL, "", nil); w.Code != 404 {
		t.Errorf("a ticket outlived its file: %d", w.Code)
	}
}

// An upload has no size limit of its own, and may come from someone with no account during first-time
// setup, so what it can't do is fill the disk, or take the place of what's been checked already.
func TestBackupUploadLeavesRoomAndKeepsItsOwnName(t *testing.T) {
	b := newBackupServer(t)
	old := uploadFreeFloor
	defer func() { uploadFreeFloor = old }()
	send := func(body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/backup/restore/upload", bytes.NewReader(body))
		r.RemoteAddr, r.Host = "192.168.1.50:4000", "localhost:9797"
		r.Header.Set("Authorization", "Bearer "+b.admin)
		r.ContentLength = -1 // sent in pieces: nothing says how long it is
		w := httptest.NewRecorder()
		b.h.ServeHTTP(w, r)
		return w
	}
	uploadFreeFloor = 1 << 62
	if w := send([]byte("hello")); w.Code != 507 {
		t.Fatalf("no room at all: %d %s", w.Code, w.Body)
	}
	if err := os.MkdirAll(b.backupFolders().Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	free := sysinfo.FreeBytes(b.backupFolders().Dir())
	uploadFreeFloor = free - 64<<10
	if w := send(make([]byte, 32<<20)); w.Code != 507 {
		t.Fatalf("more than there's room for: %d %s", w.Code, w.Body)
	}
	if list := backup.List(b.backupFolders()); len(list) != 0 {
		t.Errorf("what didn't fit stayed: %+v", list)
	}
	uploadFreeFloor = old
	names := map[string]bool{}
	for range 3 {
		w := send([]byte("PK"))
		var up struct {
			File string `json:"file"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &up); err != nil || up.File == "" {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		names[up.File] = true
	}
	if len(names) != 3 {
		t.Errorf("uploads in the same second took the same name, so one can replace what was checked: %v", names)
	}
}

// A bad archive is refused through the routes too, and the job says why in words.
func TestBackupRefusalReachesThePage(t *testing.T) {
	b := newBackupServer(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w1, _ := zw.Create(backup.ManifestName)
	_, _ = w1.Write([]byte(`{"format":1,"app":"WittyMediaServer","parts":[]}`))
	w2, _ := zw.CreateHeader(&zip.FileHeader{Name: "../../evil.jpg", Method: zip.Store})
	_, _ = w2.Write([]byte("x"))
	_ = zw.Close()
	w := b.call("POST", "/api/backup/restore/upload", b.admin, buf.Bytes())
	var up struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	if w := b.call("POST", "/api/backup/restore/inspect", b.admin, map[string]any{"file": up.File}); w.Code != 202 {
		t.Fatalf("%d", w.Code)
	}
	j := b.waitJob(t)
	if !j.Refused || !strings.Contains(j.Error, "leave its folder") {
		t.Fatalf("%+v", j)
	}
	if w := b.call("POST", "/api/backup/restore/start", b.admin, map[string]any{"parts": []string{"artwork"}, "confirm": "RESTORE", "password": backupPassword}); w.Code != 409 {
		t.Fatalf("restoring what was refused: %d", w.Code)
	}
}

// First-time setup: no accounts, so whoever could set the server up may restore, and nobody else.
// A restore brings its own administrators, so it asks exactly what setting up asks. It once let the
// whole home network in with no code, which got round the setup code altogether: a stranger there
// could get to a new server first and restore a file with an account of their own.
func TestBackupFromFirstTimeSetup(t *testing.T) {
	ts := newContractServer(t)
	data := t.TempDir()
	ts.Cfg.DataDir = data
	ts.Cfg.ImagesDir = filepath.Join(data, "cache", "images")
	do := func(remote, host, code string) int {
		r := httptest.NewRequest("GET", "/api/backup", nil)
		r.RemoteAddr, r.Host = remote, host
		if code != "" {
			r.Header.Set("X-Setup-Code", code)
		}
		w := httptest.NewRecorder()
		ts.h.ServeHTTP(w, r)
		return w.Code
	}
	if c := do("192.168.1.50:1000", "192.168.1.2:9797", ""); c != 403 {
		t.Errorf("from home with no code, on a server nobody has set up: %d", c)
	}
	if c := do("192.168.1.50:1000", "192.168.1.2:9797", "WRONG"); c != 403 {
		t.Errorf("from home with a wrong code: %d", c)
	}
	if c := do("192.168.1.50:1000", "192.168.1.2:9797", ts.SetupCode()); c != 200 {
		t.Errorf("from home with the code: %d", c)
	}
	if c := do("127.0.0.1:1000", "localhost:9797", ""); c != 200 {
		t.Errorf("on this machine, as setting up allows: %d", c)
	}
	ts.Cfg.SetupFromHome = true
	if c := do("192.168.1.50:1000", "192.168.1.2:9797", ""); c != 200 {
		t.Errorf("from home where the owner allows setting up from home: %d", c)
	}
	ts.Cfg.SetupFromHome = false
	if c := do("8.8.8.8:1000", "example.com", ""); c == 200 {
		t.Errorf("from outside with no code: %d", c)
	}
	if c := do("8.8.8.8:1000", "example.com", "WRONG"); c == 200 {
		t.Errorf("from outside with a wrong code: %d", c)
	}
	if _, err := ts.Auth.CreateUser(t.Context(), "kim", backupPassword, true); err != nil {
		t.Fatal(err)
	}
	if c := do("192.168.1.50:1000", "192.168.1.2:9797", ""); c != http.StatusUnauthorized {
		t.Errorf("with an account: %d", c)
	}
}

func TestNextBackupTime(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, loc) // a Monday
	if _, on := nextBackup(backupSchedule{Frequency: "off"}, time.Time{}, now); on {
		t.Error("off is off")
	}
	at, _ := nextBackup(backupSchedule{Frequency: "daily", Hour: 3}, time.Time{}, now)
	if at != time.Date(2026, 11, 3, 3, 0, 0, 0, loc) {
		t.Errorf("daily: %v", at)
	}
	at, _ = nextBackup(backupSchedule{Frequency: "daily", Hour: 3}, time.Date(2026, 10, 1, 3, 0, 0, 0, loc), now)
	if !at.Before(now) {
		t.Errorf("missed, so it's due at once: %v", at)
	}
	at, _ = nextBackup(backupSchedule{Frequency: "weekly", Hour: 12, Weekday: 5}, time.Time{}, now)
	if at != time.Date(2026, 11, 6, 12, 0, 0, 0, loc) {
		t.Errorf("weekly: %v", at)
	}
}

// A schedule that's due makes a backup by itself, and keeps only the newest.
func TestScheduledBackupRuns(t *testing.T) {
	b := newBackupServer(t)
	// On the server rather than in a package variable: the schedule's loop can outlive the test
	// by a moment, and putting a variable back under it was a data race.
	b.backup.soonest = 10 * time.Millisecond
	f := b.backupFolders()
	if err := os.MkdirAll(f.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	old := []string{"wms-backup-auto-20200101-000000.zip", "wms-backup-auto-20200102-000000.zip"}
	for _, name := range old {
		if err := os.WriteFile(filepath.Join(f.Dir(), name), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-30 * 24 * time.Hour)
		_ = os.Chtimes(filepath.Join(f.Dir(), name), at, at)
	}
	// Keeping fewer removes backups, so it takes more than the session.
	if w := b.call("PUT", "/api/backup/schedule", b.admin, map[string]any{"frequency": "daily", "hour": 3, "keep": 1, "parts": []string{"settings"}}); w.Code != 403 {
		t.Fatalf("kept fewer with no password: %d", w.Code)
	}
	if w := b.call("PUT", "/api/backup/schedule", b.admin, map[string]any{"frequency": "daily", "hour": 3, "keep": 1, "parts": []string{"settings"}, "current": backupPassword, "password": "short"}); w.Code != 400 {
		t.Fatalf("a short password: %d", w.Code)
	}
	if w := b.call("PUT", "/api/backup/schedule", b.admin, map[string]any{"frequency": "daily", "hour": 3, "keep": 1, "parts": []string{"settings"}, "current": backupPassword, "password": backupPassword}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// The key isn't the password, and neither is shown.
	if v := b.DB.GetSetting(t.Context(), backupKeySetting, ""); v == "" || strings.Contains(v, backupPassword) {
		t.Fatalf("the saved key: %q", v)
	}
	if w := b.call("GET", "/api/settings", b.admin, nil); strings.Contains(w.Body.String(), "memoryKiB") {
		t.Error("the key shows in the settings")
	}
	if err := b.DB.SetSetting(t.Context(), backupLastSetting, "1000"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	b.StartBackupSchedule(ctx)
	deadline := time.Now().Add(30 * time.Second)
	var found []backup.Info
	for time.Now().Before(deadline) {
		found = nil
		for _, i := range backup.List(f) {
			if i.Auto {
				found = append(found, i)
			}
		}
		if len(found) == 1 && !slices.Contains(old, found[0].Name) && found[0].Protected {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// By name, not by "2020" in it: a backup made at 16:20:20 is named ...-162020.zip.
	if len(found) != 1 || slices.Contains(old, found[0].Name) || !found[0].Protected {
		t.Fatalf("%+v", found)
	}
	// It opens with the password it was set up with.
	p, err := f.PathOf(found[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	in, err := backup.Inspect(t.Context(), p, backupPassword, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	in.Close()
}
