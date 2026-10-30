package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// The list of signed-in devices. Tokens themselves never leave the server:
// each session is shown and revoked by an ID derived from its token.

// Session is one signed-in device.
type Session struct {
	ID        string `json:"id"`
	UserID    int64  `json:"userId"`
	UserName  string `json:"userName"`
	Device    string `json:"device"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
	LastSeen  int64  `json:"lastSeen"`
	IP        string `json:"ip"`
	Where     string `json:"where"` // machine | home | vpn | internet
	Via       string `json:"via"`   // http | https | proxy
	Streaming bool   `json:"streaming"`
	Current   bool   `json:"current"`
}

// SessionID is the public name of a token.
func SessionID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:12])
}

// seen is the latest activity of a session. It lives in memory, and reaches
// the database at most once a minute: playing video makes a request every few
// seconds, which should not mean a disk write every few seconds.
type seen struct {
	at, written, streamed time.Time
	ip, where, via        string
}

type activity struct {
	mu sync.Mutex
	m  map[string]*seen
}

// streamHold: a session counts as streaming for this long after a request for
// video or sound.
const streamHold = 30 * time.Second

// Touch records that a session was just used.
func (s *Service) Touch(ctx context.Context, token, ip, where, via string, streaming bool) {
	now := time.Now()
	s.act.mu.Lock()
	if s.act.m == nil {
		s.act.m = map[string]*seen{}
	}
	if len(s.act.m) > 4096 {
		for k, v := range s.act.m {
			if now.Sub(v.at) > time.Hour {
				delete(s.act.m, k)
			}
		}
	}
	e := s.act.m[token]
	if e == nil {
		e = &seen{}
		s.act.m[token] = e
	}
	e.at = now
	if streaming {
		e.streamed = now
	}
	write := now.Sub(e.written) >= time.Minute || e.ip != ip || e.where != where || e.via != via
	e.ip, e.where, e.via = ip, where, via
	if write {
		e.written = now
	}
	s.act.mu.Unlock()
	if write {
		_, _ = s.DB.ExecContext(ctx, `UPDATE sessions SET last_seen = ?, last_ip = ?, last_where = ?, last_via = ? WHERE token = ?`, now.Unix(), ip, where, via, token)
	}
}

func (s *Service) forget(token string) {
	s.act.mu.Lock()
	delete(s.act.m, token)
	s.act.mu.Unlock()
}

// Sessions lists sessions that have not expired, most recently used first:
// one account's, or everyone's when userID is 0. current is the token of the
// caller, whose session is marked.
func (s *Service) Sessions(ctx context.Context, userID int64, current string) ([]Session, error) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, db.Now())
	q := `SELECT s.token, s.user_id, u.name, IFNULL(s.device, ''), s.created_at, s.expires_at, IFNULL(s.last_seen, s.created_at), IFNULL(s.last_ip, ''), IFNULL(s.last_where, ''), IFNULL(s.last_via, '')
	      FROM sessions s JOIN users u ON u.id = s.user_id`
	var args []any
	if userID != 0 {
		q += ` WHERE s.user_id = ?`
		args = append(args, userID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY IFNULL(s.last_seen, s.created_at) DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	now := time.Now()
	for rows.Next() {
		var token string
		var v Session
		if err := rows.Scan(&token, &v.UserID, &v.UserName, &v.Device, &v.CreatedAt, &v.ExpiresAt, &v.LastSeen, &v.IP, &v.Where, &v.Via); err != nil {
			return nil, err
		}
		v.ID = SessionID(token)
		v.Current = current != "" && token == current
		s.act.mu.Lock()
		if e := s.act.m[token]; e != nil {
			v.LastSeen, v.IP, v.Where, v.Via = e.at.Unix(), e.ip, e.where, e.via
			v.Streaming = now.Sub(e.streamed) < streamHold
		}
		s.act.mu.Unlock()
		out = append(out, v)
	}
	return out, rows.Err()
}

// EndSession signs one device out. userID limits it to that account's
// sessions (0 = any account's, for the administrator). It reports whether
// there was such a session.
func (s *Service) EndSession(ctx context.Context, userID int64, id string) (bool, error) {
	q := `SELECT token FROM sessions`
	var args []any
	if userID != 0 {
		q += ` WHERE user_id = ?`
		args = append(args, userID)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return false, err
	}
	target := ""
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return false, err
		}
		if SessionID(t) == id {
			target = t
			break
		}
	}
	rows.Close()
	if target == "" {
		return false, nil
	}
	s.Logout(ctx, target)
	return true, nil
}

// ---------------------------------------------------------------- weak passwords

// common holds passwords that are long enough but first on every guessing list.
var common = map[string]bool{"password": true, "password1": true, "password12": true, "password123": true, "passw0rd": true, "1234567890": true,
	"0123456789": true, "0987654321": true, "12345678910": true, "123456789012": true, "1q2w3e4r5t": true, "qwertyuiop": true, "qwerty12345": true, "qwerty123456": true,
	"letmein123": true, "administrator": true, "admin12345": true, "admin123456": true, "iloveyou12": true, "welcome123": true, "welcome1234": true, "changeme12": true,
	"changeme123": true, "football12": true, "baseball12": true, "superman12": true, "dragon1234": true, "sunshine12": true, "trustno123": true, "starwars12": true,
	"mediaserver": true, "wittymedia": true, "wittymediaserver": true}

// WeakPassword says whether a password would fall quickly to guessing over the
// internet. It is deliberately simple: short, a common one, based on the
// account's name, all digits, or nearly the same character over and over.
func WeakPassword(name, password string) bool {
	if len(password) < 10 {
		return true
	}
	p := strings.ToLower(password)
	if common[p] {
		return true
	}
	if n := strings.ToLower(strings.TrimSpace(name)); n != "" && strings.Contains(p, n) && len(p)-len(n) < 6 {
		return true
	}
	digits := true
	chars := map[rune]bool{}
	for _, r := range p {
		chars[r] = true
		if r < '0' || r > '9' {
			digits = false
		}
	}
	return digits || len(chars) <= 3
}

// markWeak remembers how strong a password is, the only moment it is known:
// it is never stored, and the hash can't tell.
func (s *Service) markWeak(ctx context.Context, id int64, name, password string) {
	v := 0
	if WeakPassword(name, password) {
		v = 1
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE users SET password_weak = ? WHERE id = ? AND (password_weak IS NULL OR password_weak <> ?)`, v, id, v)
}

// WeakAccount is an account whose password should be changed, or (Unknown)
// whose password has not been checked yet because nobody signed in with it
// since this check was added.
type WeakAccount struct {
	Name    string `json:"name"`
	IsAdmin bool   `json:"isAdmin"`
	Unknown bool   `json:"unknown"`
}

// WeakAccounts lists adult accounts with a weak or unchecked password.
func (s *Service) WeakAccounts(ctx context.Context) ([]WeakAccount, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, is_admin, password_weak FROM users WHERE kind = 'adult' AND (password_weak IS NULL OR password_weak = 1) ORDER BY is_admin DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WeakAccount{}
	for rows.Next() {
		var w WeakAccount
		var weak sql.NullInt64
		if err := rows.Scan(&w.Name, &w.IsAdmin, &weak); err != nil {
			return nil, err
		}
		w.Unknown = !weak.Valid
		out = append(out, w)
	}
	return out, rows.Err()
}

// AdminsWithoutTwoFactor names administrators who only have a password.
func (s *Service) AdminsWithoutTwoFactor(ctx context.Context) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = 1 AND totp_on = 0`).Scan(&n)
	return n
}
