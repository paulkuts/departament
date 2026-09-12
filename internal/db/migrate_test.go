package db

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestMigrationsLedgerAndRollback(t *testing.T) {
	dir := t.TempDir()
	conn, err := New(filepath.Join(dir, "test.db"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	migration := filepath.Join(dir, "001.up.sql")
	write := func(path, sql string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(sql), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(migration, "CREATE TABLE sample (id INTEGER); INSERT INTO sample VALUES (1);")
	for i := 0; i < 2; i++ {
		if err := RunMigrations(conn, dir, zap.NewNop()); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := conn.Get(&count, "SELECT COUNT(*) FROM sample"); err != nil || count != 1 {
		t.Fatalf("migration repeated: count=%d error=%v", count, err)
	}
	write(filepath.Join(dir, "002.up.sql"), "INSERT INTO sample VALUES (2); INVALID SQL;")
	if err := RunMigrations(conn, dir, zap.NewNop()); err == nil {
		t.Fatal("invalid migration accepted")
	}
	if err := conn.Get(&count, "SELECT COUNT(*) FROM sample"); err != nil || count != 1 {
		t.Fatalf("failed migration not rolled back: count=%d error=%v", count, err)
	}
	write(migration, "CREATE TABLE sample (id TEXT);")
	if err := RunMigrations(conn, dir, zap.NewNop()); err == nil {
		t.Fatal("changed applied migration accepted")
	}
}
