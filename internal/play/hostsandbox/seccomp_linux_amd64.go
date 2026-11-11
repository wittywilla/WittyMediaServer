// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hostsandbox

import "golang.org/x/sys/unix"

// pretendedHere are the older calls of the same kind that only this architecture has.
var pretendedHere = []uint32{unix.SYS_CHMOD, unix.SYS_CHOWN, unix.SYS_LCHOWN}
