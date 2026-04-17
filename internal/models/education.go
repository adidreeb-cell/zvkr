package models

import (
	"time"
)

// Student - контингент
type Student struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	FullName       string    `json:"full_name"`
	Faculty        string    `json:"faculty"`
	Course         int       `json:"course"`
	Status         string    `json:"status"` // "Обучается", "Отчислен", "В академе"
	EnrollmentDate time.Time `json:"enrollment_date"`
}

// Grade - успеваемость
type Grade struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	StudentID uint      `json:"student_id"`
	Subject   string    `json:"subject"`
	Score     int       `json:"score"` // 2, 3, 4, 5 или 1-100
	Date      time.Time `json:"date"`
}
