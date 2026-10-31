package update

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// trustedKeys are the public halves of the release signing keys (base64,
// 32 bytes). A release is installed only when its checksum list is signed
// by one of them. To rotate keys, add the new one here a release before
// signing with it.
var trustedKeys = []string{
	releaseKey,
}

// TrustedKeys are the keys a release must be signed with.
func TrustedKeys() []ed25519.PublicKey { return publicKeys(devKeys) }

// ModuleKeys are the keys a plugin's release may be signed with. A key that signs
// only plugins lives in many repositories' secrets; it never appears in TrustedKeys,
// so if it leaks it still can't sign a server.
func ModuleKeys() []ed25519.PublicKey {
	if moduleKey == "" {
		return TrustedKeys()
	}
	return publicKeys(append([]string{moduleKey}, devKeys...)) // and the release key, for what it signed before
}

// publicKeys decodes trustedKeys plus extra ones.
func publicKeys(extra []string) []ed25519.PublicKey {
	var out []ed25519.PublicKey
	for _, k := range append(append([]string{}, trustedKeys...), extra...) {
		if b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(k)); err == nil && len(b) == ed25519.PublicKeySize {
			out = append(out, ed25519.PublicKey(b))
		}
	}
	return out
}

// ErrBadSignature: the checksum list is not signed by a trusted key.
var ErrBadSignature = errors.New("the release's signature does not match a trusted key; it was not installed")

// VerifySums checks sig (the .sig file: base64 of an Ed25519 signature)
// against the exact bytes of the checksum list.
func VerifySums(sums, sig []byte, keys []ed25519.PublicKey) error {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(raw) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	for _, k := range keys {
		if ed25519.Verify(k, sums, raw) {
			return nil
		}
	}
	return ErrBadSignature
}

// Sign returns the .sig file contents for a checksum list.
func Sign(key ed25519.PrivateKey, sums []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, sums)) + "\n")
}

// ParseSums reads a sha256sum-style list ("<hex>  <name>" or
// "<hex> *<name>") into name → lowercase hex digest.
func ParseSums(b []byte) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sum, name, ok := strings.Cut(line, " ")
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if d, err := hex.DecodeString(sum); !ok || err != nil || len(d) != sha256.Size || name == "" {
			return nil, fmt.Errorf("unreadable checksum line %q", line)
		}
		out[name] = strings.ToLower(sum)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("the checksum list is empty")
	}
	return out, nil
}

// FileSHA256 returns the lowercase hex SHA-256 of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
