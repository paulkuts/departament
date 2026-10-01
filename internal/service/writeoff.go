// internal/service/writeoff.go
//
// Списание позиций описи: отметка, отчёт по форме материального отдела и
// применение списания. Отметка сама по себе опись не меняет — сначала
// администратор скачивает отчёт, распечатывает его и подписывает, и только
// после этого нажимает «Применить списание».

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"mitm-departament/internal/models"
	"mitm-departament/internal/report"

	"go.uber.org/zap"
)

// WriteoffRepo — отметки на списание (реализация — repository.InventoryWriteoffRepo).
type WriteoffRepo interface {
	ListDrafts(ctx context.Context) ([]models.InventoryWriteoff, error)
	ItemExists(ctx context.Context, itemID int64) (bool, error)
	SaveDraft(ctx context.Context, itemID int64, quantity float64, reason string, actorID *string) error
	DeleteDraft(ctx context.Context, itemID int64) error
	ApplyDrafts(ctx context.Context, actorID *string) (models.WriteoffResult, error)
}

var (
	ErrWriteoffItemNotFound = errors.New("позиция описи не найдена")
	ErrWriteoffQuantity     = errors.New("количество для списания должно быть больше нуля")
	ErrWriteoffEmpty        = errors.New("нет позиций, отмеченных на списание")
	ErrWriteoffReason       = errors.New("не указана причина списания")
)

// WriteoffService — работа с отметками на списание и отчётом.
type WriteoffService struct {
	repo WriteoffRepo
	log  *zap.Logger
}

func NewWriteoffService(repo WriteoffRepo, log *zap.Logger) *WriteoffService {
	return &WriteoffService{repo: repo, log: log}
}

// ListDrafts — отмеченные позиции: вкладка «Списание».
func (s *WriteoffService) ListDrafts(ctx context.Context) ([]models.InventoryWriteoff, error) {
	return s.repo.ListDrafts(ctx)
}

// SaveDraft ставит или обновляет отметку на списание.
func (s *WriteoffService) SaveDraft(ctx context.Context, itemID int64, quantity float64, reason string, actorID *string) error {
	if itemID <= 0 {
		return ErrWriteoffItemNotFound
	}
	if quantity <= 0 {
		return ErrWriteoffQuantity
	}
	exists, err := s.repo.ItemExists(ctx, itemID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrWriteoffItemNotFound
	}
	return s.repo.SaveDraft(ctx, itemID, quantity, reason, actorID)
}

// RemoveDraft снимает отметку: позиция остаётся в описи как была.
func (s *WriteoffService) RemoveDraft(ctx context.Context, itemID int64) error {
	if itemID <= 0 {
		return ErrWriteoffItemNotFound
	}
	return s.repo.DeleteDraft(ctx, itemID)
}

// ApplyDrafts применяет списание: количество в описи уменьшается, полностью
// израсходованные позиции уходят из списка, итог описи пересчитывается.
func (s *WriteoffService) ApplyDrafts(ctx context.Context, actorID *string) (models.WriteoffResult, error) {
	result, err := s.repo.ApplyDrafts(ctx, actorID)
	if err != nil {
		return result, err
	}
	if result.Items == 0 {
		return result, ErrWriteoffEmpty
	}
	s.log.Info("списание применено",
		zap.Int64("позиций", result.Items),
		zap.Float64("единиц", result.Quantity),
		zap.Float64("сумма", result.Amount))
	return result, nil
}

// ReportParams — реквизиты шапки отчёта: подразделение, ответственное лицо и
// период, за который списаны материалы. Их вводит администратор при скачивании.
type ReportParams struct {
	Department string
	Person     string
	PeriodFrom string
	PeriodTo   string
}

// BuildReport собирает отчёт по форме материального отдела по текущим отметкам.
// Реквизиты в шапке — из формы, каждая отмеченная позиция — строка отчёта:
// наименование, учётный номер, единица измерения с кодом по ОКЕИ, количество и
// причина списания. Возвращается готовый файл и имя, под которым его сохранит
// браузер.
func (s *WriteoffService) BuildReport(ctx context.Context, params ReportParams) ([]byte, string, error) {
	drafts, err := s.repo.ListDrafts(ctx)
	if err != nil {
		return nil, "", err
	}
	if len(drafts) == 0 {
		return nil, "", ErrWriteoffEmpty
	}
	for _, d := range drafts {
		if strings.TrimSpace(d.Reason) == "" {
			return nil, "", fmt.Errorf("%w: «%s»", ErrWriteoffReason, nameOrNumber(d))
		}
	}

	rows := make([]report.WriteoffRow, 0, len(drafts))
	for _, d := range drafts {
		rows = append(rows, report.WriteoffRow{
			Name:     d.Name,
			Number:   d.Number,
			Unit:     d.Unit,
			OKEI:     report.OKEIFor(d.Unit),
			Quantity: report.QuantityText(d.Quantity),
			Reason:   d.Reason,
		})
	}

	file, err := report.BuildWriteoff(report.WriteoffParams{
		Department: params.Department,
		Person:     params.Person,
		PeriodFrom: params.PeriodFrom,
		PeriodTo:   params.PeriodTo,
		Rows:       rows,
	})
	if err != nil {
		return nil, "", err
	}
	return file, "Отчет о расходовании материальных запасов от " + time.Now().Format("02.01.2006") + ".xlsx", nil
}

func nameOrNumber(d models.InventoryWriteoff) string {
	if strings.TrimSpace(d.Name) != "" {
		return d.Name
	}
	return d.Number
}
