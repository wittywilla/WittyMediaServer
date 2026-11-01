package modules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// A store is an address someone typed, so everything fetched for one is held to the rules for
// that: https on the public internet, nothing on this machine or its network unless an admin
// said so for that one store, and no redirect that breaks those rules. Sizes are enforced
// while reading, whatever the answer claims.

const (
	storeFetchTimeout = 30 * time.Second // for one small file
	maxStoreKeyFile   = 256              // a base64 key and a line break
	maxStoreRedirects = 3
	maxStoreSig       = 4 << 10
)

// ErrStorePrivate: the address is on this machine or a private network, and the admin hasn't
// allowed that for this store.
var ErrStorePrivate = errors.New("that address is on this machine or your own network, which a store can't use unless you allow it")

// ErrStoreAddress: this isn't an address a store can be at.
type ErrStoreAddress struct{ Why string }

func (e *ErrStoreAddress) Error() string { return e.Why }

// storeFetcher fetches for one store.
type storeFetcher struct {
	client  *http.Client
	private bool // the admin allowed private addresses, and plain http with them
}

// localTransport is for a store an admin allowed on their own network. Addresses that are never
// a store are still refused: link-local reaches a cloud's metadata service, and the rest aren't
// anywhere at all.
var localTransport http.RoundTripper = &http.Transport{
	DialContext: (&net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		a, err := netip.ParseAddr(host)
		if err != nil {
			return err
		}
		a = a.Unmap()
		if a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
			return fmt.Errorf("refusing to connect to %s", host)
		}
		return nil
	}}).DialContext,
	TLSHandshakeTimeout:   15 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
	MaxIdleConnsPerHost:   2,
}

// newStoreFetcher makes the fetcher for a store. public must only reach the internet; the
// server passes the same transport it uses for feeds. A store on the admin's own network uses
// localTransport instead, which can be replaced by tests.
func newStoreFetcher(public http.RoundTripper, private bool) *storeFetcher {
	f := &storeFetcher{private: private}
	tr := public
	if private {
		tr = localTransport
	}
	f.client = &http.Client{Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxStoreRedirects {
			return errors.New("it redirected too many times")
		}
		if _, err := checkStoreURL(req.URL.String(), private); err != nil {
			return fmt.Errorf("it redirected somewhere it isn't allowed to: %w", err)
		}
		return nil
	}}
	return f
}

// privateHost says whether a host is plainly on this machine or a private network, before any
// connection. The transport has the last word, after names are looked up.
func privateHost(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".lan") || !strings.Contains(h, ".") && !strings.Contains(h, ":") {
		return true
	}
	a, err := netip.ParseAddr(strings.Trim(h, "[]"))
	if err != nil {
		return false
	}
	a = a.Unmap()
	return a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsUnspecified() || netip.MustParsePrefix("100.64.0.0/10").Contains(a)
}

// checkStoreURL says whether a store may be fetched from an address, in words for the admin.
func checkStoreURL(raw string, private bool) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 || strings.ContainsAny(raw, " \\\x00\r\n\t") {
		return nil, &ErrStoreAddress{"that isn't an address"}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, &ErrStoreAddress{"that isn't an address: it should look like https://example.com/modules/catalog.json"}
	}
	if u.User != nil {
		return nil, &ErrStoreAddress{"an address with a name or password in it isn't accepted"}
	}
	if u.Fragment != "" {
		return nil, &ErrStoreAddress{"an address with a # in it isn't accepted"}
	}
	local := privateHost(u.Hostname())
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && private && local:
	case u.Scheme == "http":
		return nil, &ErrStoreAddress{"a store's address has to start with https://. Plain http is only for a store on your own network, if you allow one"}
	default:
		return nil, &ErrStoreAddress{"a store's address has to start with https://"}
	}
	if local && !private {
		return nil, ErrStorePrivate
	}
	return u, nil
}

// get asks for a file. The caller closes the answer.
func (f *storeFetcher) get(ctx context.Context, raw string) (*http.Response, error) {
	if _, err := checkStoreURL(raw, f.private); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	res, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		if res.StatusCode == 404 {
			return nil, errStoreNotFound
		}
		return nil, fmt.Errorf("the store answered %s", res.Status)
	}
	return res, nil
}

// errStoreNotFound: nothing at that address.
var errStoreNotFound = errors.New("the store answered 404: there is nothing at that address")

// small reads a whole small file, refusing anything over its limit. Its context is limited by
// the time, too, so a store that trickles can't hold a check up.
func (f *storeFetcher) small(ctx context.Context, raw string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, storeFetchTimeout)
	defer cancel()
	res, err := f.get(ctx, raw)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.ContentLength > limit {
		return nil, &errStoreTooBig{limit}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, &errStoreTooBig{limit}
	}
	return b, nil
}

// open starts reading a large file. Its caller limits it and checks its hash.
func (f *storeFetcher) open(ctx context.Context, raw string) (io.ReadCloser, int64, error) {
	res, err := f.get(ctx, raw)
	if err != nil {
		return nil, 0, err
	}
	return res.Body, res.ContentLength, nil
}

type errStoreTooBig struct{ limit int64 }

func (e *errStoreTooBig) Error() string {
	return fmt.Sprintf("the file is larger than the %d KiB a store's file may be here", e.limit>>10)
}
