// Package wittysunshine finds, pairs with and manages a gaming PC running Sunshine,
// the open-source game stream host. It talks to Sunshine over its network protocols
// only and contains none of its code, which is GPL-3.0: see docs/GAME-RIGS.md.
package wittysunshine

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Sunshine's ports, for the default base port. Every port moves with the base.
const (
	DefaultPort = 47989 // HTTP: identity and pairing
	offsetHTTPS = -5    // HTTPS: everything else, with a client certificate
	offsetWeb   = 1     // HTTPS: Sunshine's web interface
)

const (
	maxAnswer = 4 << 20 // an app list with many games
	maxImage  = 8 << 20 // box art
	// How long a question may take. A PC that doesn't answer is usually asleep, and
	// people are looking at a page while this is asked.
	questionTimeout = 5 * time.Second
)

// Rig is one PC running Sunshine. Cert is the certificate pinned at pairing: once it
// is set, nothing else is accepted on the encrypted ports, whatever the network says.
type Rig struct {
	Address  string
	Port     int // the base port; 0 means the default
	Identity *Identity
	Cert     []byte // PEM, from pairing
	Policy   Policy
}

// Why a rig didn't answer. Each of these can be explained to a person.
var (
	ErrRefused     = errors.New("the PC is on, but Sunshine isn't running on it")
	ErrNoAnswer    = errors.New("the PC isn't answering: it's asleep, off or not on this network")
	ErrCertChanged = errors.New("something other than the PC you paired with answered: its certificate is different")
	ErrNotPaired   = errors.New("the PC no longer trusts this server. Pair it again")
	ErrBadAnswer   = errors.New("the PC answered with something that isn't from Sunshine")
	ErrBlocked     = errors.New("that address isn't allowed")
	ErrBusy        = errors.New("another game is running on the PC")
)

// StatusError is a refusal from Sunshine, with its own message.
type StatusError struct {
	Code    int
	Message string
}

func (e *StatusError) Error() string { return fmt.Sprintf("Sunshine said: %s (%d)", e.Message, e.Code) }

func (r *Rig) base() int {
	if r.Port > 0 {
		return r.Port
	}
	return DefaultPort
}

// WebURL is Sunshine's own web interface on this PC.
func (r *Rig) WebURL() string {
	return "https://" + net.JoinHostPort(r.Address, strconv.Itoa(r.base()+offsetWeb))
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// dial applies the policy twice. The check while connecting catches a name that resolves
// somewhere it shouldn't. The one before is needed as well, since Go quietly turns the
// unspecified address into this machine before the first check could see it.
func (r *Rig) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if err := r.Policy.Allow(ip); err != nil {
			return nil, err
		}
	}
	d := &net.Dialer{Timeout: questionTimeout, Control: r.Policy.control}
	return d.DialContext(ctx, network, addr)
}

// plainClient is for the two things asked without encryption: who is this, and pairing.
// The pairing request can wait minutes for a person, so only connecting has a time limit.
func (r *Rig) plainClient() *http.Client {
	return &http.Client{Transport: &http.Transport{DialContext: r.dial, DisableKeepAlives: true}, CheckRedirect: noRedirect}
}

// pinnedClient only talks to the certificate pinned at pairing. Sunshine's certificate
// is self-signed, so the usual checks don't apply: the exact match below replaces them.
func (r *Rig) pinnedClient(sendIdentity bool, pin []byte) (*http.Client, error) {
	block, _ := pem.Decode(pin)
	if block == nil {
		return nil, ErrCertChanged
	}
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, // replaced by VerifyConnection
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 || subtle.ConstantTimeCompare(cs.PeerCertificates[0].Raw, block.Bytes) != 1 {
				return ErrCertChanged
			}
			return nil
		},
	}
	if sendIdentity {
		if r.Identity == nil {
			return nil, ErrNotPaired
		}
		cfg.Certificates = []tls.Certificate{r.Identity.cert}
	}
	return &http.Client{
		Transport:     &http.Transport{DialContext: r.dial, TLSClientConfig: cfg, TLSHandshakeTimeout: questionTimeout, DisableKeepAlives: true},
		CheckRedirect: noRedirect,
	}, nil
}

func (r *Rig) url(scheme string, port int, path string, q url.Values) string {
	u := url.URL{Scheme: scheme, Host: net.JoinHostPort(r.Address, strconv.Itoa(port)), Path: path}
	u.RawQuery = q.Encode()
	return u.String()
}

// params are what Sunshine expects with every question.
func (r *Rig) params() url.Values {
	q := url.Values{}
	if r.Identity != nil {
		q.Set("uniqueid", r.Identity.ID)
	} else {
		q.Set("uniqueid", "0123456789ABCDEF")
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	q.Set("uuid", hex.EncodeToString(b))
	return q
}

// explain turns a failed connection into a reason someone can act on.
func explain(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrCertChanged):
		return ErrCertChanged
	case errors.Is(err, ErrBlocked):
		return err
	case errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, syscall.ECONNREFUSED):
		return ErrRefused
	case errors.Is(err, context.DeadlineExceeded):
		return ErrNoAnswer
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return ErrNoAnswer
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "refused"): // Windows words this its own way
		return ErrRefused
	case strings.Contains(s, "tls:"), strings.Contains(s, "x509"), strings.Contains(s, "malformed HTTP"):
		return ErrBadAnswer
	}
	return ErrNoAnswer
}

func get(ctx context.Context, c *http.Client, u string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, explain(err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		return nil, ErrBadAnswer
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, explain(err)
	}
	if int64(len(body)) > limit {
		return nil, ErrBadAnswer
	}
	return body, nil
}

// answer is the envelope of every XML reply. A reply without a status code is not
// from Sunshine, whatever its outer element is called.
type answer struct {
	Code    int    `xml:"status_code,attr"`
	Message string `xml:"status_message,attr"`
}

func checkStatus(a answer) error {
	switch a.Code {
	case 200:
		return nil
	case 401:
		return ErrNotPaired
	case 0:
		return ErrBadAnswer
	}
	return &StatusError{Code: a.Code, Message: clip(a.Message, 200)}
}

// Info is what a PC says about itself.
type Info struct {
	Name        string `xml:"hostname" json:"name"`
	Version     string `xml:"appversion" json:"version"`
	UUID        string `xml:"uniqueid" json:"uuid"`
	MAC         string `xml:"mac" json:"-"`
	CurrentGame int    `xml:"currentgame" json:"currentGame"`
	State       string `xml:"state" json:"-"`
	Paired      int    `xml:"PairStatus" json:"-"`
}

// Hello asks over plain HTTP who this PC is. Nothing here can be trusted yet, and it
// says nothing that needs protecting: this is how a new rig is recognised.
func (r *Rig) Hello(ctx context.Context) (*Info, error) {
	ctx, cancel := context.WithTimeout(ctx, questionTimeout)
	defer cancel()
	body, err := get(ctx, r.plainClient(), r.url("http", r.base(), "/serverinfo", r.params()), maxAnswer)
	if err != nil {
		return nil, err
	}
	return parseInfo(body)
}

// Status asks a paired PC over its pinned connection, which also gives its MAC address
// and proves it is still the same PC, and that it still trusts this server.
func (r *Rig) Status(ctx context.Context) (*Info, error) {
	body, err := r.ask(ctx, "/serverinfo", r.params())
	if err != nil {
		return nil, err
	}
	return parseInfo(body)
}

func parseInfo(body []byte) (*Info, error) {
	var a struct {
		answer
		Info
	}
	if err := xml.Unmarshal(body, &a); err != nil {
		return nil, ErrBadAnswer
	}
	if err := checkStatus(a.answer); err != nil {
		return nil, err
	}
	if a.UUID == "" {
		return nil, ErrBadAnswer
	}
	a.Name = clip(a.Name, 128)
	a.Version = clip(a.Version, 64)
	a.UUID = clip(a.UUID, 64)
	return &a.Info, nil
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return strings.ToValidUTF8(s[:n], "")
	}
	return s
}

// ask makes one question over the pinned connection.
func (r *Rig) ask(ctx context.Context, path string, q url.Values) ([]byte, error) {
	if len(r.Cert) == 0 {
		return nil, ErrNotPaired
	}
	c, err := r.pinnedClient(true, r.Cert)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, questionTimeout)
	defer cancel()
	return get(ctx, c, r.url("https", r.base()+offsetHTTPS, path, q), maxAnswer)
}

// App is one thing a rig offers to run.
type App struct {
	ID    int    `xml:"ID" json:"id"`
	Title string `xml:"AppTitle" json:"title"`
}

// Apps lists what the PC offers. Titles come from the PC, so they are clipped here and
// only ever shown as text.
func (r *Rig) Apps(ctx context.Context) ([]App, error) {
	body, err := r.ask(ctx, "/applist", r.params())
	if err != nil {
		return nil, err
	}
	var a struct {
		answer
		Apps []App `xml:"App"`
	}
	if err := xml.Unmarshal(body, &a); err != nil {
		return nil, ErrBadAnswer
	}
	if err := checkStatus(a.answer); err != nil {
		return nil, err
	}
	out := a.Apps[:0]
	for _, app := range a.Apps {
		app.Title = clip(app.Title, 200)
		if app.Title != "" {
			out = append(out, app)
		}
	}
	return out, nil
}

// AppImage fetches an app's box art. The bytes come from another computer, so they must
// look like a PNG or JPEG, and a caller stores and serves them as nothing else.
func (r *Rig) AppImage(ctx context.Context, id int) (data []byte, ext string, err error) {
	if len(r.Cert) == 0 {
		return nil, "", ErrNotPaired
	}
	c, err := r.pinnedClient(true, r.Cert)
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*questionTimeout)
	defer cancel()
	q := r.params()
	q.Set("appid", strconv.Itoa(id))
	q.Set("AssetType", "2")
	q.Set("AssetIdx", "0")
	body, err := get(ctx, c, r.url("https", r.base()+offsetHTTPS, "/appasset", q), maxImage)
	if err != nil {
		return nil, "", err
	}
	switch {
	case len(body) > 8 && string(body[:8]) == "\x89PNG\r\n\x1a\n":
		return body, ".png", nil
	case len(body) > 3 && body[0] == 0xff && body[1] == 0xd8 && body[2] == 0xff:
		return body, ".jpg", nil
	}
	return nil, "", ErrBadAnswer
}

// Launch starts an app on the PC, for a Moonlight app to pick up with Resume. The stream
// keys sent here are thrown away, as this server never streams with them: a client that
// resumes sends its own. The PC's display settings are left alone (sops=0).
func (r *Rig) Launch(ctx context.Context, appID int) error {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	keyID := make([]byte, 3)
	if _, err := rand.Read(keyID); err != nil {
		return err
	}
	q := r.params()
	q.Set("appid", strconv.Itoa(appID))
	q.Set("mode", "1920x1080x60")
	q.Set("additionalStates", "1")
	q.Set("sops", "0")
	q.Set("rikey", hex.EncodeToString(key))
	q.Set("rikeyid", strconv.Itoa(int(keyID[0])<<16|int(keyID[1])<<8|int(keyID[2])))
	q.Set("localAudioPlayMode", "0")
	q.Set("surroundAudioInfo", "196610")
	q.Set("remoteControllersBitmap", "0")
	q.Set("gcmap", "0")
	body, err := r.ask(ctx, "/launch", q)
	if err != nil {
		return err
	}
	var a answer
	if err := xml.Unmarshal(body, &a); err != nil {
		return ErrBadAnswer
	}
	if a.Code == 400 && strings.Contains(a.Message, "already running") {
		return ErrBusy
	}
	return checkStatus(a)
}

// Quit ends whatever is running on the PC, and any stream of it.
func (r *Rig) Quit(ctx context.Context) error {
	body, err := r.ask(ctx, "/cancel", r.params())
	if err != nil {
		return err
	}
	var a answer
	if err := xml.Unmarshal(body, &a); err != nil {
		return ErrBadAnswer
	}
	return checkStatus(a)
}

// CertFingerprint is the SHA-256 of a pinned certificate, for showing to an admin.
func CertFingerprint(certPEM []byte) string {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return ""
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	return fingerprint(c)
}
