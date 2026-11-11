// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build linux || freebsd

package ffmpeg

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// FindGPUs lists the graphics devices in /dev and whether the server can open
// each: render nodes for VA-API and Quick Sync, NVIDIA's nodes, and WSL's /dev/dxg.
// Opening one is a few system calls, no process.
func FindGPUs() []GPUDevice { return findGPUsIn("/dev", "/sys") }

// findGPUsIn looks in dev and sys, which tests point at folders of their own.
func findGPUsIn(dev, sys string) []GPUDevice {
	var paths []string
	render, _ := filepath.Glob(dev + "/dri/renderD*")
	sort.Strings(render)
	paths = append(paths, render...)
	nv, _ := filepath.Glob(dev + "/nvidia[0-9]*")
	sort.Strings(nv)
	paths = append(paths, nv...)
	if _, err := os.Stat(dev + "/dxg"); err == nil {
		paths = append(paths, dev+"/dxg")
	}
	container := InContainer()
	uid, groups := os.Getuid(), OwnGroups()
	var out []GPUDevice
	for _, p := range paths {
		name := "/dev" + strings.TrimPrefix(p, dev) // what messages and FFmpeg's options call it
		d := GPUDevice{Path: name, Group: -1}
		if st, err := os.Stat(p); err == nil {
			d.Mode = fmt.Sprintf("%04o", st.Mode().Perm())
			if raw, ok := st.Sys().(*syscall.Stat_t); ok {
				d.Group = int(raw.Gid)
			}
		}
		switch {
		case strings.HasPrefix(name, "/dev/dri/"):
			sysDir := sys + "/class/drm/" + filepath.Base(p) + "/device"
			if b, err := os.ReadFile(sysDir + "/vendor"); err == nil {
				d.Vendor = vendorOf(string(b))
			}
			if link, err := os.Readlink(sysDir + "/driver"); err == nil {
				d.Driver = filepath.Base(link)
			}
			if d.Vendor == "" {
				d.Vendor = vendorOfDriver(d.Driver)
			}
		case strings.HasPrefix(name, "/dev/nvidia"):
			d.Vendor, d.Driver = "nvidia", "nvidia"
		case name == "/dev/dxg":
			d.Vendor, d.Driver = "other", "dxg"
		}
		f, err := os.OpenFile(p, os.O_RDWR, 0)
		if err == nil {
			f.Close()
			d.Usable = true
		} else {
			explainAccess(&d, errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM), err, container, uid, groups)
		}
		out = append(out, d)
	}
	return out
}

// groupName is a group's name for a command line, or its number when this system
// doesn't know it (a container's /etc/group rarely has the host's render group).
func groupName(gid int) string {
	if g, err := user.LookupGroupId(strconv.Itoa(gid)); err == nil {
		return g.Name
	}
	return strconv.Itoa(gid)
}
