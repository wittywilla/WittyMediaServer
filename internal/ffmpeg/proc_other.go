//go:build !windows

package ffmpeg

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
