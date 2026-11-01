package server

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

func TestProfileNamesPermissions(t *testing.T) {
	ts := newTestServer(t)
	admin, adminTok := ts.user(t, "owner", auth.KindAdult, true, "")
	adult, adultTok := ts.user(t, "sam", auth.KindAdult, false, "")
	other, _ := ts.user(t, "alex", auth.KindAdult, false, "")
	kid, kidTok := ts.user(t, "pip", auth.KindKids, false, "")
	_ = admin
	url := func(id int64) string { return fmt.Sprintf("/api/users/%d/profile", id) }
	name := func(s string) map[string]any { return map[string]any{"displayName": s} }

	for _, c := range []struct {
		what   string
		tok    string
		id     int64
		body   map[string]any
		status int
	}{
		{"nobody signed in", "", adult.ID, name("x"), 401},
		{"your own profile name", adultTok, adult.ID, name("Dad"), 200},
		{"your own username", adultTok, adult.ID, map[string]any{"username": "sammy"}, 200},
		{"a name in use, in another case", adultTok, adult.ID, map[string]any{"username": "ALEX"}, 400},
		{"another adult's name", adultTok, other.ID, name("x"), 403},
		{"which name the corner shows, on someone else", adultTok, other.ID, map[string]any{"showUsername": true}, 403},
		{"an adult renames a kid they manage", adultTok, kid.ID, name("Pipsqueak"), 200},
		{"a kid chooses what the corner shows", kidTok, kid.ID, map[string]any{"showUsername": true}, 200},
		{"a kid renames themself", kidTok, kid.ID, name("Boss"), 403},
		{"a kid changes their username", kidTok, kid.ID, map[string]any{"username": "boss"}, 403},
		{"a kid changes someone else's", kidTok, adult.ID, map[string]any{"showUsername": true}, 403},
		{"the administrator renames anyone", adminTok, other.ID, name("Al"), 200},
		{"no such profile", adminTok, 9999, name("x"), 404},
	} {
		if w := ts.do(t, "PUT", url(c.id), c.tok, c.body); w.Code != c.status {
			t.Errorf("%s: got %d, want %d: %s", c.what, w.Code, c.status, w.Body)
		}
	}

	// Signing in still goes by the username, and the profile list has both.
	if _, _, err := ts.Auth.Login(t.Context(), "sammy", "correct-horse-1", "test", false); err != nil {
		t.Errorf("signing in under the new username: %v", err)
	}
	w := ts.do(t, "GET", "/api/profiles", "", nil)
	var got struct {
		Profiles []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range got.Profiles {
		found = found || (p.Name == "sammy" && p.DisplayName == "Dad")
	}
	if !found {
		t.Errorf("profiles: %s", w.Body)
	}
}

func TestCustomThemeIsForAdministrators(t *testing.T) {
	ts := newTestServer(t)
	_, adminTok := ts.user(t, "owner", auth.KindAdult, true, "")
	_, adultTok := ts.user(t, "sam", auth.KindAdult, false, "")
	colors := map[string]string{}
	for _, k := range []string{"bg", "bg-2", "bg-3", "bg-4", "fg", "fg-2", "fg-3", "accent", "accent-2", "accent-fg", "accent-text", "control-line", "danger", "ok", "warn"} {
		colors[k] = "#336699"
	}
	body := map[string]any{"name": "Made", "scheme": "dark", "colors": colors, "radius": 10}
	if w := ts.do(t, "POST", "/api/themes/custom", "", body); w.Code != 401 {
		t.Errorf("nobody signed in: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/themes/custom", adultTok, body); w.Code != 403 {
		t.Errorf("not an administrator: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/themes/custom", adminTok, body); w.Code != 201 {
		t.Errorf("administrator: %d %s", w.Code, w.Body)
	}
	colors["accent"] = "#000;}body{display:none"
	if w := ts.do(t, "POST", "/api/themes/custom", adminTok, body); w.Code != 400 {
		t.Errorf("something other than a colour: %d", w.Code)
	}
}

func TestNavDefaultReachesEveryone(t *testing.T) {
	ts := newTestServer(t)
	_, adminTok := ts.user(t, "owner", auth.KindAdult, true, "")
	_, adultTok := ts.user(t, "sam", auth.KindAdult, false, "")
	value := `{"order":["tv","home"],"grouped":true}`
	if w := ts.do(t, "PUT", "/api/settings", adultTok, map[string]string{"navDefault": value}); w.Code != 403 {
		t.Errorf("not an administrator: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/settings", adminTok, map[string]string{"navDefault": `{"style":"nope"}`}); w.Code != 400 {
		t.Errorf("a value it can't use: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/settings", adminTok, map[string]string{"navDefault": value}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	w := ts.do(t, "GET", "/api/prefs", adultTok, nil)
	var got struct {
		NavDefault string `json:"navDefault"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.NavDefault != value {
		t.Errorf("%v %s", err, w.Body)
	}
}
