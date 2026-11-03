package service

import (
	"context"
	"strings"
	"errors"
	"fmt"
	"math"
	"sort"
	"mitm-departament/internal/models"

	"go.uber.org/zap"
)

// EquipmentRepo — интерфейс репозитория оборудования
type EquipmentRepo interface {
	Create(ctx context.Context, e *models.Inventory) error
	GetByID(ctx context.Context, id int64) (*models.Inventory, error)
	GetByInventoryNumber(ctx context.Context, invNumber string) (*models.Inventory, error)
	List(ctx context.Context, f models.InventoryFilter) ([]models.Inventory, int64, error)
	ListAll(ctx context.Context, f models.InventoryFilter) ([]models.Inventory, error)
	NumberIndex(ctx context.Context) ([]models.InventoryNumberIndex, error)
	ListExpiredVerification(ctx context.Context, limit, offset int64) ([]models.Inventory, int64, error)
	Update(ctx context.Context, e *models.Inventory) error
	Delete(ctx context.Context, id int64) error
}

// InventoryNumberRepo — интерфейс справочника инвентарных номеров кафедры
type InventoryNumberRepo interface {
	Search(ctx context.Context, query, sort string, limit int) ([]models.InventoryNumber, error)
	Summary(ctx context.Context, query string) (models.InventorySummary, error)
	Count(ctx context.Context) (int64, error)
	Exists(ctx context.Context, number string) (bool, error)
	GetByNumber(ctx context.Context, number string) (*models.InventoryNumber, error)
	Create(ctx context.Context, item *models.InventoryNumber) error
	Update(ctx context.Context, item *models.InventoryNumber) error
	Import(ctx context.Context, items []models.InventoryNumber, replace bool) (int, int, error)
	Delete(ctx context.Context, id int64) error
}

type EquipmentService struct {
	repo     EquipmentRepo
	numbers  InventoryNumberRepo
	photoDir string // каталог фотографий: при удалении объекта убираем его папку
	log      *zap.Logger
}

func NewEquipmentService(repo EquipmentRepo, numbers InventoryNumberRepo, photoDir string, log *zap.Logger) *EquipmentService {
	return &EquipmentService{repo: repo, numbers: numbers, photoDir: photoDir, log: log}
}

// Create создаёт оборудование с проверкой уникальности инвентарного номера
func (s *EquipmentService) Create(ctx context.Context, e *models.Inventory) error {
	if e.Name == "" {
		return errors.New("equipment name is required")
	}

	s.log.Debug("equipment", zap.Any("equipment", e))

	// Один номер может стоять у нескольких единиц: например, системный блок и
	// монитор как комплект. Раньше здесь был жёсткий запрет на повтор, и такие
	// пары заводили обходным путём (номер с ведущим нулём). Теперь повтор
	// разрешён, а форма предупреждает и просит подтверждение.

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
	items := []models.Inventory{*e}
	s.fillSharedNumbers(ctx, items)
	*e = items[0]
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

	// group=1 — реестр строками-группами: общий номер и/или общее название.
	if f.Group {
		return s.listGroups(ctx, f)
	}

	items, total, err := s.repo.List(ctx, f)
	if err != nil {
		return models.ListInventory{}, fmt.Errorf("list equipment: %w", err)
	}
	s.fillSharedNumbers(ctx, items)
	return models.ListInventory{
		PaginatedMetadata: models.MakePaginatedMetadata(f.Paginated.Limit, f.Paginated.Offset, total),
		Inventory:         items,
	}, nil
}

// fillSharedNumbers проставляет объектам счётчик «сколько единиц с этим номером»
// и до трёх соседей. Номер сравнивается в каноническом виде: «00101061212»
// и «101061212» — один и тот же номер, пробелы и дефисы не считаются.
// Ошибка не валит список: без подсказки реестр остаётся рабочим.
func (s *EquipmentService) fillSharedNumbers(ctx context.Context, items []models.Inventory) {
	if len(items) == 0 {
		return
	}
	index, err := s.repo.NumberIndex(ctx)
	if err != nil {
		s.log.Warn("failed to build inventory number index", zap.Error(err))
		return
	}
	groups := make(map[string][]models.InventoryNumberIndex, len(index))
	for _, row := range index {
		if row.InventoryNumber == nil {
			continue
		}
		key := models.CanonicalInventoryNumber(*row.InventoryNumber)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], row)
	}
	for i := range items {
		if items[i].InventoryNumber == nil {
			continue
		}
		group := groups[models.CanonicalInventoryNumber(*items[i].InventoryNumber)]
		if len(group) < 2 {
			continue
		}
		items[i].NumberSharedCount = len(group)
		for _, row := range group {
			if row.ID == items[i].ID || len(items[i].SharedWith) >= 3 {
				continue
			}
			items[i].SharedWith = append(items[i].SharedWith, models.SharedNumber{ID: row.ID, Name: row.Name, Location: row.Location})
		}
	}
}

// listGroups возвращает страницу групп. Фильтр применяется к единицам, но группу
// отдаём целиком: иначе счётчик и раскрытие врали бы, а Павел просил видеть всю группу.
func (s *EquipmentService) listGroups(ctx context.Context, f models.InventoryFilter) (models.ListInventory, error) {
	filtered, err := s.repo.ListAll(ctx, f)
	if err != nil {
		return models.ListInventory{}, fmt.Errorf("list equipment: %w", err)
	}
	pass := make(map[int64]bool, len(filtered))
	for _, it := range filtered {
		pass[it.ID] = true
	}

	all, err := s.repo.ListAll(ctx, models.InventoryFilter{})
	if err != nil {
		return models.ListInventory{}, fmt.Errorf("list all equipment: %w", err)
	}

	groups := BuildInventoryGroups(all)
	selected := make([]models.InventoryGroup, 0, len(groups))
	var objectsTotal int64
	for _, g := range groups {
		matched := false
		for _, m := range g.Members {
			if pass[m.ID] {
				matched = true
				break
			}
		}
		if matched {
			selected = append(selected, g)
			objectsTotal += int64(len(g.Members))
		}
	}

	limit, offset := f.Paginated.Limit, f.Paginated.Offset
	start := offset
	if start > int64(len(selected)) {
		start = int64(len(selected))
	}
	end := start + limit
	if end > int64(len(selected)) {
		end = int64(len(selected))
	}

	return models.ListInventory{
		PaginatedMetadata: models.MakePaginatedMetadata(limit, offset, int64(len(selected))),
		Groups:            selected[start:end],
		ObjectsTotal:      objectsTotal,
	}, nil
}

// BuildInventoryGroups собирает объекты в группы: общий канонический номер и/или
// общее нормализованное название. Связь транзитивная: A делит номер с B, B делит
// название с C — все трое в одной группе. Порядок — по id первой записи группы.
func BuildInventoryGroups(items []models.Inventory) []models.InventoryGroup {
	if len(items) == 0 {
		return nil
	}
	parent := make([]int, len(items))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if ra > rb {
			ra, rb = rb, ra
		}
		parent[rb] = ra
	}

	byNumber := make(map[string]int, len(items))
	byName := make(map[string]int, len(items))
	for i, it := range items {
		if it.InventoryNumber != nil {
			if key := models.CanonicalInventoryNumber(*it.InventoryNumber); key != "" {
				if j, ok := byNumber[key]; ok {
					union(i, j)
				} else {
					byNumber[key] = i
				}
			}
		}
		if it.NameNormalized != "" {
			if j, ok := byName[it.NameNormalized]; ok {
				union(i, j)
			} else {
				byName[it.NameNormalized] = i
			}
		}
	}

	byRoot := make(map[int][]models.Inventory, len(items))
	order := make([]int, 0, len(items))
	for i := range items {
		root := find(i)
		if _, ok := byRoot[root]; !ok {
			order = append(order, root)
		}
		byRoot[root] = append(byRoot[root], items[i])
	}
	sort.Slice(order, func(a, b int) bool {
		return byRoot[order[a]][0].ID < byRoot[order[b]][0].ID
	})

	groups := make([]models.InventoryGroup, 0, len(order))
	for _, root := range order {
		members := byRoot[root]
		sort.Slice(members, func(a, b int) bool { return members[a].ID < members[b].ID })
		groups = append(groups, buildInventoryGroup(members))
	}
	return groups
}

// buildInventoryGroup собирает сводку по группе: название первой записи, общий
// номер (если он один у всех), категории, кабинеты, даты поверки, пометки.
func buildInventoryGroup(members []models.Inventory) models.InventoryGroup {
	first := members[0]
	g := models.InventoryGroup{
		Key:          fmt.Sprintf("g%d", first.ID),
		Title:        first.Name,
		Count:        len(members),
		AllAvailable: true,
		Members:      members,
		// Пустые срезы, а не nil: в JSON это [], иначе фронт получает null.
		Types:             make([]string, 0, 4),
		Locations:         make([]string, 0, 4),
		NextVerifications: make([]string, 0, 2),
	}

	types := make(map[string]bool)
	locations := make(map[string]bool)
	dates := make(map[string]bool)
	numbers := make(map[string]bool)
	withNumber := 0
	for _, m := range members {
		types[m.Type] = true
		if m.Location != nil && *m.Location != "" {
			locations[*m.Location] = true
		}
		if m.NextVerificationDate != nil {
			dates[m.NextVerificationDate.Format("2006-01-02")] = true
		}
		if !m.Status {
			g.AllAvailable = false
		}
		if m.NoNumberOnItem {
			g.NoNumberCount++
		}
		if m.NotInRegistry {
			g.NotInRegistryCount++
		}
		if m.InventoryNumber != nil {
			if key := models.CanonicalInventoryNumber(*m.InventoryNumber); key != "" {
				numbers[key] = true
				withNumber++
			}
		}
	}

	for k := range types {
		g.Types = append(g.Types, k)
	}
	sort.Strings(g.Types)
	for k := range locations {
		g.Locations = append(g.Locations, k)
	}
	sort.Strings(g.Locations)
	for k := range dates {
		g.NextVerifications = append(g.NextVerifications, k)
	}
	sort.Strings(g.NextVerifications)

	g.HasNumbers = withNumber > 0
	if len(numbers) > 1 {
		g.NumbersDistinct = true
	}
	if len(numbers) == 1 && withNumber == len(members) && first.InventoryNumber != nil {
		number := *first.InventoryNumber
		g.InventoryNumber = &number
	}
	return g
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

	// Повтор номера разрешён и здесь — см. комментарий в Create.

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

	// Строки фотографий уходят каскадом вместе с объектом, а файлы остались бы
	// на диске сиротами — убираем папку объекта.
	if err := removePhotoDir(s.photoDir, id); err != nil {
		s.log.Warn("failed to remove equipment photo dir", zap.Int64("id", id), zap.Error(err))
	}

	s.log.Info("equipment deleted", zap.Int64("id", id))
	return nil
}
// SearchNumbers — строки описи: список на странице инвентаризации и подсказки
// в форме объекта. sort задаёт порядок вывода (пустой — как в документах).
func (s *EquipmentService) SearchNumbers(ctx context.Context, query, sort string, limit int) ([]models.InventoryNumber, error) {
	if s.numbers == nil {
		return []models.InventoryNumber{}, nil
	}
	return s.numbers.Search(ctx, query, sort, limit)
}

// SummaryNumbers — итог по описи для блока сверху страницы: сколько строк,
// сколько единиц и на какую сумму. Считается по тому же поиску, что и список.
func (s *EquipmentService) SummaryNumbers(ctx context.Context, query string) (models.InventorySummary, error) {
	if s.numbers == nil {
		return models.InventorySummary{}, errors.New("инвентаризация недоступна")
	}
	return s.numbers.Summary(ctx, query)
}

// ComputeAmount — сумма строки считается, а не вводится руками: цена ×
// количество. Нет цены или количества — суммы нет (считать нечего), поэтому
// после правки количества сумма меняется сама. Округляем до копеек: иначе
// двоичная дробь даёт хвост, а сумма сотен строк накапливает погрешность.
func ComputeAmount(item *models.InventoryNumber) {
	if item.Price != nil && item.Quantity != nil {
		amount := math.Round(*item.Price**item.Quantity*100) / 100
		item.Amount = &amount
		return
	}
	item.Amount = nil
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
	for i := range items {
		ComputeAmount(&items[i])
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

// CreateNumber — новая строка инвентаризации (опись материального отдела).
func (s *EquipmentService) CreateNumber(ctx context.Context, item *models.InventoryNumber) error {
	if s.numbers == nil {
		return errors.New("инвентаризация недоступна")
	}
	ComputeAmount(item)
	if err := s.numbers.Create(ctx, item); err != nil {
		return err
	}
	s.log.Info("инвентаризация: запись добавлена",
		zap.Int64("id", item.ID), zap.String("номер", item.Number))
	return nil
}

// UpdateNumber — правка строки инвентаризации.
func (s *EquipmentService) UpdateNumber(ctx context.Context, item *models.InventoryNumber) error {
	if s.numbers == nil {
		return errors.New("инвентаризация недоступна")
	}
	ComputeAmount(item)
	if err := s.numbers.Update(ctx, item); err != nil {
		return err
	}
	s.log.Info("инвентаризация: запись изменена", zap.Int64("id", item.ID))
	return nil
}

// LookupNumber — строка инвентаризации по инвентарному номеру объекта.
// Нет номера или нет записи — (nil, nil): карточка показывает, что сверять нечего.
func (s *EquipmentService) LookupNumber(ctx context.Context, number string) (*models.InventoryNumber, error) {
	if s.numbers == nil || strings.TrimSpace(number) == "" {
		return nil, nil
	}
	return s.numbers.GetByNumber(ctx, number)
}
