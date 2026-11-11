package totp

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"hash"
	"testing"
	"time"
)

// RFC 6238, Appendix B: 8 digits, a 30-second step, the seeds the RFC gives.
func TestRFC6238Vectors(t *testing.T) {
	seed20 := []byte("12345678901234567890")
	seed32 := []byte("12345678901234567890123456789012")
	seed64 := []byte("1234567890123456789012345678901234567890123456789012345678901234")
	algs := []struct {
		name string
		h    func() hash.Hash
		seed []byte
	}{{"SHA1", sha1.New, seed20}, {"SHA256", sha256.New, seed32}, {"SHA512", sha512.New, seed64}}
	vectors := []struct {
		unix int64
		want [3]string
	}{
		{59, [3]string{"94287082", "46119246", "90693936"}},
		{1111111109, [3]string{"07081804", "68084774", "25091201"}},
		{1111111111, [3]string{"14050471", "67062674", "99943326"}},
		{1234567890, [3]string{"89005924", "91819424", "93441116"}},
		{2000000000, [3]string{"69279037", "90698825", "38618901"}},
		{20000000000, [3]string{"65353130", "77737706", "47863826"}},
	}
	for _, v := range vectors {
		step := Step(time.Unix(v.unix, 0))
		for i, a := range algs {
			if got := HOTP(a.seed, uint64(step), 8, a.h); got != v.want[i] {
				t.Errorf("%s at %d: %s, want %s", a.name, v.unix, got, v.want[i])
			}
		}
		// The 6-digit code is the same number, shorter.
		if got := Code(seed20, time.Unix(v.unix, 0)); got != v.want[0][2:] {
			t.Errorf("6 digits at %d: %s, want %s", v.unix, got, v.want[0][2:])
		}
	}
}

// RFC 4226, Appendix D.
func TestRFC4226Vectors(t *testing.T) {
	want := []string{"755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489"}
	for i, w := range want {
		if got := HOTP([]byte("12345678901234567890"), uint64(i), 6, sha1.New); got != w {
			t.Errorf("counter %d: %s, want %s", i, got, w)
		}
	}
}

func TestVerifyWindowAndReplay(t *testing.T) {
	secret, err := NewSecret()
	if err != nil || len(secret) < 20 {
		t.Fatalf("secret: %d bytes, %v", len(secret), err)
	}
	now := time.Unix(1_800_000_015, 0)
	s := Step(now)
	if st, ok := Verify(secret, Code(secret, now), now, -1); !ok || st != s {
		t.Errorf("current code: %v %d", ok, st)
	}
	if _, ok := Verify(secret, At(secret, s-1), now, -1); !ok {
		t.Error("the previous step should pass")
	}
	if _, ok := Verify(secret, At(secret, s+1), now, -1); !ok {
		t.Error("the next step should pass")
	}
	if _, ok := Verify(secret, At(secret, s-2), now, -1); ok {
		t.Error("two steps old passed")
	}
	if _, ok := Verify(secret, Code(secret, now), now, s); ok {
		t.Error("a used step passed again")
	}
	if _, ok := Verify(secret, At(secret, s-1), now, s-1); ok {
		t.Error("an older step after a newer was used passed")
	}
	if _, ok := Verify(secret, "", now, -1); ok {
		t.Error("empty passed")
	}
	if _, ok := Verify(secret, "12345", now, -1); ok {
		t.Error("short passed")
	}
	if got := StepEnds(s); !got.After(now) || got.Sub(now) > Period {
		t.Errorf("step ends %v", got)
	}
}
