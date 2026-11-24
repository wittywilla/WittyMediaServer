// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/scrypt"
)

// webJoin is what an app at home does with a Connect a device code at a web address (Enter a
// code, or a QR code's h= and c=), as WittyMediaClient's WebJoin.kt and desktop/code.js do it and
// internal/server/api_connect.go answers: the code becomes two keys with scrypt; the app sends the
// first, the server proves it knows the code, bound to its identity key and both random numbers,
// and only then does the app prove it does. No peer-to-peer path is involved. It answers the
// sign-in token.
func webJoin(home, code, name string) (string, error) {
	code = cleanCode(code)
	if code == "" {
		return "", errors.New("not a code: eight letters and digits")
	}
	base := strings.TrimRight(home, "/") + "/api"
	c := &http.Client{Timeout: 20 * time.Second}
	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)
	t := time.Now()
	status, ident, err := postJSON(c, base+"/identity", map[string]any{"nonce": base64.StdEncoding.EncodeToString(nonce)})
	key, _ := ident["publicKey"].(string)
	step("web-identity", err == nil && status == 200 && key != "", map[string]any{"ms": time.Since(t).Milliseconds(), "at": home, "status": status, "err": errText(err)})
	if err != nil || key == "" {
		return "", fmt.Errorf("reaching the server at %s: HTTP %d %v", home, status, err)
	}
	out, err := scrypt.Key([]byte(code), []byte("wittymedia-web-join-v1"), 1<<15, 8, 1, 64)
	if err != nil {
		return "", err
	}
	find, pkey := hex.EncodeToString(out[:32]), out[32:]
	dn := make([]byte, 16)
	_, _ = rand.Read(dn)
	t = time.Now()
	status, hello, err := postJSON(c, base+"/connect/hello", map[string]any{"v": 1, "find": find, "nonce": base64.StdEncoding.EncodeToString(dn)})
	sn, _ := base64.StdEncoding.DecodeString(str(hello["nonce"]))
	said, _ := base64.StdEncoding.DecodeString(str(hello["proof"]))
	ok := err == nil && status == 200 && str(hello["publicKey"]) == key && len(sn) == 16 && hmac.Equal(said, joinProof(pkey, "server", dn, sn, key))
	step("web-hello", ok, map[string]any{"ms": time.Since(t).Milliseconds(), "status": status, "err": errText(err), "error": hello["error"]})
	if !ok {
		return "", fmt.Errorf("the server knowing and proving the code: HTTP %d %v %v", status, err, hello["error"])
	}
	t = time.Now()
	status, ans, err := postJSON(c, base+"/connect/answer", map[string]any{"v": 1, "find": find, "nonce": base64.StdEncoding.EncodeToString(dn),
		"serverNonce": base64.StdEncoding.EncodeToString(sn), "proof": base64.StdEncoding.EncodeToString(joinProof(pkey, "device", dn, sn, key)),
		"device": name, "platform": "android"})
	tok := str(ans["token"])
	step("web-answer", err == nil && status == 200 && tok != "", map[string]any{"ms": time.Since(t).Milliseconds(), "status": status, "err": errText(err), "error": ans["error"]})
	if tok == "" {
		return "", fmt.Errorf("signing in with the code: HTTP %d %v %v", status, err, ans["error"])
	}
	return tok, nil
}

func joinProof(key []byte, role string, deviceNonce, serverNonce []byte, identityKey string) []byte {
	m := hmac.New(sha256.New, key)
	for _, part := range [][]byte{[]byte("wittymedia web join 1"), []byte(role), deviceNonce, serverNonce, []byte(identityKey)} {
		var n [2]byte
		binary.BigEndian.PutUint16(n[:], uint16(len(part)))
		m.Write(n[:])
		m.Write(part)
	}
	return m.Sum(nil)
}

func cleanCode(code string) string {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	var b strings.Builder
	for _, ch := range strings.ToUpper(code) {
		switch ch {
		case ' ', '-', '\t':
			continue
		case 'O':
			ch = '0'
		case 'I', 'L':
			ch = '1'
		}
		if !strings.ContainsRune(alphabet, ch) {
			return ""
		}
		b.WriteRune(ch)
	}
	if b.Len() != 8 {
		return ""
	}
	return b.String()
}

func postJSON(c *http.Client, url string, body any) (int, map[string]any, error) {
	b, _ := json.Marshal(body)
	res, err := c.Post(url, "application/json", strings.NewReader(string(b)))
	if err != nil {
		return 0, map[string]any{}, err
	}
	defer res.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out, nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
