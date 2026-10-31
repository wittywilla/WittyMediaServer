package logging

import "syscall"

func waitReason(syscall.WaitStatus) (string, bool) { return "", false }
