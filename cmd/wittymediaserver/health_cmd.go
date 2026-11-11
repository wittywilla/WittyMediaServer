// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// cmdHealth asks the running server on this machine whether it answers: the
// container image's health check, so the image needs no HTTP client of its own.
// It follows WMS_LISTEN and the config file, so a changed port still works.
//
//	wittymediaserver health [-config file] [-listen addr]
func cmdHealth(args []string) int {
	cfg, err := config.ParseOnly(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	host, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen address:", err)
		return 2
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	c := &http.Client{Timeout: 4 * time.Second}
	resp, err := c.Get("http://" + net.JoinHostPort(host, port) + "/api/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, "not answering:", err)
		return 1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "answered", resp.Status)
		return 1
	}
	return 0
}

// unwritableData explains, in a container, a data folder the server's user can't
// write: the usual first-start problem on a NAS, where the folder belongs to
// another account than the one in user:.
func unwritableData(err error, dir string) string {
	if !errors.Is(err, fs.ErrPermission) || !ffmpeg.InContainer() {
		return ""
	}
	if dir == "" {
		dir = "/config"
	}
	return fmt.Sprintf("The server runs as user %d (group %d) and can't write %s. %s"+
		"Give that folder to this user on the host (sudo chown -R %d:%d <the folder mounted at %s>), "+
		"or set user: in the compose file to the folder's owner. The Docker guide in Help says more, under Folders.",
		os.Getuid(), os.Getgid(), dir, ownerOf(dir), os.Getuid(), os.Getgid(), dir)
}

// puidMismatch explains PUID or PGID set for a container that runs as someone else:
// other images switch to them after starting as root, and this one never starts as
// root, so user: does that job. "" when neither is set or both match.
func puidMismatch(puid, pgid string, uid, gid int) string {
	want := func(v string, have int) bool { return v != "" && v != strconv.Itoa(have) }
	if !want(puid, uid) && !want(pgid, gid) {
		return ""
	}
	return fmt.Sprintf("PUID and PGID aren't used by this image, and the server runs as %d:%d. To run as %s:%s, "+
		"set user: \"%s:%s\" in the compose file (--user with docker run) and recreate the container. "+
		"The Docker guide in Help says more, under User and group IDs.",
		uid, gid, cmp.Or(puid, strconv.Itoa(uid)), cmp.Or(pgid, strconv.Itoa(gid)), cmp.Or(puid, strconv.Itoa(uid)), cmp.Or(pgid, strconv.Itoa(gid)))
}
