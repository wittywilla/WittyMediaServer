//go:build windows

// Command wms-setup installs WittyMediaServer for the current Windows user,
// without administrator rights:
//
//   - the program goes to %LOCALAPPDATA%\Programs\WittyMediaServer, a folder
//     the user can write, so the server can update itself from Settings;
//   - the library, accounts and settings stay in %LOCALAPPDATA%\WittyMediaServer
//     and are never touched by installing, updating or removing;
//   - it adds a Start menu entry, starts the tray app at sign-in and lists
//     the app in Settings, Apps (Installed apps) with an uninstall entry.
//
// The release's setup .exe is this program with the Windows package (.zip)
// appended. Without one (wms-setup.exe inside an unpacked package) it
// installs the files of the folder it is in.
//
//	wms-setup [/S] [/noautostart]  install or update (/S: silently;
//	                               /noautostart: don't start at sign-in)
//	wms-setup /uninstall [/S]      remove the program (data is kept)
package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/wittywilla/wittymediaserver/internal/update"
)

// version is stamped by the release scripts (-X main.version=...).
var version = "0.4.0-alpha"

const (
	appName   = "WittyMediaServer"
	trayExe   = "WittyMediaServer.exe"
	serverExe = "wms.exe"
	selfName  = "wms-setup.exe"
	runKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
)

// idOK is the OK button's answer from a message box.
const idOK = 1

var silent, noAutostart bool

func main() {
	args := os.Args[1:]
	for _, a := range args {
		switch strings.ToLower(a) {
		case "/s", "-s", "/silent", "--silent":
			silent = true
		case "/noautostart", "--no-autostart":
			noAutostart = true
		}
	}
	var err error
	switch {
	case hasArg(args, "/uninstall-from"):
		err = uninstallFinish(argAfter(args, "/uninstall-from"))
	case hasArg(args, "/uninstall"):
		err = uninstallStart()
	default:
		err = install()
	}
	if err != nil {
		if !errors.Is(err, errCancelled) {
			msg(appName+" setup", err.Error(), windows.MB_OK|windows.MB_ICONERROR)
		}
		os.Exit(1)
	}
}

var errCancelled = errors.New("cancelled")

func hasArg(args []string, a string) bool {
	for _, x := range args {
		if strings.EqualFold(x, a) {
			return true
		}
	}
	return false
}

func argAfter(args []string, a string) string {
	for i, x := range args {
		if strings.EqualFold(x, a) && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// msg shows a message box (nothing when silent) and returns the button.
func msg(title, text string, flags uint32) int32 {
	if silent {
		return idOK
	}
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(title)
	r, _ := windows.MessageBox(0, t, c, flags|windows.MB_SETFOREGROUND)
	return r
}

func installDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", errors.New("LOCALAPPDATA is not set")
	}
	return filepath.Join(base, "Programs", appName), nil
}

// dataDir is where the server keeps its library unless configured otherwise
// (see internal/config).
func dataDir() string {
	if v := os.Getenv("WMS_DATA_DIR"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), appName)
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }

// ------------------------------------------------------------------ install

func install() error {
	dir, err := installDir()
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, _ = filepath.EvalSymlinks(self)

	// What to install: the appended package, or this folder's files.
	srcDir := ""
	payload := hasPayload(self)
	if !payload {
		srcDir = filepath.Dir(self)
		if !fileExists(filepath.Join(srcDir, serverExe)) || !fileExists(filepath.Join(srcDir, "LICENSE")) {
			return errors.New("This setup has nothing to install. Download the setup .exe from github.com/" + update.Repo + "/releases.")
		}
	}
	sameDir := srcDir != "" && strings.EqualFold(filepath.Clean(srcDir), filepath.Clean(dir))
	upgrading := fileExists(filepath.Join(dir, serverExe))

	q := fmt.Sprintf("Install %s %s for %s?\n\nThe program goes to:\n%s\n\nYour library, accounts and settings are kept in:\n%s",
		appName, version, os.Getenv("USERNAME"), dir, dataDir())
	if upgrading {
		q = fmt.Sprintf("Update %s in\n%s\nto version %s?\n\nIt stops for a moment and starts again. Your library, accounts and settings are kept.", appName, dir, version)
	}
	if fileExists(filepath.Join(os.Getenv("ProgramFiles"), appName, serverExe)) {
		q += "\n\nNote: " + appName + " is also installed for all users in Program Files. Remove that copy (uninstall.ps1, as administrator) so the two don't use the same port."
	}
	if msg(appName+" setup", q, windows.MB_OKCANCEL|windows.MB_ICONQUESTION) != idOK {
		return errCancelled
	}

	if !sameDir {
		// Unpack next to the program (same drive), then swap the files in:
		// running programs are renamed aside, nothing in data is touched.
		stage := filepath.Join(dir, update.StageDir, "setup")
		_ = os.RemoveAll(stage)
		if err := os.MkdirAll(stage, 0o755); err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		if payload {
			if err := update.Extract(self, stage, 2<<30); err != nil {
				return fmt.Errorf("unpacking: %w", err)
			}
		} else if err := copyTree(srcDir, stage); err != nil {
			return err
		}
		if !fileExists(filepath.Join(stage, serverExe)) {
			return errors.New("the package has no " + serverExe)
		}
		stopRunning(dir)
		olds, err := update.Swap(dir, stage, []string{dataDir(), filepath.Join(dir, "data")})
		if err != nil {
			return err
		}
		_ = update.RecordOld(dir, olds)
		update.Cleanup(dir) // nothing runs from dir now: the old files can go
	} else {
		stopRunning(dir)
	}

	if err := register(dir); err != nil {
		return err
	}
	tray := exec.Command(filepath.Join(dir, trayExe))
	tray.Dir = dir
	if err := tray.Start(); err != nil {
		return fmt.Errorf("installed, but could not start it: %w", err)
	}
	_ = tray.Process.Release()
	done := appName + " is installed and starting. Its icon is in the notification area; on first start the setup page opens in your browser.\n\nYou'll find it in the Start menu. It starts when you sign in (turn that off from the tray icon)."
	if noAutostart {
		done = strings.TrimSuffix(done, " It starts when you sign in (turn that off from the tray icon).")
	}
	if upgrading {
		done = appName + " was updated to " + version + " and is starting again."
	}
	msg(appName+" setup", done, windows.MB_OK|windows.MB_ICONINFORMATION)
	return nil
}

// hasPayload: a zip follows the program.
func hasPayload(self string) bool {
	f, err := os.Open(self)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	_, err = zip.NewReader(f, st.Size())
	return err == nil
}

// copyTree copies a package folder, leaving out what a package never brings
// (its data folder, wms.json, leftovers of updates).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil || rel == "." {
			return err
		}
		if update.SkipPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		out := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, in); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	})
}

// register adds the Start menu entry, sign-in start and the Installed apps
// entry.
func register(dir string) error {
	tray := filepath.Join(dir, trayExe)
	icon := filepath.Join(dir, "icon.ico")
	if !fileExists(icon) {
		icon = tray
	}
	lnk := filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`, appName+".lnk")
	if err := shortcut(lnk, tray, dir, icon); err != nil {
		return fmt.Errorf("Start menu entry: %w", err)
	}
	if !noAutostart {
		if k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE); err == nil {
			_ = k.SetStringValue(appName, `"`+tray+`"`)
			k.Close()
		}
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, update.UninstallKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("Installed apps entry: %w", err)
	}
	defer k.Close()
	setup := filepath.Join(dir, selfName)
	for name, v := range map[string]string{
		"DisplayName":          appName,
		"DisplayVersion":       version,
		"Publisher":            "WittyWilla",
		"DisplayIcon":          icon,
		"InstallLocation":      dir,
		"UninstallString":      `"` + setup + `" /uninstall`,
		"QuietUninstallString": `"` + setup + `" /uninstall /S`,
		"URLInfoAbout":         "https://github.com/" + update.Repo,
		"HelpLink":             "https://github.com/" + update.Repo + "/blob/master/docs/TROUBLESHOOTING.md",
	} {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	_ = k.SetDWordValue("NoModify", 1)
	_ = k.SetDWordValue("NoRepair", 1)
	_ = k.SetDWordValue("EstimatedSize", uint32(dirSize(dir)/1024))
	return nil
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if st, err := d.Info(); err == nil {
				n += st.Size()
			}
		}
		return nil
	})
	return n
}

// shortcut writes a .lnk with Windows' own scripting object (there is no
// plain Go API for shell links).
func shortcut(lnk, target, dir, icon string) error {
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := "$s = (New-Object -ComObject WScript.Shell).CreateShortcut(" + q(lnk) + "); " +
		"$s.TargetPath = " + q(target) + "; $s.WorkingDirectory = " + q(dir) + "; " +
		"$s.IconLocation = " + q(icon) + "; $s.Description = 'WittyMediaServer media server'; $s.Save()"
	if err := os.MkdirAll(filepath.Dir(lnk), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---------------------------------------------------------------- processes

// longPath gives a path's full, long form in lower case for comparing
// (Windows may name the same folder C:\Users\LONGNA~1).
func longPath(p string) string {
	p = filepath.Clean(p)
	if u, err := windows.UTF16PtrFromString(p); err == nil {
		buf := make([]uint16, windows.MAX_LONG_PATH)
		if n, err := windows.GetLongPathName(u, &buf[0], uint32(len(buf))); err == nil && n > 0 && int(n) < len(buf) {
			p = windows.UTF16ToString(buf[:n])
		}
	}
	return strings.ToLower(p)
}

// imagePath is the program file of a running process ("" if unknown).
func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return longPath(windows.UTF16ToString(buf[:n]))
}

// running lists processes whose program is inside dir (not this one).
func running(dir string) []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	prefix := longPath(dir) + `\`
	var out []uint32
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID == uint32(os.Getpid()) || e.ProcessID == 0 {
			continue
		}
		if strings.HasPrefix(imagePath(e.ProcessID), prefix) {
			out = append(out, e.ProcessID)
		}
	}
	return out
}

func kill(pid uint32) {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return
	}
	_ = windows.TerminateProcess(h, 1)
	windows.CloseHandle(h)
}

// stopRunning stops the copy running from dir. Ending the tray app closes
// the server's control pipe, so the server shuts down cleanly by itself;
// whatever still runs after a while is ended.
func stopRunning(dir string) {
	for _, pid := range running(dir) {
		if strings.HasPrefix(filepath.Base(imagePath(pid)), "wittymediaserver") {
			kill(pid)
		}
	}
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		if len(running(dir)) == 0 {
			return
		}
	}
	for _, pid := range running(dir) {
		kill(pid)
	}
	time.Sleep(time.Second)
}

// ---------------------------------------------------------------- uninstall

// uninstallStart asks, then runs a copy of this program from the temp
// folder, which can delete the program folder (this file included).
func uninstallStart() error {
	dir, err := installDir()
	if err != nil {
		return err
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, update.UninstallKey, registry.QUERY_VALUE); err == nil {
		if loc, _, err := k.GetStringValue("InstallLocation"); err == nil && loc != "" {
			dir = loc
		}
		k.Close()
	}
	q := fmt.Sprintf("Remove %s from this computer?\n\nYour library, accounts and settings stay in\n%s\nin case you install it again. Delete that folder yourself to remove them too.", appName, dataDir())
	if msg(appName+" setup", q, windows.MB_OKCANCEL|windows.MB_ICONQUESTION) != idOK {
		return errCancelled
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("wms-uninstall-%d.exe", os.Getpid()))
	if err := copyFile(self, tmp); err != nil {
		return err
	}
	args := []string{"/uninstall-from", dir}
	if silent {
		args = append(args, "/S")
	}
	cmd := exec.Command(tmp, args...)
	cmd.Dir = os.TempDir()
	if err := cmd.Start(); err != nil {
		return err
	}
	// Exit now: this file is one of those being removed.
	return cmd.Process.Release()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, in); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func uninstallFinish(dir string) error {
	dir = filepath.Clean(dir)
	// Only ever a WittyMediaServer program folder.
	if !filepath.IsAbs(dir) || !strings.EqualFold(filepath.Base(dir), appName) || !fileExists(filepath.Join(dir, serverExe)) {
		return fmt.Errorf("%s does not look like a %s program folder; nothing was removed", dir, appName)
	}
	time.Sleep(time.Second) // let the copy that started this exit
	stopRunning(dir)

	lnk := filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`, appName+".lnk")
	_ = os.Remove(lnk)
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
		if v, _, err := k.GetStringValue(appName); err == nil && strings.HasPrefix(longPath(strings.Trim(v, `" `)), longPath(dir)+`\`) {
			_ = k.DeleteValue(appName)
		}
		k.Close()
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, update.UninstallKey, registry.QUERY_VALUE); err == nil {
		loc, _, _ := k.GetStringValue("InstallLocation")
		k.Close()
		if loc != "" && longPath(loc) == longPath(dir) {
			_ = registry.DeleteKey(registry.CURRENT_USER, update.UninstallKey)
		}
	}

	// The program's files go; a data folder or wms.json someone put there stays.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var left []string
	for _, e := range entries {
		if strings.EqualFold(e.Name(), "data") || strings.EqualFold(e.Name(), "wms.json") {
			left = append(left, e.Name())
			continue
		}
		p := filepath.Join(dir, e.Name())
		err := os.RemoveAll(p)
		for i := 0; err != nil && i < 20; i++ { // a program that just exited may hold its file a moment longer
			time.Sleep(500 * time.Millisecond)
			err = os.RemoveAll(p)
		}
		if err != nil {
			left = append(left, e.Name())
		}
	}
	_ = os.Remove(dir)
	text := appName + " was removed. Your library, accounts and settings are still in\n" + dataDir()
	if len(left) > 0 {
		text += "\n\nKept in " + dir + ": " + strings.Join(left, ", ")
	}
	msg(appName+" setup", text, windows.MB_OK|windows.MB_ICONINFORMATION)

	// This copy in the temp folder deletes itself once it has exited.
	// cmd.exe has its own quoting rules, so its command line is given as is.
	self, _ := os.Executable()
	del := exec.Command("cmd.exe")
	del.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW,
		CmdLine: `cmd.exe /d /c ping -n 3 127.0.0.1 >nul & del /f /q "` + self + `"`}
	_ = del.Start()
	return nil
}
