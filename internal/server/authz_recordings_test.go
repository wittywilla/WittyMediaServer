package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

func recordingServer(t *testing.T) (call func(method, path, token, body string) (int, string), admin, alice, bob string, s *Server) {
	t.Helper()
	h, admin, _, s := tvServer(t)
	ctx := context.Background()
	login := func(name string) string {
		if _, err := s.Auth.CreateProfile(ctx, name, "correct horse", auth.KindAdult, false, ""); err != nil {
			t.Fatal(err)
		}
		tok, _, err := s.Auth.Login(ctx, name, "correct horse", "t", false)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	alice, bob = login("alice"), login("bob")
	if _, err := s.DB.Exec(`INSERT INTO tuners(id, driver, name, options, created_at) VALUES(1, 'stream', 'Test', '{"url":"http://provider.invalid/live/USERNAME/PASSWORD/1.ts"}', 1)`); err != nil {
		t.Fatal(err)
	}
	for id, enabled := range map[int]int{1: 1, 2: 0} {
		if _, err := s.DB.Exec(`INSERT INTO tuner_channels(id, tuner_id, source_id, number, name, locator, enabled) VALUES(?, 1, ?, ?, ?, 'http://provider.invalid/live/USERNAME/PASSWORD/1.ts', ?)`, id, fmt.Sprint("s", id), fmt.Sprint(id), fmt.Sprint("Channel ", id), enabled); err != nil {
			t.Fatal(err)
		}
	}
	return func(method, path, token, body string) (int, string) {
		w := tvCall(h, method, path, token, body)
		return w.Code, w.Body.String()
	}, admin, alice, bob, s
}

func schedule(key string, start int64, title string) string {
	return fmt.Sprintf(`{"channel":%q,"title":%q,"start":%d,"stop":%d}`, key, title, start, start+1800)
}

// One person's recordings and rules are theirs: someone else can't see, stop or remove them.
func TestRecordingsBelongToWhoeverSetThemUp(t *testing.T) {
	call, admin, alice, bob, _ := recordingServer(t)
	start := time.Now().Add(6 * time.Hour).Unix()
	code, body := call("POST", "/api/recordings", alice, schedule("tun-1", start, "Alice private programme"))
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var rec struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &rec); err != nil {
		t.Fatal(err)
	}
	if _, body := call("GET", "/api/recordings", bob, ""); strings.Contains(body, "Alice private") {
		t.Errorf("another adult sees it: %s", body)
	}
	if _, body := call("GET", "/api/recordings", alice, ""); !strings.Contains(body, "Alice private") {
		t.Errorf("its owner doesn't: %s", body)
	}
	if _, body := call("GET", "/api/recordings", admin, ""); !strings.Contains(body, "Alice private") {
		t.Errorf("an administrator doesn't: %s", body)
	}
	if code, _ := call("DELETE", fmt.Sprintf("/api/recordings/%d", rec.ID), bob, ""); code != 404 {
		t.Errorf("another adult cancelled it: %d", code)
	}
	if _, body := call("GET", "/api/recordings", alice, ""); !strings.Contains(body, "Alice private") {
		t.Errorf("it's gone: %s", body)
	}

	code, body = call("POST", "/api/recording-rules", alice, `{"channel":"tun-1","title":"Alice series"}`)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var rule struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &rule); err != nil {
		t.Fatal(err)
	}
	if _, body := call("GET", "/api/recording-rules", bob, ""); strings.Contains(body, "Alice series") {
		t.Errorf("another adult sees the rule: %s", body)
	}
	if code, _ := call("DELETE", fmt.Sprintf("/api/recording-rules/%d", rule.ID), bob, ""); code != 404 {
		t.Errorf("another adult removed the rule: %d", code)
	}
	if code, _ := call("DELETE", fmt.Sprintf("/api/recording-rules/%d", rule.ID), alice, ""); code != 200 {
		t.Errorf("its owner couldn't remove it: %d", code)
	}
	if code, _ := call("DELETE", fmt.Sprintf("/api/recordings/%d", rec.ID), admin, ""); code != 200 {
		t.Errorf("an administrator couldn't cancel it: %d", code)
	}
}

func TestRecordingATurnedOffChannelIsRefused(t *testing.T) {
	call, _, alice, _, _ := recordingServer(t)
	start := time.Now().Add(6 * time.Hour).Unix()
	code, body := call("POST", "/api/recordings", alice, schedule("tun-2", start, "x"))
	if code != 404 || strings.Contains(body, "Channel 2") {
		t.Errorf("%d %s", code, body)
	}
	code, body = call("POST", "/api/recording-rules", alice, `{"channel":"tun-2","title":"x"}`)
	if code != 404 || strings.Contains(body, "Channel 2") {
		t.Errorf("%d %s", code, body)
	}
}

func TestRecordingsPerAccountAreLimited(t *testing.T) {
	call, admin, alice, _, _ := recordingServer(t)
	start := time.Now().Add(6 * time.Hour).Unix()
	var last int
	var made int
	for i := range maxPendingRecordings + 2 {
		c, body := call("POST", "/api/recordings", alice, schedule("tun-1", start+int64(i)*120, fmt.Sprint("p", i)))
		last = c
		if c == 200 {
			made++
		} else if c != 409 {
			t.Fatalf("%d %s", c, body)
		}
	}
	if last != 409 || made != maxPendingRecordings {
		t.Errorf("made %d, last answer %d", made, last)
	}
	// Starting at much the same time on one channel is the same programme.
	if c, _ := call("POST", "/api/recordings", admin, schedule("tun-1", start+1, "again")); c != 400 {
		t.Errorf("a start a second from another: %d", c)
	}
	for i := range maxRecordingRules + 1 {
		c, body := call("POST", "/api/recording-rules", alice, fmt.Sprintf(`{"channel":"tun-1","title":"r%d"}`, i))
		switch {
		case i < maxRecordingRules && c != 200:
			t.Fatalf("%d %s", c, body)
		case i == maxRecordingRules && c != 409:
			t.Errorf("rule over the limit: %d", c)
		}
	}
}

// A stream's address, which can hold a login, isn't told to whoever tunes it.
func TestTuningErrorsDontGiveAwayTheAddress(t *testing.T) {
	call, _, alice, _, s := recordingServer(t)
	code, body := call("GET", "/api/tv/tun-1/tune", alice, "")
	if code < 400 {
		t.Fatalf("%d %s", code, body)
	}
	if strings.Contains(body, "USERNAME") || strings.Contains(body, "provider.invalid") {
		t.Errorf("%d %s", code, body)
	}
	// The same words for a recording that couldn't start, which is also listed to viewers.
	folder := t.TempDir()
	s.TV.Folder = func() string { return folder }
	now := time.Now().Unix()
	if code, body := call("POST", "/api/recordings", alice, schedule("tun-1", now, "Soon")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		_, body = call("GET", "/api/recordings", alice, "")
		if strings.Contains(body, `"failed"`) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(body, `"failed"`) {
		t.Fatalf("it never failed: %s", body)
	}
	if strings.Contains(body, "USERNAME") || strings.Contains(body, "provider.invalid") {
		t.Errorf("%s", body)
	}
}

// A new account that gets a removed one's number doesn't get its recordings.
func TestRecordingsDontPassToANewAccount(t *testing.T) {
	call, admin, alice, _, s := recordingServer(t)
	start := time.Now().Add(6 * time.Hour).Unix()
	if code, body := call("POST", "/api/recordings", alice, schedule("tun-1", start, "Alice private")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	u, err := s.Auth.Resolve(context.Background(), alice)
	if err != nil {
		t.Fatal(err)
	}
	if code, body := call("DELETE", fmt.Sprintf("/api/users/%d", u.ID), admin, ""); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var owner *int64
	if err := s.DB.QueryRow(`SELECT user_id FROM recordings`).Scan(&owner); err != nil || owner != nil {
		t.Errorf("%v %v", owner, err)
	}
}
