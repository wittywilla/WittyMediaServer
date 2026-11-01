package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/db"
)

func recoveryData(t *testing.T) (string, *auth.Service) {
	t.Helper()
	t.Setenv("WMS_CONFIG", "")
	dir := t.TempDir()
	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return dir, &auth.Service{DB: d}
}

func runResetPassword(input string, ask func(io.Writer) (string, error), args ...string) (int, string, string) {
	var out, errs bytes.Buffer
	if ask == nil {
		ask = func(io.Writer) (string, error) { return "", errors.New("asked at the terminal") }
	}
	code := resetPassword(args, strings.NewReader(input), &out, &errs, ask)
	return code, out.String(), errs.String()
}

// The whole way back: the old password stops working, the new one works, and every device
// that was signed in is signed out.
func TestResetPasswordGetsTheOwnerBackIn(t *testing.T) {
	dir, svc := recoveryData(t)
	ctx := context.Background()
	if _, err := svc.CreateUser(ctx, "Owner", "forgotten-password", true); err != nil {
		t.Fatal(err)
	}
	tok, _, err := svc.Login(ctx, "owner", "forgotten-password", "phone", false)
	if err != nil {
		t.Fatal(err)
	}
	code, out, errs := runResetPassword("a brand new password\n", nil, "-data", dir, "--password-stdin", "owner")
	if code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	if !strings.Contains(out, "Owner") {
		t.Errorf("didn't say whose password changed: %s", out)
	}
	if strings.Contains(out+errs, "a brand new password") {
		t.Error("the new password was repeated back")
	}
	if _, _, err := svc.Login(ctx, "owner", "forgotten-password", "x", false); err == nil {
		t.Error("the old password still works")
	}
	if _, _, err := svc.Login(ctx, "owner", "a brand new password", "x", false); err != nil {
		t.Errorf("the new password doesn't work: %v", err)
	}
	if _, err := svc.Resolve(ctx, tok); err == nil {
		t.Error("a device from before is still signed in")
	}
}

func TestResetPasswordAsksAtTheTerminal(t *testing.T) {
	dir, svc := recoveryData(t)
	ctx := context.Background()
	if _, err := svc.CreateUser(ctx, "owner", "forgotten-password", true); err != nil {
		t.Fatal(err)
	}
	asked := func(w io.Writer) (string, error) { return "typed at the terminal", nil }
	if code, out, errs := runResetPassword("", asked, "-data", dir, "owner"); code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	if _, _, err := svc.Login(ctx, "owner", "typed at the terminal", "x", false); err != nil {
		t.Errorf("the typed password doesn't work: %v", err)
	}
	// A name that's wrong is said before anyone is asked for a password.
	code, _, errs := runResetPassword("", nil, "-data", dir, "nobody")
	if code != 1 || !strings.Contains(errs, "no account") || strings.Contains(errs, "asked at the terminal") {
		t.Errorf("exit %d: %s", code, errs)
	}
}

func TestResetPasswordGenerates(t *testing.T) {
	dir, svc := recoveryData(t)
	ctx := context.Background()
	if _, err := svc.CreateUser(ctx, "owner", "forgotten-password", true); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runResetPassword("", nil, "-data", dir, "--generate", "owner")
	if code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	m := regexp.MustCompile(`(?m)^    (\S{20})$`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no password shown: %s", out)
	}
	if _, _, err := svc.Login(ctx, "owner", m[1], "x", false); err != nil {
		t.Errorf("the generated password doesn't work: %v", err)
	}
	second, _ := auth.TemporaryPassword()
	if second == m[1] || auth.WeakPassword("owner", m[1]) {
		t.Errorf("not a strong, random password: %q", m[1])
	}
}

// Nothing changes when anything is wrong, and a mistake can't lock someone out harder.
func TestResetPasswordRefuses(t *testing.T) {
	dir, svc := recoveryData(t)
	ctx := context.Background()
	if _, err := svc.CreateUser(ctx, "owner", "forgotten-password", true); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"too short":             {"-data", dir, "--password-stdin", "owner"},
		"unknown account":       {"-data", dir, "--password-stdin", "nobody"},
		"password as a word":    {"-data", dir, "owner", "secret-on-the-command-line"},
		"both ways of choosing": {"-data", dir, "--generate", "--password-stdin", "owner"},
		"no name":               {"-data", dir},
	}
	for what, args := range cases {
		if code, out, _ := runResetPassword("short\n", nil, args...); code == 0 {
			t.Errorf("%s: it went ahead: %s", what, out)
		}
	}
	if _, _, err := svc.Login(ctx, "owner", "forgotten-password", "x", false); err != nil {
		t.Errorf("the password changed anyway: %v", err)
	}
	mismatch := func(io.Writer) (string, error) { return "", errors.New("those two didn't match") }
	if code, _, _ := runResetPassword("", mismatch, "-data", dir, "owner"); code == 0 {
		t.Error("two different passwords went ahead")
	}
}

// A mistyped folder must not be left with anything in it. Nor is one made, for the same reason
// root is refused: folders it makes could stop the server starting.
func TestResetPasswordMakesNothingInAWrongFolder(t *testing.T) {
	t.Setenv("WMS_CONFIG", "")
	dir := t.TempDir()
	code, _, errs := runResetPassword("a brand new password\n", nil, "-data", dir, "--password-stdin", "owner")
	if code == 0 || !strings.Contains(errs, "no server database") {
		t.Errorf("exit %d: %s", code, errs)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("it made %v", entries)
	}
	missing := filepath.Join(dir, "not", "there")
	if code, _, _ := runResetPassword("a brand new password\n", nil, "-data", missing, "--password-stdin", "owner"); code == 0 {
		t.Error("it went ahead")
	}
	if _, err := os.Stat(missing); err == nil {
		t.Error("it made the folder")
	}
}

// What reaches a server's log can be seen afterwards, and has nothing of the password in it.
func TestResetPasswordIsOnTheRecord(t *testing.T) {
	dir, svc := recoveryData(t)
	if _, err := svc.CreateUser(context.Background(), "owner", "forgotten-password", true); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := runResetPassword("a brand new password\n", nil, "-data", dir, "--password-stdin", "owner"); code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "logs", "wms-*.log"))
	if len(files) == 0 {
		t.Fatal("nothing was logged")
	}
	b, _ := os.ReadFile(files[0])
	if !strings.Contains(string(b), "reset from the command line") || strings.Contains(string(b), "brand new") {
		t.Errorf("the log says: %s", b)
	}
}

// The smaller ways it could say something untrue, or be misled.
func TestResetPasswordOddCases(t *testing.T) {
	dir, svc := recoveryData(t)
	ctx := context.Background()
	if _, err := svc.CreateUser(ctx, "owner", "forgotten-password", true); err != nil {
		t.Fatal(err)
	}
	// Not valid text: no sign-in form could send it.
	if code, _, _ := runResetPassword("bad\xff\xfepassword\n", nil, "-data", dir, "--password-stdin", "owner"); code == 0 {
		t.Error("a password that isn't text went ahead")
	}
	// Bytes aren't characters: two emoji are 8 bytes, but no password.
	if code, _, _ := runResetPassword("\U0001F600\U0001F600\n", nil, "-data", dir, "--password-stdin", "owner"); code == 0 {
		t.Error("two emoji passed for a password")
	}
	// Far too much input is cut off, and so refused, rather than all read.
	if code, _, _ := runResetPassword(strings.Repeat("x", 1<<20)+"\n", nil, "-data", dir, "--password-stdin", "owner"); code == 0 {
		t.Error("a megabyte passed for a password")
	}
	// A database that quietly refuses the change isn't told it worked.
	if _, err := svc.DB.ExecContext(ctx, `CREATE TRIGGER keep_it BEFORE UPDATE OF password_hash ON users BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := runResetPassword("a brand new password\n", nil, "-data", dir, "--password-stdin", "owner"); code == 0 {
		t.Errorf("it said it worked: %s", out)
	}
	if _, err := svc.DB.ExecContext(ctx, `DROP TRIGGER keep_it`); err != nil {
		t.Fatal(err)
	}
	// Names from before they had to differ by more than capitals: the exact one, or none.
	if _, err := svc.DB.ExecContext(ctx, `DROP INDEX IF EXISTS users_name_nocase`); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Twin", "twin"} {
		if _, err := svc.DB.ExecContext(ctx, `INSERT INTO users(name, password_hash, is_admin, created_at, kind) VALUES(?, 'x', 0, 1, 'adult')`, n); err != nil {
			t.Skipf("this database won't hold both: %v", err)
		}
	}
	if code, _, errs := runResetPassword("a brand new password\n", nil, "-data", dir, "--password-stdin", "TWIN"); code == 0 || !strings.Contains(errs, "capitals") {
		t.Errorf("exit %d: %s", code, errs)
	}
	if code, out, errs := runResetPassword("a brand new password\n", nil, "-data", dir, "--password-stdin", "twin"); code != 0 {
		t.Errorf("exit %d: %s %s", code, out, errs)
	}
	// What a name holds can't dress up the list, or reach the terminal.
	if _, err := svc.DB.ExecContext(ctx, `INSERT INTO users(name, password_hash, is_admin, created_at, kind) VALUES(?, 'x', 0, 1, 'adult')`, "mallory      Administrator\x1b[2J"); err != nil {
		t.Fatal(err)
	}
	_, out, _ := runResetPassword("", nil, "--list", "-data", dir)
	if strings.Contains(out, "\x1b") || !strings.Contains(out, `"mallory      Administrator\x1b[2J"`) {
		t.Errorf("the list says: %q", out)
	}
}

// reset-2fa takes the same care with where it's pointed.
func TestReset2FAMakesNothingInAWrongFolder(t *testing.T) {
	t.Setenv("WMS_CONFIG", "")
	dir := t.TempDir()
	if code := cmdReset2FA([]string{"-data", dir, "owner"}); code == 0 {
		t.Error("it went ahead")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("it made %v", entries)
	}
}

func TestResetPasswordLists(t *testing.T) {
	dir, svc := recoveryData(t)
	ctx := context.Background()
	if _, err := svc.CreateUser(ctx, "owner", "forgotten-password", true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProfile(ctx, "little-one", "", auth.KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runResetPassword("", nil, "--list", "-data", dir)
	if code != 0 || !strings.Contains(out, "owner") || !strings.Contains(out, "Administrator") || !strings.Contains(out, "little-one") {
		t.Errorf("exit %d: %s %s", code, out, errs)
	}
	if strings.Index(out, "owner") > strings.Index(out, "little-one") {
		t.Errorf("administrators should come first: %s", out)
	}
}

// A Kids profile's PIN can be short, and two-factor sign-in isn't touched.
func TestResetPasswordKeepsTheRules(t *testing.T) {
	dir, svc := recoveryData(t)
	ctx := context.Background()
	if _, err := svc.CreateProfile(ctx, "little-one", "1234", auth.KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := runResetPassword("4321\n", nil, "-data", dir, "--password-stdin", "little-one"); code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	if _, _, err := svc.Login(ctx, "little-one", "4321", "x", false); err != nil {
		t.Errorf("the new PIN doesn't work: %v", err)
	}
	if _, err := svc.CreateUser(ctx, "owner", "forgotten-password", true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DB.ExecContext(ctx, `UPDATE users SET totp_on = 1, totp_secret = 'x' WHERE name = 'owner'`); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runResetPassword("a brand new password\n", nil, "-data", dir, "--password-stdin", "owner")
	if code != 0 || !strings.Contains(out, "reset-2fa") {
		t.Errorf("exit %d: %s", code, out)
	}
	var on int
	_ = svc.DB.QueryRowContext(ctx, `SELECT totp_on FROM users WHERE name = 'owner'`).Scan(&on)
	if on != 1 {
		t.Error("two-factor sign-in was turned off")
	}
}

// The recovery is for the machine itself. Nothing the network can reach may call it: the server's
// code can't, and the repair page can only ever point at the command.
func TestRecoveryIsOutOfTheServersReach(t *testing.T) {
	root := filepath.Join("..", "..", "internal")
	calls := regexp.MustCompile(`\b(RecoverPassword|RecoverableAccounts|TemporaryPassword)\b`)
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		if filepath.Dir(p) == filepath.Join(root, "auth") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if calls.Match(b) {
			t.Errorf("%s reaches the password recovery, which is only for the command line", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
