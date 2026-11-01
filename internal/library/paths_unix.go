//go:build !windows

package library

import "syscall"

// openNoBlock: a pipe waiting for a writer would otherwise hold whatever opens it for good,
// and no context can stop an open. Nothing is ever read from one: what's opened is looked at
// first. It changes nothing for a plain file.
const openNoBlock = syscall.O_NONBLOCK
