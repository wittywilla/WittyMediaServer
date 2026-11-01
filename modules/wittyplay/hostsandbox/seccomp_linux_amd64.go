package hostsandbox

import "golang.org/x/sys/unix"

// pretendedHere are the older calls of the same kind that only this architecture has.
var pretendedHere = []uint32{unix.SYS_CHMOD, unix.SYS_CHOWN, unix.SYS_LCHOWN}
