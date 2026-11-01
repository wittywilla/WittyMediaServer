//go:build !windows

package uninstall

import "errors"

// SecureServiceData is for the Windows installer.
func SecureServiceData(string) error { return errors.New("only for Windows") }
