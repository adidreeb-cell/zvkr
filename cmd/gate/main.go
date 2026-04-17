package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"exeldoctor/internal/config"
	"exeldoctor/internal/database"
	"exeldoctor/internal/handlers"
	"exeldoctor/internal/models"
	"exeldoctor/internal/routes"
	"exeldoctor/internal/services"
	"exeldoctor/internal/services/analytics"
	"exeldoctor/internal/services/excel"
	"exeldoctor/internal/services/llm"
	"exeldoctor/internal/services/sandbox"
	"exeldoctor/internal/services/scheduler"

	gigachatsvc "exeldoctor/internal/services/llm/gigachat"
	ollamasvc "exeldoctor/internal/services/llm/ollama"
	yandexsvc "exeldoctor/internal/services/llm/yandex"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"go.etcd.io/bbolt"
)

func buildLLMService(cfg *config.Config) llm.LLMService {
	switch cfg.LLMProvider {
	case "ollama":
		log.Printf("[LLM] Провайдер: Ollama | URL: %s", cfg.OllamaURL)
		return ollamasvc.NewOllamaService(cfg.OllamaURL, cfg.OllamaModel)

	case "yandex":
		log.Printf("[LLM] Провайдер: YandexGPT | FolderID: %s", cfg.YandexFolderID)
		return yandexsvc.NewYandexService(cfg.YandexAPIKey, cfg.YandexFolderID, cfg.YandexModel)

	case "gigachat":
		log.Printf("[LLM] Провайдер: GigaChat | ClientID: %s", cfg.GigaChatClientID)
		return gigachatsvc.NewGigaChatService(cfg.GigaChatClientID, cfg.GigaChatSecret)

	case "openrouter":
		log.Printf("[LLM] Провайдер: OpenRouter | Модель: %s", cfg.OpenrouterModel)
		return llm.NewOpenRouterService(
			cfg.OpenrouterApiKey,
			cfg.OpenrouterModel,
			"ExelDoctor",
			"http://localhost:3000",
		)

	default:
		log.Printf("[LLM] Провайдер не выбран или неизвестен (%s). Использую OpenRouter по умолчанию.", cfg.LLMProvider)
		return llm.NewOpenRouterService(cfg.OpenrouterApiKey, cfg.OpenrouterModel, "ExelDoctor", "http://localhost:3000")
	}
}

func main() {
	noOpenBrowser := flag.Bool("no-browser", false, "No open browser")
	flag.Parse()

	db := database.Connect()

	boltDB, err := bbolt.Open("cache.db", 0600, nil)
	if err != nil {
		log.Fatalf("Ошибка открытия BoltDB: %v", err)
	}
	defer boltDB.Close()

	boltDB.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("DashboardCache"))
		return err
	})

	if err := models.Migrate(db); err != nil {
		log.Fatal("Migration failed:", err)
	}

	vm, _ := sandbox.NewWasmRunner(context.Background())
	defer vm.Close(context.Background())

	// Канал для сигнала о мягкой перезагрузке
	reloadCh := make(chan struct{})

	for {
		log.Println("[SYSTEM] Инициализация/Перезагрузка сервера...")

		// Читаем актуальные настройки
		cfg := config.Load()
		var currentSettings models.SystemSetting
		if err := db.First(&currentSettings, 1).Error; err == nil {
			cfg.UpdateFromDB(currentSettings)
		}

		// Пересобираем сервисы с новым конфигом
		excelService := excel.NewService()
		llmService := buildLLMService(cfg)
		analyticsService := analytics.NewAnalyticsService(db, llmService)

		cronJob := scheduler.Start(db, boltDB, analyticsService)

		aiPipeline := services.NewAIPipeline(llmService, vm)

		// Пересобираем хендлеры
		authHandler := &handlers.AuthHandler{JWTSecret: cfg.JWTSecret, DB: *db}
		exportHandler := &handlers.ExportHandler{DB: db}
		datasetHandler := handlers.NewDatasetHandler(db, boltDB, excelService, llmService, vm, aiPipeline)
		analyticsHandler := &handlers.AnalyticsHandler{Service: analyticsService}

		settingsHandler := handlers.NewSettingsHandler(db, cfg, reloadCh)

		app := fiber.New(fiber.Config{
			DisableStartupMessage: true, // Чтобы не спамил в консоль при рестартах
		})

		// Middleware
		app.Use(logger.New())
		app.Use(cors.New(cors.Config{
			AllowOrigins: "*",
			AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		}))

		// Роуты
		routes.SetupRoutes(
			app, authHandler,
			datasetHandler,
			exportHandler,
			analyticsHandler,
			settingsHandler,
			boltDB,
			cfg.JWTSecret,
			cfg.Port,
			!*noOpenBrowser,
		)

		// 3. Запускаем Fiber в отдельной горутине
		go func() {
			log.Printf("[SYSTEM] Сервер запущен на порту %s", cfg.Port)
			if err := app.Listen(":" + cfg.Port); err != nil {
				log.Printf("Ошибка сервера: %v", err)
			}
		}()

		// 4. Ожидаем прерывания от ОС (Ctrl+C) или сигнала перезагрузки
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

		select {
		case <-reloadCh:
			log.Println("[SYSTEM] Получен сигнал перезагрузки! Ожидание завершения текущих запросов...")
			cronJob.Stop() // Останавливаем старые фоновые задачи

			// app.Shutdown() блокирует поток, пока все текущие запросы не завершатся
			if err := app.Shutdown(); err != nil {
				log.Printf("[SYSTEM] Ошибка при мягком завершении: %v", err)
			}
			log.Println("[SYSTEM] Старый инстанс остановлен. Пересобираю...")
			// Цикл пойдет на новый круг

		case <-quit:
			log.Println("[SYSTEM] Остановка приложения...")
			cronJob.Stop()
			app.Shutdown()
			return // Выход из main(), полное завершение программы
		}
	}
}
