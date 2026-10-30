package remote

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeCert(t *testing.T, dir, name string, notAfter time.Time) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cp, kp := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(cp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kp, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		t.Fatal(err)
	}
	return cp, kp
}

func TestHTTPSWithOwnCertificate(t *testing.T) {
	dir := t.TempDir()
	cp, kp := writeCert(t, dir, "media.example.test", time.Now().Add(30*24*time.Hour))
	h := NewHTTPS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			t.Error("the handler should see an encrypted request")
		}
		_, _ = io.WriteString(w, "hello")
	}), dir, slog.New(slog.DiscardHandler))
	if h.Running() {
		t.Fatal("nothing opens before Apply")
	}
	cfg := HTTPSConfig{Mode: "cert", Host: "127.0.0.1", CertFile: cp, KeyFile: kp}
	if err := h.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown(context.Background())
	addr := h.Addr()
	if addr == nil {
		t.Fatal("no port")
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "media.example.test"}, ForceAttemptHTTP2: true}}
	res, err := client.Get("https://" + addr.String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(b) != "hello" || res.TLS == nil || res.TLS.Version < tls.VersionTLS12 {
		t.Errorf("got %q over %+v", b, res.TLS)
	}

	st := h.Status(context.Background())
	if !st.Running || st.Cert == nil || st.Cert.Names[0] != "media.example.test" || !st.Cert.SelfSigned || st.Cert.DaysLeft < 28 {
		t.Errorf("status = %+v / %+v", st, st.Cert)
	}

	// The same settings leave the port as it is.
	if err := h.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	if h.Addr().String() != addr.String() {
		t.Error("unchanged settings should not reopen the port")
	}

	// TLS below 1.2 is refused.
	old := &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS11}
	if c, err := tls.Dial("tcp", addr.String(), old); err == nil {
		c.Close()
		t.Error("TLS 1.1 should be refused")
	}

	if err := h.Apply(HTTPSConfig{Mode: "off"}); err != nil {
		t.Fatal(err)
	}
	if h.Running() || h.Addr() != nil {
		t.Error("off closes the port")
	}
	if _, err := client.Get("https://" + addr.String() + "/"); err == nil {
		t.Error("nothing should answer once it is off")
	}
}

func TestHTTPSRefusesBadCertificate(t *testing.T) {
	dir := t.TempDir()
	h := NewHTTPS(http.NotFoundHandler(), dir, slog.New(slog.DiscardHandler))
	if err := h.Apply(HTTPSConfig{Mode: "cert", Host: "127.0.0.1", CertFile: filepath.Join(dir, "none.pem"), KeyFile: filepath.Join(dir, "none.key")}); err == nil {
		t.Fatal("a missing certificate must be an error")
	}
	if h.Running() {
		t.Error("the port must stay closed")
	}
	if st := h.Status(context.Background()); st.Error == "" {
		t.Error("the problem should be reported")
	}
}

func TestFileCertPicksUpRenewal(t *testing.T) {
	dir := t.TempDir()
	cp, kp := writeCert(t, dir, "one.example.test", time.Now().Add(24*time.Hour))
	f, err := loadFileCert(cp, kp)
	if err != nil {
		t.Fatal(err)
	}
	writeCert(t, dir, "two.example.test", time.Now().Add(48*time.Hour))
	later := time.Now().Add(time.Minute)
	_ = os.Chtimes(cp, later, later)
	c, _ := f.get()
	if c.Leaf.DNSNames[0] != "two.example.test" {
		t.Errorf("got %v", c.Leaf.DNSNames)
	}
	// A half-written renewal keeps the certificate that works.
	if err := os.WriteFile(cp, []byte("-----BEGIN"), 0o600); err != nil {
		t.Fatal(err)
	}
	later = later.Add(time.Minute)
	_ = os.Chtimes(cp, later, later)
	f.checked = time.Time{}
	if c, err := f.get(); err != nil || c.Leaf.DNSNames[0] != "two.example.test" {
		t.Errorf("got %v %v", c, err)
	}
}

func TestValidDomain(t *testing.T) {
	for _, d := range []string{"media.example.com", "my-home.duckdns.org", "a.b"} {
		if !ValidDomain(d) {
			t.Errorf("%q should be fine", d)
		}
	}
	for _, d := range []string{"", "localhost", "192.168.1.5", "-x.example.com", "x-.example.com", "UPPER.example.com", "a..b", "https://example.com", "exa mple.com"} {
		if ValidDomain(d) {
			t.Errorf("%q should be refused", d)
		}
	}
}
