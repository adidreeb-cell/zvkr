package models

import (
	"encoding/json"
	"time"
)

type GlobalMetric struct {
	ID              uint            `gorm:"primaryKey" json:"id"`
	TotalStudents   int64           `json:"total_students"`
	ActiveStudents  int64           `json:"active_students"`
	AverageScore    float64         `json:"average_score"`
	StatusBreakdown json.RawMessage `gorm:"type:jsonb" json:"status_breakdown"`

	TimeSeries json.RawMessage `gorm:"type:jsonb" json:"time_series"` // Данные по месяцам/кварталам
	Trends     json.RawMessage `gorm:"type:jsonb" json:"trends"`      // Массив строк
	Anomalies  json.RawMessage `gorm:"type:jsonb" json:"anomalies"`   // Массив строк
	UpdatedAt  time.Time       `json:"updated_at"`
}

type ProcessedPeriod struct {
	ID        uint   `gorm:"primaryKey"`
	Period    string `gorm:"uniqueIndex;not null"`
	DatasetID uint   `gorm:"not null"`
}
