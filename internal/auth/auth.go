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
}

type Service struct {
	DB *db.DB
}

var ErrInvalid = errors.New("invalid name or password")

const tokenTTL = 90 * 24 * time.Hour

func (s *Service) UserCount(ctx context.Context) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n
}

func (s *Service) CreateUser(ctx context.Context, name, password string, admin bool) (*User, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(password) < 4 {
		return nil, errors.New("name required and password must be at least 4 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO users(name, password_hash, is_admin, created_at) VALUES(?,?,?,?)`, name, string(hash), admin, db.Now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("that name is taken")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.User(ctx, id)
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

func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *Service) User(ctx context.Context, id int64) (*User, error) {
	var u User
	var admin int
	err := s.DB.QueryRowContext(ctx, `SELECT id, name, is_admin, created_at FROM users WHERE id = ?`, id).Scan(&u.ID, &u.Name, &admin, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	u.IsAdmin = admin != 0
	return &u, nil
}

func (s *Service) Users(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, is_admin, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		var admin int
		if err := rows.Scan(&u.ID, &u.Name, &admin, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.IsAdmin = admin != 0
		out = append(out, u)
	}
	return out, rows.Err()
}

// Login verifies credentials and issues a token.
func (s *Service) Login(ctx context.Context, name, password, device string) (string, *User, error) {
	var id int64
	var hash string
	err := s.DB.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE name = ? COLLATE NOCASE`, strings.TrimSpace(name)).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil) {
		return "", nil, ErrInvalid
	}
	if err != nil {
		return "", nil, err
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
