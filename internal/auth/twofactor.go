package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Optional two-factor sign-in: time-based one-time codes (RFC 6238), the kind
// every authenticator app makes, plus recovery codes for a lost phone.

var (
	// ErrCodeNeeded: the password was right, and the account also wants a code.
	ErrCodeNeeded = errors.New("Enter the 6-digit code from your authenticator app.")
	// ErrCodeWrong: the code was wrong or has already been used.
	ErrCodeWrong = errors.New("That code isn't right, or it has already been used.")
)

const (
	totpStep    = 30 // seconds
	totpWindow  = 1  // one step either way, for clocks that are slightly off
	recoveryLen = 8
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// totpCode is the 6-digit code for a time step.
func totpCode(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, secret)
	m.Write(msg[:])
	sum := m.Sum(nil)
	o := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[o:o+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1000000)
}

// CurrentCode is the code an authenticator app would show right now.
func CurrentCode(secret string) string {
	raw, err := b32.DecodeString(secret)
	if err != nil {
		return ""
	}
	return totpCode(raw, time.Now().Unix()/totpStep)
}

// matchTOTP returns the time step a code belongs to (0 = no match).
func matchTOTP(secret []byte, code string, now time.Time) int64 {
	cur := now.Unix() / totpStep
	for d := int64(-totpWindow); d <= totpWindow; d++ {
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, cur+d)), []byte(code)) == 1 {
			return cur + d
		}
	}
	return 0
}

// TwoFactorSetup is what an authenticator app needs: the secret to type in,
// and the address a QR code holds.
type TwoFactorSetup struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

// StartTwoFactor creates a new secret for an account. It takes effect only
// once ConfirmTwoFactor has seen a code made from it.
func (s *Service) StartTwoFactor(ctx context.Context, id int64, issuer string) (*TwoFactorSetup, error) {
	u, err := s.User(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.Kind != KindAdult {
		return nil, errors.New("two-factor sign-in is for adult profiles")
	}
	if u.TwoFactor {
		return nil, errors.New("two-factor sign-in is already on; turn it off first")
	}
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	secret := b32.EncodeToString(raw)
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET totp_secret = ?, totp_on = 0, totp_last = 0, recovery_codes = NULL WHERE id = ?`, secret, id); err != nil {
		return nil, err
	}
	if issuer == "" {
		issuer = "WittyMediaServer"
	}
	v := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	label := url.PathEscape(issuer) + ":" + url.PathEscape(u.Name)
	return &TwoFactorSetup{Secret: secret, URI: "otpauth://totp/" + label + "?" + v.Encode()}, nil
}

// ConfirmTwoFactor turns two-factor sign-in on after seeing one working code,
// and returns the recovery codes (shown once, stored only as hashes).
func (s *Service) ConfirmTwoFactor(ctx context.Context, id int64, code string) ([]string, error) {
	var stored sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT totp_secret FROM users WHERE id = ?`, id).Scan(&stored); err != nil {
		return nil, err
	}
	secret, err := b32.DecodeString(stored.String)
	if err != nil || len(secret) == 0 {
		return nil, errors.New("start the setup again")
	}
	step := matchTOTP(secret, cleanCode(code), time.Now())
	if step == 0 {
		return nil, ErrCodeWrong
	}
	codes := make([]string, recoveryLen)
	hashes := make([]string, recoveryLen)
	for i := range codes {
		b := make([]byte, 7)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		c := strings.ToLower(b32.EncodeToString(b))[:10]
		codes[i] = c[:5] + "-" + c[5:]
		hashes[i] = hashRecovery(c)
	}
	raw, _ := json.Marshal(hashes)
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET totp_on = 1, totp_last = ?, recovery_codes = ? WHERE id = ?`, step, string(raw), id); err != nil {
		return nil, err
	}
	return codes, nil
}

// DisableTwoFactor turns it off and forgets the secret and recovery codes.
func (s *Service) DisableTwoFactor(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET totp_secret = NULL, totp_on = 0, totp_last = 0, recovery_codes = NULL WHERE id = ?`, id)
	return err
}

// RecoveryLeft says how many recovery codes an account has left.
func (s *Service) RecoveryLeft(ctx context.Context, id int64) int {
	var raw sql.NullString
	_ = s.DB.QueryRowContext(ctx, `SELECT recovery_codes FROM users WHERE id = ?`, id).Scan(&raw)
	var hashes []string
	_ = json.Unmarshal([]byte(raw.String), &hashes)
	return len(hashes)
}

// CheckCode reports whether code is valid for the account: a code from the
// authenticator app (each usable once), or an unused recovery code (used up
// by this call).
func (s *Service) CheckCode(ctx context.Context, id int64, code string) bool {
	code = cleanCode(code)
	if code == "" {
		return false
	}
	var secret, recovery sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT totp_secret, recovery_codes FROM users WHERE id = ? AND totp_on = 1`, id).Scan(&secret, &recovery); err != nil {
		return false
	}
	if len(code) == 6 {
		raw, err := b32.DecodeString(secret.String)
		if err != nil {
			return false
		}
		step := matchTOTP(raw, code, time.Now())
		if step == 0 {
			return false
		}
		// One statement, so the same code can't be used twice at once.
		res, err := s.DB.ExecContext(ctx, `UPDATE users SET totp_last = ? WHERE id = ? AND totp_last < ?`, step, id, step)
		if err != nil {
			return false
		}
		n, _ := res.RowsAffected()
		return n == 1
	}
	var hashes []string
	if json.Unmarshal([]byte(recovery.String), &hashes) != nil {
		return false
	}
	want := hashRecovery(code)
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(h), []byte(want)) == 1 {
			rest, _ := json.Marshal(append(hashes[:i:i], hashes[i+1:]...))
			res, err := s.DB.ExecContext(ctx, `UPDATE users SET recovery_codes = ? WHERE id = ? AND recovery_codes = ?`, string(rest), id, recovery.String)
			if err != nil {
				return false
			}
			n, _ := res.RowsAffected()
			return n == 1
		}
	}
	return false
}

// cleanCode: authenticator apps show "123 456", people type "ABCDE-FGHIJ".
func cleanCode(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	c = strings.NewReplacer(" ", "", "-", "").Replace(c)
	if len(c) > 32 {
		return ""
	}
	return c
}

// hashRecovery: recovery codes are long and random, so a fast hash is enough.
func hashRecovery(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// ClearTwoFactorByName is the way back for an owner who lost both their phone
// and their recovery codes (`wms reset-2fa`, on the server itself).
func ClearTwoFactorByName(ctx context.Context, d *db.DB, name string) error {
	res, err := d.ExecContext(ctx, `UPDATE users SET totp_secret = NULL, totp_on = 0, totp_last = 0, recovery_codes = NULL WHERE name = ? COLLATE NOCASE`, strings.TrimSpace(name))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("no account with that name")
	}
	return nil
}
