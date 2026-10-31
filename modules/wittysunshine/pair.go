package wittysunshine

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Pairing, as GameStream defines it. A person types a four-digit PIN into Sunshine; both
// sides prove they know it without sending it, and swap certificates. From then on each
// recognises the other by certificate alone.
//
// The PIN is short, so a listener who saved these messages could try all 10,000. That is
// the protocol's limit, shared by every Moonlight app, and why the guide says to pair on
// your home network. A wrong PIN never completes, so it can't be guessed against a rig.

// PairTimeout is how long someone gets to walk to the PC and type the PIN.
const PairTimeout = 3 * time.Minute

var (
	ErrWrongPIN       = errors.New("That PIN didn't match. Try again, typing the PIN shown here into Sunshine")
	ErrAlreadyPairing = errors.New("Sunshine is already pairing with something. Finish or cancel that first")
	ErrPairTimedOut   = errors.New("Nobody typed the PIN into Sunshine in time. Start again when you're at the PC")
	ErrTooOld         = errors.New("This PC is running something older than Sunshine. Install a current Sunshine")
	ErrImposter       = errors.New("Something on your network tried to pose as this PC, so pairing was stopped")
)

// NewPIN makes the four digits a person types into Sunshine.
func NewPIN() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%04d", n.Int64()), nil
}

// ValidPIN: exactly four digits.
func ValidPIN(pin string) bool {
	if len(pin) != 4 {
		return false
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

type pairAnswer struct {
	answer
	Paired            string `xml:"paired"`
	PlainCert         string `xml:"plaincert"`
	ChallengeResponse string `xml:"challengeresponse"`
	PairingSecret     string `xml:"pairingsecret"`
}

// Pair runs the whole exchange. The first request waits, with the connection held open,
// until someone types the PIN into Sunshine or ctx ends. It returns the PC's certificate
// in PEM form, ready to pin.
func (r *Rig) Pair(ctx context.Context, pin string) ([]byte, error) {
	return r.pairNamed(ctx, pin, DeviceName)
}

func (r *Rig) pairNamed(ctx context.Context, pin, device string) ([]byte, error) {
	if r.Identity == nil {
		return nil, ErrNotPaired
	}
	if !ValidPIN(pin) {
		return nil, ErrWrongPIN
	}
	info, err := r.Hello(ctx)
	if err != nil {
		return nil, err
	}
	if major, _, _ := strings.Cut(info.Version, "."); !atLeast(major, 7) {
		return nil, ErrTooOld
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(append(append([]byte{}, salt...), pin...))
	key := sum[:16]

	plain := r.plainClient()
	step := func(q url.Values) (*pairAnswer, error) { return pairStep(ctx, plain, r, device, q) }

	// 1. Offer our certificate. Sunshine holds this request until the PIN is typed.
	q := url.Values{}
	q.Set("phrase", "getservercert")
	q.Set("salt", hex.EncodeToString(salt))
	q.Set("clientcert", hex.EncodeToString(r.Identity.CertPEM))
	a, err := step(q)
	if err != nil {
		var se *StatusError
		switch {
		case errors.As(err, &se) && se.Code == 408:
			return nil, ErrPairTimedOut
		case errors.As(err, &se) && (se.Code == 409 || se.Code == 503):
			return nil, ErrAlreadyPairing
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrNoAnswer) && ctx.Err() != nil:
			return nil, ErrPairTimedOut
		}
		return nil, err
	}
	if a.PlainCert == "" {
		return nil, ErrAlreadyPairing
	}
	certPEM, err := hex.DecodeString(a.PlainCert)
	if err != nil {
		return nil, ErrBadAnswer
	}
	serverCert, err := parseCertPEM(certPEM)
	if err != nil {
		return nil, ErrBadAnswer
	}

	// 2. Challenge it: only someone who knows the PIN can answer.
	challenge := make([]byte, 16)
	if _, err := rand.Read(challenge); err != nil {
		return nil, err
	}
	enc, err := aesECB(key, challenge, true)
	if err != nil {
		return nil, err
	}
	q = url.Values{}
	q.Set("clientchallenge", hex.EncodeToString(enc))
	if a, err = step(q); err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(a.ChallengeResponse)
	if err != nil || len(raw) < sha256.Size+16 {
		return nil, ErrBadAnswer
	}
	raw = raw[:(len(raw)/aes.BlockSize)*aes.BlockSize]
	dec, err := aesECB(key, raw, false)
	if err != nil {
		return nil, ErrBadAnswer
	}
	serverHash, serverChallenge := dec[:sha256.Size], dec[sha256.Size:sha256.Size+16]

	// 3. Answer its challenge, committing to our own secret.
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write(serverChallenge)
	h.Write(r.Identity.parsed.Signature)
	h.Write(secret)
	enc, err = aesECB(key, h.Sum(nil), true)
	if err != nil {
		return nil, err
	}
	q = url.Values{}
	q.Set("serverchallengeresp", hex.EncodeToString(enc))
	if a, err = step(q); err != nil {
		return nil, err
	}
	pairing, err := hex.DecodeString(a.PairingSecret)
	if err != nil || len(pairing) <= 16 {
		return nil, ErrBadAnswer
	}
	serverSecret, serverSig := pairing[:16], pairing[16:]

	// Its secret must be signed by the certificate it offered, or someone in the middle
	// has swapped the certificate.
	if err := verifySigned(serverCert, serverSecret, serverSig); err != nil {
		return nil, ErrImposter
	}
	// And its first answer must prove it knew the PIN.
	want := sha256.New()
	want.Write(challenge)
	want.Write(serverCert.Signature)
	want.Write(serverSecret)
	if subtle.ConstantTimeCompare(want.Sum(nil), serverHash) != 1 {
		return nil, ErrWrongPIN
	}

	// 4. Prove we hold our key.
	digest := sha256.Sum256(secret)
	sig, err := r.Identity.cert.PrivateKey.(*rsa.PrivateKey).Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return nil, err
	}
	q = url.Values{}
	q.Set("clientpairingsecret", hex.EncodeToString(append(append([]byte{}, secret...), sig...)))
	if _, err = step(q); err != nil {
		return nil, err
	}

	// 5. The same again over TLS to the certificate we now trust, which is the first
	// time the PC checks our certificate.
	pinned, err := r.pinnedClient(true, certPEM)
	if err != nil {
		return nil, err
	}
	q = r.params()
	q.Set("devicename", device)
	q.Set("updateState", "1")
	q.Set("phrase", "pairchallenge")
	body, err := get(ctx, pinned, r.url("https", r.base()+offsetHTTPS, "/pair", q), maxAnswer)
	if err != nil {
		return nil, err
	}
	var last pairAnswer
	if err := xml.Unmarshal(body, &last); err != nil {
		return nil, ErrBadAnswer
	}
	if err := checkStatus(last.answer); err != nil {
		return nil, err
	}
	if last.Paired != "1" {
		return nil, ErrWrongPIN
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw}), nil
}

func pairStep(ctx context.Context, c *http.Client, r *Rig, device string, extra url.Values) (*pairAnswer, error) {
	q := r.params()
	q.Set("devicename", device)
	q.Set("updateState", "1")
	for k, v := range extra {
		q[k] = v
	}
	body, err := get(ctx, c, r.url("http", r.base(), "/pair", q), maxAnswer)
	if err != nil {
		return nil, err
	}
	var a pairAnswer
	if err := xml.Unmarshal(body, &a); err != nil {
		return nil, ErrBadAnswer
	}
	if err := checkStatus(a.answer); err != nil {
		return nil, err
	}
	if a.Paired != "1" {
		return nil, ErrWrongPIN
	}
	return &a, nil
}

func atLeast(s string, n int) bool {
	v, err := strconv.Atoi(s)
	return err == nil && v >= n
}

func parseCertPEM(b []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, ErrBadAnswer
	}
	return x509.ParseCertificate(block.Bytes)
}

// aesECB is how GameStream protects the pairing messages. ECB has well known weaknesses;
// each message here is short and random, and the protocol leaves no choice.
func aesECB(key, data []byte, encrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(data)%aes.BlockSize != 0 {
		return nil, ErrBadAnswer
	}
	out := make([]byte, len(data))
	for i := 0; i < len(data); i += aes.BlockSize {
		if encrypt {
			block.Encrypt(out[i:], data[i:])
		} else {
			block.Decrypt(out[i:], data[i:])
		}
	}
	return out, nil
}

func verifySigned(c *x509.Certificate, msg, sig []byte) error {
	digest := sha256.Sum256(msg)
	switch pub := c.PublicKey.(type) {
	case *rsa.PublicKey:
		return rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig)
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pub, digest[:], sig) {
			return ErrImposter
		}
		return nil
	}
	return ErrImposter
}

func fingerprint(c *x509.Certificate) string {
	sum := sha256.Sum256(c.Raw)
	return hex.EncodeToString(sum[:])
}
