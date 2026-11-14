// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/wittywilla/wittymodule/service"
)

const tunerID = "wittytuner"

const (
	maxTunerText = 1000
	maxLocator   = 8 << 10
	// maxChannelList is how much a list of every channel may take.
	maxChannelList = 64 << 20
)

// Tuners reaches WittyTuner's service on its own routes (/tv/...), which only the server can call,
// with the service's secret: live TV's channels and streams (livetv.Remote). What it says is
// read by livetv, which holds it to what the guide can take and every stream to its own guard.
type Tuners struct{ M *Manager }

// TunerRefusal is WittyTuner saying no: busy, encrypted, not-found, no-archive, unreachable, or
// another reason in words.
type TunerRefusal struct{ Code, Message string }

func (e *TunerRefusal) Error() string {
	if e.Message == "" {
		return "WittyTuner refused"
	}
	return e.Message
}

// RefusalCode is the refusal's code, for packages that can't name TunerRefusal.
func (e *TunerRefusal) RefusalCode() string { return e.Code }

func (t Tuners) conn() (*service.Conn, error) {
	if t.M == nil {
		return nil, (*Manager)(nil).missing(tunerID)
	}
	conn, err := t.M.ServiceConn(tunerID)
	if err != nil {
		var miss *ErrMissing
		if errors.As(err, &miss) {
			return nil, err
		}
		return nil, fmt.Errorf("WittyTuner failed: %w", err)
	}
	return conn, nil
}

// Call asks WittyTuner's service one thing, as JSON both ways; an answer that isn't 200 is a
// *TunerRefusal. Without WittyTuner in use, it's an *ErrMissing.
func (t Tuners) Call(ctx context.Context, method, path string, in, out any) error {
	conn, err := t.conn()
	if err != nil {
		return err
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, conn.Base.JoinPath(path).String(), body)
	if err != nil {
		return err
	}
	req.Header.Set(service.TokenHeader, conn.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := conn.Client.Do(req)
	if err != nil {
		return fmt.Errorf("WittyTuner failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		var e struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&e)
		return &TunerRefusal{Code: e.Code, Message: clip(e.Error)}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, maxChannelList)).Decode(out)
}

// Stream is an opened channel's MPEG-TS, from its slot, for as long as ctx lasts or until it's
// closed. Reading it through the server's own guard is the caller's part.
func (t Tuners) Stream(ctx context.Context, slot string) (io.ReadCloser, error) {
	conn, err := t.conn()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", conn.Base.JoinPath("/tv/stream", url.PathEscape(slot)).String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(service.TokenHeader, conn.Token)
	res, err := conn.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("WittyTuner failed: %w", err)
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, fmt.Errorf("WittyTuner's stream answered %s", res.Status)
	}
	return res.Body, nil
}

func clip(s string) string {
	if len(s) > maxTunerText {
		return s[:maxTunerText]
	}
	return s
}

// FFmpeg options a module may ask for: each only says how to read the input, with what
// follows it a value. Anything else is refused, because FFmpeg has options that write files.
var inputOptions = map[string]bool{
	"-user_agent": true, "-headers": true, "-referer": true, "-timeout": true, "-rw_timeout": true, "-reconnect": true, "-reconnect_streamed": true,
	"-reconnect_delay_max": true, "-reconnect_on_network_error": true, "-reconnect_on_http_error": true, "-rtsp_transport": true, "-fflags": true,
	"-analyzeduration": true, "-probesize": true, "-seekable": true, "-multiple_requests": true, "-http_persistent": true, "-tls_verify": true,
	"-allowed_extensions": true, "-protocol_whitelist": true, "-live_start_index": true, "-f": true, "-framerate": true, "-video_size": true,
	"-input_format": true, "-pixel_format": true, "-standard": true, "-thread_queue_size": true, "-channel_layout": true, "-sample_rate": true, "-channels": true,
	"-rtbufsize": true, "-audio_buffer_size": true, "-pkt_size": true, "-buffer_size": true, "-fifo_size": true, "-overrun_nonfatal": true, "-mode": true,
}

// Formats that open this machine's capture devices, whose address is a device's name.
var captureFormats = map[string]bool{"v4l2": true, "alsa": true, "dshow": true, "avfoundation": true}

// Formats that read a stream. FFmpeg has others that make things up or read files.
var streamFormats = map[string]bool{"mpegts": true, "hls": true, "rtsp": true, "rtp": true, "flv": true}

// CheckInput refuses what a module asks FFmpeg to open unless it can only be a stream or a
// capture device: FFmpeg can also read files, chain other protocols and write. It is the
// last thing between a module with a bad day and this server's own program.
func CheckInput(input []string) error {
	addr, capture, limited := "", false, false
	for i := 0; i < len(input); i++ {
		a := input[i]
		if a == "-i" {
			if i+2 != len(input) {
				return errors.New("it gave FFmpeg more after the address")
			}
			addr = input[i+1]
			break
		}
		if !inputOptions[a] || i+1 >= len(input) {
			return fmt.Errorf("it asked for the FFmpeg option %q", a)
		}
		i++
		if a == "-protocol_whitelist" {
			limited = true
		}
		if a == "-protocol_whitelist" && strings.ContainsAny(input[i], "|") {
			return errors.New("it asked for a strange list of protocols")
		}
		if a == "-protocol_whitelist" {
			for _, proto := range strings.Split(input[i], ",") {
				if proto == "file" || proto == "concat" || proto == "subfile" || proto == "pipe" || proto == "data" || proto == "crypto" {
					return fmt.Errorf("it asked FFmpeg to allow %q", proto)
				}
			}
		}
		if a == "-f" {
			capture = captureFormats[input[i]]
			if !capture && !streamFormats[input[i]] {
				return fmt.Errorf("it asked for the FFmpeg format %q", input[i])
			}
		}
	}
	if addr == "" || len(addr) > maxLocator || strings.ContainsAny(addr, "\r\n") {
		return errors.New("it offered no address")
	}
	if capture {
		return nil
	}
	// Without a list FFmpeg would follow a playlist to files and to its own decryption.
	if !limited {
		return errors.New("it didn't limit what FFmpeg may open")
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return errors.New("it offered something other than an address")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "udp", "rtp", "rtsp", "rtsps", "rtmp", "rtmps", "srt":
		return nil
	}
	return fmt.Errorf("it offered a %q address", u.Scheme)
}

// VersionInUse is the version of one module in use, or "".
func (m *Manager) VersionInUse(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.mods[id]; e != nil && e.dir != "" {
		return e.man.Version
	}
	return ""
}

// failedNow counts something wrong this job saw the helper do against the version.
func (a *Attached) failedNow(err error) { a.m.failed(a.id, a.e, a.version, &misbehaved{err}) }
