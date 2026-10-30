//go:build arm || (windows && 386)

package libretro

// C aligns a double to 8 bytes on these platforms and Go aligns it to 4.
const timingPad = 4
