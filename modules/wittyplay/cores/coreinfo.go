package cores

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/wittywilla/wittyplay/ipc"
)

// Info is what a core's .info file says. These are the small text files libretro
// publishes beside its cores. An administrator supplied this one, and it says what
// its author wanted to say, so nothing in it is believed beyond being shown.
type Info struct {
	DisplayName string     `json:"displayName,omitempty"`
	CoreName    string     `json:"coreName,omitempty"`
	System      string     `json:"system,omitempty"`
	License     string     `json:"license,omitempty"`
	Version     string     `json:"version,omitempty"`
	Extensions  []string   `json:"extensions,omitempty"`
	Firmware    []Firmware `json:"firmware,omitempty"`
	Notes       string     `json:"notes,omitempty"`
	HWRender    bool       `json:"hwRender,omitempty"` // it says it wants a graphics card
}

// Firmware is a BIOS file an info file names.
type Firmware struct {
	File     string `json:"file"`
	Desc     string `json:"desc,omitempty"`
	Required bool   `json:"required"`
	Present  bool   `json:"present"` // it's in this server's BIOS folder
}

// MaxInfoSize is more than any published info file.
const MaxInfoSize = 256 << 10

const (
	maxInfoFirmware = 64
	maxInfoValue    = 1000
)

// ParseInfo reads key = "value" lines. Anything else is skipped: these files have
// grown many fields over the years, and none of the rest matter here.
func ParseInfo(r io.Reader) Info {
	vals := map[string]string{}
	sc := bufio.NewScanner(io.LimitReader(r, MaxInfoSize))
	sc.Buffer(make([]byte, 0, 4096), MaxInfoSize)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		vals[strings.ToLower(strings.TrimSpace(k))] = v
	}
	info := Info{
		DisplayName: CleanText(vals["display_name"], 200), CoreName: CleanText(vals["corename"], 200), System: CleanText(vals["systemname"], 200),
		License: CleanText(vals["license"], 200), Version: CleanText(vals["display_version"], 64), Notes: CleanText(strings.ReplaceAll(vals["notes"], "|", " "), maxInfoValue),
	}
	info.Extensions = CleanExtensions(strings.Split(vals["supported_extensions"], "|"))
	info.HWRender = vals["hw_render"] == "true" || strings.TrimSpace(vals["required_hw_api"]) != ""
	n, _ := strconv.Atoi(vals["firmware_count"])
	for i := 0; i < n && i < maxInfoFirmware; i++ {
		p := "firmware" + strconv.Itoa(i) + "_"
		file := CleanText(vals[p+"path"], 200)
		if file == "" {
			continue
		}
		info.Firmware = append(info.Firmware, Firmware{File: file, Desc: CleanText(vals[p+"desc"], 200), Required: vals[p+"opt"] != "true"})
	}
	return info
}

// CleanText makes text from a core safe to store and show: valid, one line, no
// control or invisible characters, and a sensible length.
func CleanText(s string, limit int) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > limit {
		s = string(r[:limit])
	}
	return s
}

// CleanExtensions keeps what could be a file extension, without its dot.
func CleanExtensions(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, e := range in {
		e = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e), "."))
		if e == "" || len(e) > 16 || seen[e] || len(out) >= 128 {
			continue
		}
		ok := true
		for _, r := range e {
			ok = ok && ((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '+')
		}
		if ok {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

// CleanProbe cleans what a core reported about itself in its test-load.
func CleanProbe(p ipc.Probe) ipc.Probe {
	p.Name = CleanText(p.Name, 120)
	p.Version = CleanText(p.Version, 64)
	p.Extensions = CleanExtensions(p.Extensions)
	if p.Options < 0 || p.Options > 100000 {
		p.Options = 0
	}
	return p
}
