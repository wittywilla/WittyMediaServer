// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !darwin

package modules

// weakSandboxForOthers: only a Mac boxes another publisher's module in with a sandbox it calls weak
// (sandbox_darwin.go). Elsewhere it is either the real sandbox or nothing.
func weakSandboxForOthers() bool { return false }
