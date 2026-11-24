// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/notify"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Notifications: the bell beside Activity in every app. What happens on the server (a new
// episode, a recording that finished, a scan that failed, an update) is kept for each profile
// that may know of it and hasn't turned that kind off. docs/NOTIFICATIONS.md has the rest.

func (s *Server) notifyRoutes(m *http.ServeMux) {
	s.wireNotify()
	a, ad := s.requireUser, s.requireAdmin
	m.Handle("GET /api/me/notifications", a(s.handleNotes))
	m.Handle("GET /api/me/notifications/count", a(s.handleNoteCount))
	m.Handle("POST /api/me/notifications/read", a(s.handleNotesRead))
	m.Handle("DELETE /api/me/notifications", a(s.handleNotesClear))
	m.Handle("DELETE /api/me/notifications/{id}", a(s.handleNoteDelete))
	m.Handle("GET /api/me/notifications/choices", a(s.handleNoteChoices))
	m.Handle("PUT /api/me/notifications/choices", a(s.handleNoteChoicesPut))
	m.Handle("GET /api/notifications/channels", ad(s.handleNoteChannels))
	m.Handle("PUT /api/notifications/channels", ad(s.handleNoteChannelsPut))
	m.Handle("POST /api/notifications/channels/test", ad(s.handleNoteChannelsTest))
}

const (
	smtpHostKey     = "notifySmtpHost"
	smtpPortKey     = "notifySmtpPort"
	smtpSecurityKey = "notifySmtpSecurity"
	smtpUserKey     = "notifySmtpUser"
	smtpFromKey     = "notifySmtpFrom"
	smtpPasswordKey = "notifySmtpPassword" // never in settingKeys: no settings page reads it back
)

func (s *Server) wireNotify() {
	if s.notes != nil {
		return
	}
	s.notes = notify.New(s.DB, s.Log)
	s.noteOut = &notify.Sender{DB: s.DB, Log: s.Log, MailServer: s.mailServer, ServerName: func() string { return s.ServerName(context.Background()) }}
	s.notes.Out = s.noteOut.Enqueue
	if s.TV != nil {
		s.TV.OnRecorded = s.noteRecording
	}
}

// startNotifications sends on what's queued for channels, and looks for updates to tell
// administrators of. The server's own start calls it once.
func (s *Server) startNotifications(ctx context.Context) {
	s.wireNotify()
	logging.Go("notification channels", func() { s.noteOut.Run(ctx) }, "cat", "notify")
	logging.Go("update notifications", func() {
		wait := 3 * time.Minute // after the update checks' own first look
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = 30 * time.Minute
			s.noteUpdates(ctx)
		}
	}, "cat", "notify")
}

func (s *Server) mailServer(ctx context.Context) notify.SMTP {
	port, _ := strconv.Atoi(s.DB.GetSetting(ctx, smtpPortKey, "587"))
	pw := s.DB.GetSetting(ctx, smtpPasswordKey, "")
	return notify.SMTP{Host: s.DB.GetSetting(ctx, smtpHostKey, ""), Port: port, Security: s.DB.GetSetting(ctx, smtpSecurityKey, "starttls"),
		User: s.DB.GetSetting(ctx, smtpUserKey, ""), From: s.DB.GetSetting(ctx, smtpFromKey, ""), Password: pw, PasswordSet: pw != ""}
}

// ---------------------------------------------------------------- who hears of what

// noteUsers are the profiles that may be told things: everyone but guests, whose profile is
// for anyone passing through.
func (s *Server) noteUsers(ctx context.Context) []auth.User {
	if s.Auth == nil {
		return nil
	}
	list, err := s.Auth.Users(ctx)
	if err != nil {
		s.Log.Warn("could not list profiles for a notification", "cat", "notify", "err", err)
		return nil
	}
	out := list[:0]
	for _, u := range list {
		if u.Kind != auth.KindGuest {
			out = append(out, u)
		}
	}
	return out
}

func (s *Server) noteAdmins(ctx context.Context) []int64 {
	var ids []int64
	for _, u := range s.noteUsers(ctx) {
		if u.IsAdmin {
			ids = append(ids, u.ID)
		}
	}
	return ids
}

// taskEnds is the activity history's store, which also tells administrators of a failed scan.
type taskEnds struct {
	*tasks.SQLStore
	s *Server
}

func (t taskEnds) End(task tasks.Task) {
	t.SQLStore.End(task)
	if task.Type != tasks.TypeScan || task.State != tasks.Failed || t.s.notes == nil {
		return
	}
	ctx := context.Background()
	title := i18n.T("A library scan failed")
	if task.LibraryName != "" {
		title = i18n.F("The scan of {libraryName} failed", "libraryName", task.LibraryName)
	}
	body := strings.TrimSpace(task.Error + " " + task.Fix)
	t.s.notes.Send(ctx, t.s.noteAdmins(ctx), notify.Note{Kind: notify.Scans, Title: title, Body: body, Link: "/settings/libraries"}, "scan:"+task.ID)
}

// noteRecording tells whoever set a recording up (or, for one nobody owns now, the
// administrators) that it has finished or failed.
func (s *Server) noteRecording(r livetv.Recording, status, msg string) {
	ctx := context.Background()
	to := []int64{r.UserID}
	if r.UserID == 0 {
		to = s.noteAdmins(ctx)
	} else if u, err := s.Auth.User(ctx, r.UserID); err != nil || u == nil {
		to = s.noteAdmins(ctx)
	}
	n := notify.Note{Kind: notify.Recordings, Link: "/tv/recordings"}
	if status == "done" {
		n.Title = "Recorded: " + r.Title
		n.Body = strings.Trim(r.Channel+" · "+r.Sub, " ·")
	} else {
		n.Title = "Recording failed: " + r.Title
		n.Body = msg
	}
	s.notes.Send(ctx, to, n, "rec:"+strconv.FormatInt(r.ID, 10)+":"+status)
}

// noteNewEpisodes tells each profile that may open a show of the episodes a scan found for it.
// The first scan of a library finds everything, and says nothing: that isn't news.
func (s *Server) noteNewEpisodes(libraryID int64) {
	if s.notes == nil {
		return
	}
	logging.Go("new episode notifications", func() { s.newEpisodesSince(context.Background(), libraryID) }, "cat", "notify")
}

// newEpisodesSince is noteNewEpisodes's work: what's been added since the library was last
// looked at, and where it's been looked at to now.
func (s *Server) newEpisodesSince(ctx context.Context, libraryID int64) {
	key := "notifySeen." + strconv.FormatInt(libraryID, 10)
	var newest int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM items WHERE library_id = ?`, libraryID).Scan(&newest); err != nil {
		return
	}
	was := s.DB.GetSetting(ctx, key, "")
	if err := s.DB.SetSetting(ctx, key, strconv.FormatInt(newest, 10)); err != nil {
		return
	}
	from, err := strconv.ParseInt(was, 10, 64)
	if err != nil || newest <= from {
		return
	}
	s.sendNewEpisodes(ctx, libraryID, from)
}

type newEpisodes struct {
	show          int64
	count         int
	last          int64
	season, epNum int64
	title         string
}

func (s *Server) sendNewEpisodes(ctx context.Context, libraryID, after int64) {
	rows, err := s.DB.QueryContext(ctx, `SELECT show_id, COUNT(*), MAX(id), COALESCE(MAX(season), 0), COALESCE(MAX(episode), 0), MAX(title)
		FROM items WHERE library_id = ? AND id > ? AND show_id IS NOT NULL AND missing = 0 GROUP BY show_id ORDER BY MAX(id) LIMIT 50`, libraryID, after)
	if err != nil {
		s.Log.Warn("could not look for new episodes", "cat", "notify", "err", err)
		return
	}
	var found []newEpisodes
	for rows.Next() {
		var e newEpisodes
		if rows.Scan(&e.show, &e.count, &e.last, &e.season, &e.epNum, &e.title) == nil {
			found = append(found, e)
		}
	}
	rows.Close()
	if len(found) == 0 {
		return
	}
	users := s.noteUsers(ctx)
	for _, e := range found {
		sh, err := s.Store.ShowFacts(ctx, e.show)
		if err != nil {
			continue
		}
		var to []int64
		for i := range users {
			a := s.accessOf(ctx, &users[i])
			if a == nil || (a.AllowsLibrary(libraryID) && a.AllowsShow(sh)) {
				to = append(to, users[i].ID)
			}
		}
		n := notify.Note{Kind: notify.Episodes, Link: "/show/" + strconv.FormatInt(e.show, 10)}
		if e.count == 1 {
			n.Title = i18n.F("New episode of {title}", "title", sh.Title)
			if e.season > 0 && e.epNum > 0 {
				n.Body = fmt.Sprintf("S%d E%d · %s", e.season, e.epNum, e.title)
			} else {
				n.Body = e.title
			}
		} else {
			n.Title = fmt.Sprintf("%d new episodes of %s", e.count, sh.Title)
		}
		s.notes.Send(ctx, to, n, "ep:"+strconv.FormatInt(e.show, 10)+":"+strconv.FormatInt(e.last, 10))
	}
}

// noteUpdates tells administrators of a new server version and of modules with updates, once
// for each version found.
func (s *Server) noteUpdates(ctx context.Context) {
	if s.notes == nil {
		return
	}
	admins := s.noteAdmins(ctx)
	if len(admins) == 0 {
		return
	}
	if s.Update != nil {
		if v := s.Update.Available(); v != "" {
			s.notes.Send(ctx, admins, notify.Note{Kind: notify.Server, Title: "WittyMediaServer " + v + " is out",
				Body: i18n.F("This server runs {current}. About has what's new and the update.", "current", s.Update.Current), Link: "/settings/about#updates"}, "server:"+v)
		}
	}
	if s.Modules != nil {
		var names, keys []string
		for _, st := range s.Modules.Report().Modules {
			if !st.Installed || st.Latest == nil || st.Pin != "" || st.Latest.Blocked != "" {
				continue
			}
			names = append(names, st.Name+" "+st.Latest.Version)
			keys = append(keys, st.ID+"@"+st.Latest.Version)
		}
		if len(names) > 0 {
			sort.Strings(names)
			sort.Strings(keys)
			title := i18n.F("An update for {module}", "module", strings.TrimSpace(strings.SplitN(names[0], " ", 2)[0]))
			if len(names) > 1 {
				title = i18n.N("Updates for {count} module", "Updates for {count} modules", len(names))
			}
			s.notes.Send(ctx, admins, notify.Note{Kind: notify.Modules, Title: title, Body: strings.Join(names, ", "), Link: "/settings/modules"}, "modules:"+strings.Join(keys, ","))
		}
	}
}

// ---------------------------------------------------------------- each profile's bell

// NoteList is a page of a profile's notifications.
type NoteList struct {
	Notes  []notify.Note `json:"notes"`
	Unread int           `json:"unread"`
	Latest int64         `json:"latest"`
}

// NoteCount is all a bell asks for while it waits.
type NoteCount struct {
	Unread int   `json:"unread"`
	Latest int64 `json:"latest"`
}

// GET /api/me/notifications?before=&limit=
func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.notes.List(r.Context(), u.ID, before, limit)
	if err != nil {
		writeErr(w, 500, "could not read the notifications")
		return
	}
	unread, latest, _ := s.notes.Count(r.Context(), u.ID)
	writeJSON(w, 200, NoteList{Notes: list, Unread: unread, Latest: latest})
}

// GET /api/me/notifications/count
func (s *Server) handleNoteCount(w http.ResponseWriter, r *http.Request) {
	unread, latest, err := s.notes.Count(r.Context(), userFrom(r).ID)
	if err != nil {
		writeErr(w, 500, "could not read the notifications")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, NoteCount{Unread: unread, Latest: latest})
}

// POST /api/me/notifications/read {ids?: [..]}: none named marks them all.
func (s *Server) handleNotesRead(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := readJSONUpTo(r, &in, 16<<10); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	u := userFrom(r)
	if err := s.notes.MarkRead(r.Context(), u.ID, in.IDs); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	unread, latest, _ := s.notes.Count(r.Context(), u.ID)
	writeJSON(w, 200, NoteCount{Unread: unread, Latest: latest})
}

// DELETE /api/me/notifications
func (s *Server) handleNotesClear(w http.ResponseWriter, r *http.Request) {
	if err := s.notes.Clear(r.Context(), userFrom(r).ID, 0); err != nil {
		writeErr(w, 500, "could not clear the notifications")
		return
	}
	writeJSON(w, 200, NoteCount{})
}

// DELETE /api/me/notifications/{id}
func (s *Server) handleNoteDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "bad id")
		return
	}
	u := userFrom(r)
	if err := s.notes.Clear(r.Context(), u.ID, id); err != nil {
		writeErr(w, 500, "could not remove the notification")
		return
	}
	unread, latest, _ := s.notes.Count(r.Context(), u.ID)
	writeJSON(w, 200, NoteCount{Unread: unread, Latest: latest})
}

// NoteChoices are the kinds a profile gets.
type NoteChoices struct {
	Choices []notify.Choice `json:"choices"`
}

// GET /api/me/notifications/choices
func (s *Server) handleNoteChoices(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	writeJSON(w, 200, NoteChoices{Choices: s.notes.Choices(r.Context(), u.ID, u.IsAdmin)})
}

// PUT /api/me/notifications/choices {choices: [{kind, on}]}
func (s *Server) handleNoteChoicesPut(w http.ResponseWriter, r *http.Request) {
	var in NoteChoices
	if err := readJSONUpTo(r, &in, 8<<10); err != nil || len(in.Choices) > len(notify.Kinds) {
		writeErr(w, 400, "bad request")
		return
	}
	u := userFrom(r)
	for _, c := range in.Choices {
		if notify.AdminOnly(c.Kind) && !u.IsAdmin {
			writeErr(w, 403, "only administrators get "+string(c.Kind))
			return
		}
	}
	if err := s.notes.SetChoices(r.Context(), u.ID, in.Choices); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, NoteChoices{Choices: s.notes.Choices(r.Context(), u.ID, u.IsAdmin)})
}

// ---------------------------------------------------------------- an administrator's channels

// NoteChannels are the server's mail server and the asking administrator's own channels.
// Neither password nor secret is ever read back: they say only whether one is set.
type NoteChannels struct {
	SMTP    notify.SMTP    `json:"smtp"`
	Channel notify.Channel `json:"channel"`
}

func (s *Server) noteChannels(ctx context.Context, uid int64) (NoteChannels, error) {
	m := s.mailServer(ctx)
	m.Password = ""
	c, err := s.noteOut.Channel(ctx, uid)
	c.WebhookSecret = ""
	return NoteChannels{SMTP: m, Channel: c}, err
}

// GET /api/notifications/channels
func (s *Server) handleNoteChannels(w http.ResponseWriter, r *http.Request) {
	out, err := s.noteChannels(r.Context(), userFrom(r).ID)
	if err != nil {
		writeErr(w, 500, "could not read the channels")
		return
	}
	writeJSON(w, 200, out)
}

// NoteChannelsUpdate changes channels. SMTP, when given, replaces the mail server's settings
// (an empty password keeps the one there unless ClearPassword); the secret is the same.
type NoteChannelsUpdate struct {
	SMTP          *notify.SMTP   `json:"smtp,omitempty"`
	ClearPassword bool           `json:"clearPassword,omitempty"`
	Channel       notify.Channel `json:"channel"`
	ClearSecret   bool           `json:"clearSecret,omitempty"`
}

// PUT /api/notifications/channels
func (s *Server) handleNoteChannelsPut(w http.ResponseWriter, r *http.Request) {
	var in NoteChannelsUpdate
	if err := readJSONUpTo(r, &in, 16<<10); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	ctx := r.Context()
	u := userFrom(r)
	if in.SMTP != nil {
		m := *in.SMTP
		m.Host = strings.TrimSpace(m.Host)
		if m.Security == "" {
			m.Security = "starttls"
		}
		if m.Port == 0 {
			m.Port = map[string]int{"tls": 465, "none": 25}[m.Security]
			if m.Port == 0 {
				m.Port = 587
			}
		}
		if m.Host != "" { // empty: no mail server
			if err := notify.CheckSMTP(m); err != nil {
				writeErr(w, 400, strings.TrimPrefix(err.Error(), notify.ErrBadChannel.Error()+": "))
				return
			}
		}
		for k, v := range map[string]string{smtpHostKey: m.Host, smtpPortKey: strconv.Itoa(m.Port), smtpSecurityKey: m.Security, smtpUserKey: m.User, smtpFromKey: m.From} {
			if err := s.DB.SetSetting(ctx, k, v); err != nil {
				writeErr(w, 500, "could not save the mail server")
				return
			}
		}
		if m.Password != "" || in.ClearPassword {
			if err := s.DB.SetSetting(ctx, smtpPasswordKey, m.Password); err != nil {
				writeErr(w, 500, "could not save the mail server")
				return
			}
		}
		s.Log.InfoContext(ctx, "the mail server for notifications was changed", "cat", "notify", "by", u.Name, "host", m.Host)
	}
	if err := s.noteOut.SetChannel(ctx, u.ID, in.Channel, !in.ClearSecret); err != nil {
		if errors.Is(err, notify.ErrBadChannel) {
			writeErr(w, 400, strings.TrimPrefix(err.Error(), notify.ErrBadChannel.Error()+": "))
			return
		}
		writeErr(w, 500, "could not save the channels")
		return
	}
	s.Log.InfoContext(ctx, "notification channels changed", "cat", "notify", "by", u.Name, "email", in.Channel.EmailOn, "webhook", in.Channel.WebhookOn)
	out, _ := s.noteChannels(ctx, u.ID)
	writeJSON(w, 200, out)
}

// POST /api/notifications/channels/test: sends a test through each channel that's filled in,
// on or not, and says what happened.
func (s *Server) handleNoteChannelsTest(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	c, err := s.noteOut.Channel(r.Context(), u.ID)
	if err != nil {
		writeErr(w, 500, "could not read the channels")
		return
	}
	if c.EmailTo == "" && c.WebhookURL == "" {
		writeErr(w, 400, "save an email address or a webhook first")
		return
	}
	n := notify.Note{Kind: notify.Server, Title: i18n.F("A test from {serverName}", "serverName", s.ServerName(r.Context())), Body: "Notifications reach you here.", At: time.Now().Unix()}
	if err := s.noteOut.Deliver(r.Context(), c, n, true); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
