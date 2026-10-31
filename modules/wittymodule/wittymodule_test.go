package wittymodule

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func manifest(min, max string) *Manifest {
	return &Manifest{ID: "wittyphotos", Name: "WittyPhotos", Version: "0.2.0", Repo: "wittywilla/WittyPhotos", Delivery: DeliveryHelper,
		Entry: "wittyphotos-module", API: 1, MinServer: min, MaxServer: max}
}

// The table in docs/MODULES.md, "Version ranges, including the edges".
func TestSupportsEdges(t *testing.T) {
	for _, c := range []struct {
		server, min, max string
		ok               bool
	}{
		{"0.4.0", "0.4.0", "0.5.x", true},
		{"0.4.0-alpha", "0.4.0", "0.5.x", false},
		{"0.4.0-beta", "0.4.0-alpha", "0.5.x", true},
		{"0.4.0-alpha", "0.4.0-beta", "0.5.x", false},
		{"0.5.7", "0.4.0", "0.5.x", true},
		{"0.5.7-rc1", "0.4.0", "0.5.x", true},
		{"0.6.0-alpha", "0.4.0", "0.5.x", false},
		{"0.5.0-rc1", "0.4.0", "0.5.0", true},
		{"0.5.1", "0.4.0", "0.5.0", false},
		{"0.5.0", "0.4.0", "0.5.0-rc1", false},
		{"0.9.9", "0.4.0", "0.x", true},
		{"1.0.0-alpha", "0.4.0", "0.x", false},
		{"0.3.9", "0.4.0", "0.5.x", false},
		{"0.4.0-alpha-12-gabc123", "0.4.0-alpha", "0.5.x", true}, // git describe
		{"0.4.0+build7", "0.4.0", "0.4.0", true},
		{"aab7db6", "0.4.0", "0.5.x", false},
		{"dev", "0.4.0", "0.5.x", false},
	} {
		err := manifest(c.min, c.max).Supports(c.server, MinAPI, API)
		if (err == nil) != c.ok {
			t.Errorf("server %s, range %s to %s: got %v, want ok=%v", c.server, c.min, c.max, err, c.ok)
		}
		if err != nil && !strings.Contains(err.Error(), "WittyPhotos") {
			t.Errorf("not a plain explanation: %v", err)
		}
	}
}

func TestSupportsAPI(t *testing.T) {
	m := manifest("0.1.0", "0.x")
	m.API = 2
	if err := m.Supports("0.4.0", 1, 1); err == nil || !strings.Contains(err.Error(), "newer server") {
		t.Errorf("too new: %v", err)
	}
	m.API = 1
	if err := m.Supports("0.4.0", 2, 3); err == nil || !strings.Contains(err.Error(), "too old") {
		t.Errorf("too old: %v", err)
	}
	if err := m.Supports("0.4.0", 1, 3); err != nil {
		t.Errorf("in range: %v", err)
	}
}

func TestValidate(t *testing.T) {
	good := func() *Manifest { return manifest("0.4.0", "0.5.x") }
	if err := good().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Manifest){
		"max below min":   func(m *Manifest) { m.MaxServer = "0.3.x" },
		"exact below min": func(m *Manifest) { m.MaxServer = "0.3.9" },
		"no maximum":      func(m *Manifest) { m.MaxServer = "" },
		"lone x":          func(m *Manifest) { m.MaxServer = "x" },
		"bad series":      func(m *Manifest) { m.MaxServer = "0.y.x" },
		"bad minimum":     func(m *Manifest) { m.MinServer = "soon" },
		"bad version":     func(m *Manifest) { m.Version = "new" },
		"bad id":          func(m *Manifest) { m.ID = "../evil" },
		"entry with path": func(m *Manifest) { m.Entry = "../evil" },
		"no entry":        func(m *Manifest) { m.Entry = "" },
		"bad delivery":    func(m *Manifest) { m.Delivery = "script" },
		"bad repo":        func(m *Manifest) { m.Repo = "https://evil.example/x" },
		"no api":          func(m *Manifest) { m.API = 0 },
	} {
		m := good()
		change(m)
		if m.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestPrerelease(t *testing.T) {
	if !Prerelease("0.4.0-alpha") || Prerelease("0.4.0") || Prerelease("nonsense") {
		t.Error("Prerelease is wrong")
	}
}

func pipeHelper(t *testing.T, h Handler) *Conn {
	t.Helper()
	toR, toW := io.Pipe()
	fromR, fromW := io.Pipe()
	go func() { _ = Serve(toR, fromW, Hello{Module: "x", Version: "1.0.0", API: 1}, h); fromW.Close() }()
	t.Cleanup(func() { toW.Close() })
	c := NewConn(fromR, toW)
	hello, err := c.ReadHello()
	if err != nil || hello.Module != "x" || hello.API != 1 {
		t.Fatalf("hello: %+v %v", hello, err)
	}
	return c
}

func TestProtocol(t *testing.T) {
	c := pipeHelper(t, func(method string, params json.RawMessage) (any, []byte, error) {
		switch method {
		case "echo":
			return map[string]json.RawMessage{"got": params}, []byte("blob\nwith\nlines"), nil
		case "no":
			return nil, nil, &CallError{Code: "nope", Message: "no such thing"}
		case "panic":
			panic("bad file")
		}
		return nil, nil, errors.New("unknown")
	})
	var out struct {
		Got struct{ A int }
	}
	blob, err := c.Call("echo", map[string]int{"a": 7}, &out)
	if err != nil || out.Got.A != 7 || string(blob) != "blob\nwith\nlines" {
		t.Fatalf("echo: %+v %q %v", out, blob, err)
	}
	_, err = c.Call("no", nil, nil)
	var ce *CallError
	if !errors.As(err, &ce) || ce.Code != "nope" {
		t.Fatalf("no: %v", err)
	}
	if _, err = c.Call("panic", nil, nil); !errors.As(err, &ce) || ce.Code != "panic" {
		t.Fatalf("a panic must be an answer: %v", err)
	}
	if _, err = c.Call("echo", nil, nil); err != nil {
		t.Fatalf("still working after a panic: %v", err)
	}
}

// A helper can't make the server read or set aside more than the limits.
func TestHostileHelper(t *testing.T) {
	call := func(helperSays string) error {
		var sent bytes.Buffer
		_, err := NewConn(strings.NewReader(helperSays), &sent).Call("x", nil, nil)
		return err
	}
	var ce *CallError
	for name, said := range map[string]string{
		"garbage":    "not json\n",
		"wrong id":   `{"id":9}` + "\n",
		"huge blob":  `{"id":1,"blob":99999999999}` + "\n",
		"minus blob": `{"id":1,"blob":-1}` + "\n",
		"cut short":  `{"id":1,"blob":10}` + "\nabc",
		"long line":  strings.Repeat("a", MaxLine+10),
		"nothing":    "",
	} {
		if err := call(said); err == nil || errors.As(err, &ce) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNames(t *testing.T) {
	if got := ArchiveName("wittyphotos", "v0.2.0", "linux", "arm"); got != "wittyphotos-0.2.0-linux-armv7.tar.gz" {
		t.Error(got)
	}
	if got := ArchiveName("wittyphotos", "0.2.0", "windows", "amd64"); got != "wittyphotos-0.2.0-windows-amd64.zip" {
		t.Error(got)
	}
	if got := SumsName("wittyphotos", "0.2.0"); got != "wittyphotos-0.2.0-SHA256SUMS.txt" {
		t.Error(got)
	}
}
