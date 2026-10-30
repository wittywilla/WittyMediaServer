//go:build !linux

package wittytuner

import (
	"context"
	"errors"
)

func setV4L2(string, int, float64) error { return errors.New("V4L2 is on Linux") }

func discoverV4L2(context.Context, Env) []Found { return nil }
