// Утилита: собрать образцы описи для визуальной проверки бланка
// (go run ./cmd/opis-sample /tmp/opis-samples).
package main

import (
	"fmt"
	"os"
	"time"

	"mitm-departament/internal/report"
)

func main() {
	out := os.Args[1]
	_ = os.MkdirAll(out, 0o755)
	zav := "Куценко П. А."

	items105 := []report.RoomOpisItem{
		{Section: report.SectionEquipment, Name: "Микроскоп инвертированный Carl Zeiss Axio Vert.A1 с камерой", Number: "1050005442", Qty: 1},
		{Section: report.SectionEquipment, Name: "Весы аналитические", Number: "", Qty: 2},
		{Section: report.SectionEquipment, Name: "Стол лабораторный", Number: "1050005512", Qty: 3},
		{Section: report.SectionFurniture, Name: "Стол письменный", Number: "", Qty: 4},
		{Section: report.SectionFurniture, Name: "Стул", Number: "", Qty: 6},
		{Section: report.SectionOther, Name: "Ключ от кабинета", Number: "00-1", Qty: 1},
	}
	big := make([]report.RoomOpisItem, 0, 60)
	for i := 0; i < 40; i++ {
		big = append(big, report.RoomOpisItem{
			Section: report.SectionEquipment,
			Name:    fmt.Sprintf("Компьютер персональный в комплекте (системный блок, монитор, клавиатура, мышь) №%d", i+1),
			Number:  fmt.Sprintf("10500055%02d", i),
			Qty:     1,
		})
	}
	for i := 0; i < 20; i++ {
		big = append(big, report.RoomOpisItem{Section: report.SectionFurniture, Name: "Стол компьютерный", Number: "", Qty: 1})
	}

	docs := map[string]report.RoomOpisParams{
		"105-сегодня.docx": {Room: "105", Date: time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), Zav: zav, Items: items105},
		"105-без-даты.docx": {Room: "105", Zav: zav, Items: items105},
		"026-025-проверил.docx": {Room: "026/025", Date: time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), Zav: zav,
			Checked: true, CheckName: "Сидорова А. А.", Items: big},
	}
	for name, p := range docs {
		b, err := report.BuildRoomOpis(p)
		if err != nil {
			fmt.Println("ошибка", name, err)
			os.Exit(1)
		}
		path := out + "/" + name
		if err := os.WriteFile(path, b, 0o644); err != nil {
			fmt.Println("запись", name, err)
			os.Exit(1)
		}
		fmt.Println(path, len(b))
	}
}
