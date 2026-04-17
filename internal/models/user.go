package models

import "time"

type RoleType string

const (
	UserRole    RoleType = "user"
	AnalystRole RoleType = "analyst"
	Admin       RoleType = "admin"
)

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`                   // "-" чтобы хэш никогда не попадал в JSON ответы
	Role         RoleType  `gorm:"not null;default:'user'" json:"role"` // "admin", "analyst", "viewer"
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
