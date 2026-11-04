package ffmpeg

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestExplainAccess(t *testing.T) {
	err := errors.New("permission denied")
	// The owner's NAS: user 999 in group 10, the render node owned by another group.
	d := GPUDevice{Path: "/dev/dri/renderD128", Group: 105, Mode: "0660"}
	explainAccess(&d, true, false, err, true, 999, []int{10})
	if !strings.Contains(d.Problem, "user 999, groups 10") || !strings.Contains(d.Problem, "group 105") {
		t.Errorf("problem %q", d.Problem)
	}
	if !strings.Contains(d.Fix, `group_add: ["105"]`) {
		t.Errorf("fix %q", d.Fix)
	}

	// Outside a container the user is added to the group instead.
	d = GPUDevice{Path: "/dev/dri/renderD128", Group: 105, Mode: "0660"}
	explainAccess(&d, true, false, err, false, 999, []int{10})
	if strings.Contains(d.Fix, "group_add") || !strings.Contains(d.Fix, "usermod -aG") {
		t.Errorf("fix outside a container %q", d.Fix)
	}

	// Only its owner may open it: a group won't help until the host allows the group.
	d = GPUDevice{Path: "/dev/dri/renderD128", Group: 0, Mode: "0600"}
	explainAccess(&d, true, false, err, true, 999, []int{10})
	if !strings.Contains(d.Problem, "only its owner") || !strings.Contains(d.Fix, "chmod g+rw") {
		t.Errorf("owner only: %q / %q", d.Problem, d.Fix)
	}

	// Mounted as a volume, not passed as a device.
	d = GPUDevice{Path: "/dev/dri/renderD128", Group: 105, Mode: "0660"}
	explainAccess(&d, false, true, errors.New("operation not permitted"), true, 999, []int{10})
	if !strings.Contains(d.Fix, "devices:") {
		t.Errorf("not allowed: %q", d.Fix)
	}
}

func TestPickRender(t *testing.T) {
	devs := []GPUDevice{
		{Path: "/dev/dri/renderD128", Vendor: "nvidia", Usable: true},
		{Path: "/dev/dri/renderD129", Vendor: "intel", Usable: true},
		{Path: "/dev/nvidia0", Vendor: "nvidia", Usable: true},
	}
	if d, ok := pickRender(devs, ""); !ok || d.Path != "/dev/dri/renderD129" {
		t.Errorf("VA-API should take Intel's node before NVIDIA's: %+v", d)
	}
	if d, ok := pickRender(devs, "intel"); !ok || d.Path != "/dev/dri/renderD129" {
		t.Errorf("Quick Sync: %+v", d)
	}
	if _, ok := pickRender(devs, "amd"); ok {
		t.Error("no AMD node here")
	}
	devs[1].Usable = false
	if d, ok := pickRender(devs, ""); !ok || d.Path != "/dev/dri/renderD128" {
		t.Errorf("only the NVIDIA node is left: %+v", d)
	}
}

func TestMissingDevice(t *testing.T) {
	old := unixGPU
	unixGPU = true
	defer func() { unixGPU = old }()

	denied := GPUDevice{Path: "/dev/dri/renderD128", Vendor: "intel", Group: 105, Problem: "The server isn't in group 105.", Fix: `group_add: ["105"]`}
	for _, vendor := range []string{"vaapi", "intel"} {
		if r := missingDevice(vendor, []GPUDevice{denied}, true); !strings.Contains(r, "group_add") {
			t.Errorf("%s: %q", vendor, r)
		}
	}
	if r := missingDevice("nvidia", []GPUDevice{denied}, true); !strings.Contains(r, "NVIDIA Container Toolkit") {
		t.Errorf("nvidia in a container: %q", r)
	}
	if r := missingDevice("vaapi", nil, true); !strings.Contains(r, "devices: - /dev/dri:/dev/dri") {
		t.Errorf("nothing passed to the container: %q", r)
	}
	if r := missingDevice("vaapi", []GPUDevice{{Path: "/dev/dxg", Vendor: "other", Usable: true}}, true); !strings.Contains(r, "/dev/dxg") {
		t.Errorf("WSL: %q", r)
	}
	ok := GPUDevice{Path: "/dev/dri/renderD128", Vendor: "amd", Usable: true}
	if r := missingDevice("vaapi", []GPUDevice{ok}, true); r != "" {
		t.Errorf("an AMD node does VA-API: %q", r)
	}
	if r := missingDevice("intel", []GPUDevice{ok}, true); !strings.Contains(r, "No Intel") {
		t.Errorf("Quick Sync on AMD: %q", r)
	}
	if r := missingDevice("nvidia", []GPUDevice{{Path: "/dev/nvidia0", Vendor: "nvidia", Usable: true}}, true); r != "" {
		t.Errorf("an NVIDIA node: %q", r)
	}
	if r := missingDevice("nvidia", []GPUDevice{{Path: "/dev/dxg", Vendor: "other", Usable: true}}, true); r != "" {
		t.Errorf("WSL reaches NVIDIA through /dev/dxg: %q", r)
	}
}

func TestExplainEncoderError(t *testing.T) {
	for _, c := range []struct{ vendor, msg, want string }{
		{"vaapi", "[AVHWDeviceContext @ 0x1] libva: va_openDriver() returns -1\nFailed to initialise VAAPI connection: -1 (unknown libva error).", "mesa-va-drivers"},
		{"intel", "[AVHWDeviceContext @ 0x1] Error creating a MFX session: -9.", "libmfx-gen1.2"},
		{"nvidia", "[h264_nvenc @ 0x1] Cannot load libcuda.so.1", "Container Toolkit"},
		{"nvidia", "[h264_nvenc @ 0x1] No capable devices found", "No NVIDIA"},
		{"vaapi", "something else entirely", ""},
	} {
		if got := explainEncoderError(c.vendor, c.msg, true); (c.want == "" && got != "") || !strings.Contains(got, c.want) {
			t.Errorf("%s %q: %q", c.vendor, c.msg, got)
		}
	}
}

func TestDeviceArgsFollowTheDeviceFound(t *testing.T) {
	hw.mu.Lock()
	oldV, oldQ := hw.vaapi, hw.qsv
	hw.vaapi, hw.qsv = "/dev/dri/renderD129", "/dev/dri/renderD129"
	hw.mu.Unlock()
	defer func() { hw.mu.Lock(); hw.vaapi, hw.qsv = oldV, oldQ; hw.mu.Unlock() }()

	tools := &Tools{Encoders: map[string]bool{}}
	if p := tools.planFor("h264_vaapi", Quality{}); !reflect.DeepEqual(p.InputArgs, []string{"-vaapi_device", "/dev/dri/renderD129"}) || planDevice(p) != "/dev/dri/renderD129" {
		t.Errorf("vaapi %q", p.InputArgs)
	}
	p := tools.planFor("h264_qsv", Quality{})
	if unixGPU {
		if !reflect.DeepEqual(p.InputArgs, []string{"-init_hw_device", "qsv=qs:hw,child_device=/dev/dri/renderD129"}) || planDevice(p) != "/dev/dri/renderD129" {
			t.Errorf("qsv %q", p.InputArgs)
		}
	} else if p.InputArgs != nil {
		t.Errorf("Quick Sync on Windows needs no device: %q", p.InputArgs)
	}
}
