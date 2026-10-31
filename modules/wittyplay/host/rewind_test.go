package host

import (
	"bytes"
	"math/rand"
	"testing"
)

// states makes a run of save states that change a little each time, as a game's do.
func states(n, size int, seed int64) [][]byte {
	r := rand.New(rand.NewSource(seed))
	cur := make([]byte, size)
	r.Read(cur)
	out := make([][]byte, n)
	for i := range out {
		for k := 0; k < 1+r.Intn(40); k++ {
			cur[r.Intn(size)] = byte(r.Intn(256))
		}
		out[i] = append([]byte(nil), cur...)
	}
	return out
}

func TestRewindRestoresEveryStateExactly(t *testing.T) {
	all := states(200, 4096, 1)
	b := newRewindBuffer(16<<20, 1000)
	for _, s := range all {
		if !b.push(s) {
			t.Fatal("refused a state it has room for")
		}
	}
	for i := len(all) - 2; i >= 0; i-- {
		got, ok := b.pop()
		if !ok {
			t.Fatalf("ran out at %d", i)
		}
		if !bytes.Equal(got, all[i]) {
			t.Fatalf("state %d came back different", i)
		}
	}
	if _, ok := b.pop(); ok {
		t.Fatal("went further back than there was")
	}
}

// Whatever a game does, rewind stays inside its memory, however long it runs.
func TestRewindStaysInsideItsMemory(t *testing.T) {
	const limit = 256 << 10
	r := rand.New(rand.NewSource(2))
	b := newRewindBuffer(limit, 1<<30)
	state := make([]byte, 32<<10)
	for i := 0; i < 20000; i++ {
		// Busy frames change a lot of the state, which is the hard case.
		for k := 0; k < 2000; k++ {
			state[r.Intn(len(state))] = byte(r.Intn(256))
		}
		if !b.push(state) {
			t.Fatal("refused a state that fits")
		}
		if b.held > limit {
			t.Fatalf("frame %d: holding %d bytes, over %d", i, b.held, limit)
		}
		if i > 0 && i%500 == 0 && b.len() == 0 {
			t.Fatal("kept nothing")
		}
	}
	// What's left is still a sound chain, back to the oldest kept.
	for {
		if _, ok := b.pop(); !ok {
			break
		}
	}
	if b.cur == nil {
		t.Fatal("the chain broke before its end")
	}
}

func TestRewindKeepsNoMoreThanItsSteps(t *testing.T) {
	b := newRewindBuffer(16<<20, 10)
	for _, s := range states(100, 256, 3) {
		b.push(s)
	}
	if b.len() != 10 {
		t.Fatalf("kept %d steps, want 10", b.len())
	}
}

func TestRewindRefusesAStateThatWontFit(t *testing.T) {
	b := newRewindBuffer(1024, 100)
	if b.push(make([]byte, 4096)) {
		t.Fatal("kept a state bigger than its memory")
	}
	if b.held != 0 {
		t.Fatalf("holds %d after refusing", b.held)
	}
}

// Some cores' states change size. That's a fresh start, not a corrupted chain.
func TestRewindSurvivesAStateChangingSize(t *testing.T) {
	b := newRewindBuffer(1<<20, 100)
	for _, s := range states(5, 100, 4) {
		b.push(s)
	}
	other := states(3, 200, 5)
	for _, s := range other {
		b.push(s)
	}
	got, ok := b.pop()
	if !ok || !bytes.Equal(got, other[1]) {
		t.Fatal("the states after the change came back wrong")
	}
	if _, ok := b.pop(); !ok {
		t.Fatal("lost the first state after the change")
	}
	if _, ok := b.pop(); ok {
		t.Fatal("went back across the change")
	}
}

// The delta decoder never sees untrusted data in normal use, but it must not be able
// to run off the end of the state whatever it's given.
func TestApplyDeltaRefusesNonsense(t *testing.T) {
	cur := make([]byte, 64)
	r := rand.New(rand.NewSource(6))
	for i := 0; i < 5000; i++ {
		d := make([]byte, r.Intn(64))
		r.Read(d)
		applyDelta(cur, d) // must not panic
	}
	big := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01, 0x01, 0x00}
	if applyDelta(cur, big) {
		t.Fatal("accepted a run past the end")
	}
}
