//go:build !(linux && amd64)

package stream

func forkAChild() {}
