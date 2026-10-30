package identity

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

type memStore map[string]string

func (m memStore) GetSetting(_ context.Context, k, def string) string {
	if v, ok := m[k]; ok {
		return v
	}
	return def
}
func (m memStore) SetSetting(_ context.Context, k, v string) error { m[k] = v; return nil }

func TestLoadKeepsTheIdentity(t *testing.T) {
	ctx := context.Background()
	st := memStore{}
	a, err := Load(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(a.ID) {
		t.Errorf("id %q", a.ID)
	}
	if !regexp.MustCompile(`^[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}$`).MatchString(a.Fingerprint()) {
		t.Errorf("fingerprint %q", a.Fingerprint())
	}
	b, err := Load(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID || a.PublicKey() != b.PublicKey() {
		t.Error("a restart must keep the same identity")
	}
	other, _ := Load(ctx, memStore{})
	if other.ID == a.ID || other.PublicKey() == a.PublicKey() {
		t.Error("two servers share an identity")
	}
	// A damaged key is replaced rather than used.
	st[seedKey] = "not base64"
	c, err := Load(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if c.PublicKey() == a.PublicKey() {
		t.Error("kept a damaged key")
	}
}

func TestSignAndVerify(t *testing.T) {
	a, _ := Load(context.Background(), memStore{})
	look, _ := Load(context.Background(), memStore{})
	nonce := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	sig := a.Sign(nonce)
	if !Verify(a.PublicKey(), a.ID, nonce, sig) {
		t.Fatal("a good signature was refused")
	}
	if Verify(a.PublicKey(), a.ID, base64.StdEncoding.EncodeToString([]byte("fedcba9876543210")), sig) {
		t.Error("an old answer was accepted for a new challenge")
	}
	// A look-alike knows the ID and public key, since both are public, but not the key.
	look.ID = a.ID
	if Verify(a.PublicKey(), a.ID, nonce, look.Sign(nonce)) {
		t.Error("a look-alike passed")
	}
	if Verify(a.PublicKey(), look.ID+"x", nonce, sig) || Verify("nonsense", a.ID, nonce, sig) || Verify(a.PublicKey(), a.ID, nonce, "nonsense") {
		t.Error("bad input passed")
	}
}

func TestCheckNonce(t *testing.T) {
	enc := func(n int) string { return base64.StdEncoding.EncodeToString(make([]byte, n)) }
	for s, ok := range map[string]bool{enc(16): true, enc(64): true, enc(15): false, enc(65): false, "": false, strings.Repeat("!", 24): false} {
		if (CheckNonce(s) == nil) != ok {
			t.Errorf("%q: want ok=%v", s, ok)
		}
	}
}
