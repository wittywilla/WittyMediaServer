package db

import "os"

func mkdir(p string) error { return os.MkdirAll(p, 0o755) }

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
