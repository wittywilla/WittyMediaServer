package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// recordableServer is recordingServer with one channel of each kind: a library
// channel, and tuner channels that are clear (tun-1), encrypted (tun-3), marked
// by the device as not to be recorded (tun-4) and marked so by the owner (tun-5).
func recordableServer(t *testing.T) (call func(method, path, token, body string) (int, string), admin, alice, kid, lib string) {
	t.Helper()
	call, admin, alice, _, s := recordingServer(t)
	for _, q := range []string{
		`UPDATE tuner_channels SET epg_id = 'clear.tv' WHERE id = 1`,
		`INSERT INTO tuner_channels(id, tuner_id, source_id, number, name, locator, enabled, encrypted, encrypted_why) VALUES(3, 1, 's3', '3', 'Locked', 'http://provider.invalid/3.ts', 1, 1, 'it needs a CableCARD')`,
		`INSERT INTO tuner_channels(id, tuner_id, source_id, number, name, locator, enabled, no_record, epg_id) VALUES(4, 1, 's4', '4', 'Copy never', 'http://provider.invalid/4.ts', 1, 'the device says copy never', 'never.tv')`,
		`INSERT INTO tuner_channels(id, tuner_id, source_id, number, name, locator, enabled, no_record_set) VALUES(5, 1, 's5', '5', 'Marked', 'http://provider.invalid/5.ts', 1, 1)`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now().Add(2 * time.Hour).Unix()
	for _, ch := range []string{"clear.tv", "never.tv"} {
		if _, err := s.DB.Exec(`INSERT INTO epg_programmes(channel, start, stop, title) VALUES(?, ?, ?, 'Findable news')`, ch, start, start+1800); err != nil {
			t.Fatal(err)
		}
	}
	code, body := call("POST", "/api/channels", admin, `{"name":"From the library","definition":{}}`)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var c struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &c); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, `"canRecord":true`) || !strings.Contains(body, `"canRecord":false`) {
		t.Errorf("a new library channel: %s", body)
	}
	kid = loginKid(t, s)
	return call, admin, alice, kid, fmt.Sprint("lib-", c.ID)
}

func loginKid(t *testing.T, s *Server) string {
	t.Helper()
	tok, _, err := s.Auth.Login(t.Context(), "kid", "", "t", true)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// canRecordIn finds every "canRecord" sent for a channel key in a list of
// objects that name their channel by "key" or "channel". Missing is an error:
// the flag is always sent, true or false.
func canRecordIn(t *testing.T, raw []byte, keyField string) map[string]bool {
	t.Helper()
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	out := map[string]bool{}
	for _, x := range list {
		k, _ := x[keyField].(string)
		v, ok := x["canRecord"].(bool)
		if !ok {
			t.Errorf("%s has no canRecord: %v", k, x)
			continue
		}
		out[k] = v
	}
	return out
}

var recordCases = []struct {
	name, key string
	can       bool
	code      int
	says      string
}{
	{"a library channel", "", false, 400, "library channels can't be recorded"},
	{"an encrypted tuner channel", "tun-3", false, 403, "Locked"},
	{"a channel the device marks", "tun-4", false, 403, "Copy never can't be recorded: the device says copy never"},
	{"a channel the owner marks", "tun-5", false, 403, "Marked can't be recorded"},
	{"a clear tuner channel", "tun-1", true, 200, ""},
}

// The guide's canRecord and what POST /api/recordings and /api/recording-rules
// take come from one rule, so a client that offers Record only on canRecord is
// never refused, and one that ignores it is.
func TestCanRecordIsWhatTheServerTakes(t *testing.T) {
	call, _, alice, _, lib := recordableServer(t)
	code, body := call("GET", "/api/guide?hours=3", alice, "")
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var g struct {
		Channels json.RawMessage `json:"channels"`
	}
	if err := json.Unmarshal([]byte(body), &g); err != nil {
		t.Fatal(err)
	}
	guide := canRecordIn(t, g.Channels, "key")
	start := time.Now().Add(3 * time.Hour).Unix()
	for i, c := range recordCases {
		key := c.key
		if key == "" {
			key = lib
		}
		t.Run(c.name, func(t *testing.T) {
			if got, ok := guide[key]; !ok || got != c.can {
				t.Errorf("guide canRecord for %s: %v (listed %v), want %v", key, got, ok, c.can)
			}
			code, body := call("POST", "/api/recordings", alice, schedule(key, start+int64(i)*3600, "One programme"))
			if code != c.code || !strings.Contains(body, c.says) {
				t.Errorf("POST /api/recordings: %d %s, want %d saying %q", code, body, c.code, c.says)
			}
			code, body = call("POST", "/api/recording-rules", alice, fmt.Sprintf(`{"channel":%q,"title":"Every airing %d"}`, key, i))
			if code != c.code || !strings.Contains(body, c.says) {
				t.Errorf("POST /api/recording-rules: %d %s, want %d saying %q", code, body, c.code, c.says)
			}
		})
	}
	// Only the clear channel's recording and series rule were made.
	_, body = call("GET", "/api/recordings", alice, "")
	if n := strings.Count(body, `"title":"One programme"`); n != 1 {
		t.Errorf("%d recordings made: %s", n, body)
	}
	_, body = call("GET", "/api/recording-rules", alice, "")
	if n := strings.Count(body, `"title":"Every airing`); n != 1 || !strings.Contains(body, "Every airing 4") {
		t.Errorf("%d series rules made: %s", n, body)
	}
}

// A profile that may not use recordings is told so, and sees no channel it could record.
func TestAProfileWithoutRecordingsSeesNoRecord(t *testing.T) {
	call, _, _, kid, lib := recordableServer(t)
	start := time.Now().Add(3 * time.Hour).Unix()
	for _, key := range []string{"tun-1", lib} {
		if code, body := call("POST", "/api/recordings", kid, schedule(key, start, "x")); code != 403 || !strings.Contains(body, "this profile cannot use recordings") {
			t.Errorf("%s: %d %s", key, code, body)
		}
		if code, body := call("POST", "/api/recording-rules", kid, fmt.Sprintf(`{"channel":%q,"title":"x"}`, key)); code != 403 || !strings.Contains(body, "this profile cannot use recordings") {
			t.Errorf("series %s: %d %s", key, code, body)
		}
	}
	code, body := call("GET", "/api/guide", kid, "")
	if code == 200 && strings.Contains(body, `"canRecord":true`) {
		t.Errorf("%s", body)
	}
}

// Every other answer that names a channel says canRecord the same way.
func TestCanRecordIsSentEverywhere(t *testing.T) {
	call, admin, alice, _, lib := recordableServer(t)

	// The mini guide and the channel list for dialling ask the guide with list=1.
	for _, path := range []string{"/api/guide?list=1&limit=20000", "/api/guide?kind=tuner", "/api/guide?kind=library", "/api/guide?channel=tun-1", "/api/guide?channel=" + lib} {
		code, body := call("GET", path, alice, "")
		var g struct {
			Channels json.RawMessage `json:"channels"`
		}
		if code != 200 || json.Unmarshal([]byte(body), &g) != nil {
			t.Fatalf("%s: %d %s", path, code, body)
		}
		for key, can := range canRecordIn(t, g.Channels, "key") {
			if can != (key == "tun-1") {
				t.Errorf("%s: %s canRecord %v", path, key, can)
			}
		}
	}

	// Searching the schedule.
	code, body := call("GET", "/api/guide/search?q=Findable", alice, "")
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	hits := canRecordIn(t, []byte(body), "channel")
	if !hits["tun-1"] {
		t.Errorf("search hit on a clear channel: %s", body)
	}
	if can, ok := hits["tun-4"]; !ok || can {
		t.Errorf("search hit on a channel marked not to be recorded: %s", body)
	}

	// The channel lists: library channels for everyone and an administrator, a tuner's for settings.
	for _, who := range []string{alice, admin} {
		_, body := call("GET", "/api/channels", who, "")
		for id, can := range canRecordIn(t, []byte(body), "name") {
			if can {
				t.Errorf("library channel %s can be recorded: %s", id, body)
			}
		}
	}
	_, body = call("GET", "/api/channels/"+strings.TrimPrefix(lib, "lib-"), admin, "")
	if !strings.Contains(body, `"canRecord":false`) {
		t.Errorf("one library channel: %s", body)
	}
	_, body = call("GET", "/api/tuners/1/channels", admin, "")
	tuned := canRecordIn(t, []byte(body), "name")
	if !tuned["Channel 1"] || tuned["Locked"] || tuned["Copy never"] || tuned["Marked"] {
		t.Errorf("a tuner's channels: %v", tuned)
	}

	// Reminders, which a client may offer Record beside.
	start := time.Now().Add(5 * time.Hour).Unix()
	for _, key := range []string{"tun-1", "tun-4", lib} {
		code, body := call("POST", "/api/tv/reminders", alice, fmt.Sprintf(`{"channel":%q,"title":"Remind me","start":%d,"end":%d}`, key, start, start+600))
		if code != 200 || !strings.Contains(body, fmt.Sprintf(`"canRecord":%v`, key == "tun-1")) {
			t.Errorf("reminder on %s: %d %s", key, code, body)
		}
	}
	_, body = call("GET", "/api/tv/reminders", alice, "")
	for key, can := range canRecordIn(t, []byte(body), "channel") {
		if can != (key == "tun-1") {
			t.Errorf("reminders: %s canRecord %v", key, can)
		}
	}
}
