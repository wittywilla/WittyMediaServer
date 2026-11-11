// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ebook

import (
	"bytes"
	"encoding/base64"
	"html"
	"regexp"
	"strings"
	"sync"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/charmap"
)

// Only these elements survive, with only these attributes. Everything else is
// unwrapped, keeping its text, except the elements below whose contents go too.
var allowedTags = map[string]bool{
	"p": true, "br": true, "hr": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"b": true, "strong": true, "i": true, "em": true, "u": true, "s": true, "strike": true, "del": true, "ins": true, "small": true,
	"sub": true, "sup": true, "span": true, "div": true, "blockquote": true, "pre": true, "code": true, "q": true, "cite": true, "abbr": true,
	"ul": true, "ol": true, "li": true, "dl": true, "dt": true, "dd": true, "table": true, "thead": true, "tbody": true, "tfoot": true,
	"tr": true, "td": true, "th": true, "caption": true, "a": true, "img": true, "figure": true, "figcaption": true, "section": true, "article": true,
	"aside": true, "header": true, "footer": true, "center": true, "ruby": true, "rt": true, "rp": true,
}

// Their contents aren't text for a reader, and may be active.
var droppedTags = map[string]bool{
	"script": true, "style": true, "head": true, "title": true, "meta": true, "link": true, "object": true, "embed": true, "applet": true, "iframe": true, "frame": true,
	"frameset": true, "svg": true, "math": true, "form": true, "input": true, "button": true, "select": true, "textarea": true, "base": true, "audio": true, "video": true,
	"source": true, "track": true, "canvas": true, "template": true, "noscript": true, "marquee": true, "bgsound": true, "dialog": true,
}

var voidTags = map[string]bool{"br": true, "hr": true, "img": true}

var safeLinkRe = regexp.MustCompile(`^(https?://|mailto:)`)

// htmlOptions tell the sanitiser about the book it's working for.
type htmlOptions struct {
	// image resolves an img src to a stored image's name ("" drops the image).
	image func(src string) string
	// anchors keeps links inside the book: an href that starts with # stays.
	anchors bool
}

// sanitizeHTML parses markup however broken it is and writes back only what is
// allowed. The result is well-formed: every tag is closed and every value escaped.
func sanitizeHTML(markup string, o htmlOptions) string {
	if len(markup) > maxBookText {
		markup = markup[:maxBookText]
	}
	ctx := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: bodyAtom}
	nodes, err := xhtml.ParseFragment(strings.NewReader(markup), ctx)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, n := range nodes {
		writeNode(&b, n, o, 0)
	}
	return b.String()
}

// maxDepth bounds recursion: a file can nest a million spans.
const maxDepth = 256

func writeNode(b *strings.Builder, n *xhtml.Node, o htmlOptions, depth int) {
	if depth > maxDepth {
		return
	}
	switch n.Type {
	case xhtml.TextNode:
		b.WriteString(xmlText(n.Data))
		return
	case xhtml.ElementNode:
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeNode(b, c, o, depth+1)
		}
		return
	}
	tag := strings.ToLower(n.Data)
	if droppedTags[tag] {
		return
	}
	if !allowedTags[tag] {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeNode(b, c, o, depth+1)
		}
		return
	}
	var attrs strings.Builder
	switch tag {
	case "a":
		href := attr(n, "href")
		if safeLinkRe.MatchString(strings.ToLower(strings.TrimSpace(href))) {
			attrs.WriteString(` href="` + xmlText(strings.TrimSpace(href)) + `"`)
		} else if o.anchors && strings.HasPrefix(href, "#") && len(href) < 200 {
			attrs.WriteString(` href="` + xmlText(href) + `"`)
		}
	case "img":
		src := attr(n, "src")
		if src == "" {
			src = attr(n, "recindex")
			if src != "" {
				src = "recindex:" + src
			}
		}
		name := ""
		if o.image != nil && src != "" {
			name = o.image(src)
		}
		if name == "" {
			return // no picture: leave nothing behind
		}
		attrs.WriteString(` src="images/` + xmlText(name) + `" alt="` + xmlText(attr(n, "alt")) + `"`)
	case "td", "th":
		for _, k := range []string{"colspan", "rowspan"} {
			if v := attr(n, k); v != "" && len(v) <= 3 && allDigitsStr(v) {
				attrs.WriteString(` ` + k + `="` + v + `"`)
			}
		}
	}
	if id := attr(n, "id"); id != "" && o.anchors && len(id) < 100 {
		attrs.WriteString(` id="` + xmlText(id) + `"`)
	}
	if v := attr(n, "lang"); v != "" && len(v) < 20 {
		attrs.WriteString(` lang="` + xmlText(v) + `"`)
	}
	if voidTags[tag] {
		b.WriteString("<" + tag + attrs.String() + "/>")
		return
	}
	b.WriteString("<" + tag + attrs.String() + ">")
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeNode(b, c, o, depth+1)
	}
	b.WriteString("</" + tag + ">")
}

func allDigitsStr(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

func attr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// fromHTML reads a web page as a book. Pictures come along only when they are inside
// the page as data; a page's outside links for images never load.
func fromHTML(data []byte) (*Book, error) {
	text := decodeText(data)
	b := &Book{}
	if m := titleRe().FindStringSubmatch(text); m != nil {
		b.Title = strings.TrimSpace(html.UnescapeString(m[1]))
	}
	if m := metaAuthorRe().FindStringSubmatch(text); m != nil {
		b.Author = strings.TrimSpace(html.UnescapeString(m[1]))
	}
	body := sanitizeHTML(text, htmlOptions{image: func(src string) string { return b.dataImage(src) }, anchors: true})
	b.addChapter(b.Title, body)
	return b, nil
}

var (
	titleRe      = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`(?is)<title[^>]*>(.{0,500}?)</title>`) })
	metaAuthorRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?is)<meta\s+name=["']?author["']?\s+content=["']([^"']{0,300})["']`)
	})
)

// dataImage stores a picture carried in a data: address.
func (b *Book) dataImage(src string) string {
	if !strings.HasPrefix(src, "data:image/") {
		return ""
	}
	_, payload, ok := strings.Cut(src, ";base64,")
	if !ok || len(payload) > maxImage*2 {
		return ""
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil {
		return ""
	}
	return b.addImage(data)
}

// DecodeText is decodeText for other packages.
func DecodeText(data []byte) string { return decodeText(data) }

// decodeText reads text in whatever encoding it's in, as UTF-8: a byte order mark or
// a charset named in the page, else UTF-8 if it's valid, else Windows-1252, which is
// what old text files from Europe and America nearly always are.
func decodeText(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\xEF\xBB\xBF")):
		return strings.ToValidUTF8(string(data[3:]), "�")
	case bytes.HasPrefix(data, []byte("\xFF\xFE")), bytes.HasPrefix(data, []byte("\xFE\xFF")):
		return decodeUTF16(data)
	}
	head := data[:min(len(data), 1024)]
	if m := xmlEncodingRe.FindSubmatch(head); m != nil {
		if s, ok := decodeNamed(data, string(m[1])); ok {
			return s
		}
	}
	if m := metaCharsetRe.FindSubmatch(head); m != nil {
		if s, ok := decodeNamed(data, string(m[1])); ok {
			return s
		}
	}
	if isValidUTF8(data) {
		return string(data)
	}
	out, err := charmap.Windows1252.NewDecoder().Bytes(data)
	if err != nil {
		return strings.ToValidUTF8(string(data), "�")
	}
	return string(out)
}

var (
	xmlEncodingRe = regexp.MustCompile(`(?i)<\?xml[^>]*encoding=["']([a-zA-Z0-9_-]{1,40})["']`)
	metaCharsetRe = regexp.MustCompile(`(?i)<meta[^>]*charset=["']?([a-zA-Z0-9_-]{1,40})`)
)

func decodeNamed(data []byte, label string) (string, bool) {
	enc, name := charset.Lookup(label)
	if enc == nil {
		return "", false
	}
	if name == "utf-8" {
		return strings.ToValidUTF8(string(data), "�"), true
	}
	out, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return "", false
	}
	return string(out), true
}

func isValidUTF8(b []byte) bool { return strings.ToValidUTF8(string(b), "") == string(b) }

func decodeUTF16(b []byte) string {
	le := b[0] == 0xFF
	b = b[2:]
	var out strings.Builder
	out.Grow(len(b) / 2)
	var hi rune
	for i := 0; i+1 < len(b); i += 2 {
		var u rune
		if le {
			u = rune(b[i]) | rune(b[i+1])<<8
		} else {
			u = rune(b[i])<<8 | rune(b[i+1])
		}
		switch {
		case u >= 0xD800 && u < 0xDC00:
			hi = u
			continue
		case u >= 0xDC00 && u < 0xE000 && hi != 0:
			u = (hi-0xD800)<<10 + (u - 0xDC00) + 0x10000
		}
		hi = 0
		out.WriteRune(u)
	}
	return out.String()
}
