package library

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// SVG is a picture that can hold a program. A file from a library is sent only after
// this has been through it: every element and attribute that isn't on the lists goes,
// and with them scripts, event handlers, outside references, styles that can fetch,
// and foreign content. The response is sandboxed as well. Two locks, on purpose.

const (
	maxSVGInput = 16 << 20
	maxSVGOut   = 64 << 20 // an svgz is a ZIP bomb's natural home
	maxSVGDepth = 256
)

var svgElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "symbol": true, "use": true, "title": true, "desc": true, "metadata": false,
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true,
	"text": true, "tspan": true, "textpath": true, "lineargradient": true, "radialgradient": true, "stop": true, "pattern": true, "clippath": true, "mask": true, "marker": true,
	"filter": true, "fegaussianblur": true, "feoffset": true, "feblend": true, "fecolormatrix": true, "fecomposite": true, "feflood": true, "femerge": true, "femergenode": true,
	"switch": true, "view": true,
}

var svgAttributes = map[string]bool{
	"id": true, "class": false, "width": true, "height": true, "x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true, "cx": true, "cy": true, "r": true, "rx": true, "ry": true,
	"fx": true, "fy": true, "d": true, "points": true, "viewbox": true, "preserveaspectratio": true, "transform": true, "gradienttransform": true, "gradientunits": true,
	"patternunits": true, "patterntransform": true, "patterncontentunits": true, "maskunits": true, "maskcontentunits": true, "clippathunits": true, "clip-path": true, "clip-rule": true,
	"mask": true, "fill": true, "fill-opacity": true, "fill-rule": true, "stroke": true, "stroke-width": true, "stroke-opacity": true, "stroke-linecap": true, "stroke-linejoin": true,
	"stroke-dasharray": true, "stroke-dashoffset": true, "stroke-miterlimit": true, "opacity": true, "offset": true, "stop-color": true, "stop-opacity": true, "font-family": true,
	"font-size": true, "font-weight": true, "font-style": true, "text-anchor": true, "letter-spacing": true, "dominant-baseline": true, "dx": true, "dy": true, "rotate": true,
	"textlength": true, "xmlns": true, "version": true, "href": true, "markerwidth": true, "markerheight": true, "refx": true, "refy": true, "orient": true, "markerunits": true,
	"stddeviation": true, "in": true, "in2": true, "result": true, "mode": true, "type": true, "values": true, "operator": true, "flood-color": true, "flood-opacity": true,
	"display": true, "visibility": true, "marker-start": true, "marker-mid": true, "marker-end": true, "filter": true, "vector-effect": true, "shape-rendering": true, "color": true,
}

// A value that reaches outside: script or data links, anything with a scheme, and
// references other than to something in this file.
var svgUnsafeValue = regexp.MustCompile(`(?i)(javascript|vbscript|data)\s*:|expression\s*\(|@import|<|https?:|//`)

var svgLocalURL = regexp.MustCompile(`(?i)^\s*url\(\s*["']?#[\w.:-]+["']?\s*\)\s*$`)

// SanitizeSVG returns a picture that keeps its shapes and loses anything active.
func SanitizeSVG(src []byte) ([]byte, error) {
	if len(src) > maxSVGInput {
		return nil, errors.New("this picture is too large")
	}
	if bytes.HasPrefix(src, []byte("\x1F\x8B")) {
		zr, err := gzip.NewReader(bytes.NewReader(src))
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(zr, maxSVGOut+1))
		if err != nil {
			return nil, err
		}
		if len(b) > maxSVGOut {
			return nil, errors.New("this picture is too large")
		}
		src = b
	}
	d := xml.NewDecoder(bytes.NewReader(src))
	d.Strict = false
	var out bytes.Buffer
	depth := 0
	skip := 0
	seen := false
	var stack []string
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if seen { // what was read so far is still a picture
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if skip > 0 {
				skip++
				continue
			}
			name := strings.ToLower(t.Name.Local)
			if name == "a" {
				continue // a link goes; what it wraps is still the picture
			}
			if !svgElements[name] || t.Name.Space != "" && !strings.Contains(t.Name.Space, "svg") {
				skip = 1
				continue
			}
			if depth >= maxSVGDepth {
				return nil, errors.New("this picture is nested too deeply")
			}
			if depth == 0 && name != "svg" {
				return nil, errors.New("this isn't an SVG picture")
			}
			depth++
			seen = true
			stack = append(stack, name)
			out.WriteString("<" + name)
			if depth == 1 {
				out.WriteString(` xmlns="http://www.w3.org/2000/svg"`)
			}
			for _, a := range t.Attr {
				k := strings.ToLower(a.Name.Local)
				if !svgAttributes[k] || k == "xmlns" || (a.Name.Space != "" && a.Name.Space != "xlink" && !strings.Contains(a.Name.Space, "xlink")) {
					continue
				}
				v := a.Value
				if k == "href" && !strings.HasPrefix(strings.TrimSpace(v), "#") {
					continue
				}
				if strings.HasPrefix(strings.ToLower(strings.TrimSpace(v)), "url(") && !svgLocalURL.MatchString(v) {
					continue
				}
				if svgUnsafeValue.MatchString(v) {
					continue
				}
				out.WriteString(" " + k + `="`)
				_ = xml.EscapeText(&out, []byte(v))
				out.WriteString(`"`)
			}
			out.WriteString(">")
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			if len(stack) == 0 || strings.EqualFold(t.Name.Local, "a") {
				continue
			}
			out.WriteString("</" + stack[len(stack)-1] + ">")
			stack = stack[:len(stack)-1]
			depth--
		case xml.CharData:
			if skip == 0 && len(stack) > 0 {
				_ = xml.EscapeText(&out, t)
			}
		}
		if out.Len() > maxSVGOut {
			return nil, errors.New("this picture is too large")
		}
	}
	if !seen {
		return nil, errors.New("this isn't an SVG picture")
	}
	for i := len(stack) - 1; i >= 0; i-- {
		out.WriteString("</" + stack[i] + ">")
	}
	return out.Bytes(), nil
}

// SVGFile reads and sanitises an SVG file.
func SVGFile(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > maxSVGInput {
		return nil, errors.New("this picture is too large")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return SanitizeSVG(b)
}

var svgSizeRe = regexp.MustCompile(`^\s*([0-9.]+)`)

// svgSize reads a picture's size from its width and height, or its view box.
func svgSize(path string) (w, h int) {
	b, err := SVGFile(path)
	if err != nil {
		return 0, 0
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	tok, err := d.Token()
	if err != nil {
		return 0, 0
	}
	t, ok := tok.(xml.StartElement)
	if !ok {
		return 0, 0
	}
	read := func(v string) int {
		m := svgSizeRe.FindStringSubmatch(v)
		if m == nil {
			return 0
		}
		f, _ := strconv.ParseFloat(m[1], 64)
		return int(f)
	}
	var box string
	for _, a := range t.Attr {
		switch a.Name.Local {
		case "width":
			w = read(a.Value)
		case "height":
			h = read(a.Value)
		case "viewbox":
			box = a.Value
		}
	}
	if (w == 0 || h == 0) && box != "" {
		if f := strings.Fields(strings.ReplaceAll(box, ",", " ")); len(f) == 4 {
			w, h = read(f[2]), read(f[3])
		}
	}
	return w, h
}

// ---------------------------------------------------------------------------
// Motion photos: a clip stored after the picture, in one file
// ---------------------------------------------------------------------------

const (
	// EmbeddedMotion marks a picture's motion clip as being inside its own file.
	EmbeddedMotion = "embedded:"
	motionHintHead = 256 << 10
)

var motionHints = [][]byte{[]byte("MotionPhoto"), []byte("MicroVideo"), []byte("GCamera:Motion"), []byte("Camera:MotionPhoto")}

// embeddedMotion finds the clip a phone stores after a picture. It looks for the marks
// such a photo carries before reading more, so an ordinary photo costs one small read.
func embeddedMotion(path string, size int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, min(int64(motionHintHead), size))
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	hinted := false
	for _, h := range motionHints {
		if bytes.Contains(head, h) {
			hinted = true
			break
		}
	}
	if !hinted {
		return ""
	}
	// Whatever the file says about the offset, the clip starts with its own box.
	start := max(0, size-motionClipMax)
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	from := 0
	for from < len(buf) {
		i := bytes.Index(buf[from:], []byte("ftyp"))
		if i < 0 {
			return ""
		}
		at := from + i - 4
		if at >= 0 && at+12 <= len(buf) && start+int64(at) > 0 {
			brand := string(buf[at+8 : at+12])
			boxSize := int(buf[at])<<24 | int(buf[at+1])<<16 | int(buf[at+2])<<8 | int(buf[at+3])
			if boxSize >= 16 && boxSize <= 64 && (brand == "mp42" || brand == "isom" || brand == "mp41" || brand == "iso5" || brand == "iso6" || brand == "avc1" || brand == "qt  ") {
				off := start + int64(at)
				return EmbeddedMotion + strconv.FormatInt(off, 10) + ":" + strconv.FormatInt(size-off, 10)
			}
		}
		from += i + 4
	}
	return ""
}

// ParseEmbeddedMotion reads where a clip lies in its picture's file.
func ParseEmbeddedMotion(m string) (off, length int64, ok bool) {
	body, found := strings.CutPrefix(m, EmbeddedMotion)
	if !found {
		return 0, 0, false
	}
	a, b, found := strings.Cut(body, ":")
	if !found {
		return 0, 0, false
	}
	off, e1 := strconv.ParseInt(a, 10, 64)
	length, e2 := strconv.ParseInt(b, 10, 64)
	return off, length, e1 == nil && e2 == nil && off >= 0 && length > 0
}
