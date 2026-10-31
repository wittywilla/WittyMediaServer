package host

import "encoding/binary"

// rewindBuffer keeps recent save states in a fixed amount of memory. Only the newest
// state is whole. Each older one is the difference from the one after it, stored as
// runs of unchanged bytes and XORed bytes. A game changes little in a frame, so these
// are small, and the oldest can be dropped at any time without touching the rest.
type rewindBuffer struct {
	max     int64 // the most memory it may hold, the whole state included
	maxStep int   // the most steps back it keeps
	cur     []byte
	deltas  [][]byte // oldest first
	start   int      // deltas before this have been dropped
	held    int64
	scratch []byte
}

func newRewindBuffer(maxBytes int64, maxSteps int) *rewindBuffer {
	return &rewindBuffer{max: maxBytes, maxStep: maxSteps}
}

// push adds the state from this moment. It reports false when a single state is too
// big to keep, and the buffer is empty then.
func (b *rewindBuffer) push(state []byte) bool {
	if int64(len(state)) > b.max/2 {
		b.reset()
		return false
	}
	if len(b.cur) != len(state) {
		// Some cores' states change size. There's no difference to take across that.
		b.reset()
		b.cur = append(make([]byte, 0, len(state)), state...)
		b.held = int64(len(state))
		return true
	}
	b.scratch = encodeDelta(b.scratch[:0], b.cur, state)
	copy(b.cur, state)
	d := append(make([]byte, 0, len(b.scratch)), b.scratch...)
	b.deltas = append(b.deltas, d)
	b.held += int64(len(d))
	for b.len() > 0 && (b.held > b.max || b.len() > b.maxStep) {
		b.held -= int64(len(b.deltas[b.start]))
		b.deltas[b.start] = nil
		b.start++
	}
	// The slice grows by appending and shrinks from the front: copy down once half is unused.
	if b.start > 64 && b.start > len(b.deltas)/2 {
		n := copy(b.deltas, b.deltas[b.start:])
		clear(b.deltas[n:])
		b.deltas = b.deltas[:n]
		b.start = 0
	}
	return true
}

// pop steps back one state. The slice stays valid until the next call.
func (b *rewindBuffer) pop() ([]byte, bool) {
	if b.len() == 0 {
		return nil, false
	}
	n := len(b.deltas) - 1
	d := b.deltas[n]
	if !applyDelta(b.cur, d) {
		b.reset()
		return nil, false
	}
	b.held -= int64(len(d))
	b.deltas[n] = nil
	b.deltas = b.deltas[:n]
	return b.cur, true
}

func (b *rewindBuffer) len() int { return len(b.deltas) - b.start }

func (b *rewindBuffer) reset() {
	b.cur, b.deltas, b.start, b.held = nil, nil, 0, 0
}

// encodeDelta appends the difference between two states of one size: pairs of
// "this many unchanged bytes, then this many XORed ones".
func encodeDelta(dst, a, b []byte) []byte {
	n := len(a)
	for i := 0; i < n; {
		same := i
		for same+8 <= n && binary.NativeEndian.Uint64(a[same:]) == binary.NativeEndian.Uint64(b[same:]) {
			same += 8
		}
		for same < n && a[same] == b[same] {
			same++
		}
		end := same
		for end < n && a[end] != b[end] {
			end++
		}
		dst = binary.AppendUvarint(dst, uint64(same-i))
		dst = binary.AppendUvarint(dst, uint64(end-same))
		for k := same; k < end; k++ {
			dst = append(dst, a[k]^b[k])
		}
		i = end
	}
	return dst
}

// applyDelta undoes encodeDelta in place. XOR works in both directions.
func applyDelta(cur, d []byte) bool {
	i := 0
	for len(d) > 0 {
		same, n := binary.Uvarint(d)
		if n <= 0 {
			return false
		}
		d = d[n:]
		run, n := binary.Uvarint(d)
		if n <= 0 {
			return false
		}
		d = d[n:]
		if same > uint64(len(cur)-i) || run > uint64(len(cur)-i)-same || run > uint64(len(d)) {
			return false
		}
		i += int(same)
		for k := 0; k < int(run); k++ {
			cur[i+k] ^= d[k]
		}
		i += int(run)
		d = d[run:]
	}
	return true
}
