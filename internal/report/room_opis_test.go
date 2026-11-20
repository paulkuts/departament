package report

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func buildForTest(t *testing.T, p RoomOpisParams) string {
	t.Helper()
	b, err := BuildRoomOpis(p)
	if err != nil {
		t.Fatalf("BuildRoomOpis: %v", err)
	}
	doc, err := readDocxPart(b, "word/document.xml")
	if err != nil {
		t.Fatalf("read part: %v", err)
	}
	// XML обязан быть корректным: Word иначе откажется открывать файл.
	dec := xml.NewDecoder(bytes.NewReader(doc))
	for {
		if _, err := dec.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("document.xml не разбирается: %v", err)
		}
	}
	return string(doc)
}

func TestBuildRoomOpisStructure(t *testing.T) {
	doc := buildForTest(t, RoomOpisParams{
		Room: "105",
		Date: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		Zav:  "Иванов И.И.",
		Items: []RoomOpisItem{
			{Section: SectionEquipment, Name: "Микроскоп", Number: "1050005442", Qty: 1},
			{Section: SectionEquipment, Name: "Весы", Number: "", Qty: 2},
			{Section: SectionFurniture, Name: "Стол", Number: "", Qty: 3},
		},
	})

	for _, want := range []string{
		"Опись имущества аудитории № 105",
		"20.", "11.", "2026",
		"Оборудование:", "Мебель:",
		"Микроскоп", "1050005442", "Весы", "Стол",
		"позиций 2 · единиц 3", "позиций 1 · единиц 3",
		"Иванов И.И.",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("в документе нет %q", want)
		}
	}

	if strings.Contains(doc, "{{") {
		t.Error("в документе остались метки шаблона")
	}
	if strings.Contains(doc, "Прочее:") {
		t.Error("пустой раздел «Прочее» не должен печататься")
	}
	if strings.Contains(doc, "Проверил") {
		t.Error("строка «Проверил» без галочки печататься не должна")
	}
	// Дата в первой таблице + шапка основной + 2 раздела по 3 строки (раздел,
	// позиции, итог): 1 + 1 + (1+2+1) + (1+1+1) = 9.
	if got := strings.Count(doc, "<w:tr "); got != 9 {
		t.Errorf("строк в документе %d, ожидалось 9", got)
	}
	if got := strings.Count(doc, "<w:tbl>"); got != 2 {
		t.Errorf("таблиц %d, ожидалось 2", got)
	}
	// Стиль таблицы, а в нём границы: без <w:tblPr>/<w:tblStyle> Word печатает
	// опись без линий (был такой дефект при пересборке шаблона).
	if got := strings.Count(doc, "<w:tblStyle w:val=\"ac\"/>"); got != 2 {
		t.Errorf("ссылок на стиль таблицы с границами %d, ожидалось 2", got)
	}
	if strings.Contains(doc, "___/___/_____") {
		t.Error("при заполненной дате нижняя пустая строка печататься не должна")
	}
}

func TestBuildRoomOpisEmptyDateAndCheck(t *testing.T) {
	doc := buildForTest(t, RoomOpisParams{
		Room:      "026/025",
		Zav:       "Петров П.П.",
		Checked:   true,
		CheckName: "Сидорова А.А.",
		Items: []RoomOpisItem{
			{Section: SectionOther, Name: "Ключ", Number: "00-1", Qty: 1},
		},
	})

	if strings.Contains(doc, ">20.<") || strings.Contains(doc, ">11.<") {
		t.Error("при варианте «пустая дата» дата печататься не должна")
	}
	if !strings.Contains(doc, "Дата: ___/___/_____") {
		t.Error("при варианте «без даты» должна печататься нижняя строка с пропусками")
	}
	if got := strings.Count(doc, "<w:tbl>"); got != 1 {
		t.Errorf("таблиц %d: при варианте «без даты» таблица даты убирается, остаётся 1", got)
	}
	if !strings.Contains(doc, "Проверил") || !strings.Contains(doc, "Сидорова А.А.") {
		t.Error("строка «Проверил» с ФИО не попала в документ")
	}
	if !strings.Contains(doc, "Прочее:") {
		t.Error("раздел «Прочее» не напечатан")
	}
	if !strings.Contains(doc, "№ 026/025") {
		t.Error("номер аудитории не попал в заголовок")
	}
}

func TestBuildRoomOpisEmptyItems(t *testing.T) {
	if _, err := BuildRoomOpis(RoomOpisParams{Room: "105"}); !errors.Is(err, ErrRoomOpisEmpty) {
		t.Fatalf("ожидалась ErrRoomOpisEmpty, получено %v", err)
	}
}

func TestRoomOpisFileName(t *testing.T) {
	got := RoomOpisFileName(RoomOpisParams{Room: "105", Date: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)})
	if got != "Опись имущества аудитории 105 от 20.11.2026.docx" {
		t.Errorf("имя файла %q", got)
	}
	if got := RoomOpisFileName(RoomOpisParams{Room: "105"}); got != "Опись имущества аудитории 105.docx" {
		t.Errorf("имя файла без даты %q", got)
	}
}
