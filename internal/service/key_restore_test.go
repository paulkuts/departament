package service

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"mitm-departament/internal/repository"
	_ "modernc.org/sqlite"
)

// Схема повторяет production: key_logs.user_id ссылается на users, поэтому
// у события всегда есть автор — иначе вставка падает с FOREIGN KEY constraint.
func newKeyTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	db.MustExec(`PRAGMA foreign_keys=ON;
CREATE TABLE users(id TEXT PRIMARY KEY, is_active INTEGER NOT NULL DEFAULT 1);
CREATE TABLE keys(id INTEGER PRIMARY KEY, key_number TEXT, room_description TEXT, status TEXT, notes TEXT);
CREATE TABLE key_logs(id INTEGER PRIMARY KEY, key_id INTEGER NOT NULL REFERENCES keys(id),
  user_id TEXT NOT NULL REFERENCES users(id), action_type TEXT NOT NULL,
  comment TEXT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
INSERT INTO users VALUES('admin-1',1);
INSERT INTO keys VALUES(1,'K-1','комната 1','lost',NULL);`)
	return db
}

func TestKeyRestoreClearsLostState(t *testing.T) {
	db := newKeyTestDB(t)
	svc := NewKeyService(repository.NewKeyRepo(db, zap.NewNop()), repository.NewKeyLogRepo(db, zap.NewNop()), db, zap.NewNop())
	ctx := context.Background()

	if err := svc.RestoreLost(ctx, 1, "admin-1", "отмена утери администратором"); err != nil {
		t.Fatalf("restore lost key: %v", err)
	}
	var status, action, actor string
	if err := db.Get(&status, "SELECT status FROM keys WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	row := db.QueryRowx("SELECT action_type, user_id FROM key_logs WHERE key_id=1 ORDER BY id DESC LIMIT 1")
	if err := row.Scan(&action, &actor); err != nil {
		t.Fatalf("событие отмены утери не записано в журнал: %v", err)
	}
	if status != "available" || action != "restore" || actor != "admin-1" {
		t.Fatalf("после отмены утери ожидалось available/restore/admin-1, получено %q/%q/%q", status, action, actor)
	}

	// Повторная отмена: ключ уже не утерян — ошибка, состояние и журнал не меняются.
	var before int
	if err := db.Get(&before, "SELECT COUNT(*) FROM key_logs"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RestoreLost(ctx, 1, "admin-1", ""); err == nil {
		t.Fatal("повторная отмена утери принята")
	}
	var after int
	if err := db.Get(&after, "SELECT COUNT(*) FROM key_logs"); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&status, "SELECT status FROM keys WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if before != after || status != "available" {
		t.Fatalf("отказ изменил состояние: журнал %d → %d, статус %q", before, after, status)
	}

	// Утеря после отмены снова возможна, событие тоже попадает в журнал.
	if err := svc.MarkLost(ctx, 1, "admin-1", "повторная утеря"); err != nil {
		t.Fatalf("mark lost after restore: %v", err)
	}
	if err := db.Get(&status, "SELECT status FROM keys WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	row = db.QueryRowx("SELECT action_type, user_id FROM key_logs WHERE key_id=1 ORDER BY id DESC LIMIT 1")
	if err := row.Scan(&action, &actor); err != nil {
		t.Fatalf("событие утери не записано в журнал: %v", err)
	}
	if status != "lost" || action != "lost" || actor != "admin-1" {
		t.Fatalf("после повторной утери ожидалось lost/lost/admin-1, получено %q/%q/%q", status, action, actor)
	}
}
