package drives

import "testing"

func TestDriveRoots(t *testing.T) {
	for _, p := range []string{"D:", `D:\`, "d:/", "z:"} {
		if !IsDriveRoot(p) {
			t.Errorf("%s should be a drive root", p)
		}
	}
	for _, p := range []string{`D:\VIDEO_TS`, "movie.iso", "", "DD:", `\\.\D:`} {
		if IsDriveRoot(p) {
			t.Errorf("%s isn't a drive root", p)
		}
	}
	if NormalizeRoot("d:") != `D:\` || NormalizeRoot(`e:\`) != `E:\` {
		t.Error("roots normalize to D:\\")
	}
}

// Listing never fails or hangs, with or without a drive on the machine.
func TestListWorks(t *testing.T) {
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range list {
		if d.Path == "" || d.Name == "" {
			t.Errorf("incomplete drive: %+v", d)
		}
	}
}
