package models

import "time"

type SystemSetting struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// LLM
	LLMProvider          string `json:"llm_provider"`
	OpenRouterAPIKey     string `json:"openrouter_api_key"` // Убрал лишний underscore
	OpenRouterModel      string `json:"openrouter_model"`
	OllamaURL            string `json:"ollama_url"`
	OllamaModel          string `json:"ollama_model"`
	YandexFolderID       string `json:"yandex_folder_id"`
	YandexAPIKey         string `json:"yandex_api_key"`
	YandexModel          string `json:"yandex_model"`
	GigaChatClientID     string `json:"gigachat_client_id"`
	GigaChatClientSecret string `json:"gigachat_client_secret"`

	// IMAP
	EnableIMAP   bool   `json:"enable_imap"`
	IMAPHost     string `json:"imap_host"`
	IMAPPort     int    `json:"imap_port"`
	IMAPUsername string `json:"imap_username"`
	IMAPPassword string `json:"imap_password"`

	// Moodle
	EnableMoodle bool   `json:"enable_moodle"`
	MoodleURL    string `json:"moodle_url"`
	MoodleToken  string `json:"moodle_token"`

	// ERP
	EnableUnivDB bool   `json:"enable_univ_db"`
	UnivDBType   string `json:"univ_db_type"`
	UnivDBDSN    string `json:"univ_db_dsn"` // Соответствует univ_db_dsn во фронте

	// System
	EnableEmulator bool   `json:"enable_emulator"`
	EmulatorCron   string `json:"emulator_cron"`

	// --- Настройки расписания (Cron) ---
	NewsCron      string `json:"news_cron"`      // По умолчанию "@every 3h"
	IMAPCron      string `json:"imap_cron"`      // По умолчанию "@every 30m"
	AnalyticsCron string `json:"analytics_cron"` // По умолчанию "@every 2h"
	MoodleCron    string `json:"moodle_cron"`    // По умолчанию "@every 6h"
	ERPCron       string `json:"erp_cron"`       // По умолчанию "@every 24h"

	// --- Источники новостей (RSS) ---
	// Храним как JSON строку: [{"name": "Минпросвещения", "url": "..."}]
	RSSSources string `json:"rss_sources" gorm:"type:text"`

	UpdatedAt time.Time `json:"updated_at"`
}
