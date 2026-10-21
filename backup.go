package main

// Rolling, integrity-checked backups of the Kobo's library database.
//
// Why this exists: on Oct 14 a firmware update left KoboReader.sqlite with a
// scrambled `content` table. Nickel rebuilt the library from the files on
// disk, and every finished-book flag, shelf and reading position since
// mid-September was gone, with nothing older anywhere to restore from. The
// watcher already reads this file every time she turns a page, so it is the
// natural place to keep a few known-good copies.
//
// A backup is only ever made from a database that passes SQLite's own
// integrity_check, and a failed check never touches the existing backups. That
// is the property that matters: the copy from the day before a corruption must
// still be there afterwards.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	backupPrefix = "KoboReader-"
	backupSuffix = ".sqlite"
	backupLayout = "20060102-150405"
)

// Copies the database and its -wal/-shm sidecars into a fresh temp directory,
// so nothing below ever touches the file Nickel is using. The sidecars come
// along so recent, uncheckpointed writes are included. The caller must call
// the returned cleanup.
func snapshotDB(dbPath string) (snap string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "kss-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(tmp) }

	snap = filepath.Join(tmp, "KoboReader.sqlite")
	if err := copyFile(dbPath, snap); err != nil {
		cleanup()
		return "", nil, err
	}
	for _, ext := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dbPath + ext); err == nil {
			_ = copyFile(dbPath+ext, snap+ext)
		}
	}
	return snap, cleanup, nil
}

// Runs SQLite's full integrity check. Anything but a single "ok" row is an
// error carrying the first complaint.
//
// This is not redundant with VACUUM INTO. Measured against the real corrupt
// KoboReader.sqlite from Oct 14: VACUUM INTO accepted it (exit 0) and wrote a
// 6.8MB file that was still corrupt ("Rowid 5836 out of order") with the wrong
// row count. Left to itself it would have produced a backup that looks fine.
func checkIntegrity(db *sql.DB) error {
	rows, err := db.Query("PRAGMA integrity_check")
	if err != nil {
		return err
	}
	defer rows.Close()

	var first string
	n := 0
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return err
		}
		if n == 0 {
			first = msg
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if n == 1 && first == "ok" {
		return nil
	}
	// SQLite can return a whole report as one multi-line row (hundreds of
	// "Page N: never used" lines). This ends up in a log on flash: keep the
	// first line only.
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[i+1:]
		if j := strings.IndexByte(first, '\n'); j >= 0 {
			first = first[:j]
		}
	}
	if len(first) > 160 {
		first = first[:160] + "..."
	}
	return fmt.Errorf("integrity_check failed: %s", first)
}

// Lists existing backups, newest first, by name (the name is the timestamp).
func listBackups(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, backupPrefix) && strings.HasSuffix(n, backupSuffix) {
			names = append(names, n)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names
}

func backupTime(name string, loc *time.Location) (time.Time, bool) {
	stamp := strings.TrimSuffix(strings.TrimPrefix(name, backupPrefix), backupSuffix)
	t, err := time.ParseInLocation(backupLayout, stamp, loc)
	return t, err == nil
}

// Makes a backup if one is due. Returns the new file's path, or "" with a nil
// error when nothing was due (disabled, or the newest backup is recent enough).
// A non-nil error means a backup was due but the database was not fit to copy;
// existing backups are left exactly as they were.
func backupDatabase(cfg Config, now time.Time) (string, error) {
	interval := time.Duration(cfg.int("backup_interval")) * time.Second
	if interval <= 0 {
		return "", nil
	}
	keep := cfg.int("backup_keep")
	if keep < 1 {
		keep = 1
	}
	dir := cfg.str("backup_dir")

	if existing := listBackups(dir); len(existing) > 0 {
		if t, ok := backupTime(existing[0], now.Location()); ok && now.Sub(t) < interval {
			return "", nil
		}
	}

	snap, cleanup, err := snapshotDB(cfg.str("db_path"))
	if err != nil {
		return "", err
	}
	defer cleanup()

	db, err := sql.Open("sqlite", "file:"+snap)
	if err != nil {
		return "", err
	}
	defer db.Close()
	// One connection: the pragma, the VACUUM and the check all see the same file.
	db.SetMaxOpenConns(1)

	if err := checkIntegrity(db); err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := backupPrefix + now.Format(backupLayout) + backupSuffix
	dest := filepath.Join(dir, name)
	tmp := dest + ".tmp"
	os.Remove(tmp)

	// VACUUM INTO writes one self-contained file: the WAL is folded in and
	// there are no sidecars to lose track of.
	if _, err := db.Exec("VACUUM INTO '" + strings.ReplaceAll(tmp, "'", "''") + "'"); err != nil {
		os.Remove(tmp)
		return "", err
	}

	// Check the copy too, then make it durable before it takes its real name.
	out, err := sql.Open("sqlite", "file:"+tmp+"?mode=ro&immutable=1")
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	err = checkIntegrity(out)
	out.Close()
	if err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("backup copy failed its own check: %w", err)
	}
	if f, err := os.OpenFile(tmp, os.O_RDWR, 0); err == nil {
		_ = f.Sync()
		f.Close()
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return "", err
	}

	// Only now, with a good new backup safely in place, drop the oldest.
	for i, n := range listBackups(dir) {
		if i >= keep {
			os.Remove(filepath.Join(dir, n))
		}
	}
	return dest, nil
}
