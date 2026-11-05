//go:build !windows

package appcontrol

const canVerify = false

func state() string { return "" }

func verify(string) error { return nil }

func policyBlock(error) bool { return false }
