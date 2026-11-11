// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// dataOwnerOK stops root from writing to the database of a server that runs as someone else:
// the journal files it leaves behind would belong to root, and the server couldn't open them.
func dataOwnerOK(data string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	st, err := os.Stat(filepath.Join(data, "wms.db"))
	if err != nil {
		return nil
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok || sys.Uid == 0 {
		return nil
	}
	who := strconv.FormatUint(uint64(sys.Uid), 10)
	if u, err := user.LookupId(who); err == nil {
		who = u.Username
	}
	return fmt.Errorf("the server's files belong to %s, and this is running as root, which would stop the server opening them. Run it as that account: sudo -u %s wittymediaserver <command> -data %s", who, who, data)
}

// ownerOf says who owns a folder, as a sentence for a message, "" when it can't tell.
func ownerOf(dir string) string {
	st, err := os.Stat(dir)
	if err != nil {
		return ""
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("It belongs to %d:%d. ", sys.Uid, sys.Gid)
}
