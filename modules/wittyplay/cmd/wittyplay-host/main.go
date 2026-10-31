// Command wittyplay-host runs one game for WittyMediaServer: it loads a libretro
// core, or the built-in test core, and speaks the ipc protocol on stdin and
// stdout. It has no options a person needs. docs/GAMING.md explains why it is
// a process of its own.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/host"
	"github.com/wittywilla/wittyplay/hostsandbox"
	"github.com/wittywilla/wittyplay/ipc"
	"github.com/wittywilla/wittyplay/libretro"
	"github.com/wittywilla/wittyplay/testcore"
)

// boundedClose stops a core from holding the process open: everything has been
// saved by then, and some cores hang waiting for their own threads.
type boundedClose struct{ *libretro.Core }

func (b boundedClose) Close() {
	time.AfterFunc(5*time.Second, func() { os.Exit(0) })
	b.Core.Close()
}

func main() {
	saveDir := flag.String("save", "", "the only folder this process may write to")
	memory := flag.Int64("memory-mb", 0, "address space limit in MiB; 0 for none")
	gpu := flag.Bool("gpu", false, "this game draws with a graphics card, so allow what a driver needs")
	normal := flag.Bool("normal-priority", false, "this game can't play at low priority")
	probe := flag.Bool("probe-gpu", false, "say which graphics card games would draw with, and stop")
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "wittyplay-host is started by WittyMediaServer")
		os.Exit(2)
	}
	// Cores print to stdout, so the protocol goes elsewhere before one is loaded.
	out, err := hostsandbox.ClaimStdout()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wittyplay-host:", err)
		os.Exit(1)
	}
	// The sandbox goes on from the thread that will run the core, as some of its
	// layers apply to one thread and whatever that thread starts.
	var runErr error
	err = libretro.OnCoreThread(func() {
		if err := hostsandbox.Apply(hostsandbox.Limits{SaveDir: *saveDir, MemoryMB: *memory, GPU: *gpu || *probe, Normal: *normal}); err != nil {
			// Each layer is best effort, because kernels and containers differ. The
			// server's log shows which ones this machine lacks.
			fmt.Fprintln(os.Stderr, "sandbox:", err)
		}
		if *probe {
			name, err := libretro.ProbeGPU()
			if err != nil {
				fmt.Fprintln(out, "none:", err)
				return
			}
			fmt.Fprintln(out, "gpu:", name)
			return
		}
		runErr = host.Run(os.Stdin, out, func(l ipc.Load, h engine.Handlers) (engine.Engine, error) {
			if l.Core == ipc.TestCore {
				return testcore.New(h), nil
			}
			c, err := libretro.Load(l.Core, libretro.Paths{System: l.System, Save: l.Save}, l.Options, h)
			if err != nil {
				return nil, err
			}
			return boundedClose{c}, nil
		})
	})
	if err == nil {
		err = runErr
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wittyplay-host:", err)
		os.Exit(1)
	}
}
