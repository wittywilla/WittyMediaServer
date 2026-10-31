//go:build !windows && !linux

package libretro

import "fmt"

func newGLPlatform(req glRequest) (glPlatform, error) {
	return nil, fmt.Errorf("%w: hardware-rendered games run on Windows and Linux servers", ErrNoGPU)
}
