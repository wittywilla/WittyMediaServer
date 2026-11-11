// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package uninstall

import "errors"

// SecureServiceData is for the Windows installer.
func SecureServiceData(string) error { return errors.New("only for Windows") }
