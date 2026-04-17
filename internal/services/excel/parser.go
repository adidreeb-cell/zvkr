package excel

import (
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

// Обновленный метод Parse, принимающий имя файла для определения типа
func (s *Service) Parse(reader io.Reader, filename string) ([]string, []map[string]interface{}, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	if ext == ".csv" {
		return s.parseCSV(reader)
	}
	return s.parseExcel(reader)
}

func (s *Service) parseCSV(reader io.Reader) ([]string, []map[string]interface{}, error) {
	csvReader := csv.NewReader(reader)
	rows, err := csvReader.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(rows) < 1 {
		return nil, nil, fmt.Errorf("empty csv")
	}
	return s.rowsToMap(rows)
}

func (s *Service) parseExcel(reader io.Reader) ([]string, []map[string]interface{}, error) {
	f, err := excelize.OpenReader(reader)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	rows, _ := f.GetRows(f.GetSheetName(0))
	return s.rowsToMap(rows)
}

// Вспомогательная функция конвертации
func (s *Service) rowsToMap(rows [][]string) ([]string, []map[string]interface{}, error) {
	if len(rows) < 1 {
		return nil, nil, fmt.Errorf("no data")
	}

	headers := rows[0]
	var result []map[string]interface{}

	for _, row := range rows[1:] {
		rowMap := make(map[string]interface{})
		for i, val := range row {
			if i < len(headers) {
				rowMap[headers[i]] = val
			}
		}
		result = append(result, rowMap)
	}
	return headers, result, nil
}
