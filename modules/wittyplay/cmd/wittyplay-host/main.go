// Command wittyplay-host runs one game for WittyMediaServer: it loads a libretro
// core, or the built-in test core, and speaks the ipc protocol on stdin and
// stdout. It has no options a person needs. docs/GAMING.md explains why it is
// a process of its own.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/host"
	"github.com/wittywilla/wittyplay/hostsandbox"
	"github.com/wittywilla/wittyplay/ipc"
	"github.com/wittywilla/wittyplay/libretro"
	"github.com/wittywilla/wittyplay/testcore"
)

func main() {
	saveDir := flag.String("save", "", "the only folder this process may write to")
	memory := flag.Int64("memory-mb", 0, "address space limit in MiB; 0 for none")
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "wittyplay-host is started by WittyMediaServer")
		os.Exit(2)
	}
	if err := hostsandbox.Apply(hostsandbox.Limits{SaveDir: *saveDir, MemoryMB: *memory}); err != nil {
		// Each layer is best effort, because kernels and containers differ. The
		// server's log shows which ones this machine lacks.
		fmt.Fprintln(os.Stderr, "sandbox:", err)
	}
	err := host.Run(os.Stdin, os.Stdout, func(l ipc.Load, h engine.Handlers) (engine.Engine, error) {
		if l.Core == ipc.TestCore {
			c := testcore.New(h)
			c.ApplyOptions(l.Options)
			return c, nil
		}
		return libretro.Load(l.Core, libretro.Paths{System: l.System, Save: l.Save}, l.Options, h)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "wittyplay-host:", err)
		os.Exit(1)
	}
}
