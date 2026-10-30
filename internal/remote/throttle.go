package remote

import (
	"context"
	"net/http"
	"time"
)

// Throttle limits what one response sends, in bits per second. Players read
// ahead, so the first seconds' worth goes out at once and only after that does
// the limit bite.
func Throttle(ctx context.Context, w http.ResponseWriter, bitsPerSec int64) http.ResponseWriter {
	if bitsPerSec <= 0 {
		return w
	}
	return &throttled{ResponseWriter: w, ctx: ctx, rate: bitsPerSec / 8, start: time.Now()}
}

const (
	throttleBurst = 2 * time.Second // of data, sent without waiting
	throttleChunk = 64 << 10
)

type throttled struct {
	http.ResponseWriter
	ctx   context.Context
	rate  int64 // bytes per second
	start time.Time
	sent  int64
}

func (t *throttled) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		n := min(len(b), throttleChunk)
		// When these bytes are due, counting the head start.
		due := time.Duration(float64(t.sent+int64(n))/float64(t.rate)*float64(time.Second)) - throttleBurst
		if wait := due - time.Since(t.start); wait > 0 {
			tm := time.NewTimer(wait)
			select {
			case <-tm.C:
			case <-t.ctx.Done():
				tm.Stop()
				return total, t.ctx.Err()
			}
		}
		w, err := t.ResponseWriter.Write(b[:n])
		t.sent += int64(w)
		total += w
		if err != nil {
			return total, err
		}
		b = b[n:]
	}
	return total, nil
}

func (t *throttled) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
