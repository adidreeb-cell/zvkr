package config

import (
	"exeldoctor/internal/models"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port      string
	DBPath    string
	JWTSecret []byte

	// LLM Настройки
	LLMProvider      string
	OpenrouterApiKey string
	OpenrouterModel  string
	OllamaURL        string
	OllamaModel      string
	YandexFolderID   string
	YandexAPIKey     string
	YandexModel      string
	GigaChatClientID string
	GigaChatSecret   string

	// Флаги включения модулей
	EnableIMAP     bool
	EnableMoodle   bool
	EnableUnivDB   bool
	EnableEmulator bool
}

func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		Port:      getEnv("PORT", "6331"),
		DBPath:    getEnv("DB_PATH", "doctor.db"),
		JWTSecret: []byte(getEnv("JWT_SECRET", "super-secret-key")),

		// Значения по умолчанию из ENV
		LLMProvider:      getEnv("LLM_PROVIDER", "openrouter"),
		OpenrouterApiKey: getEnv("OPENROUTER_API_KEY", ""),
		OpenrouterModel:  getEnv("OPENROUTER_MODEL", "google/gemma-3-27b-it"),
		OllamaURL:        getEnv("OLLAMA_URL", "http://localhost:11434"),
		OllamaModel:      getEnv("OLLAMA_MODEL", "llama3"),
	}
}

// UpdateFromDB синхронизирует объект Config с данными из БД
func (cfg *Config) UpdateFromDB(s models.SystemSetting) {
	if s.LLMProvider != "" {
		cfg.LLMProvider = s.LLMProvider
	}
	cfg.OpenrouterApiKey = s.OpenRouterAPIKey
	cfg.OpenrouterModel = s.OpenRouterModel
	cfg.OllamaURL = s.OllamaURL
	cfg.OllamaModel = s.OllamaModel
	cfg.YandexFolderID = s.YandexFolderID
	cfg.YandexAPIKey = s.YandexAPIKey
	cfg.YandexModel = s.YandexModel
	cfg.GigaChatClientID = s.GigaChatClientID
	cfg.GigaChatSecret = s.GigaChatClientSecret

	cfg.EnableIMAP = s.EnableIMAP
	cfg.EnableMoodle = s.EnableMoodle
	cfg.EnableUnivDB = s.EnableUnivDB
	cfg.EnableEmulator = s.EnableEmulator

	log.Println("✅ Конфигурация приложения обновлена из БД")
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
