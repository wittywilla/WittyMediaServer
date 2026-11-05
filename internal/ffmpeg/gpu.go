package ffmpeg

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// GPUDevice is a graphics device the server can see, and whether it may use it.
// Hardware encoders fail in ways FFmpeg doesn't explain ("No VA display found"),
// so the server looks at the device itself: a container that can't open the
// render node is the usual reason, and the fix is one line of the compose file.
type GPUDevice struct {
	Path    string `json:"path"`              // /dev/dri/renderD128, /dev/nvidia0, /dev/dxg
	Vendor  string `json:"vendor,omitempty"`  // intel | amd | nvidia | other, when the system says
	Driver  string `json:"driver,omitempty"`  // the kernel's driver: i915, xe, amdgpu, nvidia
	Group   int    `json:"group"`             // the group ID that owns it, -1 when unknown
	Mode    string `json:"mode,omitempty"`    // its permissions, such as 0660
	Usable  bool   `json:"usable"`            // the server opened it
	Problem string `json:"problem,omitempty"` // why it can't be used, in plain words
	Fix     string `json:"fix,omitempty"`     // what to do about it
}

// OwnGroups are the process's group IDs, its main one first.
func OwnGroups() []int {
	gs := []int{os.Getgid()}
	more, _ := os.Getgroups()
	for _, g := range more {
		if !slices.Contains(gs, g) {
			gs = append(gs, g)
		}
	}
	return gs
}

// InContainer says whether the server runs in a container (the image sets
// WMS_DOCKER=1; Docker and Podman leave a file at the root too).
func InContainer() bool {
	if os.Getenv("WMS_DOCKER") == "1" {
		return true
	}
	for _, f := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}

// vendorOf names a PCI vendor ID as /sys writes it.
func vendorOf(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "":
		return ""
	case "0x8086":
		return "intel"
	case "0x1002", "0x1022":
		return "amd"
	case "0x10de":
		return "nvidia"
	}
	return "other"
}

// vendorOfDriver guesses the vendor from the kernel driver when /sys has no vendor file.
func vendorOfDriver(driver string) string {
	switch driver {
	case "i915", "xe":
		return "intel"
	case "amdgpu", "radeon":
		return "amd"
	case "nvidia", "nouveau":
		return "nvidia"
	case "":
		return ""
	}
	return "other"
}

// explainAccess turns a failed open of a device into the problem and the fix.
// denied is a permission error (EACCES); notAllowed is EPERM, which in a container
// means the device was mounted but not passed with devices:.
func explainAccess(d *GPUDevice, denied, notAllowed bool, err error, container bool, uid int, groups []int) {
	who := "The server (user " + strconv.Itoa(uid)
	if len(groups) > 0 {
		names := make([]string, len(groups))
		for i, g := range groups {
			names[i] = strconv.Itoa(g)
		}
		who += ", groups " + strings.Join(names, ",")
	}
	who += ")"
	rw := func(c byte) bool { return c == '6' || c == '7' }
	ownerRW := len(d.Mode) == 4 && rw(d.Mode[1])
	groupRW := len(d.Mode) == 4 && rw(d.Mode[2])
	switch {
	case notAllowed && container:
		d.Problem = "The container can see " + d.Path + " but isn't allowed to use it."
		d.Fix = "Pass the device with devices: - /dev/dri:/dev/dri (not as a volume), then recreate the container."
	case denied && d.Group >= 0 && !groupRW:
		d.Problem = fmt.Sprintf("%s can't open %s: only its owner may (permissions %s).", who, d.Path, d.Mode)
		if !ownerRW {
			d.Problem = fmt.Sprintf("%s can't open %s: its permissions (%s) let nobody but root use it.", who, d.Path, d.Mode)
		}
		d.Fix = fmt.Sprintf("On the host, let its group use it (sudo chmod g+rw %s, or the NAS's own setting for it), then add that group (%d).", d.Path, d.Group)
		if container {
			d.Fix += fmt.Sprintf(" In the compose file: group_add: [\"%d\"].", d.Group)
		}
	case denied && d.Group >= 0 && container:
		d.Problem = fmt.Sprintf("%s isn't in group %d, which owns %s.", who, d.Group, d.Path)
		d.Fix = fmt.Sprintf("Add the group's ID to the container: group_add: [\"%d\"] in the compose file (or --group-add %d), then recreate it.", d.Group, d.Group)
	case denied && d.Group >= 0:
		d.Problem = fmt.Sprintf("%s isn't in group %d, which owns %s.", who, d.Group, d.Path)
		d.Fix = fmt.Sprintf("Add the server's user to that group (sudo usermod -aG %s <user>, often the render or video group), then restart the server.", groupName(d.Group))
	case denied:
		d.Problem = who + " isn't allowed to open " + d.Path + "."
		d.Fix = "Give the server's user access to it (usually the render or video group)."
	default:
		d.Problem = "It can't be opened: " + err.Error()
	}
}

// pickRender chooses the render node for VA-API (and Quick Sync's device), by
// vendor: an NVIDIA node has no VA-API driver here, so Intel and AMD come first.
// A node whose vendor isn't known (FreeBSD has no /sys) may be anyone's, so it's
// still tried, after one known to match.
func pickRender(devs []GPUDevice, vendor string) (GPUDevice, bool) {
	rank := func(d GPUDevice) int {
		switch {
		case vendor != "" && d.Vendor != "" && d.Vendor != vendor:
			return -1
		case vendor != "" && d.Vendor == "":
			return 2
		case d.Vendor == "intel" || d.Vendor == "amd":
			return 3
		case d.Vendor == "" || d.Vendor == "other":
			return 2
		}
		return 1
	}
	best, found := GPUDevice{}, false
	for _, d := range devs {
		if !strings.HasPrefix(d.Path, "/dev/dri/renderD") || !d.Usable || rank(d) < 0 {
			continue
		}
		if !found || rank(d) > rank(best) {
			best, found = d, true
		}
	}
	return best, found
}

// whyNoRender says why there is no usable render node (for VA-API, or vendor's
// for Quick Sync), for the encoder's line in Settings.
func whyNoRender(devs []GPUDevice, vendor, what string, container bool) string {
	for _, d := range devs {
		if strings.HasPrefix(d.Path, "/dev/dri/renderD") && !d.Usable && (vendor == "" || d.Vendor == vendor || d.Vendor == "") {
			return strings.TrimSpace(d.Problem + " " + d.Fix)
		}
	}
	if vendor != "" && slices.ContainsFunc(devs, func(d GPUDevice) bool { return strings.HasPrefix(d.Path, "/dev/dri/renderD") }) {
		return "No " + what + " graphics device was found."
	}
	if slices.ContainsFunc(devs, func(d GPUDevice) bool { return d.Path == "/dev/dxg" }) {
		return "Only WSL's graphics device (/dev/dxg) is here, not a /dev/dri render node."
	}
	if container {
		return "No graphics device was passed to the container: add devices: - /dev/dri:/dev/dri to the compose file."
	}
	return "No graphics device (/dev/dri/renderD*) was found: the graphics driver may not be loaded."
}

// explainEncoderError puts the end of FFmpeg's error for a hardware encoder into
// plain words, "" when it isn't one of the known ones.
func explainEncoderError(vendor, msg string, container bool) string {
	m := strings.ToLower(msg)
	has := func(s ...string) bool {
		return slices.ContainsFunc(s, func(x string) bool { return strings.Contains(m, x) })
	}
	switch vendor {
	case "vaapi":
		switch {
		case has("va_opendriver() returns -1", "vainitialize failed", "failed to initialise vaapi connection", "unknown libva error"):
			return "The VA-API driver for this graphics chip isn't installed (intel-media-va-driver for Intel, mesa-va-drivers for AMD)."
		case has("no usable encoding profile", "no usable encoding entrypoint", "unsupported"):
			return "The graphics chip or its driver can't encode H.264 with VA-API."
		case has("permission denied"):
			return "The server isn't allowed to open the render node."
		}
	case "intel":
		switch {
		case has("error creating a mfx session", "mfx_err_unsupported", "no device available"):
			return "Quick Sync couldn't start: this needs an Intel graphics chip with Intel's media driver and oneVPL runtime (intel-media-va-driver and libmfx-gen1.2 on Debian and Ubuntu)."
		case has("failed to initialise vaapi connection", "vainitialize failed"):
			return "Intel's media driver isn't installed (intel-media-va-driver)."
		}
	case "nvidia":
		switch {
		case has("cannot load libcuda", "cannot load libnvidia-encode", "cannot load nvcuda"):
			if container {
				return "NVIDIA's driver isn't available in the container: run it with the NVIDIA Container Toolkit (gpus: all, with NVIDIA_DRIVER_CAPABILITIES including video)."
			}
			return "NVIDIA's driver isn't installed, or it's too old for this FFmpeg."
		case has("no capable devices found", "no nvenc capable devices"):
			return "No NVIDIA graphics card with a video encoder was found."
		case has("driver does not support the required nvenc api version", "minimum required nvidia driver"):
			return "NVIDIA's driver is too old for this FFmpeg: update the driver on the host."
		case has("openencodesessionex failed: out of memory", "incompatible client key"):
			return "The card has no encoding session free: consumer cards allow only a few at once."
		}
	case "amd":
		if has("dll amfrt", "libamfrt", "cannot load") {
			return "AMD's AMF runtime isn't installed (it comes with AMD's own driver)."
		}
	}
	return ""
}
