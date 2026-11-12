// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package logging

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// Every string that reaches a record passes through here, so no call site has to
// remember. There are three nets: the name of the attribute, patterns in free
// text (addresses, command lines, headers, JSON) and the exact values of the
// secrets the server knows about.

const hidden = "[hidden]"

// Attribute names whose value is never kept. Compared in lower case, with the
// part after the last dot, and matched at the end of the name ("tmdbKey" is
// caught by apikey-like endings below, "userToken" by "token").
var secretEndings = []string{"token", "password", "passwd", "passphrase", "secret", "apikey", "apikeys", "api_key", "_key", "-key", "authorization", "cookie", "credential", "credentials", "privatekey", "private_key", "setupcode", "setup_code", "recoverycode", "recoverycodes", "recovery_code", "recovery_codes", "paircode", "pair_code", "pairingcode", "pairing_code", "bearer"}

var secretExact = map[string]bool{"pin": true, "st": true, "otp": true, "totp": true, "pass": true, "pwd": true, "sig": true, "signature": true, "key": true, "tmdbkey": true, "tvdbkey": true, "tvdbpin": true, "tdarrkey": true, "rawgkey": true, "tgdbkey": true, "accesskey": true, "secretkey": true, "signingkey": true,
	// Peer to peer, pairing and watch parties: the server's key, codes typed to join, a seat in a room.
	"p2pkey": true, "seed": true, "joincode": true, "join_code": true, "roomcode": true, "room_code": true, "member": true, "memberid": true, "member_id": true, "k": true}

func secretKey(k string) bool {
	k = strings.ToLower(k)
	if i := strings.LastIndexByte(k, '.'); i >= 0 {
		k = k[i+1:]
	}
	if secretExact[k] {
		return true
	}
	for _, s := range secretEndings {
		if strings.HasSuffix(k, s) {
			return true
		}
	}
	return false
}

// Each pattern says what any match must contain, so the regular expressions, which
// are slow, only run on text that could hold one. Words are checked in lower case.
var textPatterns = []struct {
	chars string   // needs all of these characters
	words []string // and one of these, if any
	re    *regexp.Regexp
	with  string
}{
	// In an address or a command line: token=..., ?st=..., password=...
	{"=", nil, regexp.MustCompile(`(?i)(\b(?:[a-z0-9_-]*(?:token|password|passwd|passphrase|secret|key)|auth|pwd|pass|pin|st|sig|signature|setup[_-]?code|recovery[_-]?code|pair(?:ing)?[_-]?code|otp|totp)=)[^&\s"'<>]+`), "${1}" + hidden},
	// Words too ordinary to hide everywhere, in an address's query, where they tend to be a login.
	{"=", []string{"?", "&"}, regexp.MustCompile(`(?i)([?&](?:sid|session|session[_-]?id|sign|hash|code|member|member[_-]?id)=)[^&\s"'<>]+`), "${1}" + hidden},
	// A peer-to-peer invite link's secret (wittymedia://p2p/<id>?s=...), and a pairing QR code's check (?k=...).
	{"=", []string{"wittymedia:"}, regexp.MustCompile(`(?i)(wittymedia://[^\s"'<>]*?[?&]s=)[^&\s"'<>]+`), "${1}" + hidden},
	{"=", []string{"pair"}, regexp.MustCompile(`(?i)([?&]k=)[^&\s"'<>]+`), "${1}" + hidden},
	// The same with its name percent-encoded: %74oken=... signs in just the same.
	{"%=", nil, regexp.MustCompile(`(?i)((?:%[0-9a-f]{2}|[a-z_-])*%[0-9a-f]{2}(?:%[0-9a-f]{2}|[a-z_-])*=)[^&\s"'<>]+`), "${1}" + hidden},
	// A header, as a line of text or in FFmpeg's -headers.
	{"", []string{"authorization", "cookie", "x-wms-token", "x-api-key", "x-witty-connect", "x-wms-pair-secret"}, regexp.MustCompile(`(?i)\b(proxy-authorization|authorization|set-cookie|cookie|x-wms-token|x-api-key|x-witty-connect|x-wms-pair-secret)(\s*[:=]\s*)[^\r\n"']+`), "${1}${2}" + hidden},
	{"", []string{"bearer"}, regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]+`), "${1}" + hidden},
	{"", []string{"basic "}, regexp.MustCompile(`\b(Basic\s+)[A-Za-z0-9+/]*[A-Z0-9+/][A-Za-z0-9+/]{10,}={0,2}`), "${1}" + hidden},
	// JSON: "password": "..."
	{`":`, nil, regexp.MustCompile(`(?i)("(?:[a-z0-9_-]*(?:token|password|passwd|passphrase|secret|api[_-]?key|apikey|private[_-]?key|secret[_-]?key|access[_-]?key|signing[_-]?key)|key|auth|credentials?|cookie|rawgkey|tgdbkey|tmdbkey|tvdbkey|tdarrkey|pin|setup[_-]?code|recovery[_-]?code|pair(?:ing)?[_-]?code)"\s*:\s*)"(?:[^"\\]|\\.)*"`), `${1}"` + hidden + `"`},
	// A two-factor code in a request's body. An error's "code" is a word, and is what a reader is after.
	{`":`, []string{`"code"`}, regexp.MustCompile(`(?i)("code"\s*:\s*)"[0-9]{4,10}"`), `${1}"` + hidden + `"`},
	// A code to join peer to peer (K7QM-2XHD) or a watch party (ABC234): capitals and digits, never a word.
	{`":`, []string{`code"`}, regexp.MustCompile(`((?i:"(?:code|join[_-]?code|room[_-]?code)")\s*:\s*)"[A-Z0-9]{4}-?[A-Z0-9]{2,4}"`), `${1}"` + hidden + `"`},
	// As a program says it on its own: "password: ...", "api key: ...".
	{":", []string{"password", "passwd", "passphrase", "token", "secret", "api key", "api_key", "api-key", "apikey"}, regexp.MustCompile(`(?i)\b((?:password|passwd|passphrase|token|secret|api[ _-]?key)\s*:\s*)[^\s"',;]+`), "${1}" + hidden},
	{`":[`, []string{"recovery"}, regexp.MustCompile(`(?i)("recovery[_-]?codes"\s*:\s*)\[[^\]]*\]`), `${1}"` + hidden + `"`},
	// A login in an address: rtsp://user:password@camera/, or a key alone: rtsp://key@camera/
	{":@", nil, regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s@]+@`), "${1}" + hidden + "@"},
	// A login in an IPTV address's path: http://host/live/user/password/1.ts
	{"://", []string{"/live/", "/movie/", "/series/", "/timeshift/"}, regexp.MustCompile(`(?i)(://[^/\s]+/(?:live|movie|series|timeshift)/)[^/\s"']+/[^/\s"']+/`), "${1}" + hidden + "/"},
	// A share link's token.
	{"/", []string{"/s/", "/share/"}, regexp.MustCompile(`(/(?:api/)?(?:s|share)/)[A-Za-z0-9_-]{8,}`), "${1}" + hidden},
	// The sign-in cookie and stream tokens, however they got there.
	{"", []string{"wms_token=", "st1."}, regexp.MustCompile(`(?i)\b(wms_token=|st1\.)[A-Za-z0-9._~-]+`), "${1}" + hidden},
}

// A long run of hex standing alone is what a session token looks like.
var longHex = regexp.MustCompile(`\b[a-fA-F0-9]{48,}\b`)

var (
	secretsMu sync.Mutex
	secrets   atomic.Pointer[[]string]
	kept      atomic.Pointer[[]string] // KeepSecret's: never pushed out by newer ones
)

// KeepSecret is AddSecret for a value that lives as long as the server: its own
// peer-to-peer key. AddSecret keeps the newest 64, and a busy day of invites
// would push the oldest out.
func KeepSecret(v string) {
	if len(v) < 6 {
		return
	}
	secretsMu.Lock()
	defer secretsMu.Unlock()
	cur := kept.Load()
	var next []string
	if cur != nil {
		if slices.Contains(*cur, v) {
			return
		}
		next = append(next, *cur...)
	}
	if len(next) >= 16 {
		next = next[1:]
	}
	next = append(next, v)
	kept.Store(&next)
}

// AddSecret makes sure this exact value never appears in the log, wherever it
// turns up: an API key, a setup code. Short values are ignored, as they would
// blot out ordinary words, and so are repeats.
func AddSecret(v string) {
	if len(v) < 6 {
		return
	}
	secretsMu.Lock()
	defer secretsMu.Unlock()
	cur := secrets.Load()
	var next []string
	if cur != nil {
		for _, s := range *cur {
			if s == v {
				return
			}
		}
		next = append(next, *cur...)
	}
	if len(next) >= 64 {
		next = next[1:] // the oldest; the patterns are still there
	}
	next = append(next, v)
	secrets.Store(&next)
}

// Redact removes secrets from free text (URLs, command lines, headers, errors).
func Redact(s string) string {
	if s == "" {
		return s
	}
	for _, list := range [...]*[]string{kept.Load(), secrets.Load()} {
		if list == nil {
			continue
		}
		for _, v := range *list {
			if strings.Contains(s, v) {
				s = strings.ReplaceAll(s, v, hidden)
			}
		}
	}
	var lower string
	for i := range textPatterns {
		p := &textPatterns[i]
		if !hasAll(s, p.chars) {
			continue
		}
		if p.words != nil {
			if lower == "" {
				lower = strings.ToLower(s)
			}
			if !hasAnyWord(lower, p.words) {
				continue
			}
		}
		s = p.re.ReplaceAllString(s, p.with)
	}
	if hasHexRun(s) {
		s = longHex.ReplaceAllString(s, hidden)
	}
	return s
}

func hasAll(s, chars string) bool {
	for i := 0; i < len(chars); i++ {
		if strings.IndexByte(s, chars[i]) < 0 {
			return false
		}
	}
	return true
}

func hasAnyWord(lower string, words []string) bool {
	for _, w := range words {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

func hasHexRun(s string) bool {
	if len(s) < 48 {
		return false
	}
	run := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' {
			if run++; run >= 48 {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}
