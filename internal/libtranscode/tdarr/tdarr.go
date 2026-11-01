// Package tdarr talks to someone's own Tdarr server over its HTTP API, which is documented
// at docs.tdarr.io. Tdarr is a separate program with its own licence. Nothing of it is
// included here, and this never downloads or installs it. See docs/LIBRARY-TRANSCODING.md.
//
// The API's answers aren't described in detail by its public reference, so they're read
// with care: an unexpected shape gives empty fields and never an error. Only the status
// call decides whether this is a Tdarr server at all.
package tdarr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Problems a person can do something about.
var (
	ErrBadAddress  = errors.New("that isn't an address this can use: it needs to start with http:// or https://, with no user name or password in it")
	ErrUnreachable = errors.New("nothing answered at that address. Check that Tdarr is running, and that the address and its server port (8266 by default) are right")
	ErrNotTdarr    = errors.New("something answered at that address, but it doesn't look like a Tdarr server")
	ErrKey         = errors.New("Tdarr refused the API key. Make one in Tdarr under Tools, API Keys")
	ErrRefused     = errors.New("Tdarr said no to that request")
)

const (
	maxAnswer = 16 << 20
	timeout   = 15 * time.Second
)

// Client is a connection to one Tdarr server. It holds nothing open.
type Client struct {
	base string
	key  string
	http *http.Client
}

// New checks an address and makes a client. Reaching a machine on the home network is the
// point, so private addresses are fine. The address where cloud machines keep their
// credentials isn't, and redirects aren't followed, so a key is only ever sent to the
// address the owner typed.
func New(address, key string) (*Client, error) {
	base, err := CleanAddress(address)
	if err != nil {
		return nil, err
	}
	return &Client{base: base, key: key, http: &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DialContext:           dialNoMetadata,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: timeout,
			MaxIdleConns:          1,
			IdleConnTimeout:       5 * time.Second,
		},
	}}, nil
}

// CleanAddress tidies an address typed by a person.
func CleanAddress(address string) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" || len(address) > 512 {
		return "", ErrBadAddress
	}
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", ErrBadAddress
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/"), nil
}

func dialNoMetadata(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 5 * time.Second, Control: nil}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var last error
	for _, ip := range ips {
		if ip.IP.IsLinkLocalUnicast() || ip.IP.IsMulticast() || ip.IP.IsUnspecified() {
			last = errors.New("that address can't be used")
			continue
		}
		c, err := d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return c, nil
		}
		last = err
	}
	if last == nil {
		last = errors.New("no addresses")
	}
	return nil, last
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return ErrBadAddress
	}
	req.Header.Set("Accept", "application/json")
	if rd != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.key != "" {
		req.Header.Set("x-api-key", c.key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrUnreachable
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer))
	if err != nil {
		return ErrUnreachable
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return ErrKey
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotTdarr
	case resp.StatusCode >= 300:
		return fmt.Errorf("%w (%d)", ErrRefused, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return ErrNotTdarr
	}
	return nil
}

// wrapped is how the API's POST requests carry what they say.
type wrapped struct {
	Data any `json:"data"`
}

// Status is what the server says about itself.
type Status struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	IsAuth  bool   `json:"isAuthenticated"`
	OS      string `json:"os"`
	Uptime  int64  `json:"uptime"`
}

// GetStatus asks the server how it is. It's how a connection is tested.
func (c *Client) GetStatus(ctx context.Context) (*Status, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v2/status", nil, &raw); err != nil {
		return nil, err
	}
	_, hasStatus := raw["status"]
	_, hasVersion := raw["version"]
	if !hasStatus && !hasVersion {
		return nil, ErrNotTdarr
	}
	st := &Status{Status: text(raw["status"]), Version: text(raw["version"]), OS: text(raw["os"])}
	if u, ok := raw["uptime"].(float64); ok {
		st.Uptime = int64(u)
	}
	return st, nil
}

// Node is a stock Tdarr node attached to that server.
type Node struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Paused  bool     `json:"paused"`
	Workers []Worker `json:"workers"`
}

// Worker is one job on a node. An idle worker has no file.
type Worker struct {
	ID      string  `json:"id"`
	File    string  `json:"file"`
	Percent float64 `json:"percent"`
	Mode    string  `json:"mode"`
	FPS     float64 `json:"fps"`
	ETA     string  `json:"eta"`
}

// GetNodes lists the nodes attached to the server and what they're doing.
func (c *Client) GetNodes(ctx context.Context) ([]Node, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v2/get-nodes", nil, &raw); err != nil {
		return nil, err
	}
	out := []Node{}
	for id, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		n := Node{ID: orText(text(m["_id"]), id), Name: orText(text(m["nodeName"]), id), Paused: m["nodePaused"] == true, Workers: []Worker{}}
		if workers, ok := m["workers"].(map[string]any); ok {
			for wid, w := range workers {
				wm, ok := w.(map[string]any)
				if !ok {
					continue
				}
				n.Workers = append(n.Workers, Worker{ID: wid, File: text(wm["file"]), Percent: num(wm["percentage"]), Mode: orText(text(wm["mode"]), text(wm["workerType"])), FPS: num(wm["fps"]), ETA: text(wm["ETA"])})
			}
		}
		sort.Slice(n.Workers, func(i, j int) bool { return n.Workers[i].ID < n.Workers[j].ID })
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Library is a library set up in Tdarr.
type Library struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Folder string `json:"folder"`
}

// GetLibraries lists Tdarr's libraries.
func (c *Client) GetLibraries(ctx context.Context) ([]Library, error) {
	var raw []map[string]any
	err := c.do(ctx, http.MethodPost, "/api/v2/cruddb", wrapped{map[string]any{"collection": "LibrarySettingsJSONDB", "mode": "getAll"}}, &raw)
	if err != nil {
		return nil, err
	}
	out := []Library{}
	for _, m := range raw {
		out = append(out, Library{ID: text(m["_id"]), Name: orText(text(m["name"]), text(m["_id"])), Folder: text(m["folder"])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ScanLibrary asks Tdarr to look through one of its libraries for files it hasn't seen.
func (c *Client) ScanLibrary(ctx context.Context, id, folder string) error {
	return c.do(ctx, http.MethodPost, "/api/v2/scan-files", wrapped{map[string]any{"scanConfig": map[string]any{"dbID": id, "arrayOrPath": folder, "mode": "scanFindNew"}}}, nil)
}

// SendFiles asks Tdarr to look at these files, by the names it knows them by.
func (c *Client) SendFiles(ctx context.Context, id string, paths []string) error {
	return c.do(ctx, http.MethodPost, "/api/v2/scan-files", wrapped{map[string]any{"scanConfig": map[string]any{"dbID": id, "arrayOrPath": paths, "mode": "scanFolderWatcher"}}}, nil)
}

func text(v any) string {
	switch s := v.(type) {
	case string:
		if len(s) > 2048 {
			return s[:2048]
		}
		return s
	case float64:
		return fmt.Sprint(s)
	}
	return ""
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		var f float64
		if _, err := fmt.Sscan(n, &f); err == nil {
			return f
		}
	}
	return 0
}

func orText(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Translate writes a path the way Tdarr knows it: the part the two have different names
// for replaced. A path that isn't under from can't be sent.
func Translate(path, from, to string) (string, bool) {
	if from == "" && to == "" {
		return path, true
	}
	p := strings.ReplaceAll(path, `\`, `/`)
	f := strings.TrimRight(strings.ReplaceAll(from, `\`, `/`), "/")
	if p != f && !strings.HasPrefix(p, f+"/") {
		return "", false
	}
	return strings.TrimRight(to, `/\`) + p[len(f):], true
}
