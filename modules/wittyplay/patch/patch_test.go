package patch

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math/rand"
	"testing"
)

func ips(records ...[]byte) []byte {
	out := []byte("PATCH")
	for _, r := range records {
		out = append(out, r...)
	}
	return append(out, "EOF"...)
}

func rec(off int, data ...byte) []byte {
	return append([]byte{byte(off >> 16), byte(off >> 8), byte(off), byte(len(data) >> 8), byte(len(data))}, data...)
}

func TestIPSWritesAndRepeats(t *testing.T) {
	game := []byte("0123456789")
	run := []byte{0, 0, 8, 0, 0, 0, 4, 'z'} // four z's from offset 8
	out, err := Apply(game, ips(rec(2, 'a', 'b'), run))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "01ab4567zzzz" {
		t.Fatalf("got %q", out)
	}
	if string(game) != "0123456789" {
		t.Fatal("the original changed")
	}
}

func TestIPSCanCut(t *testing.T) {
	p := append(ips(rec(0, 'x')), 0, 0, 3)
	out, err := Apply([]byte("0123456789"), p)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "x12" {
		t.Fatalf("got %q", out)
	}
}

// Runs that keep writing the same place cost far more than their size.
func TestIPSThatKeepsWritingIsRefused(t *testing.T) {
	run := []byte{0, 0, 0, 0, 0, 0xff, 0xff, 'z'}
	p := ips(bytes.Repeat(run, maxIPSWork/0xffff+1))
	if _, err := Detect(p); err == nil {
		t.Error("Detect accepted it")
	}
	if _, err := Apply([]byte("0123456789"), p); err == nil {
		t.Error("Apply accepted it")
	}
}

func TestIPSRefusesDamage(t *testing.T) {
	good := ips(rec(2, 'a', 'b'))
	for name, p := range map[string][]byte{
		"cut in a record": good[:len(good)-5],
		"no end":          good[:len(good)-3],
		"junk after":      append(append([]byte{}, good...), 1, 2),
		"empty run":       ips([]byte{0, 0, 1, 0, 0, 0, 0, 'z'}),
		"just the magic":  []byte("PATCH"),
	} {
		if _, err := Detect(p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// bps builds a patch from actions, the same way a tool making one would.
func bps(source, target []byte, actions []byte) []byte {
	num := func(b []byte, v uint64) []byte {
		for {
			x := v & 0x7f
			v >>= 7
			if v == 0 {
				return append(b, byte(0x80|x))
			}
			b = append(b, byte(x))
			v--
		}
	}
	out := []byte("BPS1")
	out = num(out, uint64(len(source)))
	out = num(out, uint64(len(target)))
	out = num(out, 0)
	out = append(out, actions...)
	out = binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(source))
	out = binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(target))
	return binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(out))
}

func action(kind, length int) []byte {
	v := uint64((length-1)<<2 | kind)
	var b []byte
	for {
		x := v & 0x7f
		v >>= 7
		if v == 0 {
			return append(b, byte(0x80|x))
		}
		b = append(b, byte(x))
		v--
	}
}

func TestBPSAllActions(t *testing.T) {
	source := []byte("HELLO WORLD")
	target := []byte("HELLO THERE THERE")
	var a []byte
	a = append(a, action(0, 6)...)   // "HELLO " from the same place
	a = append(a, action(1, 6)...)   // "THERE " from the patch
	a = append(a, "THERE "...)       //
	a = append(a, action(3, 5)...)   // "THERE" from what's been written
	a = append(a, action(0, 0+1)[0]) // placeholder replaced below
	a = a[:len(a)-1]
	a = append(a, 0x80|(6<<1)) // relative offset +6 from 0 into the output
	p := bps(source, target, a)
	out, err := Apply(source, p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, target) {
		t.Fatalf("got %q", out)
	}
}

func TestBPSCopiesFromElsewhereInTheSource(t *testing.T) {
	source := []byte("abcdef")
	target := []byte("defabc")
	var a []byte
	a = append(a, action(2, 3)...)
	a = append(a, 0x80|(3<<1)) // +3
	a = append(a, action(2, 3)...)
	a = append(a, 0x80|(6<<1)|1) // -6 from where that ended
	out, err := Apply(source, bps(source, target, a))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, target) {
		t.Fatalf("got %q", out)
	}
}

func TestBPSKnowsTheWrongGame(t *testing.T) {
	source := []byte("ABCD")
	p := bps(source, []byte("ABCD"), action(0, 4))
	if _, err := Apply([]byte("WXYZ"), p); !errors.Is(err, ErrSource) {
		t.Fatalf("got %v", err)
	}
	if _, err := Apply([]byte("ABC"), p); !errors.Is(err, ErrSource) {
		t.Fatalf("got %v", err)
	}
}

func TestBPSChecksItsOwnWork(t *testing.T) {
	source := []byte("ABCD")
	p := bps(source, []byte("ABCD"), action(0, 4))
	p[8] ^= 1 // damage inside, with the patch's own checksum no longer right
	if _, err := Detect(p); err == nil {
		t.Fatal("damage wasn't noticed")
	}
	// Right checksum for the patch, wrong one for what it makes.
	bad := bps(source, []byte("ABCE"), action(0, 4))
	if _, err := Apply(source, bad); err == nil {
		t.Fatal("wrong output wasn't noticed")
	}
}

// Hostile BPS actions: each has a good checksum on the patch, so only the bounds stop it.
func TestBPSHostileActions(t *testing.T) {
	source := make([]byte, 16)
	target := make([]byte, 16)
	bigRead := append(action(1, 16), 1, 2)
	for name, a := range map[string][]byte{
		"reads past the patch":    bigRead,
		"writes past the end":     action(1, 1000),
		"source read past end":    append(action(0, 16), action(0, 16)...),
		"source copy before 0":    append(action(2, 4), 0x80|(100<<1)|1),
		"source copy past end":    append(action(2, 4), 0x80|(100<<1)),
		"target copy from future": append(action(3, 4), 0x80|(1<<1)),
		"target copy before 0":    append(action(3, 4), 0x80|(100<<1)|1),
		"number never ends":       bytes.Repeat([]byte{0x7f}, 40),
	} {
		if _, err := Apply(source, bps(source, target, a)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSizesAreLimited(t *testing.T) {
	// A small patch asking for an enormous game must be refused before anything is set aside.
	huge := func() []byte {
		out := []byte("BPS1")
		out = append(out, 0x80)                                                 // source size 0
		out = append(out, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x80) // target size: huge
		out = append(out, 0x80)                                                 // no metadata
		out = binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(nil))    //
		out = binary.LittleEndian.AppendUint32(out, 0)                          //
		return binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(out))   //
	}()
	if _, err := Detect(huge); err == nil {
		t.Fatal("a patch for an enormous game was accepted")
	}
	if _, err := Detect(make([]byte, MaxPatch+1)); err == nil {
		t.Fatal("an oversized patch was accepted")
	}
	// An IPS can only reach 16 MB past its offset, but it still must not run away.
	if _, err := Apply(nil, ips([]byte{0xff, 0xff, 0xfe, 0, 0, 0xff, 0xff, 'z'})); err != nil {
		t.Fatalf("a large but legal run: %v", err)
	}
}

func TestOtherFilesAreRefused(t *testing.T) {
	for _, p := range [][]byte{nil, []byte("hello"), []byte("<svg></svg>"), []byte("PATC")} {
		if _, err := Detect(p); err == nil {
			t.Errorf("%q was accepted", p)
		}
	}
}

// Nothing a patch contains may crash the server.
func TestRandomPatchesNeverPanic(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	game := make([]byte, 1024)
	for i := 0; i < 20000; i++ {
		p := make([]byte, r.Intn(200))
		r.Read(p)
		if i%2 == 0 {
			copy(p, "PATCH")
		} else {
			copy(p, "BPS1")
			if len(p) > 16 {
				sum := crc32.ChecksumIEEE(p[:len(p)-4])
				binary.LittleEndian.PutUint32(p[len(p)-4:], sum)
			}
		}
		_, _ = Apply(game, p)
	}
}

func FuzzApply(f *testing.F) {
	f.Add(ips(rec(2, 'a')))
	f.Add(bps([]byte("ABCD"), []byte("ABCD"), action(0, 4)))
	f.Fuzz(func(t *testing.T, p []byte) {
		out, err := Apply([]byte("ABCDEFGH"), p)
		if err == nil && len(out) > MaxOutput {
			t.Fatal("too large")
		}
	})
}
