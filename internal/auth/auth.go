// Package auth provides users, password hashing and session tokens.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
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

// Shortest passwords for new ones (those set earlier keep working): a full
// account's password protects everything; a Kids or Guest profile's is a PIN
// to keep siblings apart.
const (
	MinPassword = 8
	MinPIN      = 4
)

func checkPassword(password, kind string) error {
	if kind == KindAdult && len(password) < MinPassword {
		return fmt.Errorf("the password must be at least %d characters", MinPassword)
	}
	if len(password) < MinPIN {
		return fmt.Errorf("the password or PIN must be at least %d characters", MinPIN)
	}
	if len(password) > 72 {
		return errors.New("the password must be at most 72 bytes")
	}
	return nil
}

// storedToken is what the database keeps of a session token: its hash, so a
// copy of the database (a backup, a stolen disk) holds no working sign-ins.
// Tokens are long and random, so a plain, fast hash is enough.
func storedToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hashedPrefix + hex.EncodeToString(sum[:])
}

const hashedPrefix = "s256:"

// HashStoredTokens converts sessions saved before tokens were hashed, so those
// sign-ins keep working. It runs at start-up.
func (s *Service) HashStoredTokens(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT token FROM sessions WHERE token NOT LIKE 's256:%'`)
	if err != nil {
		return err
	}
	var old []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return err
		}
		old = append(old, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, t := range old {
		if _, err := s.DB.ExecContext(ctx, `UPDATE sessions SET token = ? WHERE token = ?`, storedToken(t), t); err != nil {
			return err
		}
	}
	return nil
}

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
		if err := checkPassword(password, kind); err != nil {
			return nil, err
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
	u, err := s.User(ctx, id)
	if err != nil {
		return err
	}
	if err := checkPassword(password, u.Kind); err != nil {
		return err
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
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token <> ?`, id, storedToken(keep))
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
	if r := []rune(device); len(r) > 120 {
		device = string(r[:120])
	}
	// Sessions nobody came back to would otherwise stay for ever.
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now.Unix())
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO sessions(token, user_id, created_at, expires_at, device) VALUES(?,?,?,?,?)`, storedToken(tok), userID, now.Unix(), now.Add(tokenTTL).Unix(), device); err != nil {
		return "", nil, err
	}
	u, err := s.User(ctx, userID)
	return tok, u, err
}

// Logout revokes a token.
func (s *Service) Logout(ctx context.Context, token string) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, storedToken(token))
}

// Resolve returns the user for a token.
func (s *Service) Resolve(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrInvalid
	}
	return s.resolveStored(ctx, storedToken(token))
}

func (s *Service) resolveStored(ctx context.Context, stored string) (*User, error) {
	var userID, expires int64
	if err := s.DB.QueryRowContext(ctx, `SELECT user_id, expires_at FROM sessions WHERE token = ?`, stored).Scan(&userID, &expires); err != nil {
		return nil, ErrInvalid
	}
	if time.Now().Unix() > expires {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, stored)
		return nil, ErrInvalid
	}
	return s.User(ctx, userID)
}

// Stream tokens are for players that can't send a header (native TV players
// fetching HLS, for one) and so have to carry a credential in the address.
// Addresses end up in logs and players' caches, so instead of the session's
// own token they carry one that opens a single title or stream, only for
// reading, for a few hours, and only while the session it came from lasts:
// signing out or changing a password ends it too.
const (
	streamPrefix = "st1."
	// StreamTokenTTL is long enough for a long film with a pause in it. A
	// player that outlasts it gets 401 and asks for a new one.
	StreamTokenTTL = 6 * time.Hour
)

// streamKey is chosen at every start: stream tokens don't outlive the server
// run, and neither do the streams themselves.
var streamKey = func() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}()

var streamScopeRe = regexp.MustCompile(`^/api/(items/\d+/|shows/\d+/art/|hls/[^/]+/|dvd/[^/]+/)`)

// StreamScope is what a stream token for this media address may open: the
// title, show or stream it belongs to, or "" if it isn't one a token can be
// made for.
func StreamScope(path string) string {
	if strings.Contains(path, "..") || !mediaRoute.MatchString(path) {
		return ""
	}
	return streamScopeRe.FindString(path)
}

// IsStreamToken tells stream tokens from session tokens.
func IsStreamToken(tok string) bool { return strings.HasPrefix(tok, streamPrefix) }

func signStream(payload []byte) []byte {
	m := hmac.New(sha256.New, streamKey)
	m.Write(payload)
	return m.Sum(nil)
}

// StreamToken makes a token from a session's token for one scope.
func StreamToken(session, scope string, ttl time.Duration) (string, time.Time) {
	exp := time.Now().Add(ttl)
	payload := binary.BigEndian.AppendUint64(nil, uint64(exp.Unix()))
	payload = append(payload, storedToken(session)[len(hashedPrefix):]...)
	payload = append(payload, scope...)
	enc := base64.RawURLEncoding
	return streamPrefix + enc.EncodeToString(payload) + "." + enc.EncodeToString(signStream(payload)), exp
}

// streamSessionLen is the session's hash inside a stream token, in hex.
const streamSessionLen = sha256.Size * 2

// ResolveStream returns the user for a stream token on this request.
func (s *Service) ResolveStream(ctx context.Context, tok string, r *http.Request) (*User, error) {
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !mediaRoute.MatchString(r.URL.Path) || strings.Contains(r.URL.Path, "..") {
		return nil, ErrInvalid
	}
	body, sig, ok := strings.Cut(strings.TrimPrefix(tok, streamPrefix), ".")
	if !ok {
		return nil, ErrInvalid
	}
	enc := base64.RawURLEncoding
	payload, err1 := enc.DecodeString(body)
	mac, err2 := enc.DecodeString(sig)
	if err1 != nil || err2 != nil || !hmac.Equal(mac, signStream(payload)) || len(payload) < 8+streamSessionLen {
		return nil, ErrInvalid
	}
	if time.Now().Unix() > int64(binary.BigEndian.Uint64(payload)) {
		return nil, ErrInvalid
	}
	session, scope := string(payload[8:8+streamSessionLen]), string(payload[8+streamSessionLen:])
	if scope == "" || !strings.HasPrefix(r.URL.Path, scope) {
		return nil, ErrInvalid
	}
	return s.resolveStored(ctx, hashedPrefix+session)
}

const CookieName = "wms_token"

// mediaRoute are the addresses a page opens with <img>, <video>, <a download>
// and the like, which can't carry a header. Only these take a token in the
// address: one that leaked from a media link (a browser's history, a proxy's
// log) can't be used to call the rest of the API or change anything.
var mediaRoute = regexp.MustCompile(`^/api/(items/\d+/(poster|backdrop|file|stream|page/|epub/|disc/thumb/)|shows/\d+/art/|artwork/preview$|hls/|dvd/[^/]+/(menu/|still/|sub/|subs$)|logs/files/|themes/[^/]+/download$)`)

// queryToken is the ?token= of a media address.
func queryToken(r *http.Request) string {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return ""
	}
	if q := r.URL.Query().Get("token"); q != "" && mediaRoute.MatchString(r.URL.Path) {
		return q
	}
	return ""
}

// TokenFrom returns the token a request names most explicitly: the
// Authorization header, X-WMS-Token, the ?token= of a media address, then
// the cookie.
func TokenFrom(r *http.Request) string {
	if ts := Tokens(r); len(ts) > 0 {
		return ts[0]
	}
	return ""
}

// Tokens lists every token a request carries, most explicit first.
// Browsers share cookies between servers on one machine (ports don't
// separate them), so a request can carry another server's, or an expired,
// cookie beside its own valid token.
func Tokens(r *http.Request) []string {
	var out []string
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		out = append(out, strings.TrimSpace(h[7:]))
	}
	if h := r.Header.Get("X-WMS-Token"); h != "" {
		out = append(out, h)
	}
	if q := queryToken(r); q != "" {
		out = append(out, q)
	}
	for _, c := range r.Cookies() {
		if c.Name == CookieName && c.Value != "" {
			out = append(out, c.Value)
		}
	}
	return out
}
