// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// An administrator's channels: where their own notifications go as well as the bell. Both are
// off until they're set up, and only ever reach what the administrator typed in. The SMTP
// server is the server's (one for everyone who sends email); each administrator gives their
// own address.

// SMTP is the mail server email goes through.
type SMTP struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// Security is "starttls" (the usual, port 587), "tls" (port 465) or "none", which is only
	// allowed for a mail server on this machine: a password never crosses a network in the clear.
	Security    string `json:"security"`
	User        string `json:"user"`
	From        string `json:"from"`
	Password    string `json:"password,omitempty"` // only ever written; reads say PasswordSet
	PasswordSet bool   `json:"passwordSet"`
}

// Channel is one administrator's own channels.
type Channel struct {
	EmailOn    bool   `json:"emailOn"`
	EmailTo    string `json:"emailTo"`
	WebhookOn  bool   `json:"webhookOn"`
	WebhookURL string `json:"webhookUrl"`
	// WebhookSecret signs each delivery (X-WittyMediaServer-Signature: sha256=…), so the
	// receiver can tell it came from here. Only ever written; reads say SecretSet.
	WebhookSecret string `json:"webhookSecret,omitempty"`
	SecretSet     bool   `json:"secretSet"`
}

// ErrBadChannel is a channel that can't be used as it's written.
var ErrBadChannel = errors.New("bad channel")

func bad(msg string) error { return fmt.Errorf("%w: %s", ErrBadChannel, msg) }

func noCRLF(s string) bool { return !strings.ContainsAny(s, "\r\n") }

// CheckSMTP says what's wrong with a mail server's settings, if anything.
func CheckSMTP(s SMTP) error {
	s.Host = strings.TrimSpace(s.Host)
	if s.Host == "" {
		return bad("the mail server's name is missing")
	}
	if len(s.Host) > 253 || strings.ContainsAny(s.Host, " /\\@:\r\n") && net.ParseIP(s.Host) == nil {
		return bad("the mail server's name isn't a name or an address")
	}
	if s.Port < 1 || s.Port > 65535 {
		return bad("the port must be between 1 and 65535")
	}
	switch s.Security {
	case "starttls", "tls":
	case "none":
		if !loopback(s.Host) {
			return bad("without encryption, only a mail server on this machine can be used")
		}
	default:
		return bad("security must be starttls, tls or none")
	}
	if s.From != "" {
		if _, err := mail.ParseAddress(s.From); err != nil || !noCRLF(s.From) {
			return bad("the From address isn't an email address")
		}
	}
	if !noCRLF(s.User) || !noCRLF(s.Password) || len(s.User) > 200 || len(s.Password) > 500 {
		return bad("the user name or password can't be used")
	}
	return nil
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// CheckChannel says what's wrong with an administrator's channels, if anything.
func CheckChannel(c Channel) error {
	if c.EmailTo != "" {
		a, err := mail.ParseAddress(c.EmailTo)
		if err != nil || !noCRLF(c.EmailTo) || a.Address == "" || len(c.EmailTo) > 254 {
			return bad("the email address isn't one")
		}
	}
	if c.EmailOn && c.EmailTo == "" {
		return bad("give an email address to send to")
	}
	if c.WebhookURL != "" {
		if err := checkWebhookURL(c.WebhookURL); err != nil {
			return err
		}
	}
	if c.WebhookOn && c.WebhookURL == "" {
		return bad("give the webhook's address")
	}
	if len(c.WebhookSecret) > 200 {
		return bad("the secret is too long")
	}
	return nil
}

func checkWebhookURL(raw string) error {
	if len(raw) > 500 || !noCRLF(raw) {
		return bad("the webhook's address is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return bad("the webhook's address must start with https:// or http://")
	}
	if u.User != nil {
		// It would sit in the settings and the log in plain sight. The secret signs instead.
		return bad("put no name or password in the webhook's address; use the secret instead")
	}
	return nil
}

// Sender keeps channels and sends notifications through them, one at a time, off whatever
// made them. A queue that is full drops what doesn't fit, with a line in the log.
type Sender struct {
	DB  DB
	Log *slog.Logger
	// MailServer reads the server's SMTP settings, with the password.
	MailServer func(ctx context.Context) SMTP
	// ServerName names this server in what's sent.
	ServerName func() string
	// HTTP sends webhooks. Nil: a client with short timeouts that doesn't follow redirects.
	HTTP *http.Client
	// Dial opens connections to the mail server. Nil: net.Dialer. For tests.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)

	once sync.Once
	q    chan job
}

type job struct {
	uid int64
	n   Note
}

const queueSize = 64

func (s *Sender) init() {
	s.once.Do(func() {
		s.q = make(chan job, queueSize)
		if s.Log == nil {
			s.Log = slog.Default()
		}
		if s.HTTP == nil {
			s.HTTP = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		}
	})
}

// Run sends what's queued until ctx ends.
func (s *Sender) Run(ctx context.Context) {
	s.init()
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-s.q:
			c, err := s.Channel(ctx, j.uid)
			if err != nil || (!c.EmailOn && !c.WebhookOn) {
				continue
			}
			if err := s.Deliver(ctx, c, j.n, false); err != nil {
				s.Log.Warn("a notification could not be sent on", "cat", "notify", "account", j.uid, "err", err)
			}
		}
	}
}

// Enqueue is Hub.Out: it never waits.
func (s *Sender) Enqueue(uid int64, n Note) {
	s.init()
	select {
	case s.q <- job{uid, n}:
	default:
		s.Log.Warn("too many notifications to send on at once; one was left in the app only", "cat", "notify", "account", uid)
	}
}

// Channel reads an administrator's channels, secrets included.
func (s *Sender) Channel(ctx context.Context, uid int64) (Channel, error) {
	var c Channel
	var emailOn, hookOn int
	err := s.DB.QueryRowContext(ctx, `SELECT email_on, email_to, webhook_on, webhook_url, webhook_secret FROM notify_channels WHERE user_id = ?`, uid).
		Scan(&emailOn, &c.EmailTo, &hookOn, &c.WebhookURL, &c.WebhookSecret)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, nil
	}
	c.EmailOn, c.WebhookOn, c.SecretSet = emailOn == 1, hookOn == 1, c.WebhookSecret != ""
	return c, err
}

// SetChannel saves an administrator's channels. An empty secret keeps the one there; keepSecret
// false with an empty secret removes it.
func (s *Sender) SetChannel(ctx context.Context, uid int64, c Channel, keepSecret bool) error {
	if err := CheckChannel(c); err != nil {
		return err
	}
	if c.WebhookSecret == "" && keepSecret {
		old, err := s.Channel(ctx, uid)
		if err != nil {
			return err
		}
		c.WebhookSecret = old.WebhookSecret
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO notify_channels(user_id, email_on, email_to, webhook_on, webhook_url, webhook_secret) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET email_on = excluded.email_on, email_to = excluded.email_to, webhook_on = excluded.webhook_on,
		webhook_url = excluded.webhook_url, webhook_secret = excluded.webhook_secret`,
		uid, b2i(c.EmailOn), strings.TrimSpace(c.EmailTo), b2i(c.WebhookOn), strings.TrimSpace(c.WebhookURL), c.WebhookSecret)
	return err
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Deliver sends n through the channels that are on, or with test through both that are
// filled in, and says what went wrong.
func (s *Sender) Deliver(ctx context.Context, c Channel, n Note, test bool) error {
	s.init()
	var errs []error
	if (c.EmailOn || test) && c.EmailTo != "" {
		if err := s.email(ctx, c.EmailTo, n); err != nil {
			errs = append(errs, fmt.Errorf("email: %w", err))
		}
	}
	if (c.WebhookOn || test) && c.WebhookURL != "" {
		if err := s.webhook(ctx, c, n, test); err != nil {
			errs = append(errs, fmt.Errorf("webhook: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (s *Sender) name() string {
	if s.ServerName != nil {
		if n := strings.TrimSpace(s.ServerName()); n != "" {
			return n
		}
	}
	return "WittyMediaServer"
}

// Payload is what a webhook is sent.
type Payload struct {
	Server string `json:"server"`
	Kind   Kind   `json:"kind"`
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	Link   string `json:"link,omitempty"`
	At     int64  `json:"at"`
	Test   bool   `json:"test,omitempty"`
}

func (s *Sender) webhook(ctx context.Context, c Channel, n Note, test bool) error {
	if err := checkWebhookURL(c.WebhookURL); err != nil {
		return err
	}
	body, _ := json.Marshal(Payload{Server: s.name(), Kind: n.Kind, Title: n.Title, Body: n.Body, Link: n.Link, At: n.At, Test: test})
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "WittyMediaServer-notify")
	if c.WebhookSecret != "" {
		m := hmac.New(sha256.New, []byte(c.WebhookSecret))
		m.Write(body)
		req.Header.Set("X-WittyMediaServer-Signature", "sha256="+hex.EncodeToString(m.Sum(nil)))
	}
	res, err := s.HTTP.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			return ue.Err // without the address, which may carry a token in its path
		}
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("the webhook answered %d", res.StatusCode)
	}
	return nil
}

// header keeps a header's value to one line.
func header(s string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(s) }

func (s *Sender) email(ctx context.Context, to string, n Note) error {
	if s.MailServer == nil {
		return errors.New("no mail server is set up")
	}
	cfg := s.MailServer(ctx)
	if err := CheckSMTP(cfg); err != nil {
		return errors.New("set up the mail server first")
	}
	toAddr, err := mail.ParseAddress(to)
	if err != nil {
		return bad("the email address isn't one")
	}
	from := cfg.From
	if from == "" {
		from = cfg.User
	}
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		return bad("give a From address in the mail server's settings")
	}
	fromAddr.Name = s.name()

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", header(fromAddr.String()))
	fmt.Fprintf(&msg, "To: %s\r\n", header(toAddr.String()))
	fmt.Fprintf(&msg, "Subject: %s\r\n", header(mime.QEncoding.Encode("utf-8", n.Title)))
	fmt.Fprintf(&msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	msg.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\nAuto-Submitted: auto-generated\r\n\r\n")
	text := n.Title + "\n"
	if n.Body != "" {
		text += "\n" + n.Body + "\n"
	}
	text += "\n— " + s.name() + "\n"
	msg.WriteString(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n"))

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.sendMail(ctx, cfg, fromAddr.Address, toAddr.Address, msg.Bytes())
}

func (s *Sender) sendMail(ctx context.Context, cfg SMTP, from, to string, msg []byte) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dial := s.Dial
	if dial == nil {
		d := &net.Dialer{Timeout: 15 * time.Second}
		dial = d.DialContext
	}
	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	if cfg.Security == "tls" {
		tc := tls.Client(conn, tlsCfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			conn.Close()
			return err
		}
		conn = tc
	}
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if cfg.Security == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("the mail server doesn't offer STARTTLS; choose TLS or another server")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return err
		}
	}
	if cfg.User != "" {
		// PlainAuth itself refuses to send a password without TLS, except to this machine.
		if err := c.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
