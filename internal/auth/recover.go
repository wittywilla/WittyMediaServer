package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Getting back into an account whose password is lost. This is for the command line on
// the server itself: being able to write the server's database is the proof of who you are.
// Nothing in internal/server may call it, and a test in cmd/wittymediaserver holds it to that.

// ErrNoAccount: nobody signs in with that name.
var ErrNoAccount = errors.New("no account with that name")

// RecoverableAccount is one account as the command line lists it.
type RecoverableAccount struct {
	Name      string
	Kind      string
	IsAdmin   bool
	TwoFactor bool
}

// RecoverableAccounts lists every account, administrators first.
func RecoverableAccounts(ctx context.Context, d *db.DB) ([]RecoverableAccount, error) {
	rows, err := d.QueryContext(ctx, `SELECT name, kind, is_admin, totp_on FROM users ORDER BY is_admin DESC, name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecoverableAccount
	for rows.Next() {
		var a RecoverableAccount
		if err := rows.Scan(&a.Name, &a.Kind, &a.IsAdmin, &a.TwoFactor); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RecoverPassword gives an account a new password and signs it out everywhere, in one step:
// someone who got hold of the old password or a device loses it here too. Two-factor
// sign-in stays as it was, which ClearTwoFactorByName is for.
func RecoverPassword(ctx context.Context, d *db.DB, name, password string) (*RecoverableAccount, error) {
	id, a, err := recoverableAccount(ctx, d, name)
	if err != nil {
		return nil, err
	}
	if !utf8.ValidString(password) { // no sign-in form could ever send it
		return nil, errors.New("that password isn't valid text")
	}
	if err := checkPassword(password, a.Kind); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), id)
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return nil, errors.New("the database didn't take the change")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return nil, err
	}
	// How strong it is can only be known now. An older database has nowhere to say so yet.
	weak := 0
	if WeakPassword(a.Name, password) {
		weak = 1
	}
	_, _ = tx.ExecContext(ctx, `UPDATE users SET password_weak = ? WHERE id = ?`, weak, id)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	// "Changed" has to be true: a trigger on the table could have quietly stopped it.
	var stored string
	if err := d.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, id).Scan(&stored); err != nil || stored != string(hash) {
		return nil, errors.New("the database didn't keep the change")
	}
	return a, nil
}

// recoverableAccount finds an account as sign-in does, ignoring case. A database older than that
// rule can hold names that differ only by case: the exact one is meant, or nobody can say.
func recoverableAccount(ctx context.Context, d *db.DB, name string) (int64, *RecoverableAccount, error) {
	name = strings.TrimSpace(name)
	rows, err := d.QueryContext(ctx, `SELECT id, name, kind, is_admin, totp_on FROM users WHERE name = ? COLLATE NOCASE`, name)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	var id int64
	var found *RecoverableAccount
	n := 0
	for rows.Next() {
		var a RecoverableAccount
		var this int64
		if err := rows.Scan(&this, &a.Name, &a.Kind, &a.IsAdmin, &a.TwoFactor); err != nil {
			return 0, nil, err
		}
		n++
		if found == nil || a.Name == name {
			id, found = this, &a
		}
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}
	switch {
	case found == nil:
		return 0, nil, ErrNoAccount
	case n > 1 && found.Name != name:
		return 0, nil, errors.New("more than one account has that name in different capitals: type it exactly as the list shows it")
	}
	return id, found, nil
}

// MinimumPasswordFor is the shortest password an account of this kind may be given.
func MinimumPasswordFor(kind string) int {
	if kind == KindAdult {
		return MinPassword
	}
	return MinPIN
}

// tempAlphabet leaves out what's easy to misread when it's copied off a screen.
const tempAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

// TemporaryPassword makes a random password of 20 characters, about 115 bits.
func TemporaryPassword() (string, error) {
	out := make([]byte, 20)
	limit := byte(256 - 256%len(tempAlphabet)) // a plain % would favour the first letters
	buf := make([]byte, 64)
	for n := 0; n < len(out); {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if b < limit && n < len(out) {
				out[n] = tempAlphabet[int(b)%len(tempAlphabet)]
				n++
			}
		}
	}
	return string(out), nil
}
