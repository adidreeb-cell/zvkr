package routes

import (
	"embed"
	"exeldoctor/internal/handlers"
	"exeldoctor/internal/middleware"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gofiber/fiber/v2"
	fiberfs "github.com/gofiber/fiber/v2/middleware/filesystem"
	"go.etcd.io/bbolt"
)

func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		// xdg-open — стандартная утилита в Linux для открытия URL и файлов
		// в приложениях по умолчанию. Она сама корректно работает в фоне.
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		// В macOS достаточно просто передать ссылку команде open
		// (флаг -a не нужен, система сама выберет браузер по умолчанию)
		cmd = exec.Command("open", url)
	case "windows":
		// Использование rundll32 более безопасно для URL, так как
		// cmd /c start может "сломаться", если в ссылке есть символ "&"
		// (например, https://site.com?id=1&val=2)
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	// Отвязываем потоки ввода/вывода, чтобы процесс не зависел от консоли
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	// Start() запускает процесс и сразу возвращает управление, не дожидаясь закрытия браузера
	return cmd.Start()
}

// Встраиваем фронтенд
//
//go:embed dist/*
var distFS embed.FS

func SetupRoutes(
	app *fiber.App,
	authHandler *handlers.AuthHandler,
	datasetHandler *handlers.DatasetHandler,
	exportHandler *handlers.ExportHandler,
	analyticsHandler *handlers.AnalyticsHandler,
	settingsHandler *handlers.SettingsHandler,
	boltDB *bbolt.DB,
	jwtSecret []byte,
	port string,
	openBrowserFlag bool,
) {

	// --- ЛОГИКА ПЕРВОГО ЗАПУСКА ---
	// Вызываем InitAdminUser и, если он создан, сохраняем пароль в Handler
	password, created, err := authHandler.InitAdminUser()
	if err != nil {
		log.Printf("[Auth] Ошибка при инициализации admin: %v", err)
	} else if created {
		log.Printf("[Auth] Admin создан. Пароль: %s", password)
		authHandler.SetupPassword = password // КЛАДЕМ ПАРОЛЬ В ПАМЯТЬ

		openBrowser(fmt.Sprintf("http://localhost:%s/setup", port))
	} else {
		openBrowser(fmt.Sprintf("http://localhost:%s", port))
	}

	// ================= API =================

	api := app.Group("/api/v1")

	api.Get("/system/setup", authHandler.GetSetupInfo)
	api.Post("/system/setup/complete", authHandler.CompleteSetup)

	api.Post("/register", authHandler.Register)
	api.Post("/login", authHandler.Login)

	// Новости из BoltDB
	api.Get("/news", func(c *fiber.Ctx) error {
		var newsData []byte

		err := boltDB.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte("DashboardCache"))
			if b == nil {
				return fiber.ErrNotFound
			}

			newsData = b.Get([]byte("official_news"))
			return nil
		})

		if err != nil || len(newsData) == 0 {
			return c.Status(503).JSON(fiber.Map{
				"error": "Новости еще загружаются",
			})
		}

		c.Set("Content-Type", "application/json")
		return c.Send(newsData)
	})

	// ================= PROTECTED =================

	protected := api.Group("/", middleware.Protected(jwtSecret))

	// --- Admin ---
	protected.Get("/users", middleware.RoleCheck("admin"), authHandler.GetUsers)
	protected.Post("/users/add", middleware.RoleCheck("admin"), authHandler.CreateUser)
	protected.Post("/users/remove", middleware.RoleCheck("admin"), authHandler.DeleteUser)
	protected.Get("/settings", middleware.RoleCheck("admin"), settingsHandler.GetSettings)
	protected.Post("/settings", middleware.RoleCheck("admin"), settingsHandler.UpdateSettings)

	// --- Upload ---
	protected.Post("/upload", middleware.RoleCheck("analyst", "admin"), datasetHandler.Upload)

	// --- Analytics ---
	protected.Get("/analytics/sync", middleware.RoleCheck("analyst", "admin"), analyticsHandler.ForceSync)
	protected.Get("/analytics/metrics", analyticsHandler.GetBasicMetrics)
	protected.Get("/analytics/advanced", analyticsHandler.GetAdvancedAnalytics)
	protected.Get("/analytics/status", analyticsHandler.GetSyncStatus)

	// --- Datasets ---
	protected.Get("/datasets", datasetHandler.ListDatasets)
	protected.Get("/datasets/:id", datasetHandler.GetDataset)

	// --- Chat / AI ---
	protected.Get("/datasets/:id/chat", datasetHandler.GetChatHistory)
	protected.Post("/datasets/:id/chat", datasetHandler.Chat)

	// --- Python sandbox ---
	protected.Post("/python/run", datasetHandler.RunPython)

	// --- Export ---
	protected.Get("/export/excel/:id", exportHandler.ExportDatasetExcel)
	protected.Get("/export/pdf/:id", exportHandler.ExportDatasetPDF)

	// ================= STATIC SPA =================

	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}

	app.Use("/", fiberfs.New(fiberfs.Config{
		Root:   http.FS(dist),
		Browse: false,
	}))

	// SPA fallback
	app.Use(func(c *fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), "/api") {
			return c.Status(404).JSON(fiber.Map{
				"error": "API route not found",
			})
		}

		index, err := distFS.ReadFile("dist/index.html")
		if err != nil {
			return err
		}

		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.Send(index)
	})
}
