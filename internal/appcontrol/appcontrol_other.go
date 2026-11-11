// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package appcontrol

const canVerify = false

func state() string { return "" }

func verify(string) error { return nil }

func policyBlock(error) bool { return false }
