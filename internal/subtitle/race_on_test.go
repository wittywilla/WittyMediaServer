//go:build race

package subtitle

// The race detector runs code 15 to 30 times slower, so time limits stretch to match.
const timeScale = 20
