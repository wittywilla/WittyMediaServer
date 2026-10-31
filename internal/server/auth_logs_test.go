package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// Sign-ins and pairing leave a record, and no password, token or code ever
// reaches the log, whatever level it is at.
func TestSignInLogsNoSecrets(t *testing.T) {
	ts, _ := logServer(t)
	ts.Logs.SetCategoryLevel("auth", slog.LevelDebug)
	ts.Logs.SetCategoryLevel("http", slog.LevelDebug)
	const password = "planted-good-password-7"
	const wrong = "planted-wrong-password-7"
	if _, err := ts.Auth.CreateUser(context.Background(), "kim", password, false); err != nil {
		t.Fatal(err)
	}

	if w := ts.do(t, "POST", "/api/login", "", map[string]string{"name": "kim", "password": wrong}); w.Code != 401 {
		t.Fatalf("wrong password: %d %s", w.Code, w.Body)
	}
	w := ts.do(t, "POST", "/api/login", "", map[string]string{"name": "kim", "password": password})
	if w.Code != 200 {
		t.Fatalf("sign-in: %d %s", w.Code, w.Body)
	}
	var signedIn struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &signedIn); err != nil || signedIn.Token == "" {
		t.Fatalf("no token: %v %s", err, w.Body)
	}

	// A wrong pairing code, and one the server handed out.
	const wrongCode = "PLANTED-CODE-77"
	ts.do(t, "POST", "/api/pair/redeem", "", map[string]string{"code": wrongCode, "device": "TV"})
	ts.do(t, "POST", "/api/pair/approve", signedIn.Token, map[string]any{"code": wrongCode})
	start := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "TV"})
	var started map[string]any
	_ = json.Unmarshal(start.Body.Bytes(), &started)

	logs := logFiles(t, ts)
	for _, want := range []string{"signed in", "sign-in failed", "code sign-in failed", "pairing failed"} {
		if !strings.Contains(logs, want) {
			t.Errorf("no record of %q in:\n%s", want, logs)
		}
	}
	secrets := []string{password, wrong, signedIn.Token, wrongCode, strings.ToLower(wrongCode), ts.SetupCode()}
	for _, k := range []string{"code", "deviceSecret", "secret"} {
		if s, ok := started[k].(string); ok && s != "" {
			secrets = append(secrets, s)
		}
	}
	for _, s := range secrets {
		if strings.Contains(logs, s) {
			t.Errorf("%q reached the log:\n%s", s, logs)
		}
	}
}

// A lock-out says so at Warn, which is what the auth area keeps by default.
func TestLockOutIsAWarning(t *testing.T) {
	ts, _ := logServer(t)
	if _, err := ts.Auth.CreateUser(context.Background(), "kim", "planted-good-password-7", false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		w := ts.do(t, "POST", "/api/login", "", map[string]string{"name": "kim", "password": "wrong"})
		if w.Code == 429 {
			break
		}
	}
	logs := logFiles(t, ts)
	if !strings.Contains(logs, "sign-in locked after repeated failures") {
		t.Fatalf("no lock-out record in:\n%s", logs)
	}
	if strings.Contains(logs, `"sign-in failed"`) {
		t.Errorf("each failure is Info, and auth starts at warn:\n%s", logs)
	}
}
