package gamemeta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// UserAgent says who is asking, as Wikimedia and others require.
var UserAgent = "WittyMediaServer (https://github.com/wittywilla/WittyMediaServer)"

// maxAnswer caps what is read from one answer. Lists of a whole console's games from
// Wikidata are the largest thing anyone sends.
const maxAnswer = 32 << 20

// gate spaces out requests to one service.
type gate struct {
	mu    sync.Mutex
	gap   time.Duration
	next  time.Time
	until time.Time // asked to stop until then
	why   string
}

func newGate(gap time.Duration) *gate { return &gate{gap: gap} }

func (g *gate) wait(ctx context.Context) error {
	g.mu.Lock()
	if until := g.until; time.Now().Before(until) {
		why := g.why
		g.mu.Unlock()
		return &RateLimited{Until: until, Why: why}
	}
	wait := time.Until(g.next)
	g.next = time.Now().Add(max(wait, 0) + g.gap)
	g.mu.Unlock()
	if wait <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// delay adds to the wait before the next request.
func (g *gate) delay(d time.Duration) {
	g.mu.Lock()
	g.next = g.next.Add(d)
	g.mu.Unlock()
}

func (g *gate) pause(d time.Duration, why string) *RateLimited {
	g.mu.Lock()
	defer g.mu.Unlock()
	until := time.Now().Add(d)
	if until.After(g.until) {
		g.until, g.why = until, why
	}
	return &RateLimited{Until: g.until, Why: g.why}
}

// Client asks one service for things, and never lets an address carrying a key reach
// an error message or a log.
type Client struct {
	HTTP *http.Client
	// Secrets are scrubbed from every error this client returns.
	Secrets []string
	gate    *gate
	name    string
	// meter counts spent requests, if the service has an allowance.
	meter func(int) error
}

// NewClient makes a client that leaves gap between requests.
func NewClient(name string, h *http.Client, gap time.Duration) *Client {
	if h == nil {
		h = http.DefaultClient
	}
	return &Client{HTTP: h, gate: newGate(gap), name: name}
}

// scrub takes secrets out of an error, and the address, which may hold them, along
// with it.
func (c *Client) scrub(err error) error {
	if err == nil {
		return nil
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ue.Err
		}
		err = errors.New(c.name + " did not answer")
	}
	msg := err.Error()
	for _, s := range c.Secrets {
		if s != "" && strings.Contains(msg, s) {
			msg = strings.ReplaceAll(msg, s, "***")
		}
	}
	if msg != err.Error() {
		return errors.New(msg)
	}
	return err
}

// Do sends a request, spacing it out from the last. A request that succeeds in being
// answered counts toward the service's allowance.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if err := c.gate.wait(req.Context()); err != nil {
		return nil, err
	}
	if c.meter != nil {
		if err := c.meter(1); err != nil {
			return nil, err
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", UserAgent)
	}
	res, err := c.HTTP.Do(req)
	return res, c.scrub(err)
}

// statusError turns a status that isn't success into the error that means.
func (c *Client) statusError(res *http.Response) error {
	switch res.StatusCode {
	case 401, 403:
		return ErrKey
	case 404:
		return ErrNotFound
	case 429, 420, 430:
		d := time.Minute
		if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s > 0 {
			d = time.Duration(min(s, 86400)) * time.Second
		}
		return c.gate.pause(d, res.Status)
	}
	return fmt.Errorf("%s answered %s", c.name, res.Status)
}

// Read sends a request and returns the body of a successful answer.
func (c *Client) Read(req *http.Request, limit int64) ([]byte, error) {
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, c.statusError(res)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit))
	return b, c.scrub(err)
}

// JSON sends a request and decodes the answer.
func (c *Client) JSON(req *http.Request, out any) error {
	req.Header.Set("Accept", "application/json")
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return c.statusError(res)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxAnswer)).Decode(out); err != nil {
		return c.scrub(fmt.Errorf("%s sent something that isn't JSON", c.name))
	}
	return nil
}

// Get is JSON for a plain address.
func (c *Client) Get(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return c.scrub(err)
	}
	return c.JSON(req, out)
}

var (
	htmlTagRe = regexp.MustCompile(`(?s)<[^>]*>`)
	blockRe   = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|/h[1-6])[^>]*>`)
	blankRe   = regexp.MustCompile(`[ \t]+`)
	breaksRe  = regexp.MustCompile(`\n{3,}`)
)

const maxText = 8000

// Text makes something a source sent safe and tidy to show: plain words with no
// markup or control characters, and not too much of them. React escapes it again;
// this keeps stray tags from showing up as text, and keeps pages a sensible size.
func Text(s string) string {
	s = blockRe.ReplaceAllString(s, "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	s = blankRe.ReplaceAllString(s, " ")
	s = breaksRe.ReplaceAllString(s, "\n\n")
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxText {
		s = strings.TrimSpace(string(r[:maxText])) + "…"
	}
	return s
}

// Line is Text for a single short field, such as a publisher's name.
func Line(s string) string {
	s = strings.Join(strings.Fields(Text(s)), " ")
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200])
	}
	return s
}

// lines cleans a list of short fields, dropping empties and repeats.
func lines(in []string, most int) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		s = Line(s)
		if k := strings.ToLower(s); s != "" && !seen[k] {
			seen[k] = true
			out = append(out, s)
			if len(out) >= most {
				break
			}
		}
	}
	return out
}

// WebAddress keeps an address only if it's a plain web one with no login in it.
func WebAddress(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return ""
	}
	return u.String()
}

func unixDate(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format("2006-01-02")
}

// cleanDate keeps a date only in a form we understand: YYYY, YYYY-MM or YYYY-MM-DD.
func cleanDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 10 {
		s = s[:10]
	}
	if firstYear(s) == 0 {
		return ""
	}
	for i, c := range s {
		if (i == 4 || i == 7) && c != '-' {
			return s[:4]
		}
		if i != 4 && i != 7 && (c < '0' || c > '9') {
			return s[:4]
		}
	}
	return s
}

// limitedBody reads an answer, up to what any answer should be.
func limitedBody(res *http.Response) []byte {
	b, _ := io.ReadAll(io.LimitReader(res.Body, maxAnswer))
	return b
}
