package themes

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	builtin "github.com/wittywilla/wittymediaserver/themes"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInstallListAndUpload(t *testing.T) {
	m := &Manager{Dir: t.TempDir()}
	if err := m.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	list := m.List()
	if len(list) < 19 || list[0].ID != "default" {
		t.Fatalf("built-in themes: %d, first %q", len(list), list[0].ID)
	}
	css, ct, err := m.File("light-blue", "theme.css")
	if err != nil || !strings.HasPrefix(ct, "text/css") || !strings.Contains(string(css), "--accent: #2563eb") {
		t.Fatalf("light-blue css: %v %s", err, ct)
	}
	good := zipOf(t, map[string]string{
		"theme.json":    `{"id":"my-cinema","name":"My Cinema","scheme":"dark"}`,
		"theme.css":     `:root{--accent:#ff0}body{background:url(assets/bg.jpg)}`,
		"assets/bg.jpg": "jpeg",
		"README.md":     "hi",
	})
	meta, err := m.Add(good, "upload.zip")
	if err != nil || meta.ID != "my-cinema" {
		t.Fatalf("upload: %v %+v", err, meta)
	}
	if b, ct, err := m.File("my-cinema", "assets/bg.jpg"); err != nil || ct != "image/jpeg" || string(b) != "jpeg" {
		t.Fatalf("asset: %v %s", err, ct)
	}
	if _, _, err := m.File("my-cinema", "../default.zip"); err == nil {
		t.Fatal("path traversal served a file")
	}
	for name, files := range map[string]map[string]string{
		"no css":     {"theme.json": `{"name":"x"}`},
		"script":     {"theme.json": `{"name":"x"}`, "theme.css": "", "evil.js": "alert(1)"},
		"traversal":  {"theme.json": `{"name":"x"}`, "theme.css": "", "../x.css": ""},
		"builtin id": {"theme.json": `{"id":"default","name":"x"}`, "theme.css": ""},
	} {
		if _, err := m.Add(zipOf(t, files), "x.zip"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := m.Remove("default"); err == nil {
		t.Fatal("removed a built-in theme")
	}
	if err := m.Remove("my-cinema"); err != nil {
		t.Fatal(err)
	}
}
