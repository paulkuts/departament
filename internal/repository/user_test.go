package repository

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"mitm-departament/internal/models"
	_ "modernc.org/sqlite"
)

func strp(s string) *string { return &s }

// Дата рождения обязана читаться ровно тем значением, что записана.
// Иначе не собирается карточка пользователя: middleware после JWT не может
// загрузить пользователя и отвечает 401 на любой запрос, хотя вход проходит.
func TestUserDateOfBirthRoundTrip(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.MustExec(`CREATE TABLE users(
		id TEXT PRIMARY KEY, avatar TEXT, full_name TEXT, password TEXT, role TEXT,
		position TEXT, phone TEXT, email TEXT, date_of_birth TEXT, office TEXT,
		is_active BOOLEAN DEFAULT 1, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);`)

	repo := NewUserRepo(db, zap.NewNop())
	ctx := context.Background()
	dob := "1999-07-15"

	u := &models.User{
		ID: "u-1", FullName: "Тестовый Пользователь", Password: "hash", Role: "staff",
		Email: strp("member@example.org"), DateOfBirth: strp(dob), IsActive: true,
	}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, "u-1")
	if err != nil {
		t.Fatalf("чтение пользователя с датой рождения: %v", err)
	}
	if got.DateOfBirth == nil || *got.DateOfBirth != dob {
		t.Fatalf("после записи ожидалась дата %q, получено %v", dob, got.DateOfBirth)
	}

	// Обновление профиля с датой тоже не должно ломать чтение.
	got.Office = strp("105")
	got.Position = strp("Зав. лаб.")
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	got2, err := repo.GetByID(ctx, "u-1")
	if err != nil {
		t.Fatalf("чтение пользователя после обновления профиля: %v", err)
	}
	if got2.DateOfBirth == nil || *got2.DateOfBirth != dob {
		t.Fatalf("после обновления ожидалась дата %q, получено %v", dob, got2.DateOfBirth)
	}
	if got2.Office == nil || *got2.Office != "105" {
		t.Fatalf("кабинет не сохранился: %v", got2.Office)
	}

	// Пустая дата допустима и читается как отсутствие значения.
	got2.DateOfBirth = nil
	if err := repo.Update(ctx, got2); err != nil {
		t.Fatal(err)
	}
	got3, err := repo.GetByID(ctx, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if got3.DateOfBirth != nil {
		t.Fatalf("ожидалось пустое значение даты, получено %v", *got3.DateOfBirth)
	}

	// Устаревшее значение (формат time.Time.String) не должно валить чтение.
	db.MustExec(`UPDATE users SET date_of_birth = '1999-07-15 00:00:00 +0000 UTC' WHERE id='u-1'`)
	if _, err := repo.GetByID(ctx, "u-1"); err != nil {
		t.Fatalf("чтение устаревшей записи даты: %v", err)
	}
}
