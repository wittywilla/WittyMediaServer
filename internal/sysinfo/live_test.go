package sysinfo

import (
	"runtime"
	"testing"
)

func TestLiveReadingsAreSensible(t *testing.T) {
	if all, cores, ok := ReadCPU(); ok {
		if all.Total == 0 || all.Busy > all.Total || len(cores) == 0 {
			t.Errorf("cpu: %+v, %d cores", all, len(cores))
		}
	} else if runtime.GOOS != "darwin" {
		t.Error("no CPU reading")
	}
	m, ok := ReadMemory()
	if !ok || m.Total == 0 || m.Available > m.Total || m.SwapFree > m.SwapTotal {
		t.Errorf("memory: %+v %v", m, ok)
	}
	p, ok := ReadProcess()
	if !ok || p.RSS == 0 {
		t.Errorf("process: %+v %v", p, ok)
	}
	v := ReadVolumes()
	if len(v) == 0 {
		t.Error("no volumes")
	}
	for _, d := range v {
		if d.Mount == "" || d.Total == 0 || d.Free > d.Total {
			t.Errorf("volume: %+v", d)
		}
	}
	for _, n := range ReadNetwork() {
		if n.Name == "" {
			t.Errorf("interface with no name: %+v", n)
		}
	}
	t.Logf("cores and disks: %d volumes, %d disks, %d interfaces, %d sensors", len(v), len(ReadDiskIO()), len(ReadNetwork()), len(ReadTemperatures()))
}

func BenchmarkOneLiveReading(b *testing.B) {
	for b.Loop() {
		ReadCPU()
		ReadMemory()
		ReadNetwork()
		ReadDiskIO()
		ReadProcess()
	}
}
