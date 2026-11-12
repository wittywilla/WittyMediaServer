// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !android

package mobile

// Wait blocks until the server has answered, so call it off the main thread.
// The answer is JSON: {"ok":true,"serverId":"12D3KooW...","code":"123456",
// "serverName":"...","addrs":["/ip4/..."]} to start a connection with and a
// sign-in code to redeem over it, or {"ok":false,"reason":"expired","error":"..."}.
// An error means nobody typed the code in before it ran out, or Cancel was called.
func (j *Joining) Wait() (string, error) {
	<-j.done
	return j.answer, j.err
}
