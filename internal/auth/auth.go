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
	"flag"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

type User struct {
	ID int64 `json:"id"`
	// Name is the username: unique, and what signing in uses.
	Name string `json:"name"`
	// DisplayName is the profile name, which anyone may choose and anyone may
	// share ("" = use the username). ShowUsername: the corner of the screen shows
	// the username instead of this.
	DisplayName  string `json:"displayName"`
	ShowUsername bool   `json:"showUsername"`
	IsAdmin      bool   `json:"isAdmin"`
	CreatedAt    int64  `json:"createdAt"`
	// Kind is "adult", "kids" or "guest". Kids and Guest profiles may have
	// no password; they then sign in with a tap on the home network.
	Kind        string `json:"kind"`
	HasPassword bool   `json:"hasPassword"`
	// Restrictions is the account's library.Access as JSON ("" = none).
	Restrictions string `json:"-"`
	// TwoFactor: signing in also needs a code from an authenticator app.
	TwoFactor bool `json:"twoFactor"`
	// HomeOnly: the session this came from was made with no password, and only counts at home.
	HomeOnly bool `json:"-"`
}

// Kinds of accounts.
const (
	KindAdult = "adult"
	KindKids  = "kids"
	KindGuest = "guest"
)

const userCols = `id, name, is_admin, created_at, kind, password_hash <> '', IFNULL(restrictions, ''), totp_on, display_name, show_username`

type scanner interface{ Scan(...any) error }

func scanUser(r scanner) (*User, error) {
	var u User
	var admin, totp, showUsername int
	if err := r.Scan(&u.ID, &u.Name, &admin, &u.CreatedAt, &u.Kind, &u.HasPassword, &u.Restrictions, &totp, &u.DisplayName, &showUsername); err != nil {
		return nil, err
	}
	u.IsAdmin = admin != 0
	u.TwoFactor = totp != 0
	u.ShowUsername = showUsername != 0
	return &u, nil
}

type Service struct {
	DB  *db.DB
	act activity
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
	if kind == KindAdult && utf8.RuneCountInString(password) < MinPassword {
		return fmt.Errorf("the password must be at least %d characters", MinPassword)
	}
	if utf8.RuneCountInString(password) < MinPIN {
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

// hashCost is bcrypt's work factor for every password hash made here.
var hashCost = bcrypt.DefaultCost

// QuickHashesForTests makes password hashes cheap, for test programs that make hundreds of
// accounts: at the real cost, hashing is most of internal/server's test time. It refuses to run
// outside a test program, where it would weaken every password set after it.
func QuickHashesForTests() {
	if flag.Lookup("test.v") == nil {
		panic("auth: quick password hashes are only for tests")
	}
	hashCost = bcrypt.MinCost
}

// dummyHash is made when first needed: this slow on purpose, it would add up
// to a second to every start and every command on a small machine.
var dummyHash = sync.OnceValue(func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("not a password"), hashCost)
	return h
})

// NoAccounts is what first-run rules depend on, so it's true only when the database answered
// and has none. A failed read says false: a request hung up on halfway, or a damaged table,
// must never make a server that's set up look like a new one.
func (s *Service) NoAccounts(ctx context.Context) bool {
	var n int
	err := s.DB.QueryRowContext(context.WithoutCancel(ctx), `SELECT COUNT(*) FROM users`).Scan(&n)
	return err == nil && n == 0
}

func (s *Service) CreateUser(ctx context.Context, name, password string, admin bool) (*User, error) {
	return s.CreateProfile(ctx, name, password, KindAdult, admin, "")
}

// CreateProfile adds an account of a kind. Kids and Guest profiles may
// have no password; adults always need one.
func (s *Service) CreateProfile(ctx context.Context, name, password, kind string, admin bool, restrictions string) (*User, error) {
	name, err := s.checkUsername(ctx, name, 0)
	if err != nil {
		return nil, err
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
		b, err := bcrypt.GenerateFromPassword([]byte(password), hashCost)
		if err != nil {
			return nil, err
		}
		hash = string(b)
	}
	var restr any
	if restrictions != "" {
		restr = restrictions
	}
	// Sign-in ignores the case of a name, so names can't differ only by that.
	res, err := s.DB.ExecContext(ctx, `INSERT INTO users(name, password_hash, is_admin, created_at, kind, restrictions)
		SELECT ?,?,?,?,?,? WHERE NOT EXISTS (SELECT 1 FROM users WHERE name = ? COLLATE NOCASE)`,
		name, hash, admin, db.Now(), kind, restr, name)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("that name is taken")
		}
		return nil, err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return nil, errors.New("that name is taken")
	}
	id, _ := res.LastInsertId()
	if hash != "" {
		s.markWeak(ctx, id, name, password)
	}
	return s.User(ctx, id)
}

// MaxDisplayName is the longest profile name, in characters.
const MaxDisplayName = 60

// checkUsername tidies a username and refuses one that can't be used. Signing in
// ignores case, so "Sam" and "sam" count as the same name, and invisible characters
// are out, or one account could pass itself off as another.
func (s *Service) checkUsername(ctx context.Context, name string, id int64) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 40 {
		return "", errors.New("a name of up to 40 characters is required")
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return "", errors.New("a name can't contain invisible characters")
		}
	}
	var other int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE name = ? COLLATE NOCASE AND id <> ? LIMIT 1`, name, id).Scan(&other)
	if err == nil {
		return "", errors.New("that name is taken")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	return name, nil
}

// cleanDisplayName tidies a profile name. It takes out what could reorder or hide
// text around it (control and direction-changing characters, while the joiners that
// emoji are built with stay), closes up spaces and keeps to the length.
func cleanDisplayName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r), r == utf8.RuneError, r == 0x061C, r == 0x200E, r == 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
			return -1
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	if rs := []rune(name); len(rs) > MaxDisplayName {
		name = strings.TrimSpace(string(rs[:MaxDisplayName]))
	}
	return name
}

// SetDisplayName sets the profile name ("" goes back to the username).
func (s *Service) SetDisplayName(ctx context.Context, id int64, name string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET display_name = ? WHERE id = ?`, cleanDisplayName(name), id)
	return err
}

// SetShowUsername chooses which name the corner of the screen shows.
func (s *Service) SetShowUsername(ctx context.Context, id int64, on bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET show_username = ? WHERE id = ?`, on, id)
	return err
}

// SetUsername changes the name an account signs in with.
func (s *Service) SetUsername(ctx context.Context, id int64, name string) error {
	name, err := s.checkUsername(ctx, name, id)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET name = ? WHERE id = ?`, name, id); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errors.New("that name is taken")
		}
		return err
	}
	return nil
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
	_, err = s.DB.ExecContext(ctx, `UPDATE sessions SET home_only = 1 WHERE user_id = ?`, id)
	return err
}

func (s *Service) SetPassword(ctx context.Context, id int64, password string) error {
	u, err := s.User(ctx, id)
	if err != nil {
		return err
	}
	if err := checkPassword(password, u.Kind); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), hashCost)
	if err != nil {
		return err
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), id); err != nil {
		return err
	}
	if u, err := s.User(ctx, id); err == nil {
		s.markWeak(ctx, id, u.Name, password)
	}
	return nil
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

// CheckTwoFactor is the second step of a sensitive change: true when the
// account has none, or code is right.
func (s *Service) CheckTwoFactor(ctx context.Context, u *User, code string) bool {
	return !u.TwoFactor || s.CheckCode(ctx, u.ID, code)
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
	return s.LoginCode(ctx, name, password, "", device, allowNoPassword)
}

// LoginCode is Login with the code of an account that has two-factor sign-in.
// Whether an account has it is only revealed after the right password.
func (s *Service) LoginCode(ctx context.Context, name, password, code, device string, allowNoPassword bool) (string, *User, error) {
	var id int64
	var hash, kind, realName string
	var totp int
	err := s.DB.QueryRowContext(ctx, `SELECT id, password_hash, kind, name, totp_on FROM users WHERE name = ? COLLATE NOCASE`, strings.TrimSpace(name)).Scan(&id, &hash, &kind, &realName, &totp)
	if errors.Is(err, sql.ErrNoRows) {
		// The same work as a real check, so the time taken doesn't tell
		// which account names exist.
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		return "", nil, ErrInvalid
	}
	if err != nil {
		return "", nil, err
	}
	homeOnly := hash == ""
	if hash == "" {
		// The same work here too, so the time taken doesn't tell which names
		// are password-free profiles.
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		if kind == KindAdult || !allowNoPassword {
			return "", nil, ErrInvalid
		}
	} else if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", nil, ErrInvalid
	} else {
		s.markWeak(ctx, id, realName, password)
	}
	if totp != 0 {
		if strings.TrimSpace(code) == "" {
			return "", nil, ErrCodeNeeded
		}
		if !s.CheckCode(ctx, id, code) {
			return "", nil, ErrCodeWrong
		}
	}
	return s.issueFor(ctx, id, device, homeOnly, &hash)
}

// IssueSession signs an account in on a device that proved itself some other
// way than a password (TV pairing, approved by someone already signed in).
func (s *Service) IssueSession(ctx context.Context, userID int64, device string) (string, *User, error) {
	return s.issue(ctx, userID, device, false)
}

func (s *Service) issue(ctx context.Context, userID int64, device string, homeOnly bool) (string, *User, error) {
	return s.issueFor(ctx, userID, device, homeOnly, nil)
}

// issueFor makes a session. With a hash, only if the account's password is still the one that
// was just checked: checking takes a moment, and a new password means "signed out everywhere",
// which a sign-in already under way must not slip past.
func (s *Service) issueFor(ctx context.Context, userID int64, device string, homeOnly bool, hash *string) (string, *User, error) {
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
	const insert = `INSERT INTO sessions(token, user_id, created_at, expires_at, device, home_only) `
	args := []any{storedToken(tok), userID, now.Unix(), now.Add(tokenTTL).Unix(), device, homeOnly}
	if hash == nil {
		if _, err := s.DB.ExecContext(ctx, insert+`VALUES(?,?,?,?,?,?)`, args...); err != nil {
			return "", nil, err
		}
	} else {
		res, err := s.DB.ExecContext(ctx, insert+`SELECT ?,?,?,?,?,? WHERE EXISTS (SELECT 1 FROM users WHERE id = ? AND password_hash = ?)`, append(args, userID, *hash)...)
		if err != nil {
			return "", nil, err
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return "", nil, ErrInvalid
		}
	}
	u, err := s.User(ctx, userID)
	return tok, u, err
}

// Logout revokes a token.
func (s *Service) Logout(ctx context.Context, token string) {
	key := storedToken(token)
	s.forget(key)
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, key)
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
	var homeOnly bool
	if err := s.DB.QueryRowContext(ctx, `SELECT user_id, expires_at, home_only FROM sessions WHERE token = ?`, stored).Scan(&userID, &expires, &homeOnly); err != nil {
		return nil, ErrInvalid
	}
	if time.Now().Unix() > expires {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, stored)
		return nil, ErrInvalid
	}
	u, err := s.User(ctx, userID)
	if err != nil {
		return nil, err
	}
	u.HomeOnly = homeOnly
	return u, nil
}

// Stream tokens keep a session's token out of HLS playlists. A player that had
// to put its token in a playlist's address can't send one with the segments
// either, so the playlist names them with a stream token: it opens that one
// stream, only for reading, for a few hours, and only while the session it came
// from lasts, so a playlist in a player's cache or a log is worth little.
const (
	streamPrefix = "st1."
	// StreamTokenTTL is long enough for a long film with a pause in it. A
	// player that outlasts it gets 401 and opens the playlist again.
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
	// Strict: the spare bits at the end must be zero, so a token has one spelling only, the one
	// this server wrote, and another spelling of the same signature isn't taken for it.
	enc := base64.RawURLEncoding.Strict()
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

// mediaRoute are the addresses a page opens with <img>, <video>, <a download>,
// a WebSocket, a module script and the like, which can't carry a header. Only these take a token
// in the address, and only to read.
var mediaRoute = regexp.MustCompile(`^/api/(items/\d+/(poster|backdrop|file|stream|page/|epub/|disc/thumb/)|radio/\d+/(stream|hls)$|shows/\d+/art/|artwork/preview$|hls/|dvd/[^/]+/(menu/|still/|sub/|subs$)|logs/files/|themes/[^/]+/download$|games/\d+/art/\d+$|games/\d+/saves/\d+/(thumb|file)$|games/\d+/battery$|game-bezels/\d+$|play/[0-9a-f]+/screenshot$|play/[0-9a-f]+/ws$|play/[0-9a-f]+/stream$|game-background/image$|modules/[a-z0-9]+/web/|music/artist-photo$|music/extras/\d+/file$|logos/\d+/image$|media/\d+/canvas/file$|media/\d+/stems/[a-z]+$)`)

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
