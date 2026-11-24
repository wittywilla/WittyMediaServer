// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"archive/zip"
	"bytes"
	"testing"
)

// FuzzParse: any .wittyinfo, archive or bare manifest, is read or refused, never a panic, and
// what is read has no member outside the archive's own names.
func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"format":1}`))
	f.Add([]byte(`{"format":1,"scenes":[{"start":"0:00","end":"1:00","title":"x"}]}`))
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, n := range []string{"wittyinfo.json", "../evil.png", "images/../../x.png", "/abs.png", `images\..\y.png`} {
		w, _ := zw.Create(n)
		_, _ = w.Write([]byte(`{"format":1}`))
	}
	_ = zw.Close()
	f.Add(b.Bytes())
	f.Fuzz(func(t *testing.T, data []byte) {
		x, err := Parse(data)
		if err != nil || x == nil {
			return
		}
		for name := range x.Files {
			if bytes.Contains([]byte(name), []byte("..")) || bytes.HasPrefix([]byte(name), []byte("/")) || bytes.ContainsRune([]byte(name), '\\') {
				t.Fatalf("member %q kept", name)
			}
		}
	})
}
