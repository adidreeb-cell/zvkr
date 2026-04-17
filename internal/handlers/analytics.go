package handlers

import (
	"exeldoctor/internal/services/analytics"

	"github.com/gofiber/fiber/v2"
)

type AnalyticsHandler struct {
	Service *analytics.Service
}

func NewAnalyticsHandler(svc *analytics.Service) *AnalyticsHandler {
	return &AnalyticsHandler{Service: svc}
}

// GetSyncStatus - Возвращает статус обработки датасетов и время последнего обновления метрик
// Пример запроса: GET /api/v1/analytics/status
func (h *AnalyticsHandler) GetSyncStatus(c *fiber.Ctx) error {
	var pendingCount int64

	// Считаем, сколько датасетов еще не обработано
	// Если > 0, значит система в процессе синхронизации или требует ее запуска
	if err := h.Service.DB.Table("datasets").Where("is_processed = ?", false).Count(&pendingCount).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Не удалось получить статус синхронизации",
		})
	}

	// Получаем время последнего обновления глобальных метрик
	var lastUpdate string
	h.Service.DB.Table("global_metrics").Where("id = ?", 1).Select("updated_at").Scan(&lastUpdate)

	return c.JSON(fiber.Map{
		"success":       true,
		"is_syncing":    pendingCount > 0, // Фронтенд может использовать это, чтобы заблокировать кнопку "Синхронизация"
		"pending_count": pendingCount,     // Сколько файлов в очереди
		"last_updated":  lastUpdate,       // Время последнего обновления
	})
}

// GetAdvancedAnalytics - Полная аналитика с поддержкой фильтрации по времени
func (h *AnalyticsHandler) GetAdvancedAnalytics(c *fiber.Ctx) error {
	startDate := c.Query("from")
	endDate := c.Query("to")

	data, err := h.Service.GetAdvancedMetrics(startDate, endDate)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Аналитика пока не сформирована",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

// GetBasicMetrics - Отдает базовые метрики ТОЛЬКО за последний доступный год
func (h *AnalyticsHandler) GetBasicMetrics(c *fiber.Ctx) error {
	allData, err := h.Service.GetAdvancedMetrics("", "")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Не удалось собрать метрики",
		})
	}

	if len(allData.TimeSeries) == 0 {
		return c.JSON(fiber.Map{
			"success": true,
			"period":  "",
			"data":    allData.Metrics,
		})
	}

	latestItem := allData.TimeSeries[len(allData.TimeSeries)-1]

	latestMetrics := analytics.MetricsData{
		TotalStudents:   latestItem.TotalStudents,
		ActiveStudents:  latestItem.ActiveStudents,
		AverageScore:    latestItem.AverageScore,
		StatusBreakdown: latestItem.StatusBreakdown,
	}

	return c.JSON(fiber.Map{
		"success": true,
		"period":  latestItem.Period,
		"data":    latestMetrics,
	})
}

// GetDashboard - Алиас для фронта
func (h *AnalyticsHandler) GetDashboard(c *fiber.Ctx) error {
	return h.GetAdvancedAnalytics(c)
}

// ForceSync - Ручной запуск парсинга
func (h *AnalyticsHandler) ForceSync(c *fiber.Ctx) error {
	// Используем UserContext(), чтобы LLM не отваливалась по тайм-ауту HTTP-запроса Fiber
	ctx := c.UserContext()

	err := h.Service.ProcessUnprocessedDatasets(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Ошибка при обработке датасетов",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Данные успешно проанализированы и метрики обновлены",
	})
}
