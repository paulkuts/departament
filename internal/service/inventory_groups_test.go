package service

import (
	"testing"
	"time"

	"mitm-departament/internal/models"
)

func inv(id int64, name, nameNorm string, number *string, typ string, location *string) models.Inventory {
	return models.Inventory{ID: id, Name: name, NameNormalized: nameNorm, InventoryNumber: number, Type: typ, Location: location, Status: true}
}

func str(s string) *string { return &s }

func TestBuildInventoryGroupsByNumberAndName(t *testing.T) {
	items := []models.Inventory{
		inv(10, "Системный блок", "СИСТЕМНЫЙ БЛОК", str("101"), "equipment", str("105")),
		inv(11, "Монитор DELL", "МОНИТОР DELL", str("00101"), "equipment", str("105")), // тот же номер, другое название
		inv(12, "Стол", "СТОЛ", str("202"), "inventory", str("101")),
		inv(13, "Стол", "СТОЛ", str("203"), "inventory", str("103")), // то же название, другой номер
		inv(14, "Шкаф", "ШКАФ", nil, "inventory", str("102")),
	}
	groups := BuildInventoryGroups(items)
	if len(groups) != 3 {
		t.Fatalf("ожидали 3 группы, получили %d", len(groups))
	}
	// первая группа — комплект по общему номеру
	if groups[0].Count != 2 || groups[0].Key != "g10" || groups[0].Title != "Системный блок" {
		t.Fatalf("группа по номеру собрана неверно: %+v", groups[0])
	}
	if groups[0].InventoryNumber == nil || *groups[0].InventoryNumber != "101" {
		t.Fatalf("общий номер должен быть 101, получили %v", groups[0].InventoryNumber)
	}
	// вторая — два стола с разными номерами (группа по названию)
	if groups[1].Count != 2 || groups[1].Title != "Стол" {
		t.Fatalf("группа по названию собрана неверно: %+v", groups[1])
	}
	if groups[1].InventoryNumber != nil || !groups[1].NumbersDistinct || !groups[1].HasNumbers {
		t.Fatalf("у столов номера разные, общий номер показывать нельзя: %+v", groups[1])
	}
	// одиночка
	if groups[2].Count != 1 || groups[2].Title != "Шкаф" || groups[2].HasNumbers {
		t.Fatalf("одиночный объект собран неверно: %+v", groups[2])
	}
}

func TestBuildInventoryGroupsTransitive(t *testing.T) {
	items := []models.Inventory{
		inv(1, "Печь", "ПЕЧЬ", str("500"), "equipment", str("101")),
		inv(2, "Печь муфельная", "ПЕЧЬ МУФЕЛЬНАЯ", str("500"), "equipment", str("101")),
		inv(3, "Печь муфельная", "ПЕЧЬ МУФЕЛЬНАЯ", str("501"), "equipment", str("102")),
	}
	groups := BuildInventoryGroups(items)
	if len(groups) != 1 || groups[0].Count != 3 {
		t.Fatalf("связь должна быть транзитивной: получили %d групп", len(groups))
	}
	if groups[0].Types[0] != "equipment" || len(groups[0].Locations) != 2 {
		t.Fatalf("сводка по группе неверна: %+v", groups[0])
	}
}

func TestBuildInventoryGroupsEmptyNumbers(t *testing.T) {
	items := []models.Inventory{
		inv(1, "Кресло", "КРЕСЛО", nil, "inventory", str("101")),
		inv(2, "Кресло", "КРЕСЛО", nil, "inventory", str("102")),
		inv(3, "Принтер", "ПРИНТЕР", nil, "equipment", str("102")),
	}
	groups := BuildInventoryGroups(items)
	if len(groups) != 2 || groups[0].Count != 2 || groups[0].HasNumbers {
		t.Fatalf("пустые номера не должны склеивать разные названия: %+v", groups)
	}
}

func TestBuildInventoryGroupsAggregates(t *testing.T) {
	d1 := time.Date(2027, 1, 10, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2028, 2, 3, 0, 0, 0, 0, time.UTC)
	a := inv(5, "Стол", "СТОЛ", nil, "inventory", str("101"))
	a.NoNumberOnItem = true
	a.NotInRegistry = true
	a.NextVerificationDate = &d1
	b := inv(7, "Стол", "СТОЛ", nil, "inventory", str("102"))
	b.Status = false
	b.NextVerificationDate = &d2
	groups := BuildInventoryGroups([]models.Inventory{a, b})
	if len(groups) != 1 {
		t.Fatalf("ожидали одну группу, получили %d", len(groups))
	}
	g := groups[0]
	if g.Key != "g5" || g.Count != 2 || g.AllAvailable {
		t.Fatalf("ключ/количество/доступность неверны: %+v", g)
	}
	if g.NoNumberCount != 1 || g.NotInRegistryCount != 1 {
		t.Fatalf("пометки посчитаны неверно: %+v", g)
	}
	if len(g.NextVerifications) != 2 || g.NextVerifications[0] != "2027-01-10" {
		t.Fatalf("даты поверки неверны: %v", g.NextVerifications)
	}
	if len(g.Locations) != 2 {
		t.Fatalf("кабинеты неверны: %v", g.Locations)
	}
}

func TestBuildInventoryGroupsStableOrder(t *testing.T) {
	items := []models.Inventory{
		inv(30, "Шкаф", "ШКАФ", nil, "inventory", str("101")),
		inv(20, "Шкаф", "ШКАФ", nil, "inventory", str("101")),
		inv(40, "Тумба", "ТУМБА", nil, "inventory", str("101")),
	}
	groups := BuildInventoryGroups(items)
	if len(groups) != 2 || groups[0].Key != "g20" || groups[0].Members[0].ID != 20 {
		t.Fatalf("порядок группы должен идти по минимальному id: %+v", groups)
	}
	for _, m := range groups[0].Members {
		if m.ID != 20 && m.ID != 30 {
			t.Fatalf("в группе лишний объект: %+v", m)
		}
	}
}

func TestSortGroups(t *testing.T) {
	groups := []models.InventoryGroup{
		{Key: "a", Title: "Стол", Types: []string{"inventory"}, Locations: []string{"105"}, NextVerifications: []string{"2027-05-01"}, AllAvailable: true},
		{Key: "b", Title: "Аппарат", Types: []string{"equipment"}, Locations: []string{"101"}, NextVerifications: []string{}, AllAvailable: false},
		{Key: "c", Title: "Шкаф", Types: []string{"inventory", "equipment"}, Locations: []string{}, NextVerifications: []string{"2026-12-01"}, AllAvailable: true},
	}

	titles := func() []string {
		out := make([]string, len(groups))
		for i, g := range groups {
			out[i] = g.Title
		}
		return out
	}

	sortGroups(groups, "name", "asc")
	if titles()[0] != "Аппарат" || titles()[1] != "Стол" || titles()[2] != "Шкаф" {
		t.Fatalf("сортировка по названию asc: %v", titles())
	}
	sortGroups(groups, "name", "desc")
	if titles()[0] != "Шкаф" || titles()[2] != "Аппарат" {
		t.Fatalf("сортировка по названию desc: %v", titles())
	}

	// Пустые значения всегда в конце, независимо от порядка.
	sortGroups(groups, "location", "asc")
	if groups[2].Key != "c" {
		t.Fatalf("без расположения должно быть в конце (asc): %v", titles())
	}
	sortGroups(groups, "location", "desc")
	if groups[2].Key != "c" {
		t.Fatalf("без расположения должно быть в конце (desc): %v", titles())
	}

	// Поверка: ближайшая дата первой.
	sortGroups(groups, "verification", "asc")
	if groups[0].Key != "c" || groups[1].Key != "a" || groups[2].Key != "b" {
		t.Fatalf("сортировка по поверке: %v", titles())
	}

	// Состояние: доступные выше.
	sortGroups(groups, "status", "asc")
	if groups[2].Key != "b" {
		t.Fatalf("недоступное должно быть в конце: %v", titles())
	}

	// Неизвестная колонка ничего не меняет.
	before := titles()
	sortGroups(groups, "", "asc")
	if titles()[0] != before[0] || titles()[2] != before[2] {
		t.Fatalf("пустой ключ не должен менять порядок: %v", titles())
	}
}
