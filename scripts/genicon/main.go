// Command genicon writes the WittyMediaServer icon as PNG/ICO files for
// packaging (desktop entries, web manifest, installers).
//
//	go run ./scripts/genicon out.png [size]
//	go run ./scripts/genicon out.ico
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/trayicon"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: genicon <out.png|out.ico> [size]")
		os.Exit(2)
	}
	out := os.Args[1]
	size := 256
	if len(os.Args) > 2 {
		size, _ = strconv.Atoi(os.Args[2])
	}
	var data []byte
	if strings.EqualFold(filepath.Ext(out), ".ico") {
		data = trayicon.ICO(false)
	} else {
		data = trayicon.PNG(size, false)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
