// Command settingsini writes the settings.ini that archives and installers put beside the
// program, the same text the server writes (config.SettingsText), so there's one copy of it.
//
//	go run ./scripts/settingsini [-crlf] <file>
//
// -crlf ends its lines the Windows way, for the .zip and the MSI.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/config"
)

func main() {
	crlf := flag.Bool("crlf", false, "Windows line endings")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: settingsini [-crlf] <file>")
		os.Exit(2)
	}
	text := config.SettingsText(config.DefaultPort, false)
	if *crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if err := os.WriteFile(flag.Arg(0), []byte(text), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
