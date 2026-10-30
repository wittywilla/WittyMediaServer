// Package identity gives a server something a look-alike can't copy: a random
// ID, which only names it, and an Ed25519 key, which proves it. An app pins the
// public key the first time it connects and has the server sign a fresh challenge
// before it sends a token again, so another machine answering at the same address
// or announcing the same name is noticed. See docs/CLIENT-API.md.
package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	idKey   = "serverId"
	seedKey = "serverKey"
	// The signed text starts with this, so a signature can't be taken for
	// anything but an answer to this question.
	domain = "WittyMediaServer identity v1\n"
)

// Store is where the identity is kept: the server's settings.
type Store interface {
	GetSetting(ctx context.Context, key, def string) string
	SetSetting(ctx context.Context, key, value string) error
}

// Identity is this server's ID and key.
type Identity struct {
	ID   string
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

// Load reads the identity, making one the first time. It moves with the
// database, so restoring a backup or moving to a new computer keeps it.
func Load(ctx context.Context, st Store) (*Identity, error) {
	id := st.GetSetting(ctx, idKey, "")
	seed, _ := base64.StdEncoding.DecodeString(st.GetSetting(ctx, seedKey, ""))
	if id == "" || len(seed) != ed25519.SeedSize {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		id = hex.EncodeToString(raw)
		seed = make([]byte, ed25519.SeedSize)
		if _, err := rand.Read(seed); err != nil {
			return nil, err
		}
		if err := st.SetSetting(ctx, seedKey, base64.StdEncoding.EncodeToString(seed)); err != nil {
			return nil, fmt.Errorf("saving the server's key: %w", err)
		}
		if err := st.SetSetting(ctx, idKey, id); err != nil {
			return nil, fmt.Errorf("saving the server's ID: %w", err)
		}
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return &Identity{ID: id, priv: priv, pub: priv.Public().(ed25519.PublicKey)}, nil
}

// PublicKey is the key to pin, in base64.
func (i *Identity) PublicKey() string { return base64.StdEncoding.EncodeToString(i.pub) }

// Fingerprint is a short form of the key for people to compare by eye.
func (i *Identity) Fingerprint() string {
	sum := sha256.Sum256(i.pub)
	h := strings.ToUpper(hex.EncodeToString(sum[:6]))
	return h[:4] + "-" + h[4:8] + "-" + h[8:]
}

// CheckNonce says whether a challenge is acceptable: base64 of 16 to 64 bytes.
func CheckNonce(nonce string) error {
	b, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil || len(b) < 16 || len(b) > 64 {
		return errors.New("nonce must be base64 of 16 to 64 random bytes")
	}
	return nil
}

func message(id, nonce string) []byte { return []byte(domain + id + "\n" + nonce) }

// Sign answers a challenge. The nonce is signed as the text it was sent as.
func (i *Identity) Sign(nonce string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(i.priv, message(i.ID, nonce)))
}

// Verify is what a client does with the answer.
func Verify(publicKey, id, nonce, signature string) bool {
	pub, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	return ed25519.Verify(pub, message(id, nonce), sig)
}
