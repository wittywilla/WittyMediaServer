//go:build !windows

package libretro

import "github.com/ebitengine/purego"

func openLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
}

func symbol(lib uintptr, name string) (uintptr, error) { return purego.Dlsym(lib, name) }

func closeLibrary(lib uintptr) { _ = purego.Dlclose(lib) }
