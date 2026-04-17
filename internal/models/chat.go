package models

import "time"

// ChatMessage хранит историю общения с датасетом
type ChatMessage struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	DatasetID  uint      `gorm:"index;not null" json:"dataset_id"`
	Role       string    `gorm:"not null" json:"role"` // "user", "bot", "system"
	Content    string    `gorm:"type:text" json:"content"`
	CodeOutput string    `gorm:"type:text" json:"code_output"`
	SourceCode string    `gorm:"type:text" json:"source_code,omitempty"`
	IsError    bool      `json:"is_error"`
	CreatedAt  time.Time `json:"created_at"`
}
