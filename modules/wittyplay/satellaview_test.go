package wittyplay

import "testing"

// rom builds a file with the given header bytes at the given offset.
func rom(at int, header map[int]byte) []byte {
	b := make([]byte, 1<<20)
	for i, v := range header {
		b[at+i] = v
	}
	return b
}

func TestIsSatellaview(t *testing.T) {
	broadcast := map[int]byte{0x16: 0x50, 0x17: 0xFB, 0x18: 0x21, 0x19: 0x30, 0x1A: 0x33}
	cartridge := map[int]byte{0x14: 0x20, 0x15: 0x31, 0x16: 0x02, 0x17: 0x0B, 0x18: 0x03, 0x19: 0x01, 0x1A: 0x33}
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"hi.smc", rom(0xFFC0, broadcast), true},
		{"lo.smc", rom(0x7FC0, broadcast), true},
		{"copier.smc", append(make([]byte, 512), rom(0xFFC0, broadcast)...), true},
		{"unsold.smc", rom(0xFFC0, map[int]byte{0x15: 0x80, 0x16: 0xFF, 0x17: 0xFF, 0x18: 0x30, 0x1A: 0xFF}), true},
		{"cart.smc", rom(0xFFC0, cartridge), false},
		{"empty-dates.smc", rom(0xFFC0, map[int]byte{0x18: 0x31, 0x1A: 0x33}), false},
		{"zeros.smc", make([]byte, 1<<20), false},
		{"short.smc", []byte("not much"), false},
		{"named.bs", []byte("not much"), true},
		{"NAMED.BS", []byte("not much"), true},
	}
	for _, c := range cases {
		if got := IsSatellaview(writeTemp(t, c.name, c.data)); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
