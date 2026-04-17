package emulator

import (
	"encoding/json"
	"exeldoctor/internal/models"
	"fmt"
	"log"
	"math/rand"
	"time"

	"gorm.io/gorm"
)

type EmulatorService struct {
	DB *gorm.DB
}

func NewEmulatorService(db *gorm.DB) *EmulatorService {
	return &EmulatorService{DB: db}
}

// GenerateMockDrift создает новый датасет, который имитирует "слепок" базы на сегодняшний день.
func (s *EmulatorService) GenerateMockDrift() {
	var cfg models.SystemSetting
	s.DB.First(&cfg, 1)

	if !cfg.EnableEmulator {
		return
	}

	log.Println("[Emulator] Генерация свежего слепка базы университета...")

	// Генерируем "плавающие" данные (например, случайное число отчисленных)
	baseStudents := 1500
	drift := rand.Intn(50) - 15 // От -15 до +35 студентов изменение
	currentStudents := baseStudents + drift

	var mockData []map[string]interface{}
	faculties := []string{"ИТ", "Экономика", "Юриспруденция", "Медицина"}
	statuses := []string{"Обучается", "Обучается", "Обучается", "Отчислен", "В академе"}

	for i := 0; i < currentStudents; i++ {
		mockData = append(mockData, map[string]interface{}{
			"ФИО студента":    fmt.Sprintf("Студент Эмуляции №%d", i+1),
			"Факультет":       faculties[rand.Intn(len(faculties))],
			"Курс":            rand.Intn(5) + 1,
			"Статус":          statuses[rand.Intn(len(statuses))],
			"Средний балл":    (rand.Float64() * 2) + 3, // От 3.0 до 5.0
			"Дата зачисления": time.Now().AddDate(-rand.Intn(4), 0, 0).Format("2006-01-02"),
		})
	}

	headers := []string{"ФИО студента", "Факультет", "Курс", "Статус", "Средний балл", "Дата зачисления"}
	hJSON, _ := json.Marshal(headers)
	dJSON, _ := json.Marshal(mockData)

	dataset := models.Dataset{
		Name:      fmt.Sprintf("DB_Snapshot_%s.xlsx", time.Now().Format("02-01_15:04")),
		Source:    "emulator",
		Headers:   hJSON,
		Data:      dJSON,
		CreatedAt: time.Now(),
	}

	s.DB.Create(&dataset)
	log.Printf("[Emulator] Сгенерирован датасет на %d строк", currentStudents)
}
