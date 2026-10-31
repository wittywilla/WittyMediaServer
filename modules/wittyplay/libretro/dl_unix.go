//go:build !windows

package libretro

import "github.com/ebitengine/purego"

// Some cores use the maths library without saying so, because the frontends they
// are built for link it themselves. This runner doesn't, so it opens it for them.
var mathLibraries = []string{"libm.so.6", "libm.so", "libm.dylib"}

func openLibrary(path string) (uintptr, error) {
	for _, name := range mathLibraries {
		if _, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			break
		}
	}
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
}

func symbol(lib uintptr, name string) (uintptr, error) { return purego.Dlsym(lib, name) }

func closeLibrary(lib uintptr) { _ = purego.Dlclose(lib) }
