// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command gen writes the files made from the registry: the Supported formats page, the
// registry as JSON, and the few lists the web app needs before it has asked the server.
// Run it from the repository's root after changing the registry:
//
//	go run ./internal/mediatypes/gen
package main

import (
	"fmt"
	"os"

	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
)

func main() {
	for path, content := range map[string][]byte{
		"docs/SUPPORTED-FORMATS.md":       []byte(mediatypes.Markdown()),
		"docs/media-types.json":           mediatypes.JSON(),
		"web/src/mediaTypes.generated.ts": []byte(mediatypes.TypeScript()),
	} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}
