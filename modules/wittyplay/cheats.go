package wittyplay

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/wittywilla/wittyplay/ipc"
)

// Cheats are a person's own codes for a game, given to the core through its cheat
// interface. Nothing comes with this server. Whatever a person writes or imports is
// only ever a code for the core to read, so what it may look like is kept narrow.

// Limits on cheats.
const (
	MaxCheats     = 500
	MaxCheatCode  = 2048
	MaxCheatLabel = 120
	MaxCheatFile  = 2 << 20
)

// Cheat is one saved cheat code.
type Cheat struct {
	Label   string `json:"label"`
	Code    string `json:"code"`
	Enabled bool   `json:"enabled"`
}

func goodCheatCode(code string) bool {
	if code == "" || len(code) > MaxCheatCode {
		return false
	}
	for _, r := range code {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case strings.ContainsRune(" +-:;,.=?_", r):
		default:
			return false
		}
	}
	return true
}

// CleanCheats checks a list of cheats. It's all or nothing, with the first problem named.
func CleanCheats(list []Cheat) ([]Cheat, error) {
	if len(list) > MaxCheats {
		return nil, fmt.Errorf("that's too many cheats: %d at most", MaxCheats)
	}
	out := make([]Cheat, 0, len(list))
	for i, c := range list {
		c.Code = strings.TrimSpace(c.Code)
		c.Label = strings.TrimSpace(c.Label)
		if !goodCheatCode(c.Code) {
			return nil, fmt.Errorf("cheat %d: a code is letters, numbers and + - : ; , . = ? _ only, up to %d characters", i+1, MaxCheatCode)
		}
		if len(c.Label) > MaxCheatLabel || !goodOptionValueRunes(c.Label) {
			return nil, fmt.Errorf("cheat %d: its name is too long or has characters that can't be shown", i+1)
		}
		out = append(out, c)
	}
	return out, nil
}

func goodOptionValueRunes(v string) bool {
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// CheatsForCore is what a runner is given.
func CheatsForCore(list []Cheat) []ipc.Cheat {
	out := make([]ipc.Cheat, 0, len(list))
	for _, c := range list {
		out = append(out, ipc.Cheat{Code: c.Code, Enabled: c.Enabled})
	}
	return out
}

// ParseCheatFile reads a cheat list in the widely used "cheat0_desc = ..." text format.
// Only descriptions, codes and whether each is on are read. Anything else in the file,
// and any entry that isn't sound, is left out, and the count says how many.
func ParseCheatFile(b []byte) (list []Cheat, skipped int, err error) {
	if len(b) > MaxCheatFile {
		return nil, 0, errors.New("this file is too large to be a cheat list")
	}
	type entry struct {
		desc, code string
		on         bool
	}
	found := map[int]*entry{}
	top := -1
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 4096), 64<<10)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"`)
		rest, ok := strings.CutPrefix(key, "cheat")
		if !ok {
			continue
		}
		digits, field, ok := strings.Cut(rest, "_")
		if !ok {
			continue
		}
		n, e := strconv.Atoi(digits)
		if e != nil || n < 0 || n >= 100000 {
			continue
		}
		if len(found) >= MaxCheats && found[n] == nil {
			skipped++
			continue
		}
		en := found[n]
		if en == nil {
			en = &entry{}
			found[n] = en
		}
		if n > top {
			top = n
		}
		switch field {
		case "desc":
			en.desc = val
		case "code":
			en.code = val
		case "enable":
			en.on = val == "true"
		}
	}
	if sc.Err() != nil {
		return nil, 0, errors.New("this doesn't look like a cheat list: it has a very long line")
	}
	for i := 0; i <= top; i++ {
		en := found[i]
		if en == nil {
			continue
		}
		if !goodCheatCode(en.code) {
			skipped++
			continue
		}
		label := en.desc
		if len(label) > MaxCheatLabel {
			label = label[:MaxCheatLabel]
		}
		if !goodOptionValueRunes(label) {
			label = ""
		}
		list = append(list, Cheat{Label: label, Code: en.code, Enabled: en.on})
	}
	if len(list) == 0 && skipped == 0 {
		return nil, 0, errors.New("no cheats were found in this file")
	}
	return list, skipped, nil
}
