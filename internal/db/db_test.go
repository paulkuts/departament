package db

import (
	"context"
	"go.uber.org/zap"
	"path/filepath"
	"testing"
)

func TestForeignKeysOnReplacementConnection(t *testing.T) {
	conn, err := New(filepath.Join(t.TempDir(), "fk.db"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.MustExec("CREATE TABLE parent(id INTEGER PRIMARY KEY); CREATE TABLE child(parent_id INTEGER REFERENCES parent(id));")
	// New physical connections must enforce foreign keys too, not only the migration connection.
	conn.SetMaxIdleConns(0)
	c, err := conn.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.ExecContext(context.Background(), "INSERT INTO child(parent_id) VALUES(999)"); err == nil {
		t.Fatal("foreign key enforcement missing")
	}
}
