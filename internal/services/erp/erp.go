package erp

import (
	"encoding/json"
	"exeldoctor/internal/models"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

type ERPService struct {
	MainDB *gorm.DB
}

func NewERPService(db *gorm.DB) *ERPService {
	return &ERPService{MainDB: db}
}

func (s *ERPService) SyncFromDB() {
	var cfg models.SystemSetting
	s.MainDB.First(&cfg, 1)

	if !cfg.EnableUnivDB || cfg.UnivDBDSN == "" {
		return
	}

	log.Printf("[ERP] Подключение к внешней базе (%s)...", cfg.UnivDBType)

	var dialector gorm.Dialector
	switch cfg.UnivDBType {
	case "postgres":
		dialector = postgres.Open(cfg.UnivDBDSN)
	case "mysql":
		dialector = mysql.Open(cfg.UnivDBDSN)
	case "sqlserver":
		dialector = sqlserver.Open(cfg.UnivDBDSN)
	default:
		log.Println("[ERP Error] Неподдерживаемый тип БД")
		return
	}

	externalDB, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		log.Printf("[ERP Error] Ошибка подключения: %v", err)
		return
	}

	// Выполняем произвольный запрос (например, срез по студентам или оценкам)
	// В реальном проекте запрос можно также хранить в настройках
	var result []map[string]interface{}
	err = externalDB.Raw("SELECT * FROM students LIMIT 1000").Scan(&result).Error
	if err != nil {
		log.Printf("[ERP Error] Ошибка выполнения запроса: %v", err)
		return
	}

	if len(result) == 0 {
		return
	}

	// Извлекаем заголовки
	headers := []string{}
	for k := range result[0] {
		headers = append(headers, k)
	}

	hJSON, _ := json.Marshal(headers)
	dJSON, _ := json.Marshal(result)

	dataset := models.Dataset{
		Name:    "ERP Snapshot: " + time.Now().Format("2006-01-02 15:04"),
		Source:  "erp_database",
		Headers: hJSON,
		Data:    dJSON,
	}

	s.MainDB.Create(&dataset)
	log.Printf("[ERP] Снапшот базы данных успешно создан: %d строк", len(result))
}
