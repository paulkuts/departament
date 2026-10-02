// internal/report/writeoff.go
//
// Отчёт о расходовании материальных запасов — печатная форма материального
// отдела (лист Excel). Форма не рисуется с нуля: за основу берётся шаблон
// template.xlsx (пустой бланк, вложен в бинарник), в него вписываются реквизиты
// и строки отчёта. Строк в шаблоне 16; сколько отмечено на списание, столько
// строк и останется — лишние удаляются, недостающие добавляются.

package report

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

//go:embed template.xlsx
var template []byte

const (
	firstRow = 16 // первая строка таблицы отчёта в бланке
	lastRow  = 31 // последняя строка-заготовка бланка (их 16)
	signGap  = 8  // строк от конца таблицы до конца блока подписей
)

var months = [...]string{
	"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

// WriteoffRow — строка отчёта: позиция описи и причина списания.
// Графа «код по ОКЕИ» остаётся пустой: коды не заполняем.
type WriteoffRow struct {
	Name     string
	Number   string
	Unit     string
	Quantity string
	Reason   string
}

// WriteoffParams — реквизиты шапки и строки отчёта.
type WriteoffParams struct {
	Department string
	Person     string
	PeriodFrom string // 2006-01-02
	PeriodTo   string // 2006-01-02
	Rows       []WriteoffRow
}

// BuildWriteoff собирает готовый лист Excel по бланку материального отдела.
func BuildWriteoff(p WriteoffParams) ([]byte, error) {
	if len(p.Rows) == 0 {
		return nil, errors.New("нет позиций для отчёта")
	}

	f, err := excelize.OpenReader(bytes.NewReader(template))
	if err != nil {
		return nil, fmt.Errorf("open report template: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)

	// Шапка: подразделение, ответственное лицо и период. Пустые поля оставляют
	// в бланке прочерки — их можно заполнить от руки перед подписью.
	if v := strings.TrimSpace(p.Department); v != "" {
		f.SetCellValue(sheet, "A8", "структурное подразделение: "+v)
	}
	if v := strings.TrimSpace(p.Person); v != "" {
		f.SetCellValue(sheet, "A10", "ответственное лицо: "+v)
	}
	// «за период с «01» сентября 2026 г. по «30» сентября 2026 г.»
	if day, year, err := periodParts(p.PeriodFrom); err == nil {
		f.SetCellValue(sheet, "D5", day)
		f.SetCellValue(sheet, "E5", year)
	}
	if day, year, err := periodParts(p.PeriodTo); err == nil {
		f.SetCellValue(sheet, "G5", day)
		f.SetCellValue(sheet, "H5", year)
	}

	// Строк в отчёте — по числу отмеченных позиций, а не 16 из бланка.
	n := len(p.Rows)
	templateRows := lastRow - firstRow + 1
	switch {
	case n < templateRows:
		for i := 0; i < templateRows-n; i++ {
			if err := f.RemoveRow(sheet, firstRow+n); err != nil {
				return nil, fmt.Errorf("drop unused report row: %w", err)
			}
		}
	case n > templateRows:
		for i := 0; i < n-templateRows; i++ {
			if err := f.DuplicateRow(sheet, lastRow); err != nil {
				return nil, fmt.Errorf("add report row: %w", err)
			}
		}
	}

	// Бланк набран Times New Roman, а в строках-заготовках стоит Calibri:
	// переносим оформление бланка на заполняемые ячейки. Рамки берём из самой
	// заготовки — меняем только шрифт и выравнивание.
	sampleName := fmt.Sprintf("B%d", firstRow)
	sampleBody := fmt.Sprintf("A%d", firstRow)
	nameStyle, err := rowStyle(f, sheet, sampleName, "justify")
	if err != nil {
		return nil, err
	}
	bodyStyle, err := rowStyle(f, sheet, sampleBody, "center")
	if err != nil {
		return nil, err
	}

	for i, row := range p.Rows {
		line := firstRow + i
		cells := []struct {
			col   string
			value interface{}
		}{
			{"A", i + 1},        // код строки
			{"B", row.Name},     // наименование основное
			{"C", row.Number},   // учетный номер
			{"D", ""},           // иное (при наличии)
			{"E", row.Unit},     // единица измерения
			{"F", ""},           // код по ОКЕИ не заполняется
			{"G", row.Quantity}, // фактически израсходовано
			{"H", row.Reason},   // причина списания
		}
		for _, cell := range cells {
			cellName := fmt.Sprintf("%s%d", cell.col, line)
			if err := f.SetCellValue(sheet, cellName, cell.value); err != nil {
				return nil, fmt.Errorf("fill report row %d: %w", line, err)
			}
			style := bodyStyle
			if cell.col == "B" {
				style = nameStyle
			}
			if err := f.SetCellStyle(sheet, cellName, cellName, style); err != nil {
				return nil, fmt.Errorf("style report row %d: %w", line, err)
			}
		}
		// Высота строки — автоматическая: длинное наименование переносится по
		// строкам, и строка подрастает под него, а не обрезает текст.
		if err := f.SetRowHeight(sheet, line, -1); err != nil {
			return nil, fmt.Errorf("relax report row height %d: %w", line, err)
		}
	}

	// В бланке за таблицей остаются пустые строки до конца листа — они уходят,
	// иначе отчёт печатался бы с пустыми страницами.
	if err := trimTail(f, sheet, len(p.Rows)); err != nil {
		return nil, err
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write report: %w", err)
	}
	return buf.Bytes(), nil
}

// rowStyle собирает стиль заполняемой ячейки: шрифт бланка, перенос по строкам
// и выравнивание (наименование — по ширине, остальные графы — по центру).
// Оформление берётся из заготовки строки, поэтому рамки сохраняются.
func rowStyle(f *excelize.File, sheet, sample, horizontal string) (int, error) {
	id, err := f.GetCellStyle(sheet, sample)
	if err != nil {
		return 0, fmt.Errorf("read report cell style: %w", err)
	}
	style, err := f.GetStyle(id)
	if err != nil {
		return 0, fmt.Errorf("read report style %d: %w", id, err)
	}
	// Новый стиль собираем по частям: если отдать в NewStyle весь прочитанный
	// стиль целиком, excelize добавит пустую заливку (<fill></fill>), и файл
	// перестаёт открываться в сторонних библиотеках и редакторах.
	styleID, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "Times New Roman", Size: 10},
		Alignment: &excelize.Alignment{Horizontal: horizontal, Vertical: "center", WrapText: true},
		Border:    style.Border,
	})
	if err != nil {
		return 0, fmt.Errorf("make report style: %w", err)
	}
	return styleID, nil
}

// trimTail убирает пустые строки после блока подписей. В бланке размечена не
// только таблица, но и пустые строки до конца листа: если их оставить, отчёт
// печатался бы с лишними страницами. Размер листа при этом задаётся явно —
// иначе в файле остаётся старая «использованная» область.
func trimTail(f *excelize.File, sheet string, rows int) error {
	last := firstRow + rows - 1 + signGap
	dim, err := f.GetSheetDimension(sheet)
	if err != nil {
		return fmt.Errorf("read report dimension: %w", err)
	}
	total := trailingRowNumber(dim)
	for total > last {
		if err := f.RemoveRow(sheet, total); err != nil {
			return fmt.Errorf("trim empty report row: %w", err)
		}
		total--
	}
	if err := f.SetSheetDimension(sheet, fmt.Sprintf("A1:H%d", last)); err != nil {
		return fmt.Errorf("set report dimension: %w", err)
	}
	return nil
}

// trailingRowNumber достаёт последнюю строку из ссылки вида «A1:H127».
func trailingRowNumber(ref string) int {
	digits := ""
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] < '0' || ref[i] > '9' {
			break
		}
		digits = string(ref[i]) + digits
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// periodParts превращает дату в части бланка: ««01» сентября» и «2026 г.».
// Пустая или непонятная дата — бланк остаётся с прочерками.
func periodParts(value string) (day, year string, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", errors.New("дата не указана")
	}
	d, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("«%02d» %s", d.Day(), months[d.Month()-1]), fmt.Sprintf("%d г.", d.Year()), nil
}

// QuantityText — количество так, как его пишут в документах: с запятой,
// без хвостовых нулей (2.5 → «2,5», 10 → «10»).
func QuantityText(value float64) string {
	return strings.Replace(strconv.FormatFloat(value, 'f', -1, 64), ".", ",", 1)
}
