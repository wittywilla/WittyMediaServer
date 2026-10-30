package remote

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestThrottleKeepsToTheRate(t *testing.T) {
	rec := httptest.NewRecorder()
	const bits = 8 * 1 << 20 // 1 MB/s
	w := Throttle(context.Background(), rec, bits)
	start := time.Now()
	data := make([]byte, 3<<20) // 2 MB of head start, then 1 MB at 1 MB/s
	if n, err := w.Write(data); n != len(data) || err != nil {
		t.Fatalf("wrote %d: %v", n, err)
	}
	took := time.Since(start)
	if took < 800*time.Millisecond {
		t.Errorf("3 MB took only %v", took)
	}
	if took > 5*time.Second {
		t.Errorf("3 MB took %v", took)
	}
	if rec.Body.Len() != len(data) {
		t.Errorf("body is %d bytes", rec.Body.Len())
	}
}

func TestThrottleStopsWithTheListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := Throttle(ctx, httptest.NewRecorder(), 8000) // 1 KB/s
	time.AfterFunc(50*time.Millisecond, cancel)
	if _, err := w.Write(make([]byte, 1<<20)); err == nil {
		t.Error("expected the write to stop when the listener left")
	}
}

func TestThrottleZeroMeansNone(t *testing.T) {
	rec := httptest.NewRecorder()
	if Throttle(context.Background(), rec, 0) != rec {
		t.Error("no limit should leave the writer as it is")
	}
}
