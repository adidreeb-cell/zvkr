package models

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
)

// Dataset представляет загруженный Excel файл
type Dataset struct {
	ID            uint            `gorm:"primaryKey" json:"id"`
	Name          string          `json:"name"`
	Source        string          `json:"source"` // "upload", "email"
	Headers       json.RawMessage `gorm:"type:jsonb" json:"headers"`
	Data          json.RawMessage `gorm:"type:jsonb" json:"data"` // Массив объектов
	Summary       string          `json:"summary"`                // Краткая выжимка от LLM при загрузке
	IsProcessed   bool            `json:"is_processed"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	MetricsJSON   json.RawMessage `gorm:"type:jsonb" json:"metrics_json"`
	ColumnMapping datatypes.JSON  `json:"column_mapping" gorm:"type:jsonb"`
}
