// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build linux && !amd64 && !arm64

package hostsandbox

// On 32-bit Linux every network call goes through one shared call, and the filter
// isn't worth the risk untested. These hosts still get the other layers.
func seccomp() error { return nil }
