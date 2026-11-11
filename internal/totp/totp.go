// Package totp makes and checks one-time passwords: HOTP (RFC 4226) and the
// time-based TOTP built on it (RFC 6238), with the standard library only.
//
// The server uses it for pairing PINs: 6 digits, a new one every 30 seconds,
// one step of clock drift either way, and never the same step twice.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"hash"
	"time"
)

const (
	// Digits is how long a PIN is.
	Digits = 6
	// Period is how long one PIN lasts.
	Period = 30 * time.Second
	// Skew is how many steps either side of now are still accepted.
	Skew = 1
	// SecretBytes is the size of a new secret: 160 bits, as RFC 4226 recommends.
	SecretBytes = 20
)

var pow10 = [...]uint32{1, 10, 100, 1_000, 10_000, 100_000, 1_000_000, 10_000_000, 100_000_000, 1_000_000_000}

// NewSecret makes a random secret of SecretBytes.
func NewSecret() ([]byte, error) {
	b := make([]byte, SecretBytes)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// Step is the time step t falls in, counted from the Unix epoch.
func Step(t time.Time) int64 { return floorDiv(t.Unix(), int64(Period/time.Second)) }

// StepEnds is when the step ends, and the next PIN begins.
func StepEnds(step int64) time.Time {
	return time.Unix((step+1)*int64(Period/time.Second), 0)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// HOTP is the RFC 4226 code for a counter, with digits digits (1 to 9) and the
// given hash (sha1.New for the standard one).
func HOTP(secret []byte, counter uint64, digits int, h func() hash.Hash) string {
	if digits < 1 || digits >= len(pow10) {
		panic("totp: digits out of range")
	}
	mac := hmac.New(h, secret)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	n := bin % pow10[digits]
	out := make([]byte, digits)
	for i := digits - 1; i >= 0; i-- {
		out[i] = byte('0' + n%10)
		n /= 10
	}
	return string(out)
}

// At is the RFC 6238 code for a step, with this package's settings (HMAC-SHA1, 6 digits).
func At(secret []byte, step int64) string {
	return HOTP(secret, uint64(step), Digits, sha1.New)
}

// Code is the PIN at time t.
func Code(secret []byte, t time.Time) string { return At(secret, Step(t)) }

// Verify checks code against the steps around t, Skew either way, accepting only steps after
// `after` (the last step a code was used for, so a code can't be used twice; pass -1 when
// none has been). It answers the step that matched. Every candidate is compared, in
// constant time, whatever matches first.
func Verify(secret []byte, code string, t time.Time, after int64) (int64, bool) {
	now := Step(t)
	matched, ok := int64(0), 0
	for d := int64(-Skew); d <= Skew; d++ {
		step := now + d
		eq := subtle.ConstantTimeCompare([]byte(code), []byte(At(secret, step)))
		fresh := 0
		if step > after {
			fresh = 1
		}
		hit := eq & fresh
		if hit == 1 && ok == 0 {
			matched = step
		}
		ok |= hit
	}
	return matched, ok == 1
}
