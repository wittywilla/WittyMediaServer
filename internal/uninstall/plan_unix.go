package uninstall

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	unixName    = "wittymediaserver"
	serviceUser = "wms"
)

// What FreeBSD's rc.d script says, for a copy put wherever install.sh was told to.
var (
	rcData    = regexp.MustCompile(`wittymediaserver_data:=([^}\s]+)}`)
	rcProgram = regexp.MustCompile(`\s(/\S+?/bin)/wittymediaserver serve`)
)

// homes are the people on this machine whose settings can be cleared: the one running this,
// the one behind sudo, and when root, everyone under /home.
func homes(e *Env, base string) []string {
	var out []string
	add := func(h string) {
		if h == "" {
			return
		}
		for _, x := range out {
			if x == h {
				return
			}
		}
		out = append(out, h)
	}
	add(e.Home)
	if !e.IsAdmin {
		return out
	}
	if u := e.Getenv("SUDO_USER"); u != "" && u != "root" {
		add(filepath.Join(base, u))
	}
	if entries, err := os.ReadDir(base); err == nil {
		for _, ent := range entries {
			if ent.IsDir() {
				add(filepath.Join(base, ent.Name()))
			}
		}
	}
	return out
}

// planHomeUnix is what a person's own account holds on Linux and BSD.
func planHomeUnix(e *Env, pl *Plan, h string) {
	other := h != e.Home
	if p := filepath.Join(h, ".config", "autostart", "wittymediaserver.desktop"); exists(p) {
		pl.add(fileItem("start at sign-in", p, GroupProgram, other))
	}
	if p := filepath.Join(h, ".local", "share", "applications", "wittymediaserver.desktop"); exists(p) {
		pl.add(fileItem("menu entry", p, GroupProgram, other))
	}
	planDataFolder(e, pl, filepath.Join(h, ".local", "share", unixName), true, other)
	planConfigFile(e, pl, filepath.Join(h, ".config", unixName, "wms.json"), other)
	if other {
		return // someone else's choice of these folders is in their own environment, out of reach
	}
	// Your own account may keep them somewhere else, as the system's rules allow. They only
	// count when they're full paths: the same rules say anything else is to be ignored.
	if d := e.Getenv("XDG_DATA_HOME"); filepath.IsAbs(d) {
		planDataFolder(e, pl, filepath.Join(d, unixName), true, false)
		if p := filepath.Join(d, "applications", "wittymediaserver.desktop"); exists(p) {
			pl.add(fileItem("menu entry", p, GroupProgram, false))
		}
	}
	if d := e.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(d) {
		planConfigFile(e, pl, filepath.Join(d, unixName, "wms.json"), false)
		if p := filepath.Join(d, "autostart", "wittymediaserver.desktop"); exists(p) {
			pl.add(fileItem("start at sign-in", p, GroupProgram, false))
		}
	}
}

// packageTool asks the package manager about this machine: which of them installed it.
func packageTool(e *Env) (tool string, purge, remove []string) {
	if e.GOOS == "freebsd" {
		if _, err := e.Sys.Run("pkg", "info", "-e", unixName); err == nil {
			rm := []string{"pkg", "delete", "-y", unixName}
			return "pkg", rm, rm
		}
		return "", nil, nil
	}
	if _, err := e.Sys.Run("dpkg-query", "-W", "-f=${Status}", unixName); err == nil {
		return "deb", []string{"apt-get", "purge", "-y", unixName}, []string{"apt-get", "remove", "-y", unixName}
	}
	if _, err := e.Sys.Run("rpm", "-q", unixName); err == nil {
		rm := []string{"rpm", "-e", unixName}
		if _, err := e.Sys.Run("dnf", "--version"); err == nil {
			rm = []string{"dnf", "remove", "-y", unixName}
		}
		return "rpm", rm, rm
	}
	return "", nil, nil
}

func planUnix(e *Env, pl *Plan) {
	const unit = "wittymediaserver.service"
	etcUnit := e.p("/etc/systemd/system/" + unit)
	systemd := exists(e.p("/run/systemd/system")) || exists(etcUnit) || exists(e.p("/usr/lib/systemd/system/"+unit)) || exists(e.p("/lib/systemd/system/"+unit))

	tool, purge, remove := packageTool(e)
	if tool != "" {
		// The package takes out its own files, service and user. Asked to remove data, it
		// purges; otherwise it leaves them for next time.
		pl.add(Item{Kind: "package (" + tool + ")", Name: unixName, Group: GroupProgram, Admin: true, step: stepInstaller,
			remove: func() error {
				_, err := e.Sys.Run(remove[0], remove[1:]...)
				return err
			}})
		pl.add(Item{Kind: "package settings (" + tool + ")", Name: unixName, Group: GroupData, Admin: true, step: stepEmptyFolders,
			remove: func() error {
				if purge[1] == remove[1] {
					return nil
				}
				_, err := e.Sys.Run(purge[0], purge[1:]...)
				return err
			}})
	}

	// Settings beside the service: what an upgrade wrote, and what `systemctl edit` wrote. Each
	// is listed by name, so nothing goes unsaid, and only .conf files, which are all systemd
	// reads here. A folder with anything else in it stays.
	dropIns := e.p("/etc/systemd/system/" + unit + ".d")
	if confs, _ := filepath.Glob(filepath.Join(dropIns, "*.conf")); len(confs) > 0 {
		for _, conf := range confs {
			if b, err := os.ReadFile(conf); err == nil {
				if d := dataDirFromCommand(string(b)); d != "" {
					planDataFolder(e, pl, e.Prefix+d, false, true)
				}
			}
			pl.add(fileItem("service setting", conf, GroupProgram, true))
		}
		pl.add(emptyFolderItem(dropIns, GroupProgram, true))
	}
	if systemd && exists(etcUnit) {
		if b, err := os.ReadFile(etcUnit); err == nil {
			s := string(b)
			if d := dataDirFromCommand(s); d != "" {
				planDataFolder(e, pl, e.Prefix+d, false, true)
			}
			if prefix := execPrefix(s); prefix != "" {
				planProgramFolder(e, pl, e.Prefix+prefix, true)
			}
		}
		pl.add(Item{Kind: "service", Name: unit, Group: GroupProgram, Admin: true, step: stepStop,
			remove: func() error {
				_, err := e.Sys.Run("systemctl", "disable", "--now", unit)
				return err
			}})
		pl.add(Item{Kind: "service", Name: etcUnit, Group: GroupProgram, Admin: true, step: stepFiles,
			remove: func() error {
				if err := os.Remove(etcUnit); err != nil && !os.IsNotExist(err) {
					return err
				}
				_, _ = e.Sys.Run("systemctl", "daemon-reload")
				return nil
			}})
	}
	if rc := e.p("/usr/local/etc/rc.d/" + unixName); exists(rc) {
		if b, err := os.ReadFile(rc); err == nil {
			s := string(b)
			if m := rcData.FindStringSubmatch(s); m != nil {
				planDataFolder(e, pl, e.Prefix+m[1], false, true)
			}
			if m := rcProgram.FindStringSubmatch(s); m != nil && tool == "" {
				planProgramFolder(e, pl, e.Prefix+strings.TrimSuffix(m[1], "/bin"), true)
			}
		}
		pl.add(Item{Kind: "service", Name: unixName, Group: GroupProgram, Admin: true, step: stepStop,
			remove: func() error {
				_, _ = e.Sys.Run("service", unixName, "onestop")
				return nil
			}})
		if tool == "" {
			pl.add(fileItem("service", rc, GroupProgram, true))
			// install.sh makes this if no package has yet. Only ever goes when empty.
			pl.add(emptyFolderItem(filepath.Dir(rc), GroupProgram, true))
		}
		for _, v := range []string{"_enable", "_user", "_listen", "_data"} {
			name := unixName + v
			if out, err := e.Sys.Run("sysrc", "-qn", name); err == nil && strings.TrimSpace(out) != "" {
				if v == "_data" {
					planDataFolder(e, pl, e.Prefix+strings.TrimSpace(out), false, true)
				}
				pl.add(Item{Kind: "service setting in rc.conf", Name: name, Group: GroupProgram, Admin: true, step: stepFiles,
					remove: func() error {
						_, err := e.Sys.Run("sysrc", "-x", name)
						return err
					}})
			}
		}
		if p := e.p("/var/run/" + unixName + ".pid"); exists(p) {
			pl.add(fileItem("service's process file", p, GroupProgram, true))
		}
	}

	// install.sh's places. A package's own files are the package's to remove.
	for _, p := range []string{"/opt/" + unixName} {
		planProgramFolder(e, pl, e.p(p), true)
	}
	if tool == "" && e.SelfRoot != "" {
		planProgramFolder(e, pl, e.SelfRoot, false)
	}
	for _, p := range []string{"/usr/local/share/applications/wittymediaserver.desktop", "/usr/local/bin/" + unixName} {
		if q := e.p(p); exists(q) {
			pl.add(fileItem("program file", q, GroupProgram, true))
		}
	}
	if tool == "" {
		if q := e.p("/usr/share/applications/wittymediaserver.desktop"); exists(q) {
			pl.add(fileItem("menu entry", q, GroupProgram, true))
		}
	}

	// Data.
	if d := e.Getenv("WMS_DATA_DIR"); d != "" {
		planDataFolder(e, pl, d, false, false)
	}
	planDataFolder(e, pl, e.p("/var/lib/"+unixName), true, true)
	planDataFolder(e, pl, e.p("/var/db/"+unixName), true, true)
	for _, etc := range []string{"/etc/" + unixName, "/usr/local/etc/" + unixName} {
		planConfigFile(e, pl, e.p(etc+"/wms.json"), true)
		if d := e.p(etc); isDir(d) {
			pl.add(emptyFolderItem(d, GroupData, true))
		}
	}
	base := e.p("/home")
	if e.GOOS == "freebsd" {
		base = e.p("/usr/home")
	}
	for _, h := range homes(e, base) {
		planHomeUnix(e, pl, h)
	}

	// The service's own account, when nothing of its can be left behind.
	if _, err := e.Sys.Run("id", serviceUser); err == nil && tool != "deb" {
		pl.add(Item{Kind: "service account", Name: serviceUser, Group: GroupData, Admin: true, step: stepEmptyFolders,
			remove: func() error {
				if _, err := e.Sys.Run("userdel", serviceUser); err != nil {
					_, err := e.Sys.Run("pw", "userdel", serviceUser)
					return err
				}
				return nil
			}})
	}
}

// execPrefix is the program folder in a service's ExecStart line, for a copy put there by
// install.sh, which can be anywhere.
func execPrefix(unit string) string {
	for _, line := range strings.Split(unit, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "ExecStart=")
		if !ok {
			continue
		}
		exe, _, _ := strings.Cut(rest, " ")
		dir := filepath.ToSlash(filepath.Dir(exe))
		// bin/wittymediaserver, or wms directly in the folder before 0.6.0.
		if strings.HasSuffix(dir, "/bin") {
			dir = strings.TrimSuffix(dir, "/bin")
		}
		if dir == "" || dir == "/" || dir == "/usr" || dir == "/usr/local" {
			return ""
		}
		return dir
	}
	return ""
}
