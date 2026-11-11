// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package remote

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// fileSink is a response that notes what it was asked to copy from, as the
// connection underneath would be.
type fileSink struct {
	*httptest.ResponseRecorder
	pieces  int
	notFile bool
}

func (f *fileSink) ReadFrom(r io.Reader) (int64, error) {
	f.pieces++
	// What the system's own copy needs: the file itself, or one limit around it.
	lr, ok := r.(*io.LimitedReader)
	if !ok {
		f.notFile = true
	} else if _, isFile := lr.R.(*os.File); !isFile {
		f.notFile = true
	}
	return io.Copy(f.ResponseRecorder.Body, r)
}

// A file sent through the limit still reaches the connection as a file, a piece at a
// time, so the system copies it without the server reading it, and the limit holds.
func TestThrottleSendsAFileAsAFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "film")
	data := make([]byte, 3<<20)
	for i := range data {
		data[i] = byte(i)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		src  func(*os.File) io.Reader
		want int
	}{
		// As http.ServeContent sends them: always so many bytes of the file (io.CopyN).
		{"whole", func(f *os.File) io.Reader { return io.LimitReader(f, int64(len(data))) }, len(data)},
		{"a range", func(f *os.File) io.Reader { return io.LimitReader(f, 2<<20+7) }, 2<<20 + 7},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			sink := &fileSink{ResponseRecorder: httptest.NewRecorder()}
			const bits = 8 * 1 << 20 // 1 MB/s, after 2 MB of head start
			w := Throttle(context.Background(), sink, bits)
			start := time.Now()
			n, err := io.Copy(w, c.src(f))
			if err != nil || n != int64(c.want) {
				t.Fatalf("sent %d of %d: %v", n, c.want, err)
			}
			if sink.notFile || sink.pieces < c.want/throttleChunk {
				t.Errorf("the file didn't arrive as pieces of a file: %d pieces, not a file %v", sink.pieces, sink.notFile)
			}
			if !bytes.Equal(sink.Body.Bytes(), data[:c.want]) {
				t.Error("what arrived isn't what was sent")
			}
			if c.want == len(data) {
				if took := time.Since(start); took < 800*time.Millisecond || took > 5*time.Second {
					t.Errorf("3 MB at 1 MB/s after 2 MB at once took %v", took)
				}
			}
		})
	}
}

func TestThrottledFileStopsWithTheListener(t *testing.T) {
	p := filepath.Join(t.TempDir(), "film")
	if err := os.WriteFile(p, make([]byte, 1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx, cancel := context.WithCancel(context.Background())
	w := Throttle(ctx, &fileSink{ResponseRecorder: httptest.NewRecorder()}, 8000) // 1 KB/s
	time.AfterFunc(50*time.Millisecond, cancel)
	if _, err := io.Copy(w, f); err == nil {
		t.Error("expected the file to stop when the listener left")
	}
}
