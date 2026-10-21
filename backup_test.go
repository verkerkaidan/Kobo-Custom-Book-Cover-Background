package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A WAL-mode database with enough indexed rows to span many pages, like the
// real one. Returns its path.
func makeDB(t *testing.T, rows int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "KoboReader.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, q := range []string{
		"PRAGMA page_size=1024",
		"PRAGMA journal_mode=wal",
		"CREATE TABLE content (ContentID TEXT PRIMARY KEY, Title TEXT, ReadStatus INTEGER)",
		"CREATE INDEX content_title ON content(Title)",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for i := 0; i < rows; i++ {
		if _, err := db.Exec("INSERT INTO content VALUES (?,?,?)",
			fmt.Sprintf("file:///mnt/onboard/book-%05d.epub", i),
			fmt.Sprintf("Title number %05d with some padding to fill pages", i), i%3); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	return path
}

func testCfg(db, dir string, interval, keep int) Config {
	return Config{
		"db_path":         db,
		"backup_dir":      dir,
		"backup_interval": fmt.Sprint(interval),
		"backup_keep":     fmt.Sprint(keep),
	}
}

func countRows(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM content").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBackupMakesAVerifiedStandaloneCopy(t *testing.T) {
	db := makeDB(t, 500)
	dir := t.TempDir()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	path, err := backupDatabase(testCfg(db, dir, 86400, 7), now)
	if err != nil || path == "" {
		t.Fatalf("want a backup, got path=%q err=%v", path, err)
	}
	if got := countRows(t, path); got != 500 {
		t.Fatalf("backup has %d rows, want 500", got)
	}
	// Self-contained: no sidecars left beside it, no temp file left behind.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("want exactly one file in the backup dir, got %d", len(entries))
	}
}

func TestBackupSkipsWhenNotDue(t *testing.T) {
	db := makeDB(t, 50)
	dir := t.TempDir()
	cfg := testCfg(db, dir, 86400, 7)
	t0 := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	if p, err := backupDatabase(cfg, t0); err != nil || p == "" {
		t.Fatalf("first backup: %q %v", p, err)
	}
	if p, err := backupDatabase(cfg, t0.Add(23*time.Hour)); err != nil || p != "" {
		t.Fatalf("23h later nothing should be due, got %q %v", p, err)
	}
	if p, err := backupDatabase(cfg, t0.Add(25*time.Hour)); err != nil || p == "" {
		t.Fatalf("25h later one should be due, got %q %v", p, err)
	}
}

func TestBackupKeepsOnlyTheNewest(t *testing.T) {
	db := makeDB(t, 50)
	dir := t.TempDir()
	cfg := testCfg(db, dir, 3600, 3)
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 6; i++ {
		if _, err := backupDatabase(cfg, t0.Add(time.Duration(i)*2*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	got := listBackups(dir)
	if len(got) != 3 {
		t.Fatalf("want 3 backups kept, got %d: %v", len(got), got)
	}
	if want := backupPrefix + "20261001-100000" + backupSuffix; got[0] != want {
		t.Fatalf("newest should be %s, got %s", want, got[0])
	}
}

// The property the whole feature exists for: when the live database is
// damaged, the backups from before the damage must survive untouched.
func TestCorruptDatabaseNeverOverwritesGoodBackups(t *testing.T) {
	db := makeDB(t, 2000)
	dir := t.TempDir()
	cfg := testCfg(db, dir, 3600, 2)
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 2; i++ {
		if _, err := backupDatabase(cfg, t0.Add(time.Duration(i)*2*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	before := listBackups(dir)

	// Scramble a run of pages in the middle of the file.
	f, err := os.OpenFile(db, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	junk := make([]byte, 8*1024)
	for i := range junk {
		junk[i] = byte(0xA5 ^ i)
	}
	if _, err := f.WriteAt(junk, 40*1024); err != nil {
		t.Fatal(err)
	}
	f.Close()

	path, err := backupDatabase(cfg, t0.Add(10*time.Hour))
	if err == nil || path != "" {
		t.Fatalf("a damaged database must not be backed up, got path=%q err=%v", path, err)
	}
	after := listBackups(dir)
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("existing backups changed: before %v, after %v", before, after)
	}
	for _, n := range after {
		if got := countRows(t, filepath.Join(dir, n)); got != 2000 {
			t.Fatalf("%s has %d rows, want 2000", n, got)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != len(after) {
		t.Fatalf("stray files left in the backup dir")
	}
}

func TestBackupDisabledWithZeroInterval(t *testing.T) {
	db := makeDB(t, 10)
	dir := filepath.Join(t.TempDir(), "never-created")
	if p, err := backupDatabase(testCfg(db, dir, 0, 7), time.Now()); err != nil || p != "" {
		t.Fatalf("disabled: got %q %v", p, err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("disabled backups must not create the directory")
	}
}

// Structural damage to an index page: refused, and existing backups untouched.
func TestDamagedIndexIsRefused(t *testing.T) {
	db := makeDB(t, 2000)
	dir := t.TempDir()
	cfg := testCfg(db, dir, 3600, 2)
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	if _, err := backupDatabase(cfg, t0); err != nil {
		t.Fatal(err)
	}
	before := listBackups(dir)

	// Find the index's root page and overwrite the whole page.
	conn, err := sql.Open("sqlite", "file:"+db+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	var root int
	err = conn.QueryRow("SELECT rootpage FROM sqlite_master WHERE name='content_title'").Scan(&root)
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(db, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	junk := make([]byte, 1024)
	for i := range junk {
		junk[i] = byte(0x5A + i)
	}
	if _, err := f.WriteAt(junk, int64(root-1)*1024); err != nil {
		t.Fatal(err)
	}
	f.Close()

	path, err := backupDatabase(cfg, t0.Add(5*time.Hour))
	if err == nil || path != "" {
		t.Fatalf("index damage must be caught, got path=%q err=%v", path, err)
	}
	if got := listBackups(dir); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatalf("existing backups changed: %v -> %v", before, got)
	}
}

// Damage that VACUUM INTO copies without complaint (it ignores CHECK
// constraints, like it ignored the real database's out-of-order rows), so only
// the integrity checks stand between it and a "good" backup. Without them this
// test fails: the bad row is faithfully copied into a new backup.
func TestDamageThatVacuumCopiesPastIsStillRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "KoboReader.sqlite")
	conn, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	for _, q := range []string{
		"CREATE TABLE content (ContentID TEXT PRIMARY KEY, ReadStatus INTEGER CHECK (ReadStatus IN (0,1,2)))",
		"INSERT INTO content VALUES ('a',0),('b',1),('c',2)",
		"PRAGMA ignore_check_constraints=ON",
		"INSERT INTO content VALUES ('bad',9)", // violates the CHECK
	} {
		if _, err := conn.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	conn.Close()

	dir := t.TempDir()
	p, err := backupDatabase(testCfg(path, dir, 3600, 3), time.Now())
	if err == nil || p != "" {
		t.Fatalf("a database failing integrity_check must not be backed up, got %q %v", p, err)
	}
	if got := listBackups(dir); len(got) != 0 {
		t.Fatalf("no backup should exist, got %v", got)
	}
}
