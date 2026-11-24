// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package notify

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/db/dbtest"
)

func openDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := dbtest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	for _, name := range []string{"ann", "bob"} {
		if _, err := d.Exec(`INSERT INTO users(name, password_hash, is_admin, created_at) VALUES (?, '', 0, 0)`, name); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func TestSendListReadClear(t *testing.T) {
	ctx := context.Background()
	h := New(openDB(t), nil)
	var out []int64
	h.Out = func(uid int64, n Note) { out = append(out, uid) }

	if got := h.Send(ctx, []int64{1, 2, 2}, Note{Kind: Episodes, Title: "New episode of A", Link: "/show/1"}, "ep:1:9"); got != 2 {
		t.Fatalf("sent to %d, want 2 (a profile named twice gets it once)", got)
	}
	if got := h.Send(ctx, []int64{1, 2}, Note{Kind: Episodes, Title: "New episode of A"}, "ep:1:9"); got != 0 {
		t.Fatalf("the same thing was said again: %d", got)
	}
	h.Send(ctx, []int64{1}, Note{Kind: Recordings, Title: "Recorded: B", Link: "https://elsewhere.example/x"}, "")
	if len(out) != 3 {
		t.Fatalf("channels heard of %d, want 3", len(out))
	}

	list, err := h.List(ctx, 1, 0, 0)
	if err != nil || len(list) != 2 {
		t.Fatalf("list %v %v", list, err)
	}
	if list[0].Title != "Recorded: B" || list[0].Link != "" {
		t.Errorf("newest first, and a link out of the app dropped: %+v", list[0])
	}
	if n, latest, _ := h.Count(ctx, 1); n != 2 || latest != list[0].ID {
		t.Errorf("count %d latest %d", n, latest)
	}
	if err := h.MarkRead(ctx, 1, []int64{list[1].ID}); err != nil {
		t.Fatal(err)
	}
	if n, _, _ := h.Count(ctx, 1); n != 1 {
		t.Errorf("after reading one, %d unread", n)
	}
	// Someone else's id changes nothing.
	if err := h.MarkRead(ctx, 2, []int64{list[0].ID}); err != nil {
		t.Fatal(err)
	}
	if n, _, _ := h.Count(ctx, 1); n != 1 {
		t.Errorf("another profile marked it read")
	}
	if err := h.Clear(ctx, 2, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if n, _, _ := h.Count(ctx, 1); n != 1 {
		t.Errorf("another profile cleared it")
	}
	_ = h.MarkRead(ctx, 1, nil)
	if n, _, _ := h.Count(ctx, 1); n != 0 {
		t.Errorf("mark all read left %d", n)
	}
	_ = h.Clear(ctx, 1, 0)
	if list, _ := h.List(ctx, 1, 0, 0); len(list) != 0 {
		t.Errorf("clear left %d", len(list))
	}
	if list, _ := h.List(ctx, 2, 0, 0); len(list) != 1 {
		t.Errorf("clearing one profile's touched another's: %d", len(list))
	}
}

func TestChoices(t *testing.T) {
	ctx := context.Background()
	h := New(openDB(t), nil)
	if err := h.SetChoices(ctx, 1, []Choice{{Kind: Episodes, On: false}}); err != nil {
		t.Fatal(err)
	}
	if h.Send(ctx, []int64{1, 2}, Note{Kind: Episodes, Title: "x"}, "") != 1 {
		t.Error("a kind turned off was still sent")
	}
	if err := h.SetChoices(ctx, 1, []Choice{{Kind: "spam", On: true}}); err == nil {
		t.Error("an unknown kind was saved")
	}
	c := h.Choices(ctx, 1, false)
	if len(c) != 2 || c[0] != (Choice{Episodes, false}) || c[1] != (Choice{Recordings, true}) {
		t.Errorf("choices for someone who isn't an administrator: %+v", c)
	}
	if len(h.Choices(ctx, 1, true)) != len(Kinds) {
		t.Error("an administrator should see every kind")
	}
	_ = h.SetChoices(ctx, 1, []Choice{{Kind: Episodes, On: true}})
	if !h.Wants(ctx, 1, Episodes) {
		t.Error("turning a kind back on didn't")
	}
}

func TestPrune(t *testing.T) {
	ctx := context.Background()
	h := New(openDB(t), nil)
	for i := 0; i < keepPerUser+5; i++ {
		h.Send(ctx, []int64{1}, Note{Kind: Episodes, Title: "x"}, "")
	}
	list, _ := h.List(ctx, 1, 0, 100)
	var n int
	_ = h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = 1`).Scan(&n)
	if n != keepPerUser || len(list) != 100 {
		t.Errorf("kept %d, listed %d", n, len(list))
	}
	h.now = func() time.Time { return time.Now().Add((keepDays + 1) * 24 * time.Hour) }
	h.Send(ctx, []int64{1}, Note{Kind: Episodes, Title: "new"}, "")
	_ = h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = 1`).Scan(&n)
	if n != 1 {
		t.Errorf("old ones kept: %d", n)
	}
}

func TestCheckChannel(t *testing.T) {
	for _, c := range []Channel{
		{EmailOn: true},
		{EmailTo: "a@b.c\r\nBcc: x@y.z"},
		{WebhookURL: "ftp://x/y"},
		{WebhookURL: "https://user:pw@hooks.example/x"},
		{WebhookOn: true},
	} {
		if err := CheckChannel(c); !errors.Is(err, ErrBadChannel) {
			t.Errorf("%+v was allowed", c)
		}
	}
	if err := CheckChannel(Channel{EmailOn: true, EmailTo: "me@example.com", WebhookOn: true, WebhookURL: "http://192.168.1.5:8123/api/webhook/x"}); err != nil {
		t.Error(err)
	}
	for _, s := range []SMTP{
		{Host: "mail.example.com", Port: 25, Security: "none"},
		{Host: "mail.example.com", Port: 0, Security: "tls"},
		{Host: "", Port: 587, Security: "starttls"},
		{Host: "mail.example.com", Port: 587, Security: "starttls", From: "x\r\n@y"},
	} {
		if CheckSMTP(s) == nil {
			t.Errorf("%+v was allowed", s)
		}
	}
	if err := CheckSMTP(SMTP{Host: "127.0.0.1", Port: 25, Security: "none"}); err != nil {
		t.Error("a mail server on this machine needs no encryption:", err)
	}
}

func TestWebhookIsSigned(t *testing.T) {
	var got Payload
	var sig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sig = r.Header.Get("X-WittyMediaServer-Signature")
		m := hmac.New(sha256.New, []byte("s3cret"))
		m.Write(body)
		if sig != "sha256="+hex.EncodeToString(m.Sum(nil)) {
			w.WriteHeader(401)
			return
		}
		_ = json.Unmarshal(body, &got)
	}))
	defer srv.Close()
	s := &Sender{ServerName: func() string { return "Den" }}
	err := s.Deliver(context.Background(), Channel{WebhookOn: true, WebhookURL: srv.URL + "/hook", WebhookSecret: "s3cret"}, Note{Kind: Server, Title: "Hi", At: 5}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Server != "Den" || got.Title != "Hi" || got.Kind != Server {
		t.Errorf("payload %+v", got)
	}
	// Off and not a test: nothing goes.
	sig = ""
	_ = s.Deliver(context.Background(), Channel{WebhookURL: srv.URL}, Note{Title: "x"}, false)
	if sig != "" {
		t.Error("a channel that's off was used")
	}
}

func TestWebhookDoesNotFollowRedirects(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	s := &Sender{}
	if err := s.Deliver(context.Background(), Channel{WebhookOn: true, WebhookURL: srv.URL}, Note{Title: "x"}, false); err == nil || hit {
		t.Errorf("redirect followed: err=%v hit=%v", err, hit)
	}
}

// A mail server that takes one message, on this machine and without encryption, which is
// the one case CheckSMTP allows that.
func fakeSMTP(t *testing.T) (port int, got chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got = make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		say := func(s string) { _, _ = io.WriteString(c, s+"\r\n") }
		say("220 fake")
		var data strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				say("250 fake")
			case cmd == "DATA":
				say("354 go on")
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					data.WriteString(l)
				}
				say("250 ok")
				got <- data.String()
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, got
}

func TestEmail(t *testing.T) {
	port, got := fakeSMTP(t)
	s := &Sender{ServerName: func() string { return "Den" }, MailServer: func(context.Context) SMTP {
		return SMTP{Host: "127.0.0.1", Port: port, Security: "none", From: "wms@example.com"}
	}}
	err := s.Deliver(context.Background(), Channel{EmailOn: true, EmailTo: "me@example.com"}, Note{Kind: Episodes, Title: "New episode of Ünïcode", Body: "S1 E2"}, false)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-got:
		if !strings.Contains(msg, "To: <me@example.com>") || !strings.Contains(msg, "Subject: =?utf-8?q?") || !strings.Contains(msg, "S1 E2") {
			t.Errorf("message:\n%s", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no message arrived")
	}
}

func TestEnqueueNeverWaits(t *testing.T) {
	s := &Sender{}
	done := make(chan struct{})
	go func() {
		for i := 0; i < queueSize*3; i++ {
			s.Enqueue(1, Note{Title: "x"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue waited with nobody sending")
	}
}
