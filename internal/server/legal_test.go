package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/legal"
)

func TestLegalRoutesAreOpenAndSayWhatTheOwnerWrote(t *testing.T) {
	ts := newTestServer(t)
	ctx := t.Context()
	if err := ts.DB.SetSetting(ctx, "legalOwnerNotice", "House rules: <b>family only</b>"); err != nil {
		t.Fatal(err)
	}
	if err := ts.DB.SetSetting(ctx, "legalOwnerContact", "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	var short struct {
		AckVersion int                     `json:"ackVersion"`
		Notices    map[string]legal.Notice `json:"notices"`
		Page       string                  `json:"page"`
		Owner      struct{ Notice, Contact string }
	}
	w := ts.do(t, "GET", "/api/legal/notices", "", nil)
	if w.Code != 200 {
		t.Fatalf("notices: %d %s", w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &short); err != nil {
		t.Fatal(err)
	}
	if short.Notices["webResults"].Text == "" || short.Page != "/legal" || short.AckVersion < 1 {
		t.Errorf("notices: %+v", short)
	}
	if short.Owner.Notice != "House rules: <b>family only</b>" || short.Owner.Contact != "owner@example.com" {
		t.Errorf("owner's words came back as %+v", short.Owner)
	}
	var full struct {
		Sections []legal.Section `json:"sections"`
		Services []legal.Service `json:"services"`
	}
	w = ts.do(t, "GET", "/api/legal", "", nil)
	if w.Code != 200 {
		t.Fatalf("legal: %d %s", w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &full); err != nil {
		t.Fatal(err)
	}
	if len(full.Sections) < 10 || len(full.Services) < 10 {
		t.Errorf("legal: %d sections, %d services", len(full.Sections), len(full.Services))
	}
}

func TestOwnerNoticeLimits(t *testing.T) {
	for _, c := range []struct {
		k, v string
		ok   bool
	}{
		{"legalOwnerNotice", "Family only.\nAsk first.", true},
		{"legalOwnerNotice", strings.Repeat("é", legal.MaxOwnerNotice), true},
		{"legalOwnerNotice", strings.Repeat("é", legal.MaxOwnerNotice+1), false},
		{"legalOwnerContact", "owner@example.com", true},
		{"legalOwnerContact", "two\nlines", false},
		{"legalOwnerContact", strings.Repeat("x", legal.MaxOwnerContact+1), false},
	} {
		if err := checkSetting(c.k, c.v); (err == nil) != c.ok {
			t.Errorf("%s %.20q: %v", c.k, c.v, err)
		}
	}
}

func TestFirstRunAcknowledgement(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	status := func(tok string) bool {
		var st struct {
			LegalAck bool `json:"legalAck"`
		}
		w := ts.do(t, "GET", "/api/status", tok, nil)
		if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
			t.Fatal(err)
		}
		return st.LegalAck
	}
	if !status("") {
		t.Error("someone signed out has nothing to acknowledge")
	}
	if status(admin) {
		t.Error("an administrator who hasn't seen the notice should be asked")
	}
	if w := ts.do(t, "POST", "/api/legal/acknowledge", "", nil); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/legal/acknowledge", admin, nil); w.Code != 200 {
		t.Fatalf("acknowledge: %d %s", w.Code, w.Body)
	}
	if !status(admin) {
		t.Error("it should only ask once")
	}
	if ts.DB.GetSetting(t.Context(), "legalAckAt", "") == "" {
		t.Error("when it was acknowledged should be kept")
	}
}

func TestSetupRecordsTheTick(t *testing.T) {
	for _, tick := range []bool{true, false} {
		ts := newTestServer(t)
		w := ts.do(t, "POST", "/api/setup", "", map[string]any{"name": "owner", "password": "a-long-password", "acknowledgedLegal": tick})
		if w.Code != 200 {
			t.Fatalf("setup: %d %s", w.Code, w.Body)
		}
		if got := ts.legalAcked(t.Context()); got != tick {
			t.Errorf("ticked %v, recorded %v", tick, got)
		}
	}
}

// The standing rule for these words: no other media servers are named.
func TestLegalWordsNameNoOtherServers(t *testing.T) {
	word := regexp.MustCompile(`[A-Za-z]+`)
	for _, p := range []string{"../legal/notices.json", "../legal/legal.json", "../../docs/LEGAL-NOTICES-CHECKLIST.md"} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range word.FindAll(b, -1) {
			if sum := sha256.Sum256(bytes.ToLower(w)); otherServers[hex.EncodeToString(sum[:])] {
				t.Errorf("%s names %q", p, w)
			}
		}
	}
}
