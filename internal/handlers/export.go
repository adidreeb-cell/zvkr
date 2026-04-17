package handlers

import (
	"exeldoctor/internal/models"
	"exeldoctor/internal/services/export"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type ExportHandler struct {
	DB *gorm.DB
}

// ExportDatasetExcel - выгрузка в XLSX
func (h *ExportHandler) ExportDatasetExcel(c *fiber.Ctx) error {
	id := c.Params("id")

	var dataset models.Dataset
	if err := h.DB.First(&dataset, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dataset not found"})
	}

	buf, err := export.GenerateExcel(dataset.Data, "Dataset")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate Excel"})
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.xlsx", dataset.Name))

	return c.SendStream(buf)
}

func (h *ExportHandler) ExportDatasetPDF(c *fiber.Ctx) error {
	id := c.Params("id")

	var dataset models.Dataset
	if err := h.DB.First(&dataset, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dataset not found"})
	}

	// Вызываем генератор PDF
	buf, err := export.GeneratePDF(dataset.Data, dataset.Name)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate PDF"})
	}

	// Устанавливаем правильные MIME-типы для скачивания PDF
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", dataset.Name))

	return c.SendStream(buf) // Отдаем прямо из оперативной памяти
}
