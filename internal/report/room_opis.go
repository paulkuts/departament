// internal/report/room_opis.go
//
// Опись имущества аудитории — печатная форма по образцу кафедры (Word).
// Форма не рисуется с нуля: берётся шаблон template-room-opis.docx с метками
// (он вложен в бинарник), строки таблицы собираются из него же — поэтому
// шрифт, рамки, ширины колонок и повтор шапки на каждой странице остаются
// такими, какими их сделал материальный отдел.
//
// Метки в шаблоне:
//   {{DAYDOT}} {{MONTHDOT}} {{YEAR}} — дата сверху (пустые строки, если дата не нужна)
//   {{ROOM}}                         — номер аудитории в заголовке
//   {{SECTION}}                      — строка-раздел таблицы (Оборудование/Мебель/Прочее)
//   {{N}} {{NAME}} {{QTY}} {{INV}}    — строка позиции
//   {{TOTALS}}                       — итог по разделу
//   {{ZAV}}                          — ФИО заведующего лабораторией
//   {{CHECKPARA}}                    — абзац «Проверил» (целиком заменяется или убирается)

package report

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	_ "embed"
)

//go:embed template-room-opis.docx
var roomTemplate []byte

// Разделы формы — в том порядке, в котором они идут в бланке.
const (
	SectionEquipment = "Оборудование:"
	SectionFurniture = "Мебель:"
	SectionOther     = "Прочее:"
)

// RoomOpisItem — позиция описи: одна строка таблицы.
type RoomOpisItem struct {
	Section string
	Name    string
	Number  string
	Qty     int
	Unit    string // единица измерения: «шт», «кг», «компл»; пусто — штуки
}

// RoomOpisParams — реквизиты описи.
type RoomOpisParams struct {
	Room      string    // номер аудитории, например «105»
	Date      time.Time // дата формирования; нулевая — дата не печатается
	Zav       string    // ФИО заведующего лабораторией
	Checked   bool      // печатать ли строку «Проверил»
	CheckName string    // ФИО проверившего
	Items     []RoomOpisItem
}

// RoomOpisFileName — имя файла для скачивания.
func RoomOpisFileName(p RoomOpisParams) string {
	name := "Опись имущества аудитории " + strings.TrimSpace(p.Room)
	if !p.Date.IsZero() {
		name += " от " + p.Date.Format("02.01.2006")
	}
	return name + ".docx"
}

// BuildRoomOpis собирает готовый .docx по бланку кафедры.
func BuildRoomOpis(p RoomOpisParams) ([]byte, error) {
	if strings.TrimSpace(p.Room) == "" {
		return nil, errors.New("не указана аудитория")
	}
	if len(p.Items) == 0 {
		return nil, ErrRoomOpisEmpty
	}

	doc, err := readDocxPart(roomTemplate, "word/document.xml")
	if err != nil {
		return nil, err
	}
	s := string(doc)

	sectRow, sectStart, sectEnd, err := rowBlock(s, "{{SECTION}}")
	if err != nil {
		return nil, err
	}
	dataRow, dataStart, dataEnd, err := rowBlock(s, "{{N}}")
	if err != nil {
		return nil, err
	}
	totalRow, totalStart, totalEnd, err := rowBlock(s, "{{TOTALS}}")
	if err != nil {
		return nil, err
	}
	// «Итого» не отрывается от подписей: иначе Заведующий/Проверил уезжают
	// на отдельный лист или разрываются между страницами.
	totalRow = strings.Replace(totalRow, "<w:pPr>", "<w:pPr><w:keepNext/>", 1)
	if !(sectEnd <= dataStart && dataEnd <= totalStart) {
		return nil, errors.New("шаблон описи: строки-образцы идут не подряд")
	}

	var body strings.Builder
	for _, sec := range []string{SectionEquipment, SectionFurniture, SectionOther} {
		items := filterSection(p.Items, sec)
		if len(items) == 0 {
			continue
		}
		body.WriteString(strings.ReplaceAll(sectRow, "{{SECTION}}", esc(sec)))
		units := 0
		for i, it := range items {
			row := dataRow
			row = strings.ReplaceAll(row, "{{N}}", strconv.Itoa(i+1))
			row = strings.ReplaceAll(row, "{{NAME}}", esc(it.Name))
			row = strings.ReplaceAll(row, "{{QTY}}", esc(qtyLabel(it)))
			row = strings.ReplaceAll(row, "{{INV}}", esc(numberOrDash(it.Number)))
			body.WriteString(row)
			units += it.Qty
		}
		totals := fmt.Sprintf("позиций %d · единиц %d", len(items), units)
		body.WriteString(strings.ReplaceAll(totalRow, "{{TOTALS}}", esc(totals)))
	}
	s = s[:sectStart] + body.String() + s[totalEnd:]

	// Дата: два взаимоисключающих варианта бланка. «С датой» — только верхняя
	// таблица (одна ячейка со слитной датой), «без даты» — только нижняя строка
	// с пропусками.
	if p.Date.IsZero() {
		s, err = replaceTable(s, "{{DAYDOT}}", "")
		if err != nil {
			return nil, err
		}
		s = strings.ReplaceAll(s, "{{DATEBLANK}}", esc("Дата: ___/___/_____"))
	} else {
		s = strings.ReplaceAll(s, "{{DAYDOT}}", esc(p.Date.Format("02.01.2006")))
		s, err = replaceParagraph(s, "{{DATEBLANK}}", "")
		if err != nil {
			return nil, err
		}
	}

	s = strings.ReplaceAll(s, "{{ROOM}}", esc(strings.TrimSpace(p.Room)))

	zav := strings.TrimSpace(p.Zav)
	if zav == "" {
		zav = "____________________"
	}
	s = strings.ReplaceAll(s, "{{ZAV}}", esc(zav))

	checkPara := ""
	if p.Checked {
		name := strings.TrimSpace(p.CheckName)
		if name == "" {
			name = "____________________"
		}
		checkPara = `<w:p><w:pPr><w:keepNext/><w:spacing w:before="0" w:after="0" w:line="240" w:lineRule="auto"/><w:jc w:val="both"/></w:pPr>` +
			`<w:r><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/>` +
			`<w:sz w:val="24"/><w:szCs w:val="24"/></w:rPr>` +
			`<w:t xml:space="preserve">Проверил ______________________ / ` + esc(name) + ` /</w:t></w:r></w:p>`
	}
	s, err = replaceParagraph(s, "{{CHECKPARA}}", checkPara)
	if err != nil {
		return nil, err
	}

	// Текст в ячейках — по центру по высоте строки: иначе у позиций с длинным
	// наименованием (2–3 строки) номер и инвентарный номер прилипают к верху.
	s = centerCellsVertically(s)

	// Подпись заведующего — на одну пустую строку ниже: так на бланке остаётся
	// место для печати/подписи от руки.
	s, err = insertBlankLineBefore(s, "Заведующий лабораторией")
	if err != nil {
		return nil, err
	}

	return writeDocxPart(roomTemplate, "word/document.xml", []byte(s))
}

// ErrRoomOpisEmpty — в аудитории нет объектов: пустой бланк не печатаем.
var ErrRoomOpisEmpty = errors.New("в этой аудитории нет объектов")

func filterSection(items []RoomOpisItem, section string) []RoomOpisItem {
	out := make([]RoomOpisItem, 0, len(items))
	for _, it := range items {
		if it.Section == section {
			out = append(out, it)
		}
	}
	return out
}

func numberOrDash(n string) string {
	n = strings.TrimSpace(n)
	if n == "" {
		return "—"
	}
	return n
}

// qtyLabel — количество с единицей измерения: «3 шт.», «12 кг».
// В бланке всегда с точкой сокращения, как принято в документах кафедры.
func qtyLabel(it RoomOpisItem) string {
	u := strings.TrimSpace(it.Unit)
	if u == "" {
		u = "шт"
	}
	if !strings.HasSuffix(u, ".") {
		u += "."
	}
	return strconv.Itoa(it.Qty) + " " + u
}

// esc готовит текст для узла <w:t>: служебные символы XML и неразрывный пробел
// не нужны, переводы строк в ячейке заменяются обычным пробелом.
func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\r", " ", "\n", " ")
	return r.Replace(s)
}

// insertBlankLineBefore вставляет пустой абзац перед абзацем с меткой-текстом.
// Нужен, чтобы сдвинуть подпись на одну строку, не трогая шаблон.
func insertBlankLineBefore(doc, marker string) (string, error) {
	i := strings.Index(doc, marker)
	if i < 0 {
		return doc, fmt.Errorf("шаблон описи: нет абзаца %q", marker)
	}
	start := strings.LastIndex(doc[:i], "<w:p ")
	if alt := strings.LastIndex(doc[:i], "<w:p>"); alt > start {
		start = alt
	}
	if start < 0 {
		return doc, fmt.Errorf("шаблон описи: абзац %q вне абзаца", marker)
	}
	blank := `<w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="240" w:lineRule="auto"/></w:pPr></w:p>`
	return doc[:start] + blank + doc[start:], nil
}

// centerCellsVertically ставит <w:vAlign w:val="center"/> каждой ячейке таблицы,
// у которой выравнивание не задано. Шаблон местами уже содержит vAlign (шапка),
// поэтому дубли не добавляем. Работает по уже собранному XML — покрывает и
// размноженные строки позиций, и таблицу даты.
func centerCellsVertically(doc string) string {
	doc = strings.ReplaceAll(doc, "<w:tcPr/>", `<w:tcPr><w:vAlign w:val="center"/></w:tcPr>`)
	var b strings.Builder
	b.Grow(len(doc) + 512)
	rest := doc
	for {
		i := strings.Index(rest, "<w:tcPr>")
		if i < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:i])
		rest = rest[i:]
		end := strings.Index(rest, "</w:tcPr>")
		if end < 0 { // битый XML — оставляем как есть, его поймает проверка
			b.WriteString(rest)
			return b.String()
		}
		block := rest[:end+len("</w:tcPr>")]
		if !strings.Contains(block, "<w:vAlign") {
			block = `<w:tcPr><w:vAlign w:val="center"/>` + rest[len("<w:tcPr>"):end+len("</w:tcPr>")]
		}
		b.WriteString(block)
		rest = rest[end+len("</w:tcPr>"):]
	}
}

// rowBlock находит строку таблицы, в которой стоит метка, и возвращает её XML
// вместе с границами — по ним строка заменяется набором своих копий.
func rowBlock(doc, marker string) (row string, start, end int, err error) {
	i := strings.Index(doc, marker)
	if i < 0 {
		return "", 0, 0, fmt.Errorf("шаблон описи: нет метки %s", marker)
	}
	start = strings.LastIndex(doc[:i], "<w:tr ")
	if start < 0 {
		return "", 0, 0, fmt.Errorf("шаблон описи: метка %s вне строки таблицы", marker)
	}
	closeAt := strings.Index(doc[i:], "</w:tr>")
	if closeAt < 0 {
		return "", 0, 0, fmt.Errorf("шаблон описи: строка %s не закрыта", marker)
	}
	end = i + closeAt + len("</w:tr>")
	return doc[start:end], start, end, nil
}

// replaceTable заменяет всю таблицу, внутри которой стоит метка.
func replaceTable(doc, marker, repl string) (string, error) {
	i := strings.Index(doc, marker)
	if i < 0 {
		return "", fmt.Errorf("шаблон описи: не найдена метка %s", marker)
	}
	start := strings.LastIndex(doc[:i], "<w:tbl ")
	if alt := strings.LastIndex(doc[:i], "<w:tbl>"); alt > start {
		start = alt
	}
	if start < 0 {
		return "", fmt.Errorf("шаблон описи: метка %s вне таблицы", marker)
	}
	end := strings.Index(doc[i:], "</w:tbl>")
	if end < 0 {
		return "", fmt.Errorf("шаблон описи: таблица с меткой %s не закрыта", marker)
	}
	end = i + end + len("</w:tbl>")
	return doc[:start] + repl + doc[end:], nil
}

// replaceParagraph заменяет абзац, в котором стоит метка, готовым XML
// (или убирает его, если замена пустая).
func replaceParagraph(doc, marker, repl string) (string, error) {
	i := strings.Index(doc, marker)
	if i < 0 {
		return doc, fmt.Errorf("шаблон описи: нет метки %s", marker)
	}
	start := strings.LastIndex(doc[:i], "<w:p ")
	if alt := strings.LastIndex(doc[:i], "<w:p>"); alt > start {
		start = alt
	}
	if start < 0 {
		return doc, fmt.Errorf("шаблон описи: метка %s вне абзаца", marker)
	}
	closeAt := strings.Index(doc[i:], "</w:p>")
	if closeAt < 0 {
		return doc, fmt.Errorf("шаблон описи: абзац %s не закрыт", marker)
	}
	end := i + closeAt + len("</w:p>")
	return doc[:start] + repl + doc[end:], nil
}

func readDocxPart(file []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("в шаблоне нет %s", name)
}

func writeDocxPart(file []byte, name string, content []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		dst, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate})
		if err != nil {
			return nil, err
		}
		if f.Name == name {
			if _, err := dst.Write(content); err != nil {
				return nil, err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(dst, rc); err != nil {
			rc.Close()
			return nil, err
		}
		rc.Close()
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
