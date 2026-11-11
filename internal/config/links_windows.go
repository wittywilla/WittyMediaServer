// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package config

import "os"

// extraLinks is for Unix, where a service's folder is one that root installers write in. On
// Windows the installer secures the service's folder first (uninstall.SecureServiceData).
func extraLinks(os.FileInfo) bool { return false }
