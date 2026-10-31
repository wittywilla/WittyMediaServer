// Command wittytuner-module is WittyTuner's network tuners and IPTV as a helper
// process, so a new version can reach a server without a new server. A process answers
// one question and leaves, so nothing is left running while no one is watching. It
// runs no other program, so drivers that need FFmpeg stay with the server, and the
// server watches every stream for encryption itself. See WittyModule for the protocol.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittytuner"
)

func main() {
	m, err := wittymodule.ParseManifest(wittytuner.ManifestJSON)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(os.Args) != 2 || os.Args[1] != wittymodule.HelperFlag {
		fmt.Fprintf(os.Stderr, "%s %s is a helper that WittyMediaServer starts by itself; there is nothing to do by hand.\n", m.Name, m.Version)
		os.Exit(2)
	}
	// Leaving when the server closes the pipe is what cancels whatever is under way.
	if err := wittymodule.ServeStream(os.Stdin, os.Stdout, wittymodule.Hello{Module: m.ID, Version: m.Version, API: m.API}, handle); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// env has no FFmpeg and no custom commands: a helper runs nothing else.
var env = wittytuner.Env{HTTP: &http.Client{Timeout: 30 * time.Second}, Stream: &http.Client{}}

func handle(method string, raw json.RawMessage) (any, func(io.Writer) error, error) {
	var c wittytuner.HelperCall
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, nil, &wittymodule.CallError{Code: "bad-call", Message: "the call is unreadable"}
	}
	ctx := context.Background()
	if method == "discover" {
		return nil, func(w io.Writer) error {
			return send(w, wittytuner.HelperDoc{Found: wittytuner.DiscoverNetwork(ctx, env), Done: true})
		}, nil
	}
	src, err := wittytuner.New(c.Driver, wittytuner.Config{Name: c.Name, Options: c.Options}, env)
	if err != nil {
		return nil, nil, coded(err)
	}
	switch method {
	case "channels":
		return nil, func(w io.Writer) error {
			list, err := src.Channels(ctx)
			return finish(w, src, list, err)
		}, nil
	case "scan":
		return nil, func(w io.Writer) error {
			list, err := src.Scan(ctx, wittytuner.ScanOptions{Source: c.Source}, func(p wittytuner.Progress) {
				_ = send(w, wittytuner.HelperDoc{Progress: &p})
			})
			return finish(w, src, list, err)
		}, nil
	case "open":
		if c.Channel == nil {
			return nil, nil, &wittymodule.CallError{Code: "bad-call", Message: "no channel was given"}
		}
		st, err := src.Open(ctx, *c.Channel)
		if err != nil {
			return nil, nil, coded(err)
		}
		if st.Reader == nil {
			return wittytuner.HelperOpen{Input: plainInput(st.Input)}, func(io.Writer) error { return nil }, nil
		}
		// The server guards the stream for itself, so it isn't looked through twice.
		r := io.Reader(st.Reader)
		if st.Guard != nil {
			r = st.Guard.Underneath()
		}
		return nil, func(w io.Writer) error {
			defer st.Close()
			_, err := io.Copy(w, r)
			return err
		}, nil
	}
	return nil, nil, &wittymodule.CallError{Code: "unknown-method", Message: fmt.Sprintf("this version has no %q", method)}
}

func send(w io.Writer, d wittytuner.HelperDoc) error { return json.NewEncoder(w).Encode(d) }

func finish(w io.Writer, src wittytuner.Source, list []wittytuner.Channel, err error) error {
	if err != nil {
		var ce *wittymodule.CallError
		errors.As(coded(err), &ce)
		return send(w, wittytuner.HelperDoc{Error: ce.Message, Code: ce.Code, Done: true})
	}
	info := src.Info()
	return send(w, wittytuner.HelperDoc{Channels: list, Info: &info, Done: true})
}

// plainInput leaves only the address: the server decides how FFmpeg opens anything.
func plainInput(input []string) []string {
	for i, a := range input {
		if a == "-i" && i+1 < len(input) {
			return []string{"-i", input[i+1]}
		}
	}
	return input
}

// coded names the errors the server acts on.
func coded(err error) error {
	code, msg := "", err.Error()
	var enc *wittytuner.EncryptedError
	switch {
	case errors.As(err, &enc):
		code, msg = "encrypted", enc.Why
	case errors.Is(err, wittytuner.ErrBusy):
		code = "busy"
	case errors.Is(err, fs.ErrPermission):
		code = "denied" // the helper's limits may be why, so the server tries itself
	}
	return &wittymodule.CallError{Code: code, Message: msg}
}
