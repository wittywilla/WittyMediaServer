// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import "testing"

// A build that went wrong, or was got at, can't have a list signed that names another version
// or something else altogether: the signature is what makes an update trusted.
func TestSignRefusesAListForAnotherRelease(t *testing.T) {
	const sum = "0000000000000000000000000000000000000000000000000000000000000000"
	good := sum + "  wittymediaserver-1.0.0-linux-amd64.tar.gz\n" + sum + "  wittymediaserver-1.0.0-windows-amd64.zip\n"
	if err := checkList([]byte(good), "wittymediaserver-1.0.0"); err != nil {
		t.Errorf("this release's own list: %v", err)
	}
	for name, list := range map[string]string{
		"a newer version": sum + "  wittymediaserver-9.9.9-linux-amd64.tar.gz\n",
		"one among many":  good + sum + "  wittymediaserver-9.9.9-linux-amd64.tar.gz\n",
		"a longer number": sum + "  wittymediaserver-1.0.01-linux-amd64.tar.gz\n",
		"something else":  sum + "  wittydvd-1.0.0-linux-amd64.tar.gz\n",
		"nothing":         "",
	} {
		if err := checkList([]byte(list), "wittymediaserver-1.0.0"); err == nil {
			t.Errorf("%s: signed", name)
		}
	}
	if err := checkList([]byte("anything"), ""); err != nil {
		t.Errorf("asked to check nothing: %v", err)
	}
}
