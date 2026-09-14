package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"

	"mitm-departament/internal/models"
	"mitm-departament/internal/repository"
)

// Схема повторяет продакшн после миграции key_scans: user_id необязателен,
// у гостя заполняются ФИО, телефон и метка браузера.
func newScanTestDB(t *testing.T) *sqlx.DB {
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
CREATE TABLE key_logs(id INTEGER PRIMARY KEY AUTOINCREMENT, key_id INTEGER NOT NULL REFERENCES keys(id),
  user_id TEXT REFERENCES users(id), action_type TEXT NOT NULL, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
  comment TEXT, guest_name TEXT, guest_phone TEXT, guest_token TEXT);
INSERT INTO users VALUES('staff-1',1);
INSERT INTO users VALUES('staff-2',1);
INSERT INTO keys VALUES(1,'K-1','комната 1','available',NULL);
INSERT INTO keys VALUES(2,'K-2','комната 2','available',NULL);
INSERT INTO keys VALUES(3,'K-3','комната 3','lost',NULL);`)
	return db
}

func newScanService(t *testing.T, db *sqlx.DB) *KeyService {
	t.Helper()
	return NewKeyService(repository.NewKeyRepo(db, zap.NewNop()), repository.NewKeyLogRepo(db, zap.NewNop()), db, zap.NewNop())
}

func keyState(t *testing.T, db *sqlx.DB, id int64) (string, int) {
	t.Helper()
	var status string
	var logs int
	if err := db.Get(&status, `SELECT status FROM keys WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&logs, `SELECT COUNT(*) FROM key_logs WHERE key_id=?`, id); err != nil {
		t.Fatal(err)
	}
	return status, logs
}

func holderRow(t *testing.T, db *sqlx.DB, id int64) (action string, userID, guest, phone, token *string) {
	t.Helper()
	row := db.QueryRowx(`SELECT action_type,user_id,guest_name,guest_phone,guest_token FROM key_logs
		WHERE key_id=? ORDER BY id DESC LIMIT 1`, id)
	if err := row.Scan(&action, &userID, &guest, &phone, &token); err != nil {
		t.Fatal(err)
	}
	return action, userID, guest, phone, token
}

func staff(id string) ScanActor { return ScanActor{UserID: id} }
func guest(name, phone, token string) ScanActor {
	return ScanActor{Name: name, Phone: phone, Token: token}
}

// Сотрудник берёт свободный ключ и сдаёт его повторным сканом.
func TestScanStaffTakesThenReturns(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	out, err := svc.Scan(ctx, 1, staff("staff-1"))
	if err != nil {
		t.Fatalf("выдача: %v", err)
	}
	if out.Action != models.ActionIssue || out.KeyNumber != "K-1" || out.Transferred {
		t.Fatalf("ожидалась выдача K-1 без передачи, получено %+v", out)
	}
	status, logs := keyState(t, db, 1)
	if status != "issued" || logs != 1 {
		t.Fatalf("после выдачи ожидалось issued/1, получено %s/%d", status, logs)
	}
	if action, userID, _, _, _ := holderRow(t, db, 1); action != "issue" || userID == nil || *userID != "staff-1" {
		t.Fatalf("в журнале ожидалось issue от staff-1, получено %s/%v", action, userID)
	}

	out, err = svc.Scan(ctx, 1, staff("staff-1"))
	if err != nil {
		t.Fatalf("возврат: %v", err)
	}
	if out.Action != models.ActionReturn || out.Transferred {
		t.Fatalf("ожидался возврат, получено %+v", out)
	}
	if status, logs = keyState(t, db, 1); status != "available" || logs != 2 {
		t.Fatalf("после возврата ожидалось available/2, получено %s/%d", status, logs)
	}
}

// Ключ, взятый другим сотрудником, повторным сканом переходит к сканирующему.
func TestScanStaffHandsKeyOver(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	if _, err := svc.Scan(ctx, 1, staff("staff-1")); err != nil {
		t.Fatal(err)
	}
	out, err := svc.Scan(ctx, 1, staff("staff-2"))
	if err != nil {
		t.Fatalf("передача: %v", err)
	}
	if out.Action != models.ActionIssue || !out.Transferred {
		t.Fatalf("ожидалась выдача с передачей, получено %+v", out)
	}
	status, logs := keyState(t, db, 1)
	if status != "issued" || logs != 3 {
		t.Fatalf("ожидалось issued/3 (выдача, возврат, выдача), получено %s/%d", status, logs)
	}
	if action, userID, _, _, _ := holderRow(t, db, 1); action != "issue" || userID == nil || *userID != "staff-2" {
		t.Fatalf("держателем должен быть staff-2, в журнале %s/%v", action, userID)
	}
}

// Гость берёт ключ по QR: в журнале нет user_id, но есть имя, телефон и метка браузера.
func TestScanGuestTakesKey(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	out, err := svc.Scan(ctx, 2, guest("Иван Петров", "+7 999 123-45-67", "tok-1"))
	if err != nil {
		t.Fatalf("гостевая выдача: %v", err)
	}
	if out.Action != models.ActionIssue || out.HolderName != "Иван Петров" {
		t.Fatalf("ожидалась выдача гостю, получено %+v", out)
	}
	action, userID, name, phone, token := holderRow(t, db, 2)
	if action != "issue" || userID != nil {
		t.Fatalf("у гостя не должно быть user_id: %s/%v", action, userID)
	}
	if name == nil || *name != "Иван Петров" || phone == nil || *phone != "+7 999 123-45-67" || token == nil || *token != "tok-1" {
		t.Fatalf("сведения о госте записаны неверно: %v %v %v", name, phone, token)
	}

	// Журнал ключа отдаёт те же сведения, а время — текущее (UTC из базы).
	logs, err := svc.HistoryForKey(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].GuestName == nil || *logs[0].GuestName != "Иван Петров" ||
		logs[0].GuestPhone == nil || *logs[0].GuestPhone != "+7 999 123-45-67" {
		t.Fatalf("в журнале нет данных гостя: %+v", logs)
	}
	if age := time.Since(logs[0].Timestamp.UTC()); age < 0 || age > time.Minute {
		t.Fatalf("время события не соответствует текущему: %v (лаг %v)", logs[0].Timestamp, age)
	}
}

// Тот же гость (та же метка браузера) сканирует повторно — ключ сдан.
func TestScanGuestReturnsByToken(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	if _, err := svc.Scan(ctx, 2, guest("Иван Петров", "+79991234567", "tok-1")); err != nil {
		t.Fatal(err)
	}
	out, err := svc.Scan(ctx, 2, ScanActor{Token: "tok-1"})
	if err != nil {
		t.Fatalf("возврат по метке браузера: %v", err)
	}
	if out.Action != models.ActionReturn {
		t.Fatalf("ожидался возврат, получено %+v", out)
	}
	if status, _ := keyState(t, db, 2); status != "available" {
		t.Fatalf("ключ должен быть свободен, получено %s", status)
	}
}

// Тот же номер телефона с другого браузера (метка новая) — тоже сдача.
func TestScanGuestReturnsByPhone(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	if _, err := svc.Scan(ctx, 2, guest("Иван Петров", "+7 (999) 123-45-67", "tok-1")); err != nil {
		t.Fatal(err)
	}
	out, err := svc.Scan(ctx, 2, guest("Иван Петров", "8 999 123 45 67", "tok-2"))
	if err != nil {
		t.Fatalf("возврат по телефону: %v", err)
	}
	if out.Action != models.ActionReturn {
		t.Fatalf("ожидался возврат, получено %+v", out)
	}
	if status, _ := keyState(t, db, 2); status != "available" {
		t.Fatalf("ключ должен быть свободен, получено %s", status)
	}
}

// Другой гость сканирует занятый ключ — ключ переходит к нему.
func TestScanGuestHandsKeyToAnotherGuest(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	if _, err := svc.Scan(ctx, 2, guest("Иван Петров", "+79991234567", "tok-1")); err != nil {
		t.Fatal(err)
	}
	out, err := svc.Scan(ctx, 2, guest("Мария Сидорова", "+79990001122", "tok-9"))
	if err != nil {
		t.Fatalf("передача между гостями: %v", err)
	}
	if out.Action != models.ActionIssue || !out.Transferred || out.HolderName != "Мария Сидорова" {
		t.Fatalf("ожидалась передача Марии, получено %+v", out)
	}
	status, logs := keyState(t, db, 2)
	if status != "issued" || logs != 3 {
		t.Fatalf("ожидалось issued/3, получено %s/%d", status, logs)
	}
	if action, userID, name, _, token := holderRow(t, db, 2); action != "issue" || userID != nil || name == nil || *name != "Мария Сидорова" || token == nil || *token != "tok-9" {
		t.Fatalf("держателем должна быть Мария, в журнале %s/%v/%v", action, userID, name)
	}
}

// Утерянный ключ: скан отклоняется, состояние и журнал не меняются.
func TestScanLostKeyRefused(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	if _, err := svc.Scan(ctx, 3, staff("staff-1")); !errors.Is(err, ErrKeyLost) {
		t.Fatalf("ожидалась ошибка об утере, получено %v", err)
	}
	status, logs := keyState(t, db, 3)
	if status != "lost" || logs != 0 {
		t.Fatalf("утерянный ключ изменился: %s/%d", status, logs)
	}
}

// Гость без метки браузера обязан представиться.
func TestScanGuestWithoutDataRefused(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	if _, err := svc.Scan(ctx, 1, ScanActor{}); !errors.Is(err, ErrGuestDataNeeded) {
		t.Fatalf("ожидался запрос данных гостя, получено %v", err)
	}
	if _, err := svc.Scan(ctx, 1, guest("", "", "")); !errors.Is(err, ErrGuestDataNeeded) {
		t.Fatalf("пустые данные гостя приняты: %v", err)
	}
	if status, logs := keyState(t, db, 1); status != "available" || logs != 0 {
		t.Fatalf("свободный ключ изменился: %s/%d", status, logs)
	}
}

// Несуществующий ключ.
func TestScanMissingKey(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)

	if _, err := svc.Scan(context.Background(), 999, staff("staff-1")); err == nil {
		t.Fatal("скан несуществующего ключа не отклонён")
	}
}

// HolderInfo: держатель виден только у выданного ключа.
func TestHolderInfo(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	if holder, err := svc.HolderInfo(ctx, 1); err != nil || holder != nil {
		t.Fatalf("свободный ключ: ожидался nil, получено %v (%v)", holder, err)
	}
	if _, err := svc.Scan(ctx, 1, staff("staff-1")); err != nil {
		t.Fatal(err)
	}
	holder, err := svc.HolderInfo(ctx, 1)
	if err != nil || holder == nil {
		t.Fatalf("выданный ключ: ожидался держатель, получено %v (%v)", holder, err)
	}
	if holder.UserID == nil || *holder.UserID != "staff-1" {
		t.Fatalf("держателем должен быть staff-1, получено %v", holder.UserID)
	}
	if _, err := svc.Scan(ctx, 1, staff("staff-1")); err != nil {
		t.Fatal(err)
	}
	if holder, err := svc.HolderInfo(ctx, 1); err != nil || holder != nil {
		t.Fatalf("после сдачи ожидался nil, получено %v (%v)", holder, err)
	}
}

// Кнопки экрана передают намерение: «взять» на занятый своим же ключ, «сдать» —
// на уже свободный ключ не выполняются молча, а сообщают о состоянии.
func TestScanIntentGuards(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	if _, err := svc.Scan(ctx, 1, staff("staff-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Scan(ctx, 1, ScanActor{UserID: "staff-1", Intent: IntentTake}); !errors.Is(err, ErrAlreadyHolder) {
		t.Fatalf("повторное «взять» своим же сотрудником: %v", err)
	}
	if _, err := svc.Scan(ctx, 1, ScanActor{UserID: "staff-2", Intent: IntentReturn}); !errors.Is(err, ErrNotHolder) {
		t.Fatalf("«сдать» чужой ключ: %v", err)
	}
	if status, logs := keyState(t, db, 1); status != "issued" || logs != 1 {
		t.Fatalf("состояние изменилось при отклонённых действиях: %s/%d", status, logs)
	}

	// Кнопка «сдать» от держателя срабатывает, даже если это тот же сотрудник.
	out, err := svc.Scan(ctx, 1, ScanActor{UserID: "staff-1", Intent: IntentReturn})
	if err != nil || out.Action != models.ActionReturn {
		t.Fatalf("сдача по кнопке: %+v (%v)", out, err)
	}
	if _, err := svc.Scan(ctx, 1, ScanActor{UserID: "staff-1", Intent: IntentReturn}); !errors.Is(err, ErrNotHolder) {
		t.Fatalf("«сдать» свободный ключ: %v", err)
	}
	if status, _ := keyState(t, db, 1); status != "available" {
		t.Fatalf("ключ должен быть свободен, получено %s", status)
	}
}

// Гость с меткой браузера, но без имени не забирает ключ: иначе держатель в
// журнале был бы неизвестен. Сдача при этом работает без имени.
func TestScanGuestWithTokenStillNeedsNameToTake(t *testing.T) {
	db := newScanTestDB(t)
	svc := newScanService(t, db)
	ctx := context.Background()

	if _, err := svc.Scan(ctx, 1, ScanActor{Token: "tok-1"}); !errors.Is(err, ErrGuestDataNeeded) {
		t.Fatalf("ключ выдан гостю без имени: %v", err)
	}
	if status, logs := keyState(t, db, 1); status != "available" || logs != 0 {
		t.Fatalf("состояние ключа изменилось: %s/%d", status, logs)
	}

	// Гость взял ключ с именем и сдаёт его одной меткой браузера.
	if _, err := svc.Scan(ctx, 1, guest("Иван Петров", "+79991234567", "tok-1")); err != nil {
		t.Fatal(err)
	}
	out, err := svc.Scan(ctx, 1, ScanActor{Token: "tok-1", Intent: IntentReturn})
	if err != nil || out.Action != models.ActionReturn {
		t.Fatalf("сдача по метке браузера: %+v (%v)", out, err)
	}
}
