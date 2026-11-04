package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// cmdHardware says which graphics devices the server can see and which hardware
// encoders work, the same test Settings shows, without a server running. In a
// container: docker exec wittymediaserver wittymediaserver hardware
//
//	wittymediaserver hardware [-config file] [-ffmpeg path] [--json]
func cmdHardware(args []string) int {
	asJSON := slices.Contains(args, "--json") || slices.Contains(args, "-json")
	args = slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == "--json" || a == "-json" })
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	cfg, err := config.ParseOnly(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	tools, err := ffmpeg.Locate(cfg.FFmpegPath, cfg.FFprobePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FFmpeg:", err)
		return 2
	}
	tools.ProbeHardware(context.Background())
	list, _ := tools.HardwareStatus()
	gpus := ffmpeg.GPUs()
	working := slices.ContainsFunc(list, func(h ffmpeg.HWStatus) bool { return h.Working })
	if asJSON {
		out, _ := json.MarshalIndent(map[string]any{"container": ffmpeg.InContainer(), "user": os.Getuid(), "groups": groups(), "gpus": gpus, "encoders": list}, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("FFmpeg: %s\nRunning as user %d, groups %v", tools.FFmpeg, os.Getuid(), groups())
		if ffmpeg.InContainer() {
			fmt.Print(", in a container")
		}
		fmt.Println()
		fmt.Println("\nGraphics devices:")
		if len(gpus) == 0 {
			fmt.Println("  none found")
		}
		for _, d := range gpus {
			fmt.Printf("  %s  vendor %s, driver %s, group %d, mode %s\n", d.Path, orDash(d.Vendor), orDash(d.Driver), d.Group, orDash(d.Mode))
			if d.Usable {
				fmt.Println("    can be used")
			} else {
				fmt.Println("    can't be used:", d.Problem)
				if d.Fix != "" {
					fmt.Println("    fix:", d.Fix)
				}
			}
		}
		fmt.Println("\nHardware encoders:")
		for _, h := range list {
			state := "works"
			switch {
			case !h.Compiled:
				state = "not in this FFmpeg"
			case !h.Working && h.Reason != "":
				state = "not used: " + h.Reason
			case !h.Working && h.Tested:
				state = "failed: " + h.Error
			case !h.Working:
				state = "not for this system"
			}
			dev := ""
			if h.Device != "" {
				dev = " (" + h.Device + ")"
			}
			fmt.Printf("  %-18s %s%s\n", h.Name, state, dev)
		}
	}
	if !working {
		return 1
	}
	return 0
}

func groups() []int {
	g := []int{os.Getgid()}
	more, _ := os.Getgroups()
	for _, x := range more {
		if !slices.Contains(g, x) {
			g = append(g, x)
		}
	}
	return g
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
