package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

func taskListOf(t *testing.T, ts *testServer, tok string) TaskList {
	t.Helper()
	w := ts.do(t, "GET", "/api/system/tasks", tok, nil)
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	var l TaskList
	if err := json.Unmarshal(w.Body.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestActivityRolesAndWhatEachSees(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "grownup", "adult", false, "")
	_, kid := ts.user(t, "kiddo", "kids", false, "")
	lib, err := ts.Store.CreateLibrary(context.Background(), "Films", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	h := ts.Tasks.Start(tasks.Spec{Type: tasks.TypeScan, Title: "Scanning Films", LibraryID: lib.ID, LibraryName: "Films", By: tasks.By{ID: 1, Name: "boss"}, Cancel: func() {}, Pausable: true})
	h.Item("A Made Up Film.mkv", "/secret/place/A Made Up Film.mkv")
	h.Problem("/secret/place/broken.mkv", "This file couldn't be added.")
	defer h.Finish(nil)

	// An administrator sees everything, with controls.
	l := taskListOf(t, ts, admin)
	if !l.Access.Visible || l.Access.ReadOnly || len(l.Tasks) != 1 || l.Tasks[0].Path == "" || !l.Tasks[0].CanCancel {
		t.Fatalf("admin: %+v", l)
	}

	// Others get nothing until the owner allows it, and the stream and history say no too.
	l = taskListOf(t, ts, adult)
	if l.Access.Visible || len(l.Tasks) != 0 {
		t.Fatalf("off for adults by default: %+v", l)
	}
	if w := ts.do(t, "GET", "/api/system/tasks/stream", adult, nil); w.Code != 403 {
		t.Errorf("stream while off: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/system/tasks/history", adult, nil); w.Code != 403 {
		t.Errorf("history while off: %d", w.Code)
	}

	// Adults only: the adult reads it with no paths or names, and the child still doesn't.
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"activityViewers": "adults"}); w.Code != 200 {
		t.Fatalf("settings: %d %s", w.Code, w.Body)
	}
	w := ts.do(t, "GET", "/api/system/tasks", adult, nil)
	body := w.Body.String()
	for _, leak := range []string{"secret", "Made Up", "broken", "boss"} {
		if strings.Contains(body, leak) {
			t.Errorf("an adult could read %q: %s", leak, body)
		}
	}
	l = taskListOf(t, ts, adult)
	if !l.Access.Visible || !l.Access.ReadOnly || len(l.Tasks) != 1 || l.Tasks[0].CanCancel || l.Tasks[0].ProblemCount != 1 {
		t.Fatalf("adult: %+v", l)
	}
	if taskListOf(t, ts, kid).Access.Visible {
		t.Error("adults only means adults")
	}
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"activityViewers": "everyone"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if !taskListOf(t, ts, kid).Access.Visible {
		t.Error("everyone means children too")
	}

	// Controls are for administrators alone, whatever the settings say.
	for _, action := range []string{"pause", "resume", "cancel", "retry"} {
		if w := ts.do(t, "POST", "/api/system/tasks/"+h.ID()+"/"+action, adult, nil); w.Code != 403 {
			t.Errorf("%s as an adult: %d", action, w.Code)
		}
	}
	if w := ts.do(t, "DELETE", "/api/system/tasks/history", adult, nil); w.Code != 403 {
		t.Errorf("clearing as an adult: %d", w.Code)
	}

	// The owner's switch ends it for everyone else, and hides it for admins.
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"activityEnabled": "0"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if taskListOf(t, ts, adult).Access.Visible || taskListOf(t, ts, admin).Access.Visible {
		t.Error("the switch turns it off")
	}
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"activityViewers": "cats"}); w.Code != 400 {
		t.Errorf("a bad value: %d", w.Code)
	}
}

func TestActivityHidesLibrariesAnAccountCantSee(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	ctx := context.Background()
	open, _ := ts.Store.CreateLibrary(ctx, "Open", "movies", []string{t.TempDir()})
	shut, _ := ts.Store.CreateLibrary(ctx, "Shut", "movies", []string{t.TempDir()})
	_, limited := ts.user(t, "limited", "adult", false, fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"libraries":[%d]}`, open.ID))
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"activityViewers": "adults"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	a := ts.Tasks.Start(tasks.Spec{Title: "Scanning Open", LibraryID: open.ID})
	b := ts.Tasks.Start(tasks.Spec{Title: "Scanning Shut", LibraryID: shut.ID})
	p := ts.Tasks.Start(tasks.Spec{Title: "Someone's transcode", Private: true, By: tasks.By{ID: 99, Name: "else"}})
	o := ts.Tasks.Start(tasks.Spec{Title: "A module's work", AdminOnly: true})
	defer func() { a.Finish(nil); b.Finish(nil); p.Finish(nil); o.Finish(nil) }()
	l := taskListOf(t, ts, limited)
	if len(l.Tasks) != 1 || l.Tasks[0].Title != "Scanning Open" {
		t.Fatalf("%+v", l.Tasks)
	}
	if l := taskListOf(t, ts, admin); len(l.Tasks) != 4 {
		t.Fatalf("admin: %+v", l.Tasks)
	}
}

func TestActivityScanCancelledThenRetried(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	dir := t.TempDir()
	for i := range 3 {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("Made Up Song %d.mp3", i)), []byte("not really"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib, err := ts.Store.CreateLibrary(context.Background(), "Songs", "music", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	// Holding the turn keeps the scan queued, so it can be cancelled in that state.
	release, err := ts.Scanner.HoldScans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if w := ts.do(t, "POST", fmt.Sprintf("/api/libraries/%d/scan", lib.ID), admin, nil); w.Code != 202 {
		t.Fatal(w.Code)
	}
	l := taskListOf(t, ts, admin)
	if len(l.Tasks) != 1 || l.Tasks[0].State != tasks.Queued || l.Tasks[0].By != "boss" || l.Tasks[0].LibraryID != lib.ID {
		t.Fatalf("%+v", l.Tasks)
	}
	id := l.Tasks[0].ID
	if w := ts.do(t, "POST", "/api/system/tasks/"+id+"/cancel", admin, nil); w.Code != 200 {
		t.Fatalf("cancel: %d %s", w.Code, w.Body)
	}
	waitFor(t, func() bool { return len(taskListOf(t, ts, admin).Tasks) == 0 })
	got := taskListOf(t, ts, admin).Recent[0]
	if got.State != tasks.Cancelled || !got.CanRetry {
		t.Fatalf("%+v", got)
	}
	ts.Tasks.Settle()
	if w := ts.do(t, "GET", "/api/system/tasks/history", admin, nil); !strings.Contains(w.Body.String(), `"canRetry":true`) {
		t.Fatalf("history: %s", w.Body)
	}

	release()
	if w := ts.do(t, "POST", "/api/system/tasks/"+id+"/retry", admin, nil); w.Code != 200 {
		t.Fatalf("retry: %d %s", w.Code, w.Body)
	}
	waitFor(t, func() bool {
		r := taskListOf(t, ts, admin).Recent
		return len(r) > 0 && r[0].ID != id && r[0].State == tasks.Done
	})
	done := taskListOf(t, ts, admin).Recent[0]
	if done.Total != 3 || done.Done != 3 || done.By != "boss" || done.Result == "" {
		t.Fatalf("%+v", done)
	}
	if w := ts.do(t, "POST", "/api/system/tasks/"+done.ID+"/retry", admin, nil); w.Code != 409 {
		t.Errorf("what worked isn't retried: %d", w.Code)
	}

	// Clearing the history forgets it all.
	ts.Tasks.Settle()
	if w := ts.do(t, "DELETE", "/api/system/tasks/history", admin, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if l := taskListOf(t, ts, admin); len(l.Recent) != 0 {
		t.Errorf("recent after clearing: %+v", l.Recent)
	}
	if w := ts.do(t, "GET", "/api/system/tasks/history", admin, nil); strings.Contains(w.Body.String(), `"id"`) {
		t.Errorf("history after clearing: %s", w.Body)
	}
}

func waitFor(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !f() {
		if time.Now().After(deadline) {
			t.Fatal("it never happened")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestActivityInterruptedTasksAreToldAbout(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	ts.Tasks.Start(tasks.Spec{Type: tasks.TypeBackup, Title: "Backing up"})
	ts.Tasks.Settle()
	// What the next start of the server does.
	ts.taskStore.MarkInterrupted(context.Background())
	w := ts.do(t, "GET", "/api/system/tasks/history", admin, nil)
	if !strings.Contains(w.Body.String(), "The server stopped") || !strings.Contains(w.Body.String(), `"state":"failed"`) {
		t.Fatalf("%s", w.Body)
	}
}

func TestActivityHistoryIsTrimmed(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"activityKeepCount": "10"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	for i := range 25 {
		ts.Tasks.Start(tasks.Spec{Title: fmt.Sprint("job ", i)}).Finish(nil)
	}
	ts.Tasks.Settle()
	w := ts.do(t, "GET", "/api/system/tasks/history?limit=200", admin, nil)
	var out struct {
		Tasks []tasks.Task `json:"tasks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tasks) != 10 {
		t.Fatalf("kept %d", len(out.Tasks))
	}
}

func TestActivityStream(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "grownup", "adult", false, "")
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"activityViewers": "adults"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	srv := httptest.NewServer(ts.h)
	defer srv.Close()
	open := func(tok string) (*bufio.Reader, func()) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/system/tasks/stream", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		ctx, cancel := context.WithCancel(context.Background())
		res, err := http.DefaultClient.Do(req.WithContext(ctx))
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatalf("%d %s", res.StatusCode, res.Header.Get("Content-Type"))
		}
		return bufio.NewReader(res.Body), func() { cancel(); res.Body.Close() }
	}
	next := func(r *bufio.Reader) (string, string) {
		t.Helper()
		var event, data string
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				event = line[7:]
			case strings.HasPrefix(line, "data: "):
				data = line[6:]
			case line == "" && event != "":
				return event, data
			}
		}
	}
	ra, closeA := open(admin)
	defer closeA()
	ru, closeU := open(adult)
	defer closeU()
	if e, _ := next(ra); e != "snapshot" {
		t.Fatal(e)
	}
	if e, _ := next(ru); e != "snapshot" {
		t.Fatal(e)
	}
	lib, err := ts.Store.CreateLibrary(context.Background(), "Films", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	h := ts.Tasks.Start(tasks.Spec{Title: "Scanning Films", LibraryID: lib.ID})
	h.Item("Made Up.mkv", "/secret/Made Up.mkv")
	h.Progress(1, 4)
	e, data := next(ra)
	if e != "update" || !strings.Contains(data, "/secret/") || !strings.Contains(data, `"active":1`) {
		t.Fatalf("admin: %s %s", e, data)
	}
	e, data = next(ru)
	if e != "update" || strings.Contains(data, "secret") || strings.Contains(data, "Made Up") || !strings.Contains(data, "Scanning Films") {
		t.Fatalf("adult: %s %s", e, data)
	}
	h.Finish(nil)
	for {
		_, data = next(ru)
		if strings.Contains(data, `"state":"done"`) {
			break
		}
	}
	if !strings.Contains(data, `"active":0`) {
		t.Fatalf("summary after finishing: %s", data)
	}
}
