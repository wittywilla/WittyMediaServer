package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wittywilla/wittymediaserver/internal/legal"
)

// What the project says about itself (internal/legal), and what a server's owner
// adds for the people using their server. Open to anyone: a shared link and an app
// with no account yet need the same words, and nothing here is private. The owner's
// form says so before they write anything.

type ownerNotice struct {
	Notice  string `json:"notice"`
	Contact string `json:"contact"`
}

func (s *Server) ownerNotice(ctx context.Context) ownerNotice {
	return ownerNotice{Notice: s.DB.GetSetting(ctx, "legalOwnerNotice", ""), Contact: s.DB.GetSetting(ctx, "legalOwnerContact", "")}
}

// GET /api/legal/notices: the short notices, for any client to show beside content.
func (s *Server) handleLegalNotices(w http.ResponseWriter, r *http.Request) {
	n := legal.Short()
	writeJSON(w, 200, map[string]any{
		"version": n.Version, "ackVersion": n.AckVersion, "linkText": n.LinkText, "ack": n.Ack, "notices": n.Notices,
		"page": "/legal", "owner": s.ownerNotice(r.Context()),
	})
}

// GET /api/legal: the whole Legal page.
func (s *Server) handleLegal(w http.ResponseWriter, r *http.Request) {
	t := legal.Full()
	writeJSON(w, 200, map[string]any{"updated": t.Updated, "sections": t.Sections, "services": t.Services, "page": "/legal", "owner": s.ownerNotice(r.Context())})
}

// POST /api/legal/acknowledge: the administrator read the first-run notice. Kept on
// this server and sent nowhere, so it doesn't ask again.
func (s *Server) handleLegalAcknowledge(w http.ResponseWriter, r *http.Request) {
	if err := s.recordLegalAck(r.Context()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) recordLegalAck(ctx context.Context) error {
	if err := s.DB.SetSetting(ctx, "legalAck", strconv.Itoa(legal.Short().AckVersion)); err != nil {
		return err
	}
	return s.DB.SetSetting(ctx, "legalAckAt", strconv.FormatInt(time.Now().Unix(), 10))
}

// legalAcked says whether an administrator has seen the current first-run notice.
func (s *Server) legalAcked(ctx context.Context) bool {
	n, _ := strconv.Atoi(s.DB.GetSetting(ctx, "legalAck", "0"))
	return n >= legal.Short().AckVersion
}

func checkLegalSetting(k, v string) error {
	switch k {
	case "legalOwnerNotice":
		if utf8.RuneCountInString(v) > legal.MaxOwnerNotice {
			return errors.New("that notice is too long (" + strconv.Itoa(legal.MaxOwnerNotice) + " characters at most)")
		}
	case "legalOwnerContact":
		if utf8.RuneCountInString(v) > legal.MaxOwnerContact || strings.ContainsAny(v, "\r\n") {
			return errors.New("the contact must be one line of " + strconv.Itoa(legal.MaxOwnerContact) + " characters at most")
		}
	}
	return nil
}
