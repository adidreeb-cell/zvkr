package handlers

import (
	"exeldoctor/internal/config"
	"exeldoctor/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type SettingsHandler struct {
	DB       *gorm.DB
	Cfg      *config.Config
	ReloadCh chan struct{}
}

func NewSettingsHandler(db *gorm.DB, cfg *config.Config, reloadCh chan struct{}) *SettingsHandler {
	return &SettingsHandler{DB: db, Cfg: cfg, ReloadCh: reloadCh}
}

// Маска для скрытия паролей
const mask = "********"

// GetSettings возвращает текущие настройки
func (h *SettingsHandler) GetSettings(c *fiber.Ctx) error {
	var settings models.SystemSetting
	if err := h.DB.FirstOrCreate(&settings, models.SystemSetting{ID: 1}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Не удалось загрузить настройки"})
	}

	// Скрываем чувствительные данные перед отправкой на фронтенд
	if settings.IMAPPassword != "" {
		settings.IMAPPassword = mask
	}
	if settings.OpenRouterAPIKey != "" {
		settings.OpenRouterAPIKey = mask
	}
	if settings.YandexAPIKey != "" {
		settings.YandexAPIKey = mask
	}
	if settings.MoodleToken != "" {
		settings.MoodleToken = mask
	}
	if settings.GigaChatClientSecret != "" {
		settings.GigaChatClientSecret = mask
	}

	return c.JSON(settings)
}

// UpdateSettings сохраняет настройки и применяет их к приложению
func (h *SettingsHandler) UpdateSettings(c *fiber.Ctx) error {
	var req models.SystemSetting
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Неверный формат данных"})
	}

	var settings models.SystemSetting
	h.DB.FirstOrCreate(&settings, models.SystemSetting{ID: 1})

	// 1. Обновляем обычные поля
	settings.LLMProvider = req.LLMProvider
	settings.OpenRouterModel = req.OpenRouterModel
	settings.OllamaURL = req.OllamaURL
	settings.OllamaModel = req.OllamaModel
	settings.YandexFolderID = req.YandexFolderID
	settings.YandexModel = req.YandexModel
	settings.GigaChatClientID = req.GigaChatClientID

	settings.EnableIMAP = req.EnableIMAP
	settings.IMAPHost = req.IMAPHost
	settings.IMAPPort = req.IMAPPort
	settings.IMAPUsername = req.IMAPUsername

	settings.EnableMoodle = req.EnableMoodle
	settings.MoodleURL = req.MoodleURL

	settings.EnableUnivDB = req.EnableUnivDB
	settings.UnivDBType = req.UnivDBType
	settings.UnivDBDSN = req.UnivDBDSN

	settings.EnableEmulator = req.EnableEmulator
	settings.EmulatorCron = req.EmulatorCron
	settings.UpdatedAt = time.Now()

	// 2. Обновляем секретные поля (только если они изменены и это не маска)
	updateSecret(&settings.IMAPPassword, req.IMAPPassword)
	updateSecret(&settings.OpenRouterAPIKey, req.OpenRouterAPIKey)
	updateSecret(&settings.YandexAPIKey, req.YandexAPIKey)
	updateSecret(&settings.MoodleToken, req.MoodleToken)
	updateSecret(&settings.GigaChatClientSecret, req.GigaChatClientSecret)

	// 3. Сохраняем в базу
	if err := h.DB.Save(&settings).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Ошибка сохранения в БД"})
	}

	// 4. ПРИМЕНЯЕМ настройки в реальном времени к объекту Config
	h.Cfg.UpdateFromDB(settings)

	go func() {
		time.Sleep(time.Second * 5)
		h.ReloadCh <- struct{}{}
	}()

	return c.JSON(fiber.Map{"success": true, "message": "Настройки сохранены и применены. СЕРВЕР БУДЕТ ПЕРЕЗАГРУЖЕН"})
}

// Вспомогательная функция: если пришла маска или пустота - не меняем старое значение
func updateSecret(target *string, newValue string) {
	if newValue != "" && newValue != mask {
		*target = newValue
	}
}
