package service

import (
	"context"
	"testing"

	"mitm-departament/internal/models"

	"go.uber.org/zap"
)

// fakeImportRepo запоминает, что сервис отдал в репозиторий: проверяем
// проверку строк до записи, не поднимая базу.
type fakeImportRepo struct {
	batch *models.InventoryImportBatch
	items []models.InventoryNumber
}

func (f *fakeImportRepo) Search(context.Context, string, string, int) ([]models.InventoryNumber, error) {
	return nil, nil
}
func (f *fakeImportRepo) Summary(context.Context, string) (models.InventorySummary, error) {
	return models.InventorySummary{}, nil
}
func (f *fakeImportRepo) Count(context.Context) (int64, error) { return 0, nil }
func (f *fakeImportRepo) Exists(context.Context, string) (bool, error) {
	return false, nil
}
func (f *fakeImportRepo) GetByNumber(context.Context, string) (*models.InventoryNumber, error) {
	return nil, nil
}
func (f *fakeImportRepo) Create(context.Context, *models.InventoryNumber) error { return nil }
func (f *fakeImportRepo) Update(context.Context, *models.InventoryNumber) error { return nil }
func (f *fakeImportRepo) Import(context.Context, []models.InventoryNumber, bool) (int, int, error) {
	return 0, 0, nil
}
func (f *fakeImportRepo) Delete(context.Context, int64) error { return nil }
func (f *fakeImportRepo) RollbackImportBatch(context.Context, int64) (*models.InventoryImportRollback, error) {
	return &models.InventoryImportRollback{}, nil
}
func (f *fakeImportRepo) ImportBatch(_ context.Context, batch *models.InventoryImportBatch, items []models.InventoryNumber) (*models.InventoryImportReport, error) {
	f.batch, f.items = batch, items
	return &models.InventoryImportReport{BatchID: 7, Added: len(items), Problems: []models.InventoryImportProblem{}}, nil
}

func TestNormalizeAcceptedAt(t *testing.T) {
	cases := []struct {
		in, want string
		bad      bool
	}{
		{in: "", want: ""},
		{in: "2020-02-01", want: "2020-02-01"},
		{in: "01.02.2020", want: "2020-02-01"},
		{in: "1.2.2020", want: "2020-02-01"},
		{in: "01.02.20", want: "2020-02-01"},
		{in: "43862", want: "2020-02-01"}, // серийный номер Excel
		{in: "01.02.2020 0:00:00", want: "2020-02-01"},
		{in: "не дата", bad: true},
		{in: "01.02.1800", bad: true},
	}
	for _, c := range cases {
		got, err := NormalizeAcceptedAt(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("NormalizeAcceptedAt(%q) = %q, ожидалась ошибка", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeAcceptedAt(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeAcceptedAt(%q) = %q, ожидалось %q", c.in, got, c.want)
		}
	}
}

func TestImportNumbersTableValidatesRows(t *testing.T) {
	qty := 3.0
	badQty := -1.0
	wear := 45.0
	badWear := 120.0
	repo := &fakeImportRepo{}
	svc := &EquipmentService{numbers: repo, log: zap.NewNop()}

	rows := []models.InventoryNumber{
		{Number: "", Name: "Пустой номер"},
		{Number: "101.24-1", Name: " "},
		{Number: "101.24-2", Name: "Плохая дата", AcceptedAt: "вчера"},
		{Number: "101.24-3", Name: "Плохой износ", WearPercent: &badWear},
		{Number: "101.24-4", Name: "Плохое количество", Quantity: &badQty},
		{Number: "/", Name: "Только слэш"},
		{Number: "101.24-5", Name: "Стол лабораторный", Unit: "шт", Quantity: &qty, AcceptedAt: "01.02.2020", WearPercent: &wear},
		{Number: "101.24-5", Name: "стол   лабораторный", Unit: "шт"},  // дубль по канонике
		{Number: "101.24-6", Name: "Стол лабораторный", Unit: "шт"},    // тот же номер, другое наименование
		{Number: " 101.24-5 ", Name: "Шкаф", Unit: "шт"},               // тот же номер, другое наименование
	}

	report, err := svc.ImportNumbersTable(context.Background(), "вставка", "101.24", "user-1", rows)
	if err != nil {
		t.Fatalf("ImportNumbersTable: %v", err)
	}
	if report.BatchID != 7 {
		t.Errorf("BatchID = %d, ожидалось 7", report.BatchID)
	}
	if report.Skipped != 6 {
		t.Errorf("Skipped = %d, ожидалось 6", report.Skipped)
	}
	if report.Duplicates != 1 {
		t.Errorf("Duplicates = %d, ожидалось 1", report.Duplicates)
	}
	if len(repo.items) != 3 {
		t.Fatalf("в репозиторий ушло %d строк, ожидалось 3", len(repo.items))
	}
	if repo.items[0].AcceptedAt != "2020-02-01" {
		t.Errorf("дата = %q, ожидалось 2020-02-01", repo.items[0].AcceptedAt)
	}
	if repo.items[0].DocumentNumber != "101.24" {
		t.Errorf("№ документа = %q, у всех строк должен быть 101.24", repo.items[0].DocumentNumber)
	}
	if repo.batch.Label != "вставка" || repo.batch.CreatedBy != "user-1" {
		t.Errorf("пакет = %+v", repo.batch)
	}
}
