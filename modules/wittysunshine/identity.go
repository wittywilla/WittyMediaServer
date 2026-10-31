package wittysunshine

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DeviceName is how this server appears in Sunshine's list of paired devices.
const DeviceName = "WittyMediaServer"

// Identity is the certificate this server shows to a rig. Sunshine trusts a client by
// its certificate, so the private key is what stands between anyone else and the rig's
// games: it stays in a file of its own, never in the database or an API answer.
type Identity struct {
	ID      string // 16 hex characters, sent as uniqueid
	CertPEM []byte
	cert    tls.Certificate
	parsed  *x509.Certificate
}

// NewIdentity makes a new certificate. GameStream expects RSA, and a rig keeps a
// certificate for as long as it stays paired, so this one lasts twenty years.
func NewIdentity() (*Identity, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: DeviceName},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(20, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	return parseIdentity(hex.EncodeToString(id),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func parseIdentity(id string, certPEM, keyPEM []byte) (*Identity, error) {
	c, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return nil, err
	}
	if _, ok := c.PrivateKey.(*rsa.PrivateKey); !ok {
		return nil, errors.New("the rig identity's key is not RSA")
	}
	return &Identity{ID: strings.TrimSpace(id), CertPEM: certPEM, cert: c, parsed: parsed}, nil
}

// LoadOrCreateIdentity reads the identity kept in dir, making one the first time. The
// key file is readable only by this server's account where the system allows it.
func LoadOrCreateIdentity(dir string) (*Identity, error) {
	idPath, certPath, keyPath := filepath.Join(dir, "client.id"), filepath.Join(dir, "client.crt"), filepath.Join(dir, "client.key")
	id, e1 := os.ReadFile(idPath)
	cert, e2 := os.ReadFile(certPath)
	key, e3 := os.ReadFile(keyPath)
	if e1 == nil && e2 == nil && e3 == nil {
		return parseIdentity(string(id), cert, key)
	}
	if e1 != nil && !os.IsNotExist(e1) {
		return nil, e1
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	n, err := NewIdentity()
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(n.cert.PrivateKey)
	if err != nil {
		return nil, err
	}
	// The ID goes last: a crash part-way makes a new identity next time rather than a
	// half-written one being trusted.
	for _, f := range []struct {
		path string
		data []byte
	}{{keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})}, {certPath, n.CertPEM}, {idPath, []byte(n.ID)}} {
		if err := writeSecret(f.path, f.data); err != nil {
			return nil, err
		}
	}
	return n, nil
}

func writeSecret(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_ = tmp.Chmod(0o600) // Windows has no such mode: the data folder's permissions apply there
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
