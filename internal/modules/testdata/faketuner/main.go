// A WittyTuner helper for the tests. It reads its manifest from beside itself, and the
// tuner's "behave" setting says how to misbehave.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittytuner"
)

func packets(n int, scrambled bool) []byte {
	p := bytes.Repeat([]byte{0x47}, 188)
	p[1], p[2], p[3] = 0x01, 0x00, 0x10
	if scrambled {
		p[3] |= 0x80
	}
	return bytes.Repeat(p, n)
}

func main() {
	exe, _ := os.Executable()
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), wittymodule.ManifestFile))
	if err != nil {
		os.Exit(9)
	}
	m, err := wittymodule.ParseManifest(raw)
	if err != nil {
		os.Exit(9)
	}
	if err := wittymodule.ServeStream(os.Stdin, os.Stdout, wittymodule.Hello{Module: m.ID, Version: m.Version, API: m.API}, handle); err != nil {
		os.Exit(1)
	}
}

func doc(d wittytuner.HelperDoc) func(io.Writer) error {
	return func(w io.Writer) error { return json.NewEncoder(w).Encode(d) }
}

func nothing(io.Writer) error { return nil }

func handle(method string, raw json.RawMessage) (any, func(io.Writer) error, error) {
	var c wittytuner.HelperCall
	_ = json.Unmarshal(raw, &c)
	behave := c.Options["behave"]
	switch behave {
	case "crash":
		os.Exit(4)
	case "denied":
		return nil, nil, &wittymodule.CallError{Code: "denied", Message: "not allowed"}
	case "garbage":
		return nil, func(w io.Writer) error { _, err := io.WriteString(w, "this is not the protocol"); return err }, nil
	}
	switch method {
	case "discover":
		return nil, doc(wittytuner.HelperDoc{Found: []wittytuner.Found{{Driver: "network", Name: "From the helper", Options: map[string]string{"address": "192.168.1.9"}}, {Driver: "command", Name: "A program"}}, Done: true}), nil
	case "channels", "scan":
		switch behave {
		case "dies-listing":
			return nil, func(io.Writer) error { os.Exit(5); return nil }, nil
		case "empty":
			return nil, doc(wittytuner.HelperDoc{Channels: []wittytuner.Channel{}, Done: true}), nil
		case "error-doc":
			return nil, doc(wittytuner.HelperDoc{Error: "the helper could not reach it", Done: true}), nil
		case "hang":
			return nil, func(io.Writer) error { time.Sleep(time.Hour); return nil }, nil
		case "progress-then-hang":
			return nil, func(w io.Writer) error {
				_ = json.NewEncoder(w).Encode(wittytuner.HelperDoc{Progress: &wittytuner.Progress{Percent: 40, Found: 2, Message: "going"}})
				time.Sleep(time.Hour)
				return nil
			}, nil
		}
		return nil, func(w io.Writer) error {
			if method == "scan" {
				if err := json.NewEncoder(w).Encode(wittytuner.HelperDoc{Progress: &wittytuner.Progress{Percent: 500, Found: -3, Message: "half"}}); err != nil {
					return err
				}
			}
			return doc(wittytuner.HelperDoc{Done: true, Info: &wittytuner.Info{Detail: "seen by the helper", MaxStreams: 3, GuideURL: c.Options["guideFromHelper"]},
				Channels: []wittytuner.Channel{
					{ID: "clear", Number: "1", Name: "Says clear", Locator: c.Options["url"], Checked: true, Logo: "file:///etc/passwd"},
					{ID: "protected", Number: "2", Name: "Says protected", Locator: c.Options["url"], Encrypted: "the device says so", Checked: true},
					{ID: "elsewhere", Number: "3", Name: "Somewhere else", Locator: "file:///etc/passwd"},
					{ID: "", Number: "4", Name: "No id", Locator: c.Options["url"]},
				}})(w)
		}, nil
	case "open":
		// Fields a server must not believe go along, as a bad version might send them.
		lies := map[string]any{"verified": true, "encrypted": ""}
		switch behave {
		case "scrambled":
			return lies, func(w io.Writer) error { _, err := w.Write(packets(5000, true)); return err }, nil
		case "clear":
			return lies, func(w io.Writer) error { _, err := w.Write(packets(5000, false)); return err }, nil
		case "forever":
			return nil, func(w io.Writer) error {
				for {
					if _, err := w.Write(packets(50, false)); err != nil {
						return err
					}
					time.Sleep(10 * time.Millisecond)
				}
			}, nil
		case "dies-midstream":
			return nil, func(w io.Writer) error {
				_, _ = w.Write(packets(100, false))
				os.Exit(6)
				return nil
			}, nil
		case "dies-opening":
			os.Exit(7)
		case "encrypted":
			return nil, nil, &wittymodule.CallError{Code: "encrypted", Message: "the device said so"}
		case "busy":
			return nil, nil, &wittymodule.CallError{Code: "busy", Message: "no tuner free"}
		case "input-plain":
			return map[string]any{"input": []string{"-i", c.Channel.Locator}, "verified": true}, nothing, nil
		case "input-empty":
			return map[string]any{"input": []string{}}, nothing, nil
		case "input-options":
			return map[string]any{"input": []string{"-protocol_whitelist", "file,crypto,http", "-i", c.Channel.Locator}}, nothing, nil
		case "input-file":
			return map[string]any{"input": []string{"-i", "file:///etc/passwd"}}, nothing, nil
		case "input-concat":
			return map[string]any{"input": []string{"-i", "concat:http://a/1|file:///etc/passwd"}}, nothing, nil
		case "input-crypto":
			return map[string]any{"input": []string{"-i", "crypto+http://example.test/x"}}, nothing, nil
		case "input-elsewhere":
			return map[string]any{"input": []string{"-i", "http://example.test/x"}}, nothing, nil
		}
		return nil, nil, &wittymodule.CallError{Code: "unknown", Message: "no behaviour named"}
	}
	return nil, nil, &wittymodule.CallError{Code: "unknown-method", Message: "no such thing"}
}
