package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/diagnose"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/server"
)

const logsUsage = `usage:
  wittymediaserver logs zip [-data <dir>] [-o out.zip] [-days N] [--raw]
                  the logs as a ZIP, sanitized for sharing unless --raw
  wittymediaserver logs sanitize [-data <dir>] [-o out.txt] <file>
                  a sanitized copy of any text log, to out.txt or the screen`

// cmdLogs gets the logs out with no server running. It only reads: the database is looked
// at for the names to hide, and if it can't be read that's said, and it carries on.
func cmdLogs(args []string) int {
	// A database it can't read is said here in plain words, not as a logged error.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, logsUsage)
		return 2
	}
	switch args[0] {
	case "zip":
		return cmdLogsZip(args[1:])
	case "sanitize":
		return cmdLogsSanitize(args[1:])
	case "help", "-h", "--help":
		fmt.Println(logsUsage)
		return 0
	}
	fmt.Fprintln(os.Stderr, logsUsage)
	return 2
}

// logsFlags takes out this command's own flags. What's left goes to the usual
// config parsing, so -data, -config and the environment work as anywhere.
func logsFlags(args []string) (out string, days int, raw bool, rest []string, err error) {
	days = 3
	for i := 0; i < len(args); i++ {
		name, inline, has := strings.Cut(strings.TrimLeft(args[i], "-"), "=")
		value := func() (string, error) {
			if has {
				return inline, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", args[i])
			}
			i++
			return args[i], nil
		}
		if !strings.HasPrefix(args[i], "-") {
			rest = append(rest, args[i])
			continue
		}
		switch name {
		case "o", "out", "output":
			if out, err = value(); err != nil {
				return
			}
		case "days":
			var v string
			if v, err = value(); err != nil {
				return
			}
			if days, err = strconv.Atoi(v); err != nil || days < 1 || days > 14 {
				err = errors.New("-days is a number from 1 to 14")
				return
			}
		case "raw":
			raw = true
		default:
			rest = append(rest, args[i])
		}
	}
	return
}

// logsKnown reads what to hide, and where the logs are if Settings moved them.
func logsKnown(cfg *config.Config) (logging.Known, string) {
	logDir := cfg.LogDir
	k := logging.Known{}
	d, err := db.OpenForReading(cfg.DataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "The database can't be read, so names the server knows can't be looked up. Everything that looks like a name, address or path is still replaced:", err)
	} else {
		defer d.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		k = logging.KnownFromDB(ctx, d)
		var moved string
		if d.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'logDir'`).Scan(&moved) == nil && moved != "" {
			logDir = moved
		}
	}
	k.Folders = append(k.Folders, cfg.DataDir, cfg.CacheDir, cfg.TranscodeDir, cfg.ImagesDir, cfg.LogDir, cfg.ConfigFile, config.ProgramDir())
	if cfg.PublicURL != "" {
		k.Hosts = append(k.Hosts, cfg.PublicURL)
	}
	return k, logDir
}

func cmdLogsZip(args []string) int {
	out, days, raw, rest, err := logsFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		fmt.Fprintln(os.Stderr, logsUsage)
		return 2
	}
	cfg, err := config.Parse(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	known, logDir := logsKnown(cfg)
	if out == "" {
		name := "wittymediaserver-logs-sanitized-"
		if raw {
			name = "wittymediaserver-logs-"
		}
		out = name + time.Now().Format("2006-01-02-150405") + ".zip"
	}
	summary, _ := json.MarshalIndent(map[string]any{
		"version": version, "os": runtime.GOOS, "arch": runtime.GOARCH, "madeBy": "wittymediaserver logs zip, with no server running", "dataDir": cfg.DataDir, "logDir": logDir,
	}, "", "  ")
	o := logging.BundleOptions{Days: days, Sanitize: !raw, Known: known, Extra: []logging.BundleFile{{Name: "summary.json", Description: "Version and system", Data: summary}}}
	o.Extra = append(o.Extra, diagnose.NewManager(diagnose.Env{DataDir: cfg.DataDir}).BundleFiles(3)...)
	o.Earlier = server.EarlierLogs(cfg.DataDir, cfg.LogDir)
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := logging.WriteBundleDir(f, logDir, logging.OpenCrashReports(filepath.Join(cfg.DataDir, "crash")), o); err != nil {
		f.Close()
		_ = os.Remove(out)
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if raw {
		fmt.Println("Wrote", out, "- NOT sanitized: it holds your file names, what's in your libraries, account names and addresses.")
	} else {
		fmt.Println("Wrote", out, "- sanitized for sharing. Nothing was sent anywhere.")
	}
	return 0
}

func cmdLogsSanitize(args []string) int {
	out, _, _, rest, err := logsFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	var file string
	var flags []string
	for i := 0; i < len(rest); i++ {
		switch {
		case !strings.HasPrefix(rest[i], "-") && file == "":
			file = rest[i]
		case strings.HasPrefix(rest[i], "-") && !strings.Contains(rest[i], "=") && i+1 < len(rest):
			flags = append(flags, rest[i], rest[i+1])
			i++
		default:
			flags = append(flags, rest[i])
		}
	}
	if file == "" {
		fmt.Fprintln(os.Stderr, logsUsage)
		return 2
	}
	cfg, err := config.Parse(flags)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	known, _ := logsKnown(cfg)
	in, err := os.Open(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer in.Close()
	var w io.Writer = os.Stdout
	if out != "" {
		f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer f.Close()
		w = f
	}
	counts, err := logging.SanitizeLog(w, in, known)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if out != "" {
		fmt.Println("Wrote", out)
	}
	fmt.Fprintln(os.Stderr, "Replaced:", countWords(counts))
	return 0
}

func countWords(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}
