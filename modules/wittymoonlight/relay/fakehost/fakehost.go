// Package fakehost stands in for the Moonlight host program, in tests. A test program calls
// Run when it's started as a host, which speaks the protocol with made-up frames.
package fakehost

import (
	"encoding/binary"
	"io"
	"os"
	"strings"
	"time"
)

// Env is set in the environment of a process that should act as the host.
const Env = "WITTYMOONLIGHT_FAKE_HOST"

func send(kind byte, body []byte) {
	head := [5]byte{kind}
	binary.LittleEndian.PutUint32(head[1:], uint32(len(body)))
	_, _ = os.Stdout.Write(append(head[:], body...))
}

func event(s string) { send(1, []byte(s)) }

// Run acts as the host program until stdin closes. If WITTYMOONLIGHT_FAKE_HOST is "fail", it
// reports a failed connection; "die" ends straight after starting; "flood" sends too much.
func Run() {
	mode := os.Getenv(Env)
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		os.Stdout.WriteString("wittymoonlight-host fake protocol 1\n")
		return
	}
	event(`{"type":"hello","version":"fake","protocol":1}`)
	var head [5]byte
	for {
		if _, err := io.ReadFull(os.Stdin, head[:]); err != nil {
			return
		}
		body := make([]byte, binary.LittleEndian.Uint32(head[1:]))
		if _, err := io.ReadFull(os.Stdin, body); err != nil {
			return
		}
		switch head[0] {
		case 71:
			return
		case 64:
			if !strings.Contains(string(body), "rikey=") {
				return
			}
			switch mode {
			case "fail":
				event(`{"type":"failed","stage":"RTSP handshake","code":-1}`)
				return
			case "die":
				return
			case "flood":
				head := [5]byte{2}
				binary.LittleEndian.PutUint32(head[1:], 1<<30)
				_, _ = os.Stdout.Write(head[:])
				time.Sleep(time.Second)
				return
			}
			event(`{"type":"video","format":1,"width":1280,"height":720,"fps":60}`)
			event(`{"type":"audio","rate":48000,"channels":2,"streams":1,"coupled":1,"samples":240}`)
			event(`{"type":"started"}`)
			go func() {
				ts := uint32(1)
				for {
					// One small keyframe in Annex B.
					send(2, append([]byte{1, byte(ts), byte(ts >> 8), byte(ts >> 16), byte(ts >> 24)}, 0, 0, 0, 1, 0x67, 0x42, 0, 0, 0, 1, 0x65, 0x88))
					send(3, []byte{0xfc, 0xff, 0xfe})
					ts += 1500
					time.Sleep(16 * time.Millisecond)
				}
			}()
		case 65: // a controller message comes back as rumble, so a test can see it arrived
			send(4, []byte{body[0], 0, 1, 0, 2, 0})
		}
	}
}
