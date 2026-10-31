//go:build !linux

package libretro

// OnCoreThread runs fn. Only Linux needs a thread of its own for this: see
// thread_linux.go.
func OnCoreThread(fn func()) error {
	fn()
	return nil
}
