// Package fake is a stand-in for Sunshine, for tests. It speaks the same protocol from the
// server's side, including the certificate checks and the long wait for a PIN, and it can
// be told to misbehave. It is written from the protocol, not from Sunshine's code.
package fake

import (
	"crypto"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"math/big"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// App is one thing the fake offers.
type App struct {
	ID    int
	Title string
	Image []byte // nil means the same default picture for all, as Sunshine does
}

// Sunshine is a running fake.
type Sunshine struct {
	Name, UUID, MAC, Version string
	Apps                     []App

	// Misbehaviour, set by tests.
	WrongCertInPairing bool          // offers one certificate, signs with another: someone in the middle
	WrongHash          bool          // doesn't know the PIN
	HoldPairFor        time.Duration // how long a PIN may take before 408
	Garbage            bool          // answers something that isn't XML
	Huge               bool          // answers something endless
	HugeImage          bool
	NotImage           bool
	WebUser, WebPass   string

	mu      sync.Mutex
	base    int
	http    *http.Server
	https   *http.Server
	web     *http.Server
	lns     []net.Listener
	cert    tls.Certificate
	certPEM []byte
	x509    *x509.Certificate
	other   *rsa.PrivateKey
	paired  map[string]bool // certificate bytes
	pending map[string]*pairing
	current int
	launchs int
	cancels int
	waiting chan struct{}
	closed  bool
}

type pairing struct {
	id, name   string
	salt       []byte
	clientCert *x509.Certificate
	clientPEM  []byte
	pin        chan string
	key        []byte
	challenge  []byte
	secret     []byte
	hashSeen   []byte
	done       bool
}

// Start runs a fake on three free ports in Sunshine's arrangement (base-5, base, base+1).
func Start() (*Sunshine, error) {
	s := &Sunshine{
		Name: "Test Rig", UUID: "11111111-2222-3333-4444-555555555555", MAC: "aa:bb:cc:dd:ee:02", Version: "7.1.431.-1",
		Apps: []App{{ID: 881448767, Title: "Desktop"}, {ID: 1093255277, Title: "Steam Big Picture"}, {ID: 7, Title: "Space Game", Image: tinyPNG(1)}},
		WebUser: "admin", WebPass: "secret",
		paired: map[string]bool{}, pending: map[string]*pairing{}, waiting: make(chan struct{}, 8),
		HoldPairFor: time.Minute,
	}
	var err error
	if s.cert, s.certPEM, s.x509, err = selfSigned("Sunshine Gamestream Host"); err != nil {
		return nil, err
	}
	if s.other, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		return nil, err
	}
	// Three ports close together, away from the range the system hands out by itself:
	// Windows gives out neighbouring ports, which are often still closing.
	for try := 0; try < 200; try++ {
		base := 20000 + mrand.IntN(20000)
		var lns []net.Listener
		ok := true
		for _, p := range []int{base, base - 5, base + 1} {
			ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
			if err != nil {
				ok = false
				break
			}
			lns = append(lns, ln)
		}
		if !ok {
			for _, ln := range lns {
				ln.Close()
			}
			continue
		}
		s.base, s.lns = base, lns
		s.serve()
		return s, nil
	}
	return nil, fmt.Errorf("no free ports")
}

// Port is the base port, as a Rig wants it.
func (s *Sunshine) Port() int { return s.base }

// CertPEM is the certificate it serves.
func (s *Sunshine) CertPEM() []byte { return s.certPEM }

// Close stops it. Connections waiting on a PIN are dropped.
func (s *Sunshine) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	s.http.Close()
	s.https.Close()
	s.web.Close()
}

// WaitForPairing blocks until something has started to pair, or times out.
func (s *Sunshine) WaitForPairing(d time.Duration) bool {
	select {
	case <-s.waiting:
		return true
	case <-time.After(d):
		return false
	}
}

// TypePIN is someone typing a PIN into the web interface, for whatever is waiting.
func (s *Sunshine) TypePIN(pin string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.pending {
		if !p.done {
			p.done = true
			p.pin <- pin
			return true
		}
	}
	return false
}

// IsPaired reports whether a certificate in PEM form has paired.
func (s *Sunshine) IsPaired(certPEM []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	block, _ := pem.Decode(certPEM)
	return block != nil && s.paired[string(block.Bytes)]
}

// Forget is someone removing this client from Sunshine's list.
func (s *Sunshine) Forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paired = map[string]bool{}
}

// Running is what's running now.
func (s *Sunshine) Running() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// SetRunning pretends something else started a game.
func (s *Sunshine) SetRunning(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = id
}

// Counts: how many launches and cancels it has seen.
func (s *Sunshine) Counts() (launches, cancels int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.launchs, s.cancels
}

// PendingDevice pretends a Moonlight app is waiting to pair, for the web interface's list.
func (s *Sunshine) PendingDevice(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending["device-"+name] = &pairing{id: strings.Repeat("a", 32), name: name, pin: make(chan string, 1)}
}

func (s *Sunshine) serve() {
	plain := http.NewServeMux()
	plain.HandleFunc("GET /serverinfo", s.handleInfo(false))
	plain.HandleFunc("GET /pair", s.handlePair)
	s.http = &http.Server{Handler: plain}
	go s.http.Serve(s.lns[0])

	secure := http.NewServeMux()
	secure.HandleFunc("GET /serverinfo", s.requirePaired(s.handleInfo(true)))
	secure.HandleFunc("GET /applist", s.requirePaired(s.handleApps))
	secure.HandleFunc("GET /appasset", s.requirePaired(s.handleAsset))
	secure.HandleFunc("GET /launch", s.requirePaired(s.handleLaunch))
	secure.HandleFunc("GET /cancel", s.requirePaired(s.handleCancel))
	secure.HandleFunc("GET /pair", s.requirePaired(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<root status_code="200"><paired>1</paired></root>`)
	}))
	s.https = &http.Server{Handler: secure, TLSConfig: &tls.Config{Certificates: []tls.Certificate{s.cert}, ClientAuth: tls.RequestClientCert}}
	go s.https.ServeTLS(s.lns[1], "", "")

	web := http.NewServeMux()
	web.HandleFunc("/api/pin", s.handleWebPin)
	s.web = &http.Server{Handler: web, TLSConfig: &tls.Config{Certificates: []tls.Certificate{s.cert}}}
	go s.web.ServeTLS(s.lns[2], "", "")
}

func (s *Sunshine) requirePaired(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		ok := r.TLS != nil && len(r.TLS.PeerCertificates) > 0 && s.paired[string(r.TLS.PeerCertificates[0].Raw)]
		s.mu.Unlock()
		if !ok {
			fmt.Fprint(w, `<root status_code="401" status_message="The client is not authorized. Certificate verification failed."/>`)
			return
		}
		h(w, r)
	}
}

func (s *Sunshine) handleInfo(secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Garbage {
			fmt.Fprint(w, "<html>this is a printer</html>")
			return
		}
		if s.Huge {
			for range 6000 {
				if _, err := w.Write(make([]byte, 1024)); err != nil {
					return
				}
			}
			return
		}
		s.mu.Lock()
		cur := s.current
		s.mu.Unlock()
		mac, status := "00:00:00:00:00:00", 0
		if secure {
			mac, status = s.MAC, 1
		}
		state := "SUNSHINE_SERVER_FREE"
		if cur > 0 {
			state = "SUNSHINE_SERVER_BUSY"
		}
		fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><root status_code="200"><hostname>%s</hostname><appversion>%s</appversion><GfeVersion>3.23.0.74</GfeVersion><uniqueid>%s</uniqueid><HttpsPort>%d</HttpsPort><ExternalPort>%d</ExternalPort><mac>%s</mac><LocalIP>127.0.0.1</LocalIP><ServerCodecModeSupport>1</ServerCodecModeSupport><PairStatus>%d</PairStatus><currentgame>%d</currentgame><state>%s</state></root>`,
			esc(s.Name), s.Version, s.UUID, s.base-5, s.base, mac, status, cur, state)
	}
}

func (s *Sunshine) handleApps(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><root status_code="200">`)
	for _, a := range s.Apps {
		fmt.Fprintf(&b, `<App><IsHdrSupported>0</IsHdrSupported><AppTitle>%s</AppTitle><ID>%d</ID></App>`, esc(a.Title), a.ID)
	}
	b.WriteString(`</root>`)
	fmt.Fprint(w, b.String())
}

func (s *Sunshine) handleAsset(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("appid"))
	w.Header().Set("Content-Type", "image/png")
	switch {
	case s.HugeImage:
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
		_, _ = w.Write(make([]byte, 9<<20))
		return
	case s.NotImage:
		_, _ = w.Write([]byte("<svg onload=alert(1)>"))
		return
	}
	for _, a := range s.Apps {
		if a.ID == id && a.Image != nil {
			_, _ = w.Write(a.Image)
			return
		}
	}
	_, _ = w.Write(tinyPNG(0))
}

func (s *Sunshine) handleLaunch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for _, need := range []string{"rikey", "rikeyid", "localAudioPlayMode", "appid"} {
		if q.Get(need) == "" {
			fmt.Fprint(w, `<root status_code="400" status_message="Missing a required launch parameter"><resume>0</resume></root>`)
			return
		}
	}
	id, _ := strconv.Atoi(q.Get("appid"))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current > 0 {
		fmt.Fprint(w, `<root status_code="400" status_message="An app is already running on this host"><resume>0</resume></root>`)
		return
	}
	found := false
	for _, a := range s.Apps {
		found = found || a.ID == id
	}
	if !found {
		fmt.Fprint(w, `<root status_code="404" status_message="Cannot find requested application"><gamesession>0</gamesession></root>`)
		return
	}
	s.current = id
	s.launchs++
	fmt.Fprintf(w, `<root status_code="200"><sessionUrl0>rtsp://127.0.0.1:%d</sessionUrl0><gamesession>1</gamesession></root>`, s.base+21)
}

func (s *Sunshine) handleCancel(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = 0
	s.cancels++
	fmt.Fprint(w, `<root status_code="200"><cancel>1</cancel></root>`)
}

func (s *Sunshine) handleWebPin(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	if !ok || user != s.WebUser || pass != s.WebPass {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		type entry struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		list := []entry{}
		s.mu.Lock()
		for _, p := range s.pending {
			if !p.done {
				list = append(list, entry{p.id, p.name})
			}
		}
		s.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"pairings": list})
	case http.MethodPost:
		var b struct {
			ID   string `json:"pairing_id"`
			PIN  string `json:"pin"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		status := false
		s.mu.Lock()
		for k, p := range s.pending {
			if p.id == b.ID && !p.done {
				status = b.PIN == "4321" // the fake's device always shows this PIN
				delete(s.pending, k)
				break
			}
		}
		s.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]bool{"status": status})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Sunshine) handlePair(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("uniqueid")
	fail := func(code int, msg string) {
		fmt.Fprintf(w, `<root status_code="%d" status_message="%s"><paired>0</paired></root>`, code, esc(msg))
	}
	switch {
	case q.Get("phrase") == "getservercert":
		certPEM, err := hex.DecodeString(q.Get("clientcert"))
		if err != nil {
			fail(400, "bad certificate")
			return
		}
		block, _ := pem.Decode(certPEM)
		if block == nil {
			fail(400, "bad certificate")
			return
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			fail(400, "bad certificate")
			return
		}
		salt, err := hex.DecodeString(q.Get("salt"))
		if err != nil || len(salt) != 16 {
			fail(400, "Salt too short")
			return
		}
		p := &pairing{id: strings.Repeat("b", 32), name: q.Get("devicename"), salt: salt, clientCert: c, clientPEM: certPEM, pin: make(chan string, 1)}
		s.mu.Lock()
		if old := s.pending[id]; old != nil && old.key == nil && !old.done {
			s.mu.Unlock()
			fail(409, "A pairing session with this uniqueid already exists")
			return
		}
		s.pending[id] = p
		s.mu.Unlock()
		select {
		case s.waiting <- struct{}{}:
		default:
		}
		var pin string
		select {
		case pin = <-p.pin:
		case <-time.After(s.HoldPairFor):
			fail(408, "Pairing session expired")
			return
		case <-r.Context().Done():
			return
		}
		sum := sha256.Sum256(append(append([]byte{}, salt...), pin...))
		s.mu.Lock()
		p.key = sum[:16]
		s.mu.Unlock()
		fmt.Fprintf(w, `<root status_code="200"><paired>1</paired><plaincert>%s</plaincert></root>`, hex.EncodeToString(s.certPEM))
	case q.Get("clientchallenge") != "":
		p := s.session(id)
		if p == nil {
			fail(400, "Invalid uniqueid")
			return
		}
		enc, err := hex.DecodeString(q.Get("clientchallenge"))
		if err != nil {
			fail(400, "bad challenge")
			return
		}
		challenge := ecb(p.key, enc, false)
		p.secret = make([]byte, 16)
		_, _ = rand.Read(p.secret)
		h := sha256.New()
		h.Write(challenge)
		h.Write(s.x509.Signature)
		h.Write(p.secret)
		sum := h.Sum(nil)
		if s.WrongHash {
			sum[0] ^= 0xff
		}
		p.challenge = make([]byte, 16)
		_, _ = rand.Read(p.challenge)
		out := ecb(p.key, append(sum, p.challenge...), true)
		fmt.Fprintf(w, `<root status_code="200"><paired>1</paired><challengeresponse>%s</challengeresponse></root>`, hex.EncodeToString(out))
	case q.Get("serverchallengeresp") != "":
		p := s.session(id)
		if p == nil {
			fail(400, "Invalid uniqueid")
			return
		}
		enc, err := hex.DecodeString(q.Get("serverchallengeresp"))
		if err != nil {
			fail(400, "bad response")
			return
		}
		p.hashSeen = ecb(p.key, enc, false)
		key := s.cert.PrivateKey.(*rsa.PrivateKey)
		if s.WrongCertInPairing {
			key = s.other
		}
		digest := sha256.Sum256(p.secret)
		sig, err := key.Sign(rand.Reader, digest[:], crypto.SHA256)
		if err != nil {
			fail(500, err.Error())
			return
		}
		fmt.Fprintf(w, `<root status_code="200"><paired>1</paired><pairingsecret>%s</pairingsecret></root>`, hex.EncodeToString(append(append([]byte{}, p.secret...), sig...)))
	case q.Get("clientpairingsecret") != "":
		p := s.session(id)
		if p == nil {
			fail(400, "Invalid uniqueid")
			return
		}
		raw, err := hex.DecodeString(q.Get("clientpairingsecret"))
		if err != nil || len(raw) <= 16 {
			fail(400, "bad secret")
			return
		}
		secret, sig := raw[:16], raw[16:]
		h := sha256.New()
		h.Write(p.challenge)
		h.Write(p.clientCert.Signature)
		h.Write(secret)
		good := string(h.Sum(nil)) == string(p.hashSeen)
		digest := sha256.Sum256(secret)
		pub, ok := p.clientCert.PublicKey.(*rsa.PublicKey)
		if !ok || rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) != nil {
			good = false
		}
		s.mu.Lock()
		delete(s.pending, id)
		if good {
			s.paired[string(p.clientCert.Raw)] = true
		}
		s.mu.Unlock()
		if good {
			fmt.Fprint(w, `<root status_code="200"><paired>1</paired></root>`)
		} else {
			fmt.Fprint(w, `<root status_code="200"><paired>0</paired></root>`)
		}
	default:
		fail(400, "Invalid pairing request")
	}
}

func (s *Sunshine) session(id string) *pairing {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending[id]
}

func ecb(key, data []byte, encrypt bool) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	out := make([]byte, len(data))
	for i := 0; i+aes.BlockSize <= len(data); i += aes.BlockSize {
		if encrypt {
			block.Encrypt(out[i:], data[i:])
		} else {
			block.Decrypt(out[i:], data[i:])
		}
	}
	return out
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func selfSigned(cn string) (tls.Certificate, []byte, *x509.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(1, 0, 0), IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), c, nil
}

// tinyPNG returns a small picture whose bytes differ by n.
func tinyPNG(n byte) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), n, 'p', 'n', 'g')
}
