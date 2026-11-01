package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// Options say what to make.
type Options struct {
	Folders Folders
	Parts   []string
	// LiveDB is the running server's database. Without it, the file is opened to read.
	LiveDB *sql.DB
	// Password or Key protects the archive. With neither, secrets are left out.
	Password string
	Key      *Key
	// SecretSettings are settings that hold keys and passwords, on top of those that
	// look like it by their name.
	SecretSettings []string
	ServerVersion  string
	Auto           bool
	// Output is the archive's path. Empty: a new name in the backups folder.
	Output string
	// Progress is told how far along it is. It may be nil.
	Progress func(Step)
	// Problem is told about something left out that the backup carries on without.
	Problem func(file, why string)
}

// Step is progress: which stage, how much of how much, what it's on.
type Step struct {
	Phase string
	Done  int64
	Total int64
	Item  string
}

// PartSize is an estimate for the page that asks what to include.
type PartSize struct {
	Part
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

// Estimate says about how large each part is, before compression. It reads no file
// contents, only the folder listings and the database's size.
func Estimate(ctx context.Context, f Folders, live *sql.DB) []PartSize {
	out := []PartSize{}
	idx := map[string]int{}
	for _, p := range Parts() {
		idx[p.ID] = len(out)
		out = append(out, PartSize{Part: p})
	}
	seen := map[string]bool{}
	for _, src := range sources() {
		root := src.Root(f)
		_ = walkRegular(root, func(rel string, fi fs.FileInfo) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if src.Skip != nil && src.Skip(rel) {
				return nil
			}
			abs := filepath.Join(root, rel)
			if seen[abs] {
				return nil
			}
			seen[abs] = true
			i := idx[src.Part]
			out[i].Files++
			out[i].Bytes += fi.Size()
			return nil
		})
	}
	// The database is shared out by how many rows each part has, which is rough but
	// says the right thing: a library of 100,000 films is mostly that.
	if st, err := os.Stat(filepath.Join(f.Data, "wms.db")); err == nil && live != nil {
		rows := map[string]int64{}
		var total int64
		for table, part := range tableParts {
			var n int64
			if live.QueryRowContext(ctx, `SELECT COUNT(*) FROM "`+table+`"`).Scan(&n) == nil {
				rows[part] += n
				total += n
			}
		}
		if total > 0 {
			for part, n := range rows {
				if i, ok := idx[part]; ok {
					out[i].Bytes += st.Size() * n / total
				}
			}
		}
	} else if err == nil {
		out[idx[PartLibrary]].Bytes += st.Size()
	}
	return out
}

// newName makes a name for an archive that isn't taken.
func newName(f Folders, auto bool, at time.Time) string {
	prefix := namePrefix
	if auto {
		prefix = autoPrefix
	}
	name := prefix + at.Format(stampForm) + nameSuffix
	for i := 2; ; i++ {
		if _, err := os.Lstat(filepath.Join(f.Dir(), name)); errors.Is(err, fs.ErrNotExist) {
			return name
		}
		name = fmt.Sprintf("%s%s-%d%s", prefix, at.Format(stampForm), i, nameSuffix)
	}
}

// stored says a file is already as small as it gets, so nothing is gained trying.
func stored(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".zip", ".mp4", ".mkv", ".ts", ".m2ts", ".mov", ".webm", ".mp3", ".aac", ".flac", ".ogg", ".m4a", ".7z", ".gz":
		return true
	}
	return false
}

type zipSink struct {
	zw   *zip.Writer
	opts *Options
	step Step
	man  *Manifest
	buf  []byte
}

func (z *zipSink) report(item string) {
	if z.opts.Progress != nil {
		s := z.step
		s.Item = item
		z.opts.Progress(s)
	}
}

// addFile copies one file in, hashing what really went in.
func (z *zipSink) addFile(ctx context.Context, part, name, src string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		if z.opts.Problem != nil {
			z.opts.Problem(name, "it couldn't be read, so it was left out: "+plainFSError(err))
		}
		return nil
	}
	defer f.Close()
	method := zip.Deflate
	if stored(name) {
		method = zip.Store
	}
	w, err := z.zw.CreateHeader(&zip.FileHeader{Name: name, Method: method, Modified: z.man.Created})
	if err != nil {
		return err
	}
	h := sha256.New()
	if z.buf == nil {
		z.buf = make([]byte, 256<<10)
	}
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, rerr := f.Read(z.buf)
		if n > 0 {
			if _, err := w.Write(z.buf[:n]); err != nil {
				return err
			}
			h.Write(z.buf[:n])
			total += int64(n)
			z.step.Done += int64(n)
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			// The entry has started, and all that's left is to say it's short.
			if z.opts.Problem != nil {
				z.opts.Problem(name, "it stopped being readable part way: "+plainFSError(rerr))
			}
			break
		}
	}
	z.report(name)
	z.man.Files = append(z.man.Files, FileEntry{Path: name, Part: part, Size: total, SHA256: hex.EncodeToString(h.Sum(nil))})
	return nil
}

func plainFSError(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// Create makes an archive, and returns its path. Everything happens in a temporary file
// beside it, so a backup that fails or is cancelled leaves nothing that looks like one.
func Create(ctx context.Context, o Options) (out string, man *Manifest, err error) {
	parts, err := CleanParts(o.Parts)
	if err != nil {
		return "", nil, err
	}
	want := map[string]bool{}
	for _, p := range parts {
		want[p] = true
	}
	for _, p := range parts {
		part, _ := partByID(p)
		for _, n := range part.Needs {
			if !want[n] {
				need, _ := partByID(n)
				return "", nil, fmt.Errorf("%s needs %s too", part.Label, need.Label)
			}
		}
	}
	key := o.Key
	if key == nil && o.Password != "" {
		if key, err = DeriveKey(o.Password); err != nil {
			return "", nil, err
		}
	}
	if err := os.MkdirAll(o.Folders.Dir(), 0o700); err != nil {
		return "", nil, err
	}
	at := time.Now()
	out = o.Output
	if out == "" {
		out = filepath.Join(o.Folders.Dir(), newName(o.Folders, o.Auto, at))
	}
	work, err := os.MkdirTemp(filepath.Dir(out), ".backup-work-")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(work)

	report := func(phase string) {
		if o.Progress != nil {
			o.Progress(Step{Phase: phase})
		}
	}
	// What's coming, to check there's room and to say how far along it is.
	report("Working out what to include")
	sizes := Estimate(ctx, o.Folders, o.LiveDB)
	var estimate int64
	for _, s := range sizes {
		if want[s.ID] {
			estimate += s.Bytes
		}
	}
	if free := sysinfo.FreeBytes(work); free > 0 && free < estimate/2 {
		return "", nil, fmt.Errorf("there isn't room for this backup: it's about %d MB and the drive holding the backups folder has %d MB free", estimate>>20, free>>20)
	}

	man = &Manifest{Format: FormatVersion, App: "WittyMediaServer", ServerVersion: o.ServerVersion, Created: at.UTC(), Protected: key != nil, Auto: o.Auto, DataDir: o.Folders.Data}

	// The database: a consistent copy, with what wasn't asked for taken out of it.
	dbFile := filepath.Join(work, "wms.db")
	hasDB := false
	for _, p := range parts {
		if part, _ := partByID(p); part.HasTables {
			hasDB = true
		}
	}
	if hasDB {
		report("Copying the database")
		if man.Schema, err = copyDatabase(ctx, o, dbFile, want, key != nil); err != nil {
			return "", nil, err
		}
	} else if v, verr := liveVersions(ctx, o); verr == nil {
		man.Schema = v
	}

	tmp, err := os.CreateTemp(filepath.Dir(out), ".backup-*.tmp")
	if err != nil {
		return "", nil, err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	_ = tmp.Chmod(0o600)

	// Protected: the inner archive goes through the seal, into one entry of the outer one.
	var outer *zip.Writer
	var seal *sealWriter
	var inner io.Writer = tmp
	if key != nil {
		h, herr := key.header()
		if herr != nil {
			return "", nil, herr
		}
		man.Crypt = h
		outer = zip.NewWriter(tmp)
		if w, werr := outer.Create(ReadmeName); werr == nil {
			_, _ = io.WriteString(w, protectedReadme)
		}
		entry, cerr := outer.CreateHeader(&zip.FileHeader{Name: PayloadName, Method: zip.Store, Modified: at.UTC()})
		if cerr != nil {
			return "", nil, cerr
		}
		if seal, err = newSealWriter(entry, h, key.Secret); err != nil {
			return "", nil, err
		}
		inner = seal
	}
	zw := zip.NewWriter(inner)
	sink := &zipSink{zw: zw, opts: &o, man: man, step: Step{Phase: "Writing the backup", Total: estimate}}

	if hasDB {
		if err = sink.addFile(ctx, "", dbEntry, dbFile); err != nil {
			return "", nil, err
		}
	}
	summary := map[string]*PartSummary{}
	for _, p := range parts {
		summary[p] = &PartSummary{ID: p}
	}
	seen := map[string]bool{}
	for _, src := range sources() {
		if !want[src.Part] {
			continue
		}
		root := src.Root(o.Folders)
		walkErr := walkRegular(root, func(rel string, fi fs.FileInfo) error {
			if src.Skip != nil && src.Skip(rel) {
				return nil
			}
			abs := filepath.Join(root, rel)
			if seen[abs] {
				return nil
			}
			seen[abs] = true
			// What a restore would refuse can't go in, or the whole backup would be refused.
			head := readHead(abs)
			if err := src.Check(rel, head); err != nil {
				return nil
			}
			if src.Prefix == "themes/" {
				if b, rerr := os.ReadFile(abs); rerr != nil || themes.CheckZip(b) != nil {
					return nil // a built-in one, which every server makes for itself
				}
			}
			if unsafeName(src.Prefix+rel) != "" {
				if o.Problem != nil {
					o.Problem(src.Prefix+rel, "its name can't be kept in a backup, so it was left out")
				}
				return nil
			}
			before := len(man.Files)
			if err := sink.addFile(ctx, src.Part, src.Prefix+rel, abs); err != nil {
				return err
			}
			if len(man.Files) > before {
				last := man.Files[len(man.Files)-1]
				summary[src.Part].Files++
				summary[src.Part].Bytes += last.Size
			}
			return nil
		})
		if walkErr != nil {
			err = walkErr
			return "", nil, err
		}
	}
	for _, p := range parts {
		man.Parts = append(man.Parts, *summary[p])
	}
	if len(man.Files) > maxEntries {
		err = errors.New("this backup would hold too many files")
		return "", nil, err
	}

	// The manifest goes last, when every hash is known.
	report("Finishing")
	mw, cerr := zw.Create(ManifestName)
	if cerr != nil {
		err = cerr
		return "", nil, err
	}
	innerMan := *man
	innerMan.Crypt = nil
	b, jerr := json.MarshalIndent(&innerMan, "", " ")
	if jerr != nil {
		err = jerr
		return "", nil, err
	}
	if _, err = mw.Write(b); err != nil {
		return "", nil, err
	}
	if rw, rerr := zw.Create(ReadmeName); rerr == nil {
		_, _ = io.WriteString(rw, plainReadme)
	}
	if err = zw.Close(); err != nil {
		return "", nil, err
	}
	if key != nil {
		if err = seal.Close(); err != nil {
			return "", nil, err
		}
		outerMan := *man
		outerMan.Files, outerMan.DataDir, outerMan.Parts = nil, "", nil
		for _, p := range man.Parts {
			outerMan.Parts = append(outerMan.Parts, PartSummary{ID: p.ID}) // how much of each says something too
		}
		w, cerr := outer.Create(ManifestName)
		if cerr != nil {
			err = cerr
			return "", nil, err
		}
		ob, jerr := json.MarshalIndent(&outerMan, "", " ")
		if jerr != nil {
			err = jerr
			return "", nil, err
		}
		if _, err = w.Write(ob); err != nil {
			return "", nil, err
		}
		if err = outer.Close(); err != nil {
			return "", nil, err
		}
	}
	if err = tmp.Sync(); err != nil {
		return "", nil, err
	}
	if err = tmp.Close(); err != nil {
		return "", nil, err
	}
	if err = os.Rename(tmpName, out); err != nil {
		return "", nil, err
	}
	return out, man, nil
}

func liveVersions(ctx context.Context, o Options) (db.Versions, error) {
	if o.LiveDB != nil {
		return db.ReadVersions(ctx, o.LiveDB, "")
	}
	d, err := db.OpenExisting(o.Folders.Data, true)
	if err != nil {
		return db.Versions{}, err
	}
	defer d.Close()
	return db.ReadVersions(ctx, d.DB, "")
}

func readHead(p string) []byte {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	b := make([]byte, 16)
	n, _ := io.ReadFull(f, b)
	return b[:n]
}

// secretName is how a setting says it holds a secret, by its name.
var secretName = regexp.MustCompile(`(?i)(key|secret|token|password|passwd|pin|credential|agents)$`)

// credentialsInAddress finds a login in an address, and values that look like one in its query.
var (
	credentialsInAddress = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s:@"]+:[^/\s@"]*@`)
	secretInQuery        = regexp.MustCompile(`(?i)([?&](?:[a-z0-9_-]*(?:token|password|passwd|secret|key|auth)|pass|pwd|sig|st|username|user)=)[^&\s"']*`)
)

// copyDatabase writes the working copy: only what was asked for, and no sign-ins. Without a
// password, nothing secret either. It finishes with a second VACUUM, so what was deleted
// isn't still lying in free pages.
func copyDatabase(ctx context.Context, o Options, out string, want map[string]bool, protected bool) (db.Versions, error) {
	var none db.Versions
	raw := out + ".raw"
	defer os.Remove(raw)
	live := o.LiveDB
	if live == nil {
		d, err := db.OpenExisting(o.Folders.Data, false) // VACUUM INTO only reads, and needs a connection that may
		if err != nil {
			return none, fmt.Errorf("the database can't be opened: %w", err)
		}
		defer d.Close()
		live = d.DB
	}
	if err := db.SnapshotTo(ctx, live, raw); err != nil {
		return none, err
	}
	d, err := db.OpenPlainFile(raw)
	if err != nil {
		return none, err
	}
	defer d.Close()
	v, err := db.ReadVersions(ctx, d, "")
	if err != nil {
		return none, err
	}
	rows, err := d.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return none, err
	}
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return none, err
		}
		tables = append(tables, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return none, err
	}
	// Triggers would fire while this deletes, and matter only to a live database.
	if err := dropTriggers(ctx, d); err != nil {
		return none, err
	}
	for _, t := range tables {
		part, known := tableParts[t]
		switch {
		case t == "schema" || t == "sqlite_sequence":
			continue
		case t == "settings":
			if want[PartSettings] {
				continue
			}
			if _, err := d.ExecContext(ctx, `DELETE FROM settings WHERE key NOT LIKE 'schema.%'`); err != nil {
				return none, err
			}
		case neverTables[t] || !known || !want[part]:
			if _, err := d.ExecContext(ctx, `DELETE FROM "`+t+`"`); err != nil {
				return none, err
			}
		}
	}
	if !protected {
		if err := stripSecrets(ctx, d, o.SecretSettings); err != nil {
			return none, err
		}
	}
	if _, err := d.ExecContext(ctx, `VACUUM INTO ?`, out); err != nil {
		return none, err
	}
	return v, nil
}

func dropTriggers(ctx context.Context, d *sql.DB) error {
	rows, err := d.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'trigger'`)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, n)
		}
	}
	rows.Close()
	for _, n := range names {
		if _, err := d.ExecContext(ctx, `DROP TRIGGER "`+strings.ReplaceAll(n, `"`, `""`)+`"`); err != nil {
			return err
		}
	}
	return nil
}

// isSecretSetting says whether a setting is one to leave out of an archive with no password.
func isSecretSetting(key string, listed map[string]bool) bool {
	if strings.HasPrefix(key, "schema.") {
		return false
	}
	return listed[key] || secretName.MatchString(key) || key == "serverId"
}

// stripSecrets leaves out what would let someone in or cost someone money.
func stripSecrets(ctx context.Context, d *sql.DB, secret []string) error {
	listed := map[string]bool{}
	for _, k := range secret {
		listed[k] = true
	}
	rows, err := d.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return err
	}
	type kv struct{ k, v string }
	var drop, clean []kv
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return err
		}
		if isSecretSetting(k, listed) {
			drop = append(drop, kv{k: k})
		} else if c := scrubAddress(v); c != v {
			clean = append(clean, kv{k, c})
		}
	}
	rows.Close()
	for _, x := range drop {
		if _, err := d.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, x.k); err != nil {
			return err
		}
	}
	for _, x := range clean {
		if _, err := d.ExecContext(ctx, `UPDATE settings SET value = ? WHERE key = ?`, x.v, x.k); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`UPDATE users SET totp_secret = NULL, totp_on = 0, recovery_codes = NULL`,
		`DELETE FROM share_links`,
		`UPDATE epg_sources SET auth = ''`,
	} {
		if _, err := d.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	// Addresses of TV sources may have a login in them.
	for _, col := range []struct{ table, id, col string }{{"tuners", "id", "options"}, {"epg_sources", "id", "url"}, {"podcasts", "id", "url"}} {
		if err := scrubColumn(ctx, d, col.table, col.id, col.col); err != nil {
			return err
		}
	}
	return nil
}

func scrubAddress(v string) string {
	if !strings.Contains(v, "://") {
		return v
	}
	v = credentialsInAddress.ReplaceAllString(v, "$1")
	return secretInQuery.ReplaceAllString(v, "${1}")
}

func scrubColumn(ctx context.Context, d *sql.DB, table, id, col string) error {
	rows, err := d.QueryContext(ctx, `SELECT `+id+`, `+col+` FROM `+table+` WHERE `+col+` LIKE '%://%'`)
	if err != nil {
		return err
	}
	type row struct {
		id int64
		v  string
	}
	var change []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.v); err != nil {
			rows.Close()
			return err
		}
		if c := scrubAddress(r.v); c != r.v {
			r.v = c
			change = append(change, r)
		}
	}
	rows.Close()
	for _, r := range change {
		if _, err := d.ExecContext(ctx, `UPDATE `+table+` SET `+col+` = ? WHERE `+id+` = ?`, r.v, r.id); err != nil {
			return err
		}
	}
	return nil
}

const plainReadme = `WittyMediaServer backup

This holds your server's data and metadata, as of the date in manifest.json. To bring it
back, use Settings, Backup, or first-time setup, or: wms backup restore <this file>

What isn't here, on purpose: your media files, modules and their programs, game cores,
BIOS files, logs, sign-ins, API keys, two-factor secrets and share links. Passwords are
only ever kept as a hash, and it's still best to keep this file somewhere private.

metadata/ holds one plain text file for each title, if you made them. You can fix them by
hand, in this ZIP, before restoring. Anything else changed here stops the restore.
`

const protectedReadme = `WittyMediaServer backup, protected with a password

Everything is inside payload.wmsenc, sealed with AES-256-GCM. It can't be opened or changed
without the password. To bring it back, use Settings, Backup, or first-time setup, or:
wms backup restore <this file>

If you lose the password, nobody can open this file.
`
