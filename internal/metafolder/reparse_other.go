// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package metafolder

import "os"

func isReparsePoint(fi os.FileInfo) bool { return fi.Mode()&os.ModeSymlink != 0 }
