package service

import (
	"context"
	"strings"
	"errors"
	"fmt"
	"mitm-departament/internal/models"

	"go.uber.org/zap"
)

// EquipmentRepo — интерфейс репозитория оборудования
type EquipmentRepo interface {
	Create(ctx context.Context, e *models.Inventory) error
	GetByID(ctx context.Context, id int64) (*models.Inventory, error)
	GetByInventoryNumber(ctx context.Context, invNumber string) (*models.Inventory, error)
	List(ctx context.Context, f models.InventoryFilter) ([]models.Inventory, int64, error)
	ListExpiredVerification(ctx context.Context, limit, offset int64) ([]models.Inventory, int64, error)
	Update(ctx context.Context, e *models.Inventory) error
	Delete(ctx context.Context, id int64) error
}

// InventoryNumberRepo — интерфейс справочника инвентарных номеров кафедры
type InventoryNumberRepo interface {
	Search(ctx context.Context, query string, limit int) ([]models.InventoryNumber, error)
	Count(ctx context.Context) (int64, error)
	Exists(ctx context.Context, number string) (bool, error)
	Import(ctx context.Context, items []models.InventoryNumber, replace bool) (int, int, error)
	Delete(ctx context.Context, id int64) error
}

type EquipmentService struct {
	repo    EquipmentRepo
	numbers InventoryNumberRepo
	log  *zap.Logger
}

func NewEquipmentService(repo EquipmentRepo, numbers InventoryNumberRepo, log *zap.Logger) *EquipmentService {
	return &EquipmentService{repo: repo, numbers: numbers, log: log}
}

// Create создаёт оборудование с проверкой уникальности инвентарного номера
func (s *EquipmentService) Create(ctx context.Context, e *models.Inventory) error {
	if e.Name == "" {
		return errors.New("equipment name is required")
	}

	s.log.Debug("equipment", zap.Any("equipment", e))

	// Проверка уникальности инвентарного номера
	if e.InventoryNumber != nil && *e.InventoryNumber != "" {
		existing, err := s.repo.GetByInventoryNumber(ctx, *e.InventoryNumber)
		if err != nil {
			return fmt.Errorf("check inventory number: %w", err)
		}
		if existing != nil {
			return fmt.Errorf("equipment with inventory number %q already exists", *e.InventoryNumber)
		}
	}

	if err := s.repo.Create(ctx, e); err != nil {
		s.log.Error("failed to create equipment",
			zap.String("name", e.Name),
			zap.Error(err),
		)
		return fmt.Errorf("create equipment: %w", err)
	}

	s.log.Info("equipment created",
		zap.Int64("id", e.ID),
		zap.String("name", e.Name),
	)
	return nil
}

// GetByID возвращает оборудование по ID
func (s *EquipmentService) GetByID(ctx context.Context, id int64) (*models.Inventory, error) {
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get equipment by id: %w", err)
	}
	if e == nil {
		return nil, fmt.Errorf("equipment %d not found", id)
	}
	return e, nil
}

// List возвращает оборудование с фильтрами и пагинацией
func (s *EquipmentService) List(ctx context.Context, f models.InventoryFilter) (models.ListInventory, error) {
	// Дефолты и ограничения пагинации
	if f.Paginated.Limit <= 0 {
		f.Paginated.Limit = 20
	}
	if f.Paginated.Limit > 100 {
		f.Paginated.Limit = 100 // защита от слишком больших запросов
	}
	if f.Paginated.Offset < 0 {
		f.Paginated.Offset = 0
	}

	items, total, err := s.repo.List(ctx, f)
	if err != nil {
		return models.ListInventory{}, fmt.Errorf("list equipment: %w", err)
	}
	return models.ListInventory{
		PaginatedMetadata: models.MakePaginatedMetadata(f.Paginated.Limit, f.Paginated.Offset, total),
		Inventory:         items,
	}, nil
}

// ListExpiredVerification возвращает оборудование с просроченной поверкой
func (s *EquipmentService) ListExpiredVerification(ctx context.Context, limit, offset int64) (models.ListInventory, error) {
	items, total, err := s.repo.ListExpiredVerification(ctx, limit, offset)
	if err != nil {
		return models.ListInventory{}, fmt.Errorf("list expired verification: %w", err)
	}
	return models.ListInventory{
		PaginatedMetadata: models.MakePaginatedMetadata(limit, offset, total),
		Inventory:         items,
	}, nil
}

// Update обновляет оборудование
func (s *EquipmentService) Update(ctx context.Context, e *models.Inventory) error {
	if e.ID == 0 {
		return errors.New("equipment id is required for update")
	}
	if e.Name == "" {
		return errors.New("equipment name is required")
	}

	// Проверка уникальности инвентарного номера (если изменился)
	if e.InventoryNumber != nil && *e.InventoryNumber != "" {
		existing, err := s.repo.GetByInventoryNumber(ctx, *e.InventoryNumber)
		if err != nil {
			return fmt.Errorf("check inventory number: %w", err)
		}
		if existing != nil && existing.ID != e.ID {
			return fmt.Errorf("equipment with inventory number %q already exists", *e.InventoryNumber)
		}
	}

	if err := s.repo.Update(ctx, e); err != nil {
		s.log.Error("failed to update equipment",
			zap.Int64("id", e.ID),
			zap.Error(err),
		)
		return fmt.Errorf("update equipment: %w", err)
	}

	s.log.Info("equipment updated", zap.Int64("id", e.ID))
	return nil
}

// Delete удаляет оборудование
func (s *EquipmentService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		s.log.Error("failed to delete equipment",
			zap.Int64("id", id),
			zap.Error(err),
		)
		return fmt.Errorf("delete equipment: %w", err)
	}

	s.log.Info("equipment deleted", zap.Int64("id", id))
	return nil
}
// SearchNumbers — номера из таблицы кафедры для подсказки в форме объекта.
func (s *EquipmentService) SearchNumbers(ctx context.Context, query string, limit int) ([]models.InventoryNumber, error) {
	if s.numbers == nil {
		return []models.InventoryNumber{}, nil
	}
	return s.numbers.Search(ctx, query, limit)
}

// NotInRegistryMark — нужно ли пометить номер как отсутствующий в таблице.
// Пустая таблица означает «сверять не с чем»: пометка не ставится.
func (s *EquipmentService) NotInRegistryMark(ctx context.Context, number string) (bool, error) {
	if strings.TrimSpace(number) == "" || s.numbers == nil {
		return false, nil
	}
	total, err := s.numbers.Count(ctx)
	if err != nil {
		return false, err
	}
	if total == 0 {
		return false, nil
	}
	found, err := s.numbers.Exists(ctx, number)
	if err != nil {
		return false, err
	}
	return !found, nil
}

// ImportNumbers — загрузка таблицы номеров: новые добавляются, известные обновляются.
func (s *EquipmentService) ImportNumbers(ctx context.Context, items []models.InventoryNumber, replace bool) (added, updated, total int, err error) {
	if s.numbers == nil {
		return 0, 0, 0, errors.New("справочник инвентарных номеров недоступен")
	}
	added, updated, err = s.numbers.Import(ctx, items, replace)
	if err != nil {
		return 0, 0, 0, err
	}
	n, err := s.numbers.Count(ctx)
	if err != nil {
		return added, updated, 0, err
	}
	s.log.Info("справочник инвентарных номеров обновлён",
		zap.Int("добавлено", added), zap.Int("обновлено", updated), zap.Int("всего", int(n)))
	return added, updated, int(n), nil
}

// DeleteNumber — удаление номера из справочника.
func (s *EquipmentService) DeleteNumber(ctx context.Context, id int64) error {
	if s.numbers == nil {
		return errors.New("справочник инвентарных номеров недоступен")
	}
	return s.numbers.Delete(ctx, id)
}
