package wittymoonlight

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The browser bridge is moonlight-web-stream, a separate GPL-3.0 program that takes
// Sunshine's stream and hands it to a browser over WebRTC. It is never linked or bundled:
// an admin asks for it, it comes from the project's own GitHub release, and it must match
// the digest recorded here. Everything else talks to it over HTTP on the loopback address.

const (
	BridgeVersion = "3.0.1"
	BridgeProject = "moonlight-web-stream"
	BridgeSource  = "https://github.com/MrCreativ3001/moonlight-web-stream"
	BridgeLicense = "GPL-3.0-or-later"
	bridgeBase    = BridgeSource + "/releases/download/v" + BridgeVersion + "/"
	bridgeMaxSize = 40 << 20
	// BridgeUserHeader names the signed-in person to the bridge. It trusts this from
	// anyone who reaches its port, which is why that port is on the loopback address only.
	BridgeUserHeader = "X-Wms-Rig-User"
	// BridgeDevice is how the bridge appears in Sunshine's list of paired devices.
	BridgeDevice = "WittyMediaServer browser play"
)

type bridgeAsset struct {
	name, sha256 string
	size         int64
}

// bridgeAssets are the builds the project publishes. Digests are from the release itself,
// so a release changed afterwards is refused rather than run.
var bridgeAssets = map[string]bridgeAsset{
	"windows/amd64": {"moonlight-web-x86_64-pc-windows-gnu.zip", "7680f8ebb547f6bf1ba7e2056794bdd10d849685d954167fecdec79f2f240b1f", 13611995},
	"linux/amd64":   {"moonlight-web-x86_64-unknown-linux-musl.tar.gz", "1d56239579f05852af7c0ac1ecb1e95c1a6c123799f4edab9fe6d155364a687e", 10585519},
	"linux/arm64":   {"moonlight-web-aarch64-unknown-linux-musl.tar.gz", "2f981862c222cdaecc484c699f06bae927abee16708535abeb48653be855fbec", 10231390},
	"darwin/amd64":  {"moonlight-web-x86_64-apple-darwin.tar.gz", "728a81c98045117f44f3191214b50e7485db677d99328cc8de06ddb0dc56e48a", 9645670},
	"darwin/arm64":  {"moonlight-web-aarch64-apple-darwin.tar.gz", "6f28c814ddfe3ffdef22f1e2f49c5b96722967a4b192eb28dd41e38c76422720", 9245115},
}

// BridgeAvailable says whether the project publishes a build for a platform.
func BridgeAvailable(goos, goarch string) bool {
	_, ok := bridgeAssets[goos+"/"+goarch]
	return ok
}

// Bridge is the program on disk, and the process while it runs.
type Bridge struct {
	Dir    string       // its own folder under the server's data
	Client *http.Client // for the download
	// Port range and addresses for WebRTC, from settings.
	UDPMin, UDPMax int
	PublicIP       string
	STUN           []string

	mu      sync.Mutex
	cmd     *exec.Cmd
	exited  chan struct{}
	port    int
	token   string // the bridge's own admin session; never leaves this process
	roleSet bool
}

func (b *Bridge) pkgDir() string { return filepath.Join(b.Dir, BridgeVersion, "package") }

func (b *Bridge) program() string {
	name := "web-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(b.pkgDir(), name)
}

// Installed reports whether this version is on disk.
func (b *Bridge) Installed() bool {
	st, err := os.Stat(b.program())
	return err == nil && !st.IsDir()
}

// Install downloads the release for this platform, checks its digest and unpacks it. Each
// step leaves whatever was there before alone until the last one.
func (b *Bridge) Install(ctx context.Context) error {
	asset, ok := bridgeAssets[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return fmt.Errorf("%s doesn't publish a build for this kind of server (%s/%s)", BridgeProject, runtime.GOOS, runtime.GOARCH)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bridgeBase+asset.name, nil)
	if err != nil {
		return err
	}
	c := b.Client
	if c == nil {
		c = http.DefaultClient
	}
	res, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't download it: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("couldn't download it: the release answered %s", res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, bridgeMaxSize+1))
	if err != nil {
		return fmt.Errorf("couldn't download it: %w", err)
	}
	return b.installFrom(data, asset)
}

func (b *Bridge) installFrom(data []byte, asset bridgeAsset) error {
	if int64(len(data)) != asset.size {
		return errors.New("the download isn't the size this release should be, so it wasn't used")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != asset.sha256 {
		return errors.New("the download doesn't match this release's digest, so it wasn't used")
	}
	if err := os.MkdirAll(b.Dir, 0o700); err != nil {
		return err
	}
	work, err := os.MkdirTemp(b.Dir, ".install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	if strings.HasSuffix(asset.name, ".zip") {
		err = unzipTo(data, work)
	} else {
		err = untarTo(data, work)
	}
	if err != nil {
		return err
	}
	final := filepath.Join(b.Dir, BridgeVersion)
	_ = os.RemoveAll(final)
	return os.Rename(work, final)
}

// Remove deletes the program. Its data folder goes too: the certificates it held stop
// mattering once a rig forgets them, and leaving keys behind helps nobody.
func (b *Bridge) Remove() error {
	b.Stop()
	return os.RemoveAll(b.Dir)
}

const maxUnpacked = 200 << 20

// safeJoin keeps an archive inside the folder it's unpacked to.
func safeJoin(root, name string) (string, error) {
	name = filepath.ToSlash(name)
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, ":") {
		return "", errors.New("the archive names a file outside its folder")
	}
	clean := filepath.Join(root, filepath.FromSlash(name))
	rel, err := filepath.Rel(root, clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("the archive names a file outside its folder")
	}
	return clean, nil
}

func untarTo(data []byte, root string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	total := int64(0)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := safeJoin(root, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			total += h.Size
			if total > maxUnpacked {
				return errors.New("the archive unpacks to far more than it should")
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			if err := writeFrom(p, tr, h.Size, mode); err != nil {
				return err
			}
		default:
			return errors.New("the archive has links or devices in it, which it shouldn't")
		}
	}
}

func unzipTo(data []byte, root string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	total := int64(0)
	for _, f := range zr.File {
		p, err := safeJoin(root, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 || !f.Mode().IsRegular() {
			return errors.New("the archive has links or devices in it, which it shouldn't")
		}
		total += int64(f.UncompressedSize64)
		if total > maxUnpacked {
			return errors.New("the archive unpacks to far more than it should")
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFrom(p, rc, int64(f.UncompressedSize64), 0o755)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeFrom(p string, r io.Reader, size int64, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.CopyN(f, r, size)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// config writes the bridge's settings. The two booleans have no defaults in its parser. The
// list of STUN servers is written out, even when empty, as otherwise it asks Google's and
// tells every browser to as well: nothing leaves the network unless an admin asked for it.
func (b *Bridge) config(port int) ([]byte, error) {
	ice := []map[string]any{}
	if len(b.STUN) > 0 {
		ice = append(ice, map[string]any{"urls": b.STUN})
	}
	webrtc := map[string]any{"ice_servers": ice, "include_loopback_candidates": false}
	if b.UDPMin > 0 && b.UDPMax >= b.UDPMin {
		webrtc["port_range"] = map[string]int{"min": b.UDPMin, "max": b.UDPMax}
	}
	if b.PublicIP != "" {
		webrtc["nat_1to1"] = map[string]any{"ips": []string{b.PublicIP}, "ice_candidate_type": "host"}
	}
	return json.MarshalIndent(map[string]any{
		"web_server": map[string]any{
			"bind_address":                    net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
			"first_login_create_admin":        true,
			"first_login_assign_global_hosts": true,
			"forwarded_header":                map[string]any{"username_header": BridgeUserHeader, "auto_create_missing_user": true},
		},
		"data_storage": map[string]any{
			"type": "json", "path": filepath.Join(b.Dir, "data", "data.json"),
			"session_expiration_check_interval": map[string]int{"secs": 300, "nanos": 0},
		},
		"webrtc":    webrtc,
		"moonlight": map[string]any{"pair_device_name": BridgeDevice, "default_http_port": 47989},
	}, "", " ")
}

// Port is where the running bridge listens, or 0.
func (b *Bridge) Port() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd == nil {
		return 0
	}
	return b.port
}

// Start runs the program if it isn't running, waits for it to answer, and sets it up. It
// runs with almost nothing in its environment, and its working folder is its own, as it
// looks for its web files there.
func (b *Bridge) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd != nil {
		select {
		case <-b.exited:
			b.cmd = nil
		default:
			return nil
		}
	}
	if !b.Installed() {
		return errors.New("browser play isn't installed")
	}
	port, err := freePort()
	if err != nil {
		return err
	}
	cfg, err := b.config(port)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(b.Dir, "data"), 0o700); err != nil {
		return err
	}
	cfgPath := filepath.Join(b.Dir, "data", "config.json")
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		return err
	}
	cmd := exec.Command(b.program(), "-c", cfgPath, "run")
	cmd.Dir = b.pkgDir()
	cmd.Env = bridgeEnv()
	cmd.SysProcAttr = dieWithParent()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("couldn't start browser play: %w", err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	b.cmd, b.exited, b.port = cmd, exited, port
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		if _, err := b.do(deadline, http.MethodGet, "/api/authenticate", nil, ""); err == nil {
			break
		}
		select {
		case <-exited:
			b.cmd = nil
			return errors.New("browser play stopped as soon as it started. Antivirus software may have blocked it: see the guide")
		case <-deadline.Done():
			b.stopLocked()
			return errors.New("browser play didn't start in time")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err := b.bootstrap(deadline); err != nil {
		b.stopLocked()
		return err
	}
	return nil
}

func bridgeEnv() []string {
	var env []string
	for _, k := range []string{"PATH", "SystemRoot", "TEMP", "TMP", "HOME", "USERPROFILE"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Stop ends the program. The port it held goes with it.
func (b *Bridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopLocked()
}

func (b *Bridge) stopLocked() {
	if b.cmd == nil {
		return
	}
	_ = b.cmd.Process.Kill()
	select {
	case <-b.exited:
	case <-time.After(5 * time.Second):
	}
	b.cmd, b.token, b.roleSet = nil, "", false
}

// do makes one request to the running bridge.
func (b *Bridge) do(ctx context.Context, method, path string, body any, session string) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(b.port))+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func expect(res *http.Response, err error, what string, out any) error {
	if err != nil {
		return fmt.Errorf("browser play: %s: %w", what, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("browser play: %s: %s", what, res.Status)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out)
}

// bootstrap signs in as the bridge's admin, whose password is generated here and kept in the
// bridge's own folder, and makes sure people arriving through the proxy can do little: the
// first sign-in made with no users creates the admin, so it happens before anyone is let in.
func (b *Bridge) bootstrap(ctx context.Context) error {
	pwFile := filepath.Join(b.Dir, "data", "admin.secret")
	pw, err := os.ReadFile(pwFile)
	if err != nil {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		pw = []byte(hex.EncodeToString(buf))
		if err := os.WriteFile(pwFile, pw, 0o600); err != nil {
			return err
		}
	}
	res, err := b.do(ctx, http.MethodPost, "/api/login", map[string]string{"name": bridgeAdmin, "password": string(pw)}, "")
	if err != nil {
		return fmt.Errorf("browser play: signing in: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("browser play: signing in: %s", res.Status)
	}
	for _, c := range res.Cookies() {
		if c.Name == "mlSession" {
			b.token = c.Value
		}
	}
	if b.token == "" {
		return errors.New("browser play: signing in gave no session")
	}
	var role struct {
		ID json.RawMessage `json:"id"`
	}
	if err := expect2(b, ctx, http.MethodGet, "/api/role/default", nil, &role); err != nil {
		return err
	}
	// Everyone but this server's own admin session: no adding hosts anywhere, no way around
	// the check on what's being launched, and H.264 or better within what the rig offers.
	patch := map[string]any{
		"id": role.ID, "ty": "User",
		"permissions": map[string]any{
			"allow_add_hosts": false, "maximum_bitrate_kbps": nil,
			"allow_codec_h264": true, "allow_codec_h265": true, "allow_codec_av1": true, "allow_hdr": true,
			"allow_transport_webrtc": true, "allow_transport_websockets": false,
		},
	}
	if err := expect2(b, ctx, http.MethodPatch, "/api/role", patch, nil); err != nil {
		return err
	}
	b.roleSet = true
	return nil
}

// bridgeAdmin is a name the proxy never sends, as everyone it names is "wms-u<number>".
const bridgeAdmin = "wms-bridge-admin"

func expect2(b *Bridge, ctx context.Context, method, path string, body, out any) error {
	res, err := b.do(ctx, method, path, body, b.token)
	return expect(res, err, method+" "+path, out)
}

// AddRig registers a PC with the bridge and returns its id there. Hosts belong to the
// bridge's admin until pairing is done, when PairRig shares it.
func (b *Bridge) AddRig(ctx context.Context, address string, port int) (json.RawMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	body := map[string]any{"address": address}
	if port > 0 {
		body["http_port"] = port
	}
	var out struct {
		Host struct {
			ID json.RawMessage `json:"host_id"`
		} `json:"host"`
	}
	if err := expect2(b, ctx, http.MethodPost, "/api/host", body, &out); err != nil {
		return nil, err
	}
	return out.Host.ID, nil
}

// PairRig starts the bridge's own pairing with a PC. The bridge makes the PIN, which comes
// back here as soon as it's known; done gets the result once someone has typed it.
func (b *Bridge) PairRig(ctx context.Context, hostID json.RawMessage) (pin string, done <-chan error, err error) {
	b.mu.Lock()
	token := b.token
	b.mu.Unlock()
	res, err := b.do(ctx, http.MethodPost, "/api/pair", map[string]any{"host_id": hostID}, token)
	if err != nil {
		return "", nil, err
	}
	if res.StatusCode >= 300 {
		res.Body.Close()
		return "", nil, fmt.Errorf("browser play: pairing: %s", res.Status)
	}
	rd := bufio.NewReader(io.LimitReader(res.Body, 1<<20))
	line, err := rd.ReadBytes('\n')
	if err != nil {
		res.Body.Close()
		return "", nil, fmt.Errorf("browser play: pairing: %w", err)
	}
	var first struct {
		PIN string `json:"Pin"`
	}
	if err := json.Unmarshal(line, &first); err != nil || first.PIN == "" {
		res.Body.Close()
		return "", nil, errors.New("browser play: pairing gave no PIN")
	}
	result := make(chan error, 1)
	go func() {
		defer res.Body.Close()
		line, err := rd.ReadBytes('\n')
		if err != nil && len(line) == 0 {
			result <- fmt.Errorf("browser play: pairing: %w", err)
			return
		}
		if strings.Contains(string(line), "PairError") {
			result <- errors.New("browser play couldn't pair with this PC. Check the PIN, or that this PC isn't already paired with it")
			return
		}
		if !strings.Contains(string(line), "Paired") {
			result <- errors.New("browser play gave an answer it shouldn't")
			return
		}
		// Shared with everyone who comes through the proxy, which limits them by game.
		err = expect2Locked(b, context.Background(), hostID)
		result <- err
	}()
	return first.PIN, result, nil
}

func expect2Locked(b *Bridge, ctx context.Context, hostID json.RawMessage) error {
	b.mu.Lock()
	token := b.token
	b.mu.Unlock()
	res, err := b.do(ctx, http.MethodPatch, "/api/host", map[string]any{"host_id": hostID, "change_owner": true, "owner": nil}, token)
	return expect(res, err, "sharing the PC", nil)
}

// ForgetRig removes a PC from the bridge. It can't take itself off the PC's list.
func (b *Bridge) ForgetRig(ctx context.Context, hostID json.RawMessage) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	q := url.Values{"host_id": {strings.Trim(string(hostID), `"`)}}
	return expect2(b, ctx, http.MethodDelete, "/api/host?"+q.Encode(), nil, nil)
}
