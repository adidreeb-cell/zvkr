package export

import (
	"bytes"
	_ "embed" // 1. Обязательно импортируем пакет embed
	"encoding/json"
	"fmt"

	"github.com/go-pdf/fpdf"
)

// 2. Указываем путь к шрифту (относительно этого .go файла).
// Шрифты должны лежать в папке fonts рядом с этим файлом.
//
//go:embed fonts/arial.ttf
var arialFontBytes []byte

// Если вам нужен еще и жирный шрифт, нужно добавить и его:
// //go:embed fonts/arialbd.ttf
// var arialBoldFontBytes []byte

func GeneratePDF(rawData []byte, title string) (*bytes.Buffer, error) {
	var data []map[string]interface{}
	if err := json.Unmarshal(rawData, &data); err != nil {
		return nil, err
	}

	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.AddPage()

	// 3. Используем метод AddUTF8FontFromBytes вместо AddUTF8Font
	pdf.AddUTF8FontFromBytes("Arial", "", arialFontBytes)

	// Если добавили жирный шрифт выше, подключаем его так:
	// pdf.AddUTF8FontFromBytes("Arial", "B", arialBoldFontBytes)

	// ВАЖНО: В вашем старом коде ниже вызывалось pdf.SetFont("Arial", "B", 16).
	// Если вы не встроили жирный шрифт ("B" - arialbd.ttf), fpdf переключится
	// на стандартный системный шрифт и русский текст превратится в иероглифы.
	// Поэтому здесь мы используем обычный шрифт (""):
	pdf.SetFont("Arial", "", 16)

	// Заголовок документа
	pdf.Cell(40, 10, fmt.Sprintf("Report: %s", title))
	pdf.Ln(12)

	if len(data) == 0 {
		pdf.SetFont("Arial", "", 12)
		pdf.Cell(40, 10, "No data available")
		var buf bytes.Buffer
		err := pdf.Output(&buf)
		return &buf, err
	}

	// 1. Извлекаем заголовки колонок
	pdf.SetFont("Arial", "", 10) // Тут тоже убрал "B", чтобы русский работал
	pdf.SetFillColor(200, 220, 255)

	var keys []string
	for k := range data[0] {
		keys = append(keys, k)
	}

	pageWidth := 277.0
	colWidth := pageWidth / float64(len(keys))
	if colWidth < 20 {
		colWidth = 20
	}

	for _, key := range keys {
		header := key
		if len(header) > 15 {
			header = header[:12] + "..."
		}
		pdf.CellFormat(colWidth, 10, header, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// 2. Рисуем строки с данными
	pdf.SetFont("Arial", "", 9)
	pdf.SetFillColor(255, 255, 255)

	for _, row := range data {
		for _, key := range keys {
			valStr := fmt.Sprintf("%v", row[key])

			if len(valStr) > 20 {
				valStr = valStr[:17] + "..."
			}

			pdf.CellFormat(colWidth, 8, valStr, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}

	return &buf, nil
}
