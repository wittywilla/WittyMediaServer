// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build android

package mobile

// Await waits for the server to answer the code, as Wait does on the other systems. Java's Object already has a
// final wait(), which a method named Wait would become in the Android library, so there it has this name.
func (j *Joining) Await() (string, error) {
	<-j.done
	return j.answer, j.err
}
