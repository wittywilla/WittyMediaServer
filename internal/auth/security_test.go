package auth

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

func newService(t *testing.T) *Service {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return &Service{DB: d}
}

// The test vectors of RFC 6238, appendix B (SHA-1, truncated to 6 digits).
func TestTOTPVectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	for unix, want := range map[int64]string{59: "287082", 1111111109: "081804", 1111111111: "050471", 1234567890: "005924", 2000000000: "279037"} {
		if got := totpCode(secret, unix/totpStep); got != want {
			t.Errorf("t=%d: got %s, want %s", unix, got, want)
		}
	}
}

func TestMatchTOTPWindow(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1111111111, 0)
	step := now.Unix() / totpStep
	for d := int64(-1); d <= 1; d++ {
		if matchTOTP(secret, totpCode(secret, step+d), now) != step+d {
			t.Errorf("step %+d should match", d)
		}
	}
	for _, d := range []int64{-2, 2} {
		if matchTOTP(secret, totpCode(secret, step+d), now) != 0 {
			t.Errorf("step %+d is too far away", d)
		}
	}
}

func currentCode(t *testing.T, secret string) string {
	t.Helper()
	raw, err := b32.DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return totpCode(raw, time.Now().Unix()/totpStep)
}

func TestTwoFactorFlow(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	u, err := s.CreateUser(ctx, "Sam", "a long enough passphrase", true)
	if err != nil {
		t.Fatal(err)
	}
	if u.TwoFactor {
		t.Fatal("off by default")
	}
	setup, err := s.StartTwoFactor(ctx, u.ID, "My Server")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(setup.URI, "otpauth://totp/My%20Server:Sam?") || !strings.Contains(setup.URI, "secret="+setup.Secret) {
		t.Errorf("uri = %s", setup.URI)
	}
	// Until a code is confirmed, sign-in is unchanged.
	if _, _, err := s.LoginCode(ctx, "Sam", "a long enough passphrase", "", "test", false); err != nil {
		t.Fatalf("unconfirmed setup must not lock the account out: %v", err)
	}
	if _, err := s.ConfirmTwoFactor(ctx, u.ID, "000000x"); err == nil {
		t.Fatal("a wrong code must not turn it on")
	}
	codes, err := s.ConfirmTwoFactor(ctx, u.ID, currentCode(t, setup.Secret))
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != recoveryLen || s.RecoveryLeft(ctx, u.ID) != recoveryLen {
		t.Fatalf("recovery codes: %v", codes)
	}
	// The confirming code is used up, the password alone no longer signs in.
	if _, _, err := s.LoginCode(ctx, "Sam", "a long enough passphrase", "", "test", false); err != ErrCodeNeeded {
		t.Fatalf("no code: %v", err)
	}
	if _, _, err := s.LoginCode(ctx, "Sam", "a long enough passphrase", "123", "test", false); err != ErrCodeWrong {
		t.Fatalf("wrong code: %v", err)
	}
	if _, _, err := s.LoginCode(ctx, "Sam", "wrong password", "", "test", false); err != ErrInvalid {
		t.Fatalf("a wrong password must not reveal the second step: %v", err)
	}
	// Recovery codes work once, written any way people type them.
	if _, _, err := s.LoginCode(ctx, "Sam", "a long enough passphrase", " "+strings.ToUpper(codes[0])+" ", "test", false); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	if _, _, err := s.LoginCode(ctx, "Sam", "a long enough passphrase", codes[0], "test", false); err != ErrCodeWrong {
		t.Fatalf("a recovery code must work once: %v", err)
	}
	if s.RecoveryLeft(ctx, u.ID) != recoveryLen-1 {
		t.Fatal("the used code should be gone")
	}
	if err := s.DisableTwoFactor(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.LoginCode(ctx, "Sam", "a long enough passphrase", "", "test", false); err != nil {
		t.Fatalf("after turning it off: %v", err)
	}
}

func TestTOTPCodeWorksOnce(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	u, _ := s.CreateUser(ctx, "Sam", "a long enough passphrase", true)
	setup, _ := s.StartTwoFactor(ctx, u.ID, "")
	raw, _ := b32.DecodeString(setup.Secret)
	now := time.Now().Unix() / totpStep
	// Confirming with the previous step leaves the current code unused.
	if _, err := s.ConfirmTwoFactor(ctx, u.ID, totpCode(raw, now-1)); err != nil {
		t.Fatal(err)
	}
	code := totpCode(raw, now)
	if !s.CheckCode(ctx, u.ID, code) {
		t.Fatal("first use should work")
	}
	if s.CheckCode(ctx, u.ID, code) {
		t.Fatal("a code seen once must not work again")
	}
}

func TestKidsCannotUseTwoFactor(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	u, err := s.CreateProfile(ctx, "Kid", "", KindKids, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartTwoFactor(ctx, u.ID, ""); err == nil {
		t.Fatal("kids profiles have no second step")
	}
}

func TestSessionsListAndRevoke(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	a, _ := s.CreateUser(ctx, "Sam", "a long enough passphrase", true)
	b, _ := s.CreateUser(ctx, "Alex", "another long passphrase", false)
	ta, _, err := s.Login(ctx, "Sam", "a long enough passphrase", "Laptop", false)
	if err != nil {
		t.Fatal(err)
	}
	ta2, _, _ := s.Login(ctx, "Sam", "a long enough passphrase", "Phone", false)
	tb, _, _ := s.Login(ctx, "Alex", "another long passphrase", "TV", false)

	s.Touch(ctx, ta, "203.0.113.7", "internet", "https", true)
	list, err := s.Sessions(ctx, a.ID, ta)
	if err != nil || len(list) != 2 {
		t.Fatalf("own sessions: %v %v", list, err)
	}
	var mine Session
	for _, v := range list {
		if strings.Contains(v.ID, ta) || strings.Contains(v.ID, ta2) || len(v.ID) != 24 {
			t.Fatalf("the ID must not reveal the token: %s", v.ID)
		}
		if _, err := hex.DecodeString(v.ID); err != nil {
			t.Fatal(err)
		}
		if v.Current {
			mine = v
		}
	}
	if mine.Device != "Laptop" || mine.IP != "203.0.113.7" || mine.Where != "internet" || mine.Via != "https" || !mine.Streaming || mine.UserName != "Sam" {
		t.Errorf("session = %+v", mine)
	}
	if all, _ := s.Sessions(ctx, 0, ""); len(all) != 3 {
		t.Errorf("everyone's sessions: %d", len(all))
	}

	// Another account's session can't be ended by its ID, unless by the admin.
	if ok, _ := s.EndSession(ctx, b.ID, SessionID(ta2)); ok {
		t.Fatal("ended someone else's session")
	}
	if _, err := s.Resolve(ctx, ta2); err != nil {
		t.Fatal("the session should still work")
	}
	if ok, _ := s.EndSession(ctx, 0, SessionID(ta2)); !ok {
		t.Fatal("the administrator can end it")
	}
	if _, err := s.Resolve(ctx, ta2); err == nil {
		t.Fatal("a revoked session must stop working")
	}
	if ok, _ := s.EndSession(ctx, b.ID, "nonsense"); ok {
		t.Fatal("an unknown ID ends nothing")
	}
	if _, err := s.Resolve(ctx, tb); err != nil {
		t.Fatal("other sessions are untouched")
	}
}

func TestWeakPassword(t *testing.T) {
	for _, pw := range []string{"", "abc", "123456789", "1234567890", "PASSWORD123", "Password123", "1111111111", "abababababab", "wittymediaserver", "Sam123456"} {
		if !WeakPassword("Sam", pw) {
			t.Errorf("%q should be weak", pw)
		}
	}
	for _, pw := range []string{"correct horse battery", "Tr0ub4dor&3x", "sam goes to the shops"} {
		if WeakPassword("Sam", pw) {
			t.Errorf("%q should be fine", pw)
		}
	}
}

func TestWeakAccounts(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	admin, _ := s.CreateUser(ctx, "Sam", "1234", true)
	if _, err := s.CreateUser(ctx, "Alex", "correct horse battery", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProfile(ctx, "Kid", "1234", KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	list, err := s.WeakAccounts(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "Sam" || !list[0].IsAdmin {
		t.Fatalf("got %+v %v", list, err)
	}
	if err := s.SetPassword(ctx, admin.ID, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.WeakAccounts(ctx); len(list) != 0 {
		t.Fatalf("after a better password: %+v", list)
	}
}
