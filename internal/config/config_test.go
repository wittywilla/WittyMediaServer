package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, FileName)
	if err := os.WriteFile(file, []byte(`{"portable": true, "dataDir": "d", "metadataDir": "art", "listen": ":1234", "maxTranscodes": 2, "openBrowser": false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WMS_DATA_DIR", "")
	t.Setenv("WMS_LISTEN", "")
	c, err := Parse([]string{"-config", file})
	if err != nil {
		t.Fatal(err)
	}
	if c.DataDir != filepath.Join(dir, "d") {
		t.Errorf("data dir %q: relative paths must be relative to the config file", c.DataDir)
	}
	if c.ImagesDir != filepath.Join(dir, "art") || c.Listen != ":1234" || c.MaxTranscode != 2 || c.OpenBrowser || !c.Portable {
		t.Errorf("file values not applied: %+v", c)
	}
	if c.LogDir != filepath.Join(dir, "d", "logs") {
		t.Errorf("log dir %q should default inside the data dir", c.LogDir)
	}

	// Environment beats the file, flags beat both.
	t.Setenv("WMS_LISTEN", ":5555")
	c, _ = Parse([]string{"-config", file})
	if c.Listen != ":5555" {
		t.Errorf("env did not override the file: %s", c.Listen)
	}
	c, _ = Parse([]string{"-config", file, "-listen", ":7777"})
	if c.Listen != ":7777" {
		t.Errorf("flag did not override env: %s", c.Listen)
	}
}

func TestConfigTypo(t *testing.T) {
	file := filepath.Join(t.TempDir(), FileName)
	_ = os.WriteFile(file, []byte(`{"lisen": ":1"}`), 0o644)
	if _, err := Parse([]string{"-config", file}); err == nil {
		t.Fatal("an unknown setting should be reported, not ignored")
	}
}

func TestMaxConnections(t *testing.T) {
	dir := t.TempDir()
	load := func(args ...string) (*Config, error) { return Parse(append([]string{"-data", dir}, args...)) }
	t.Setenv("WMS_CONFIG", "")
	c, err := load()
	if err != nil || c.MaxConnections != DefaultConnections {
		t.Fatalf("%+v %v", c, err)
	}
	if c, err := load("-max-connections", "5000"); err != nil || c.MaxConnections != 5000 {
		t.Fatalf("%+v %v", c, err)
	}
	t.Setenv("WMS_MAX_CONNECTIONS", "300")
	if c, err := load(); err != nil || c.MaxConnections != 300 {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := load("-max-connections", "5"); err == nil {
		t.Error("too few to use the server at all")
	}
}
