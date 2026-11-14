// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command sdui-catalog is a developer tool and never ships. It prints the server's own SDUI catalogue
// (what GET /api/v1/sdui/catalog answers before any module adds to it) as JSON, for apps that bundle
// a copy to draw with before the server has answered (WittyMediaClient apple/scripts/sync-sdui-catalog.sh).
//
//	go run ./scripts/sdui-catalog > catalog.json
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

func write(w io.Writer) error {
	b, err := json.Marshal(sdui.Catalog())
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

func main() {
	if err := write(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "sdui-catalog:", err)
		os.Exit(1)
	}
}
