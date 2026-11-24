// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittymediaserver/internal/notify"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

func notesOf(t *testing.T, ts *testServer, tok string) NoteList {
	t.Helper()
	w := ts.do(t, "GET", "/api/me/notifications", tok, nil)
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	var l NoteList
	if err := json.Unmarshal(w.Body.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestNotificationsBellAndChoices(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, kid := ts.user(t, "kiddo", "kids", false, "")
	if w := ts.do(t, "GET", "/api/me/notifications", "", nil); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}

	// A failed scan reaches administrators, and no one else.
	h := ts.Tasks.Start(tasks.Spec{Type: tasks.TypeScan, Title: "Scanning Films", LibraryName: "Films"})
	h.Fail("The folder couldn't be read.", "Check the drive is connected.", nil)
	ts.Tasks.Settle()
	l := notesOf(t, ts, admin)
	if len(l.Notes) != 1 || l.Unread != 1 || l.Notes[0].Kind != notify.Scans || !strings.Contains(l.Notes[0].Title, "Films") {
		t.Fatalf("admin: %+v", l)
	}
	if l := notesOf(t, ts, kid); len(l.Notes) != 0 {
		t.Fatalf("a child heard of a failed scan: %+v", l)
	}

	// Count, read, clear.
	var c NoteCount
	w := ts.do(t, "GET", "/api/me/notifications/count", admin, nil)
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	if c.Unread != 1 || c.Latest != l.Notes[0].ID {
		t.Errorf("count %+v", c)
	}
	if w := ts.do(t, "POST", "/api/me/notifications/read", kid, map[string]any{"ids": []int64{l.Notes[0].ID}}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if notesOf(t, ts, admin).Unread != 1 {
		t.Error("someone else marked it read")
	}
	if w := ts.do(t, "POST", "/api/me/notifications/read", admin, map[string]any{}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if notesOf(t, ts, admin).Unread != 0 {
		t.Error("mark all read")
	}
	if w := ts.do(t, "DELETE", fmt.Sprintf("/api/me/notifications/%d", l.Notes[0].ID), admin, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if len(notesOf(t, ts, admin).Notes) != 0 {
		t.Error("clear one")
	}

	// Choices: a child sees only its own kinds, and can't ask for an administrator's.
	var ch NoteChoices
	w = ts.do(t, "GET", "/api/me/notifications/choices", kid, nil)
	_ = json.Unmarshal(w.Body.Bytes(), &ch)
	if len(ch.Choices) != 2 {
		t.Errorf("child's kinds: %+v", ch)
	}
	if w := ts.do(t, "PUT", "/api/me/notifications/choices", kid, NoteChoices{Choices: []notify.Choice{{Kind: notify.Scans, On: true}}}); w.Code != 403 {
		t.Errorf("a child asked for scans: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/me/notifications/choices", admin, NoteChoices{Choices: []notify.Choice{{Kind: notify.Scans, On: false}}}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	h = ts.Tasks.Start(tasks.Spec{Type: tasks.TypeScan, Title: "Scanning Films", LibraryName: "Films"})
	h.Fail("Again.", "", nil)
	ts.Tasks.Settle()
	if len(notesOf(t, ts, admin).Notes) != 0 {
		t.Error("a kind turned off still arrived")
	}
}

func TestNotificationsNewEpisodesFollowLimits(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	tv := ts.library(t, "shows", root)
	other := ts.library(t, "movies", t.TempDir())
	if _, err := ts.DB.Exec(`INSERT INTO shows(id, library_id, path, title, sort_title, content_rating, added_at, updated_at) VALUES(5, ?, '/s', 'The Show', 'show', 'TV-MA', 1, 1)`, tv); err != nil {
		t.Fatal(err)
	}
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "bob", "adult", false, "")
	_, elsewhere := ts.user(t, "eve", "adult", false, fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"libraries":[%d]}`, other))
	_, kid := ts.user(t, "kid", "kids", false, "")
	_, guest := ts.user(t, "guest", "guest", false, "")

	show := int64(5)
	one, two, three := 1, 2, 3
	ep := func(n *int) {
		ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: filepath.Join(root, fmt.Sprintf("s1e%d.mkv", *n)), Title: fmt.Sprintf("Part %d", *n), ShowID: &show, Season: &one, Episode: n, ContentRating: "TV-MA"})
	}
	ep(&one)
	ctx := context.Background()
	ts.newEpisodesSince(ctx, tv) // the first look: nothing is news
	if len(notesOf(t, ts, admin).Notes) != 0 {
		t.Fatal("the first scan told people things")
	}
	ep(&two)
	ts.newEpisodesSince(ctx, tv)
	l := notesOf(t, ts, adult)
	if len(l.Notes) != 1 || l.Notes[0].Title != "New episode of The Show" || l.Notes[0].Body != "S1 E2 · Part 2" || l.Notes[0].Link != "/show/5" {
		t.Fatalf("adult: %+v", l)
	}
	for name, tok := range map[string]string{"another library only": elsewhere, "a child, for a TV-MA show": kid, "a guest": guest} {
		if l := notesOf(t, ts, tok); len(l.Notes) != 0 {
			t.Errorf("%s heard of it: %+v", name, l)
		}
	}
	ts.newEpisodesSince(ctx, tv)
	if len(notesOf(t, ts, adult).Notes) != 1 {
		t.Error("the same episodes were news twice")
	}
	ep(&three)
	four := 4
	ep(&four)
	ts.newEpisodesSince(ctx, tv)
	if l := notesOf(t, ts, admin); len(l.Notes) != 2 || l.Notes[0].Title != "2 new episodes of The Show" {
		t.Errorf("two at once: %+v", l)
	}
}

func TestNotificationsRecordingGoesToWhoeverSetItUp(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	bob, adult := ts.user(t, "bob", "adult", false, "")
	ts.noteRecording(livetv.Recording{ID: 7, UserID: bob.ID, Title: "The News", Channel: "One"}, "done", "")
	if l := notesOf(t, ts, adult); len(l.Notes) != 1 || l.Notes[0].Title != "Recorded: The News" || l.Notes[0].Body != "One" {
		t.Fatalf("bob: %+v", l)
	}
	if len(notesOf(t, ts, admin).Notes) != 0 {
		t.Error("an administrator heard of bob's recording")
	}
	ts.noteRecording(livetv.Recording{ID: 8, Title: "Nobody's"}, "failed", "the disk it records to is nearly full")
	if l := notesOf(t, ts, admin); len(l.Notes) != 1 || l.Notes[0].Kind != notify.Recordings {
		t.Errorf("admin: %+v", l)
	}
}

func TestNotificationChannelsAreAdminsAndSecretsStayIn(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "bob", "adult", false, "")
	for _, r := range [][2]string{{"GET", "/api/notifications/channels"}, {"PUT", "/api/notifications/channels"}, {"POST", "/api/notifications/channels/test"}} {
		if w := ts.do(t, r[0], r[1], adult, map[string]any{}); w.Code != 403 {
			t.Errorf("%s %s as an adult: %d", r[0], r[1], w.Code)
		}
	}
	// Nothing set up: the test says so, and nothing goes anywhere.
	if w := ts.do(t, "POST", "/api/notifications/channels/test", admin, nil); w.Code != 400 {
		t.Errorf("test with nothing set: %d %s", w.Code, w.Body)
	}
	bad := NoteChannelsUpdate{SMTP: &notify.SMTP{Host: "mail.example.com", Port: 25, Security: "none"}}
	if w := ts.do(t, "PUT", "/api/notifications/channels", admin, bad); w.Code != 400 {
		t.Errorf("a password in the clear across a network: %d", w.Code)
	}
	good := NoteChannelsUpdate{
		SMTP:    &notify.SMTP{Host: "mail.example.com", Security: "starttls", User: "me", Password: "hunter2-smtp", From: "wms@example.com"},
		Channel: notify.Channel{EmailTo: "me@example.com", WebhookURL: "https://hooks.example.com/x", WebhookSecret: "hook-secret-1"},
	}
	w := ts.do(t, "PUT", "/api/notifications/channels", admin, good)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, secret := range []string{"hunter2-smtp", "hook-secret-1"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Errorf("%q was read back", secret)
		}
	}
	var got NoteChannels
	_ = json.Unmarshal(ts.do(t, "GET", "/api/notifications/channels", admin, nil).Body.Bytes(), &got)
	if !got.SMTP.PasswordSet || !got.Channel.SecretSet || got.SMTP.Port != 587 || got.Channel.EmailOn || got.Channel.WebhookOn {
		t.Errorf("saved: %+v", got)
	}
	// Saving again with the secrets left out keeps them.
	good.SMTP.Password, good.Channel.WebhookSecret = "", ""
	good.Channel.WebhookOn = true
	ts.do(t, "PUT", "/api/notifications/channels", admin, good)
	_ = json.Unmarshal(ts.do(t, "GET", "/api/notifications/channels", admin, nil).Body.Bytes(), &got)
	if !got.SMTP.PasswordSet || !got.Channel.SecretSet || !got.Channel.WebhookOn {
		t.Errorf("kept: %+v", got)
	}
	// The general settings never show the password.
	if w := ts.do(t, "GET", "/api/settings", admin, nil); strings.Contains(w.Body.String(), "hunter2-smtp") {
		t.Error("the settings page reads the mail password back")
	}
}
