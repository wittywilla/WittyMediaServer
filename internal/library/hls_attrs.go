package library

import "strings"

// HLSAttr is one attribute of an HLS tag.
type HLSAttr struct {
	Key   string // in capitals
	Value string // without its quotes
	Raw   string // as written
}

// ParseHLSAttrs reads KEY=value,KEY="value, with commas". A value can hold words that look like
// another attribute, such as IV="METHOD=NONE", so looking for those words in the line isn't enough.
func ParseHLSAttrs(s string) []HLSAttr {
	var out []HLSAttr
	for len(s) > 0 {
		s = strings.TrimLeft(s, " \t,")
		if s == "" {
			break
		}
		i := 0
		for i < len(s) && s[i] != '=' && s[i] != ',' {
			i++
		}
		key := strings.ToUpper(strings.TrimSpace(s[:i]))
		if i == len(s) || s[i] == ',' {
			out = append(out, HLSAttr{Key: key, Raw: s[:i]})
			s = s[i:]
			continue
		}
		j := i + 1
		var value string
		if j < len(s) && s[j] == '"' {
			end := strings.IndexByte(s[j+1:], '"')
			if end < 0 {
				value, j = s[j+1:], len(s)
			} else {
				value = s[j+1 : j+1+end]
				j += end + 2
			}
		} else {
			start := j
			for j < len(s) && s[j] != ',' {
				j++
			}
			value = s[start:j]
		}
		out = append(out, HLSAttr{Key: key, Value: value, Raw: s[:j]})
		s = s[j:]
	}
	return out
}

// HLSKeyProtection says why a key line means a stream can't be played, or "". Only METHOD=NONE, said
// plainly and with nothing else claiming otherwise, means no key.
func HLSKeyProtection(line string) string {
	_, rest, _ := strings.Cut(line, ":")
	attrs := ParseHLSAttrs(rest)
	none, encrypted, drm := false, false, false
	for _, a := range attrs {
		switch a.Key {
		case "METHOD":
			switch strings.ToUpper(a.Value) {
			case "NONE":
				none = true
			case "SAMPLE-AES", "SAMPLE-AES-CTR", "SAMPLE-AES-CENC":
				encrypted, drm = true, true
			default:
				encrypted = true
			}
		case "KEYFORMAT":
			drm = true
		}
	}
	switch {
	case none && !encrypted:
		return ""
	case drm:
		return "its HLS playlist uses DRM"
	}
	return "its HLS playlist is encrypted"
}
