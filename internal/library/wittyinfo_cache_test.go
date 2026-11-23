// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"testing"

	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

func infoWith(bytes int) infoCacheEntry {
	info := &wittyinfo.Info{Files: map[string][]byte{"poster.jpg": make([]byte, bytes)}}
	return infoCacheEntry{info: info, bytes: infoBytes(info)}
}

// The cache of .wittyinfo files never holds more than infoCacheBytes of pictures and subtitles.
func TestInfoCacheKeepsToItsBytes(t *testing.T) {
	infoCache.Lock()
	clear(infoCache.m)
	infoCache.bytes = 0
	infoCache.Unlock()
	t.Cleanup(func() {
		infoCache.Lock()
		clear(infoCache.m)
		infoCache.bytes = 0
		infoCache.Unlock()
	})
	held := func() (int, int64) {
		infoCache.Lock()
		defer infoCache.Unlock()
		var sum int64
		for _, e := range infoCache.m {
			sum += e.bytes
		}
		if sum != infoCache.bytes {
			t.Fatalf("the cache counts %d bytes but holds %d", infoCache.bytes, sum)
		}
		return len(infoCache.m), infoCache.bytes
	}
	six := 6 << 20
	keepInfo("a", infoWith(six))
	keepInfo("b", infoWith(six))
	if n, b := held(); n != 2 || b != int64(2*six) {
		t.Fatalf("two archives: %d kept, %d bytes", n, b)
	}
	// A third would pass the limit: the cache starts again with it.
	keepInfo("c", infoWith(six))
	if n, b := held(); n != 1 || b != int64(six) {
		t.Fatalf("past the limit: %d kept, %d bytes", n, b)
	}
	// The same file read again replaces itself, not counted twice.
	keepInfo("c", infoWith(1<<20))
	if n, b := held(); n != 1 || b != 1<<20 {
		t.Fatalf("read again: %d kept, %d bytes", n, b)
	}
	// One larger than the whole limit isn't kept, and doesn't push the others out.
	keepInfo("huge", infoWith(infoCacheBytes+1))
	if n, _ := held(); n != 1 {
		t.Fatalf("a huge archive: %d kept", n)
	}
	ForgetInfoFile("c")
	if n, b := held(); n != 0 || b != 0 {
		t.Fatalf("forgotten: %d kept, %d bytes", n, b)
	}
	// A plain manifest, with nothing in it but words, costs nothing here.
	keepInfo("plain", infoCacheEntry{info: &wittyinfo.Info{}})
	if n, b := held(); n != 1 || b != 0 {
		t.Fatalf("plain: %d kept, %d bytes", n, b)
	}
}
