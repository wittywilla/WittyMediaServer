package themes

import (
	"archive/zip"
	"bytes"
	"testing"
)

func FuzzTokens(f *testing.F) {
	for _, s := range []string{":root{--accent:#fff}", "/* */ :root { --a: var(--b); --b: var(--a) }", "--x:url(https://h/)", "#fff", "rgb(1,2,3)", "red; background:url(x)"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		parseTokens(s)
		NormalizeColor(s)
	})
}

// A theme is an administrator's upload, and what it holds goes to every browser.
func FuzzAddTheme(f *testing.F) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("theme.json")
	_, _ = w.Write([]byte(`{"id":"x","name":"x"}`))
	w, _ = zw.Create("theme.css")
	_, _ = w.Write([]byte(":root{}"))
	w, _ = zw.Create("../../evil.css")
	_, _ = w.Write([]byte("x"))
	zw.Close()
	f.Add(buf.Bytes(), "x.zip")
	f.Add([]byte("PK\x03\x04"), "../../x.zip")
	f.Fuzz(func(t *testing.T, b []byte, name string) {
		m := &Manager{Dir: t.TempDir()}
		_, _ = m.Add(b, name)
	})
}
