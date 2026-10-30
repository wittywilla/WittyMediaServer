package remote

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// HTTPS runs the server's encrypted port, beside the plain one the home network
// uses. It stays closed until the owner turns it on, with a certificate from
// Let's Encrypt or one they supply, and can be changed without a restart.

// HTTPSConfig is what the HTTPS port should be doing.
type HTTPSConfig struct {
	Mode     string // off | acme | cert
	Host     string // address to listen on ("" = every network card)
	Port     int    // 0 picks a free port (tests)
	Domain   string // acme
	Email    string // acme, optional
	CertFile string // cert
	KeyFile  string // cert
}

// CertInfo describes the certificate in use.
type CertInfo struct {
	Names      []string `json:"names"`
	Issuer     string   `json:"issuer"`
	NotAfter   int64    `json:"notAfter"`
	DaysLeft   int      `json:"daysLeft"`
	SelfSigned bool     `json:"selfSigned"`
}

// HTTPSStatus is what Settings shows.
type HTTPSStatus struct {
	Mode      string    `json:"mode"`
	Running   bool      `json:"running"`
	Port      int       `json:"port"`
	Error     string    `json:"error,omitempty"`
	Obtaining bool      `json:"obtaining,omitempty"`
	Cert      *CertInfo `json:"cert,omitempty"`
}

// HTTPS owns the listener and certificates.
type HTTPS struct {
	handler  http.Handler
	cacheDir string // Let's Encrypt account and certificates
	log      *slog.Logger

	mu        sync.Mutex
	cfg       HTTPSConfig
	srv       *http.Server
	ln        net.Listener
	acme      *autocert.Manager
	file      *fileCert
	certErr   string
	obtaining bool
}

// NewHTTPS prepares an HTTPS port serving handler. Nothing opens until Apply.
func NewHTTPS(handler http.Handler, dataDir string, log *slog.Logger) *HTTPS {
	return &HTTPS{handler: handler, cacheDir: filepath.Join(dataDir, "certs", "acme"), log: log}
}

// Apply brings the port in line with cfg. Unchanged settings leave it running.
// A certificate that can't be used is reported, and the port stays closed.
func (h *HTTPS) Apply(cfg HTTPSConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cfg.Mode == "" {
		cfg.Mode = "off"
	}
	if cfg == h.cfg && (h.srv != nil || cfg.Mode == "off") {
		return nil
	}
	h.stopLocked(context.Background())
	h.cfg, h.acme, h.file, h.certErr = cfg, nil, nil, ""
	if cfg.Mode == "off" {
		return nil
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"h2", "http/1.1"}}
	switch cfg.Mode {
	case "acme":
		if err := os.MkdirAll(h.cacheDir, 0o700); err != nil {
			return h.failed(err)
		}
		h.acme = &autocert.Manager{
			Prompt:     autocert.AcceptTOS, // the owner agreed in Settings before this mode can be chosen
			Cache:      autocert.DirCache(h.cacheDir),
			HostPolicy: autocert.HostWhitelist(cfg.Domain),
			Email:      cfg.Email,
		}
		// Let's Encrypt proves the domain over this same port (TLS-ALPN-01).
		tc.NextProtos = append(tc.NextProtos, "acme-tls/1")
		m := h.acme
		tc.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			c, err := m.GetCertificate(hello)
			if hello.ServerName == cfg.Domain {
				h.noteCert(err)
			}
			return c, err
		}
	case "cert":
		f, err := loadFileCert(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return h.failed(err)
		}
		h.file = f
		tc.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return f.get() }
	default:
		return h.failed(fmt.Errorf("unknown HTTPS mode %q", cfg.Mode))
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)))
	if err != nil {
		return h.failed(fmt.Errorf("port %d can't be opened: %w", cfg.Port, err))
	}
	srv := &http.Server{
		Handler:           h.handler,
		TLSConfig:         tc,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(h.log.Handler(), slog.LevelDebug), // internet scanners fail handshakes all day
	}
	h.srv, h.ln = srv, ln
	go func() {
		if err := srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			h.log.Error("https", "err", err)
		}
	}()
	h.log.Info("HTTPS port open", "port", ln.Addr().(*net.TCPAddr).Port, "mode", cfg.Mode)
	return nil
}

func (h *HTTPS) failed(err error) error {
	h.certErr = err.Error()
	h.log.Warn("HTTPS is not available", "err", err)
	return err
}

func (h *HTTPS) noteCert(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err == nil {
		h.certErr = ""
		return
	}
	h.certErr = err.Error()
}

// Obtain asks Let's Encrypt for the certificate now, instead of waiting for the
// first visitor, so that Settings can report how it went. It runs in the
// background; Status follows it.
func (h *HTTPS) Obtain() {
	h.mu.Lock()
	cfg, m := h.cfg, h.acme
	if m == nil || h.obtaining {
		h.mu.Unlock()
		return
	}
	h.obtaining = true
	h.mu.Unlock()
	go func() {
		hello := &tls.ClientHelloInfo{
			ServerName:        cfg.Domain,
			SignatureSchemes:  []tls.SignatureScheme{tls.ECDSAWithP256AndSHA256},
			SupportedCurves:   []tls.CurveID{tls.CurveP256},
			SupportedPoints:   []uint8{0},
			CipherSuites:      []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
			SupportedVersions: []uint16{tls.VersionTLS13, tls.VersionTLS12},
		}
		_, err := m.GetCertificate(hello)
		h.noteCert(err)
		h.mu.Lock()
		h.obtaining = false
		h.mu.Unlock()
		if err != nil {
			h.log.Warn("Let's Encrypt certificate", "domain", cfg.Domain, "err", err)
		} else {
			h.log.Info("Let's Encrypt certificate ready", "domain", cfg.Domain)
		}
	}()
}

// Addr is the port's address (nil when closed).
func (h *HTTPS) Addr() net.Addr {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ln == nil {
		return nil
	}
	return h.ln.Addr()
}

// Running: the port is open.
func (h *HTTPS) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.srv != nil
}

// Status reports the port and its certificate. It never starts getting one.
func (h *HTTPS) Status(ctx context.Context) HTTPSStatus {
	h.mu.Lock()
	st := HTTPSStatus{Mode: h.cfg.Mode, Running: h.srv != nil, Port: h.cfg.Port, Error: h.certErr, Obtaining: h.obtaining}
	if st.Mode == "" {
		st.Mode = "off"
	}
	if h.ln != nil {
		st.Port = h.ln.Addr().(*net.TCPAddr).Port
	}
	m, f, domain := h.acme, h.file, h.cfg.Domain
	h.mu.Unlock()
	switch {
	case m != nil:
		if b, err := m.Cache.Get(ctx, domain); err == nil {
			st.Cert = certInfoPEM(b)
		}
	case f != nil:
		if c, err := f.get(); err == nil {
			st.Cert = certInfo(c.Leaf)
		}
	}
	return st
}

// Shutdown closes the port.
func (h *HTTPS) Shutdown(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopLocked(ctx)
	h.cfg = HTTPSConfig{}
}

func (h *HTTPS) stopLocked(ctx context.Context) {
	if h.srv == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := h.srv.Shutdown(cctx); err != nil {
		_ = h.srv.Close()
	}
	h.srv, h.ln = nil, nil
}

func certInfo(c *x509.Certificate) *CertInfo {
	if c == nil {
		return nil
	}
	info := &CertInfo{
		Names:      c.DNSNames,
		Issuer:     c.Issuer.CommonName,
		NotAfter:   c.NotAfter.Unix(),
		DaysLeft:   int(time.Until(c.NotAfter).Hours() / 24),
		SelfSigned: c.CheckSignatureFrom(c) == nil,
	}
	for _, ip := range c.IPAddresses {
		info.Names = append(info.Names, ip.String())
	}
	if len(info.Names) == 0 && c.Subject.CommonName != "" {
		info.Names = []string{c.Subject.CommonName}
	}
	if info.Issuer == "" {
		info.Issuer = strings.Join(c.Issuer.Organization, ", ")
	}
	return info
}

func certInfoPEM(b []byte) *CertInfo {
	for len(b) > 0 {
		var block *pem.Block
		block, b = pem.Decode(b)
		if block == nil {
			return nil
		}
		if block.Type == "CERTIFICATE" {
			c, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil
			}
			return certInfo(c)
		}
	}
	return nil
}

// fileCert is a certificate from files the owner looks after, such as those a
// renewal tool on the same machine rewrites. They are looked at again at most
// once a minute, so a renewed certificate is picked up without a restart.
type fileCert struct {
	certPath, keyPath string
	mu                sync.Mutex
	cert              *tls.Certificate
	stamp             time.Time
	checked           time.Time
}

func loadFileCert(certPath, keyPath string) (*fileCert, error) {
	f := &fileCert{certPath: certPath, keyPath: keyPath}
	if err := f.load(); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *fileCert) newest() (time.Time, error) {
	a, err := os.Stat(f.certPath)
	if err != nil {
		return time.Time{}, err
	}
	b, err := os.Stat(f.keyPath)
	if err != nil {
		return time.Time{}, err
	}
	if b.ModTime().After(a.ModTime()) {
		return b.ModTime(), nil
	}
	return a.ModTime(), nil
}

func (f *fileCert) load() error {
	stamp, err := f.newest()
	if err != nil {
		return err
	}
	c, err := tls.LoadX509KeyPair(f.certPath, f.keyPath)
	if err != nil {
		return fmt.Errorf("the certificate and key can't be used together: %w", err)
	}
	if c.Leaf == nil {
		if c.Leaf, err = x509.ParseCertificate(c.Certificate[0]); err != nil {
			return err
		}
	}
	f.cert, f.stamp = &c, stamp
	return nil
}

func (f *fileCert) get() (*tls.Certificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if time.Since(f.checked) >= time.Minute {
		f.checked = time.Now()
		if stamp, err := f.newest(); err == nil && !stamp.Equal(f.stamp) {
			// Half-written files during a renewal keep the old certificate.
			_ = f.load()
		}
	}
	return f.cert, nil
}

// ValidDomain checks a name Let's Encrypt could issue for.
func ValidDomain(d string) bool {
	if len(d) > 253 || !strings.Contains(d, ".") || net.ParseIP(d) != nil {
		return false
	}
	for _, label := range strings.Split(d, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
