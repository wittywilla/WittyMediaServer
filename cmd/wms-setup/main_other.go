//go:build !windows

// Command wms-setup is the Windows installer; on other systems use
// install.sh from the release package.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "wms-setup installs WittyMediaServer on Windows. Here, run install.sh from the release package.")
	os.Exit(2)
}
