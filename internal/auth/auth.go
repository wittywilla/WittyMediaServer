// Package auth provides users, password hashing and session tokens.
package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

type User struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	IsAdmin   bool   `json:"isAdmin"`
	CreatedAt int64  `json:"createdAt"`
	// Kind is "adult", "kids" or "guest". Kids and Guest profiles may have
	// no password; they then sign in with a tap on the home network.
	Kind        string `json:"kind"`
	HasPassword bool   `json:"hasPassword"`
	// Restrictions is the account's library.Access as JSON ("" = none).
	Restrictions string `json:"-"`
}

// Kinds of accounts.
const (
	KindAdult = "adult"
	KindKids  = "kids"
	KindGuest = "guest"
)

const userCols = `id, name, is_admin, created_at, kind, password_hash <> '', IFNULL(restrictions, '')`

type scanner interface{ Scan(...any) error }

func scanUser(r scanner) (*User, error) {
	var u User
	var admin int
	if err := r.Scan(&u.ID, &u.Name, &admin, &u.CreatedAt, &u.Kind, &u.HasPassword, &u.Restrictions); err != nil {
		return nil, err
	}
	u.IsAdmin = admin != 0
	return &u, nil
}

type Service struct {
	DB *db.DB
}

var ErrInvalid = errors.New("invalid name or password")

const tokenTTL = 90 * 24 * time.Hour

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not a password"), bcrypt.DefaultCost)

func (s *Service) UserCount(ctx context.Context) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n
}

func (s *Service) CreateUser(ctx context.Context, name, password string, admin bool) (*User, error) {
	return s.CreateProfile(ctx, name, password, KindAdult, admin, "")
}

// CreateProfile adds an account of a kind. Kids and Guest profiles may
// have no password; adults always need one.
func (s *Service) CreateProfile(ctx context.Context, name, password, kind string, admin bool, restrictions string) (*User, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 40 {
		return nil, errors.New("a name of up to 40 characters is required")
	}
	switch kind {
	case KindAdult:
	case KindKids, KindGuest:
		admin = false
	default:
		return nil, errors.New("unknown profile kind")
	}
	hash := ""
	if password != "" || kind == KindAdult {
		if len(password) < 4 {
			return nil, errors.New("the password must be at least 4 characters")
		}
		b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		hash = string(b)
	}
	var restr any
	if restrictions != "" {
		restr = restrictions
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO users(name, password_hash, is_admin, created_at, kind, restrictions) VALUES(?,?,?,?,?,?)`,
		name, hash, admin, db.Now(), kind, restr)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("that name is taken")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.User(ctx, id)
}

// SetRestrictions stores an account's limits (JSON, "" = none).
func (s *Service) SetRestrictions(ctx context.Context, id int64, restrictions string) error {
	var v any
	if restrictions != "" {
		v = restrictions
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET restrictions = ? WHERE id = ?`, v, id)
	return err
}

// UI returns an account's interface choices (JSON, "" = defaults).
func (s *Service) UI(ctx context.Context, id int64) string {
	var v sql.NullString
	_ = s.DB.QueryRowContext(ctx, `SELECT ui FROM users WHERE id = ?`, id).Scan(&v)
	return v.String
}

// SetUI stores an account's interface choices.
func (s *Service) SetUI(ctx context.Context, id int64, ui string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET ui = ? WHERE id = ?`, ui, id)
	return err
}

// ClearPassword lets a Kids or Guest profile sign in without one.
func (s *Service) ClearPassword(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE users SET password_hash = '' WHERE id = ? AND kind <> 'adult'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("only Kids and Guest profiles can be used without a password")
	}
	return nil
}

func (s *Service) SetPassword(ctx context.Context, id int64, password string) error {
	if len(password) < 4 {
		return errors.New("password must be at least 4 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), id)
	return err
}

// CheckPassword reports whether password is the account's current one.
func (s *Service) CheckPassword(ctx context.Context, id int64, password string) bool {
	var hash string
	if err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, id).Scan(&hash); err != nil || hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// EndSessions signs the account out everywhere except the session keep
// (after a password change, so a stolen session stops working).
func (s *Service) EndSessions(ctx context.Context, id int64, keep string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token <> ?`, id, keep)
	return err
}

func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *Service) User(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

func (s *Service) Users(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// Login verifies credentials and issues a token. allowNoPassword permits
// password-free Kids/Guest profiles (the server only allows it on the home
// network); adults always need their password.
func (s *Service) Login(ctx context.Context, name, password, device string, allowNoPassword bool) (string, *User, error) {
	var id int64
	var hash, kind string
	err := s.DB.QueryRowContext(ctx, `SELECT id, password_hash, kind FROM users WHERE name = ? COLLATE NOCASE`, strings.TrimSpace(name)).Scan(&id, &hash, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		// The same work as a real check, so the time taken doesn't tell
		// which account names exist.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", nil, ErrInvalid
	}
	if err != nil {
		return "", nil, err
	}
	if hash == "" {
		if kind == KindAdult || !allowNoPassword {
			return "", nil, ErrInvalid
		}
	} else if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", nil, ErrInvalid
	}
	return s.issue(ctx, id, device)
}

func (s *Service) issue(ctx context.Context, userID int64, device string) (string, *User, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	tok := hex.EncodeToString(b)
	now := time.Now()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO sessions(token, user_id, created_at, expires_at, device) VALUES(?,?,?,?,?)`, tok, userID, now.Unix(), now.Add(tokenTTL).Unix(), device); err != nil {
		return "", nil, err
	}
	u, err := s.User(ctx, userID)
	return tok, u, err
}

// Logout revokes a token.
func (s *Service) Logout(ctx context.Context, token string) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
}

// Resolve returns the user for a token.
func (s *Service) Resolve(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrInvalid
	}
	var userID, expires int64
	if err := s.DB.QueryRowContext(ctx, `SELECT user_id, expires_at FROM sessions WHERE token = ?`, token).Scan(&userID, &expires); err != nil {
		return nil, ErrInvalid
	}
	if time.Now().Unix() > expires {
		s.Logout(ctx, token)
		return nil, ErrInvalid
	}
	return s.User(ctx, userID)
}

const CookieName = "wms_token"

// TokenFrom extracts a token from the Authorization header, cookie or query.
func TokenFrom(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if h := r.Header.Get("X-WMS-Token"); h != "" {
		return h
	}
	if c, err := r.Cookie(CookieName); err == nil {
		return c.Value
	}
	return r.URL.Query().Get("token")
}
