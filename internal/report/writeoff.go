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
type WriteoffRow struct {
	Name     string
	Number   string
	Unit     string
	OKEI     string
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
			{"F", row.OKEI},     // код по ОКЕИ
			{"G", row.Quantity}, // фактически израсходовано
			{"H", row.Reason},   // причина списания
		}
		for _, cell := range cells {
			if err := f.SetCellValue(sheet, fmt.Sprintf("%s%d", cell.col, line), cell.value); err != nil {
				return nil, fmt.Errorf("fill report row %d: %w", line, err)
			}
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

// Коды ОКЕИ — по единице измерения из описи. Незнакомая единица даёт пустую
// графу: выдумывать код нельзя, бланк заполняют вручную.
var okei = map[string]string{
	"шт":        "796",
	"штука":     "796",
	"штук":      "796",
	"компл":     "839",
	"комплект":  "839",
	"набор":     "839",
	"кг":        "166",
	"килограмм": "166",
	"г":         "163",
	"грамм":     "163",
	"т":         "168",
	"тонна":     "168",
	"л":         "112",
	"литр":      "112",
	"мл":        "111",
	"м":         "006",
	"м2":        "055",
	"м²":        "055",
	"м3":        "113",
	"м³":        "113",
	"пач":       "728",
	"пачка":     "728",
	"упак":      "778",
	"упаковка":  "778",
	"рул":       "736",
	"рулон":     "736",
	"банка":     "736",
}

// OKEIFor — код по ОКЕИ для единицы измерения («шт» → 796).
func OKEIFor(unit string) string {
	key := strings.ToLower(strings.TrimSpace(unit))
	key = strings.TrimSuffix(key, ".")
	key = strings.ReplaceAll(key, " ", "")
	return okei[key]
}
