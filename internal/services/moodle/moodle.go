package moodle

import (
	"encoding/json"
	"exeldoctor/internal/models"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"gorm.io/gorm"
)

type MoodleService struct {
	DB *gorm.DB
}

func NewMoodleService(db *gorm.DB) *MoodleService {
	return &MoodleService{DB: db}
}

func (s *MoodleService) FetchData() {
	var cfg models.SystemSetting
	s.DB.First(&cfg, 1)

	if !cfg.EnableMoodle || cfg.MoodleURL == "" || cfg.MoodleToken == "" {
		return
	}

	log.Println("[Moodle] Синхронизация данных...")

	// Пример: Получаем список курсов через функцию core_course_get_courses
	// Формат Moodle: URL + /webservice/rest/server.php?wstoken=TOKEN&wsfunction=FUNC&moodlewsrestformat=json
	apiURL := fmt.Sprintf("%s/webservice/rest/server.php?wstoken=%s&wsfunction=core_course_get_courses&moodlewsrestformat=json",
		cfg.MoodleURL, cfg.MoodleToken)

	resp, err := http.Get(apiURL)
	if err != nil {
		log.Printf("[Moodle Error] %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Парсим как массив интерфейсов для универсальности
	var rawData []map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		log.Printf("[Moodle Error] Ошибка парсинга JSON: %v", err)
		return
	}

	if len(rawData) == 0 {
		return
	}

	// Формируем заголовки из ключей первой записи
	headers := []string{}
	for k := range rawData[0] {
		headers = append(headers, k)
	}

	hJSON, _ := json.Marshal(headers)
	dJSON, _ := json.Marshal(rawData)

	dataset := models.Dataset{
		Name:        "Moodle Sync: " + time.Now().Format("2006-01-02 15:04"),
		Source:      "moodle",
		Headers:     hJSON,
		Data:        dJSON,
		IsProcessed: false,
	}

	s.DB.Create(&dataset)
	log.Printf("[Moodle] Успешно загружен датасет: %d записей", len(rawData))
}
