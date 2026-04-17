package export

import (
	"bytes"
	"encoding/json"

	"github.com/xuri/excelize/v2"
)

// GenerateExcel принимает JSON массив объектов и возвращает буфер с XLSX файлом
func GenerateExcel(rawData []byte, sheetName string) (*bytes.Buffer, error) {
	var data []map[string]interface{}
	if err := json.Unmarshal(rawData, &data); err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer f.Close()

	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, err
	}

	if len(data) > 0 {
		// Извлекаем заголовки
		var colIndex = 1
		keys := make([]string, 0, len(data[0]))
		for k := range data[0] {
			keys = append(keys, k)
			cell, _ := excelize.CoordinatesToCellName(colIndex, 1)
			f.SetCellValue(sheetName, cell, k)
			colIndex++
		}

		// Заполняем данные
		for rowIndex, rowData := range data {
			for cIdx, key := range keys {
				cell, _ := excelize.CoordinatesToCellName(cIdx+1, rowIndex+2)
				f.SetCellValue(sheetName, cell, rowData[key])
			}
		}
	}

	f.SetActiveSheet(index)

	// Пишем в буфер памяти
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}

	return &buf, nil
}
