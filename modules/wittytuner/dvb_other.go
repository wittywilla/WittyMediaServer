//go:build !linux

package wittytuner

import (
	"context"
	"errors"
	"io"
	"time"
)

func (d *dvb) openTuned(context.Context, Tuning, time.Duration) (io.ReadCloser, error) {
	return nil, errors.New("DVB tuner cards are read through Linux")
}

func discoverDVB(context.Context, Env) []Found { return nil }
