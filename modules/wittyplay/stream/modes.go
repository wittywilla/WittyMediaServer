package stream

// Mode is a way a client can receive a game. The server says which ones it has, so
// a client never has to guess, and new modes can appear without breaking old clients.
type Mode struct {
	ID    string   `json:"id"`              // webrtc | ws-h264 | ws-jpeg
	Video []string `json:"video"`           // codecs, best first
	Audio string   `json:"audio"`           // opus | pcm
	Input string   `json:"input"`           // datachannel | websocket
	Needs string   `json:"needs,omitempty"` // what a client must have, in words
	// BrowserOnly marks a mode only a web browser can use. None exists yet; in-browser
	// emulation will be one, and is left out of the answer for native clients.
	BrowserOnly bool `json:"browserOnly,omitempty"`
}

// Client kinds a server can be asked about.
const (
	ClientBrowser = "browser"
	ClientNative  = "native"
)

// Modes lists what this server can do for a kind of client, best first.
func (m *Manager) Modes(client string) []Mode {
	cfg := m.config()
	have := func(names ...string) bool {
		for _, n := range names {
			if cfg.HaveEncoder(n) {
				return true
			}
		}
		return false
	}
	var codecs []string
	h264 := len(cfg.HWEncoders) > 0 || have("libx264", "libopenh264")
	if h264 {
		codecs = append(codecs, "h264")
	}
	if have("libvpx") {
		codecs = append(codecs, "vp8")
	}
	var out []Mode
	if len(codecs) > 0 {
		out = append(out, Mode{ID: "webrtc", Video: codecs, Audio: "opus", Input: "datachannel", Needs: "A WebRTC stack with a data channel"})
	}
	if h264 {
		out = append(out, Mode{ID: "ws-h264", Video: []string{"h264"}, Audio: "pcm", Input: "websocket", Needs: "A WebSocket and an H.264 decoder that accepts Annex B"})
	}
	// JPEG is encoded here, so it works even with no usable FFmpeg encoder.
	out = append(out, Mode{ID: "ws-jpeg", Video: []string{"jpeg"}, Audio: "pcm", Input: "websocket", Needs: "A WebSocket and a JPEG decoder"})
	if client == ClientNative {
		kept := out[:0]
		for _, mode := range out {
			if !mode.BrowserOnly {
				kept = append(kept, mode)
			}
		}
		out = kept
	}
	return out
}
