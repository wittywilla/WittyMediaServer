//go:build !linux

package libretro

// RunOnCoreThread just runs fn. Coroutine stacks only trouble Go's Linux runtime,
// as far as has been tested: bsnes runs on Windows as it is.
func RunOnCoreThread(fn func()) { fn() }
