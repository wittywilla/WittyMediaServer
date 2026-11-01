//go:build !windows

package uninstall

import "errors"

// StartWorker is for Windows, where a running program can't be deleted.
func StartWorker([]string, bool) error { return errors.New("not needed here") }

// RemoveSelfLater is for Windows, where a running program can't be deleted.
func RemoveSelfLater() {}
