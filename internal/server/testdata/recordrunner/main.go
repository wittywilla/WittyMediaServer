// recordrunner stands in for a game runner in tests: it writes what it was started with, and the
// game it was asked to load, to runner.json in its save folder, then says it can't go on.
package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

func main() {
	save := ""
	for i, a := range os.Args {
		if a == "-save" && i+1 < len(os.Args) {
			save = os.Args[i+1]
		}
	}
	var head [5]byte
	var load json.RawMessage
	if _, err := io.ReadFull(os.Stdin, head[:]); err == nil && head[0] == 64 {
		body := make([]byte, binary.LittleEndian.Uint32(head[1:]))
		if _, err := io.ReadFull(os.Stdin, body); err == nil {
			load = body
		}
	}
	out, _ := json.Marshal(map[string]any{"args": os.Args[1:], "load": load})
	_ = os.WriteFile(filepath.Join(save, "runner.json"), out, 0o644)
	msg := []byte("recorded")
	var h [5]byte
	h[0] = 8 // MsgError
	binary.LittleEndian.PutUint32(h[1:], uint32(len(msg)))
	_, _ = os.Stdout.Write(append(h[:], msg...))
	os.Exit(1)
}
