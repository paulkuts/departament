package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"mitm-departament/internal/models"
	"mitm-departament/internal/repository"

	"go.uber.org/zap"
)

// ErrImportBatchNotFound — пакет вставки с таким id не найден.
var ErrImportBatchNotFound = repository.ErrImportBatchNotFound

// Лимиты полей строки описи при вставке таблицей. Совпадают с формой записи;
// «№ документа» длиннее прочих: в нём пишут код вида «101.24, Машины и
// оборудование — особо ценное движимое имущество учреждения».
const (
	importMaxNumberLen   = 100
	importMaxNameLen     = 500
	importMaxUnitLen     = 50
	importMaxDocumentLen = 200
)

// ImportNumbersTable — вставка таблицы описи из Excel/Sheets одним пакетом.
//
// Строки проверяются до записи: пустой номер или наименование, нераспознанная
// дата, износ вне 0–100, превышение длины — в отчёт «пропущено» с причиной.
// Совпадение номера и наименования внутри одной вставки — «дубль»: остаётся
// первая строка, остальные не пишутся. Существующие строки обновляются
// (upsert по каноническому номеру и нормализованному наименованию).
func (s *EquipmentService) ImportNumbersTable(ctx context.Context, label, documentNumber, createdBy string, rows []models.InventoryNumber) (*models.InventoryImportReport, error) {
	if s.numbers == nil {
		return nil, errors.New("инвентаризация недоступна")
	}
	documentNumber = strings.TrimSpace(documentNumber)
	if len([]rune(documentNumber)) > importMaxDocumentLen {
		return nil, fmt.Errorf("№ документа длиннее %d символов", importMaxDocumentLen)
	}

	report := &models.InventoryImportReport{Problems: []models.InventoryImportProblem{}}
	prepared := make([]models.InventoryNumber, 0, len(rows))
	seen := make(map[string]int, len(rows))

	for i := range rows {
		row := rows[i]
		rowNo := i + 1
		skip := func(reason string, duplicate bool) {
			report.Problems = append(report.Problems, models.InventoryImportProblem{
				Row: rowNo, Reason: reason,
				Number: strings.TrimSpace(row.Number), Name: strings.TrimSpace(row.Name),
			})
			if duplicate {
				report.Duplicates++
			} else {
				report.Skipped++
			}
		}

		number := strings.TrimSpace(row.Number)
		name := strings.TrimSpace(row.Name)
		unit := strings.TrimSpace(row.Unit)
		canonical := models.CanonicalInventoryNumber(number)

		switch {
		case canonical == "" || strings.Trim(canonical, "/") == "":
			skip("пустой инвентарный номер", false)
			continue
		case len([]rune(number)) > importMaxNumberLen:
			skip(fmt.Sprintf("номер длиннее %d символов", importMaxNumberLen), false)
			continue
		case name == "":
			skip("пустое наименование", false)
			continue
		case len([]rune(name)) > importMaxNameLen:
			skip(fmt.Sprintf("наименование длиннее %d символов", importMaxNameLen), false)
			continue
		case len([]rune(unit)) > importMaxUnitLen:
			skip(fmt.Sprintf("единица измерения длиннее %d символов", importMaxUnitLen), false)
			continue
		}

		acceptedAt, err := NormalizeAcceptedAt(row.AcceptedAt)
		if err != nil {
			skip("дата принятия к учёту не распознана: "+strings.TrimSpace(row.AcceptedAt), false)
			continue
		}
		if row.Quantity != nil && (math.IsNaN(*row.Quantity) || math.IsInf(*row.Quantity, 0) || *row.Quantity < 0) {
			skip("количество не число", false)
			continue
		}
		if row.WearPercent != nil {
			wear := *row.WearPercent
			if math.IsNaN(wear) || math.IsInf(wear, 0) || wear < 0 || wear > 100 {
				skip("износ вне 0–100", false)
				continue
			}
		}

		key := canonical + "\x00" + repository.NormalizeName(name)
		if first, ok := seen[key]; ok {
			skip(fmt.Sprintf("дубль строки %d: номер и наименование совпадают", first), true)
			continue
		}
		seen[key] = rowNo

		prepared = append(prepared, models.InventoryNumber{
			Number:         number,
			Name:           name,
			Unit:           unit,
			Quantity:       row.Quantity,
			DocumentNumber: documentNumber,
			AcceptedAt:     acceptedAt,
			WearPercent:    row.WearPercent,
		})
	}

	if len(prepared) == 0 {
		return report, nil
	}

	batch := &models.InventoryImportBatch{
		Label:          strings.TrimSpace(label),
		DocumentNumber: documentNumber,
		CreatedBy:      strings.TrimSpace(createdBy),
	}
	dbReport, err := s.numbers.ImportBatch(ctx, batch, prepared)
	if err != nil {
		return nil, err
	}
	dbReport.Skipped = report.Skipped
	dbReport.Duplicates = report.Duplicates
	dbReport.Problems = report.Problems
	s.log.Info("инвентаризация: вставка таблицей записана",
		zap.Int64("пакет", dbReport.BatchID), zap.String("метка", batch.Label),
		zap.Int("принято", len(prepared)), zap.Int("пропущено", report.Skipped), zap.Int("дублей", report.Duplicates))
	return dbReport, nil
}

// RollbackNumbersBatch — откат пакета вставки (добавленные строки удаляются,
// обновлённым возвращаются прежние значения).
func (s *EquipmentService) RollbackNumbersBatch(ctx context.Context, id int64) (*models.InventoryImportRollback, error) {
	if s.numbers == nil {
		return nil, errors.New("инвентаризация недоступна")
	}
	return s.numbers.RollbackImportBatch(ctx, id)
}

// NormalizeAcceptedAt приводит дату принятия к учёту к виду YYYY-MM-DD.
// Excel и Google Sheets отдают её по-разному: «01.02.2020», «1.2.2020»,
// «2020-02-01», а при копировании числового формата — серийным номером
// (дни от 30.12.1899). Пустая строка допустима: дату могли не заполнить.
func NormalizeAcceptedAt(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if fields := strings.Fields(value); len(fields) > 1 {
		value = fields[0]
	}
	for _, layout := range []string{"2006-01-02", "02.01.2006", "2.1.2006", "02.01.06", "2.1.06", "2006/01/02", "02/01/2006"} {
		if t, err := time.Parse(layout, value); err == nil {
			return acceptedAtString(t)
		}
	}
	serial, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", "."), 64)
	if err == nil {
		serial = math.Trunc(serial)
		if serial >= 1 && serial <= 80000 {
			base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
			return acceptedAtString(base.AddDate(0, 0, int(serial)))
		}
	}
	return "", errors.New("не распознана")
}

func acceptedAtString(t time.Time) (string, error) {
	if t.Year() < 1950 || t.Year() > 2100 {
		return "", errors.New("год вне диапазона")
	}
	return t.Format("2006-01-02"), nil
}
