package scheduler

import (
	"context"
	"exeldoctor/internal/integrations"
	"exeldoctor/internal/models"
	"exeldoctor/internal/services/analytics"
	"exeldoctor/internal/services/email"
	"exeldoctor/internal/services/erp"
	"exeldoctor/internal/services/excel"
	"exeldoctor/internal/services/moodle"
	"log"

	"github.com/robfig/cron/v3"
	"go.etcd.io/bbolt"
	"gorm.io/gorm"
)

func Start(db *gorm.DB, boltDB *bbolt.DB, analyticsSvc *analytics.Service) *cron.Cron {
	c := cron.New()

	// 1. Загружаем настройки из БД для получения Cron-выражений
	var settings models.SystemSetting
	db.FirstOrCreate(&settings, models.SystemSetting{ID: 1})

	// 2. Инициализация всех сервисов
	excelSvc := excel.NewService()
	newsModule := integrations.NewOfficialNewsModule(boltDB)
	imapSvc := email.NewIMAPService(db, excelSvc)
	moodleSvc := moodle.NewMoodleService(db)
	erpSvc := erp.NewERPService(db)
	// emulatorSvc := emulator.NewEmulatorService(db)

	// Вспомогательная функция для выбора расписания (БД или Дефолт)
	getCron := func(dbVal, defaultVal string) string {
		if dbVal == "" {
			return defaultVal
		}
		return dbVal
	}

	// go newsModule.Run(context.Background(), db)

	// --- РЕГИСТРАЦИЯ ЗАДАЧ ---

	// 1. Новости (RSS)
	c.AddFunc(getCron(settings.NewsCron, "@every 3h"), func() {
		log.Println("[Scheduler] Запуск: Обновление новостей")
		newsModule.Run(context.Background(), db)
	})

	// 2. Почта (IMAP)
	c.AddFunc(getCron(settings.IMAPCron, "@every 30m"), func() {
		log.Println("[Scheduler] Запуск: Проверка почты")
		imapSvc.FetchUnreadExcel()
	})

	// 3. Аналитика (LLM обработка)
	c.AddFunc(getCron(settings.AnalyticsCron, "@every 1h"), func() {
		log.Println("[Scheduler] Запуск: Анализ новых датасетов")
		analyticsSvc.ProcessUnprocessedDatasets(context.Background())
	})

	// 4. Moodle
	c.AddFunc(getCron(settings.MoodleCron, "@every 6h"), func() {
		log.Println("[Scheduler] Запуск: Синхронизация Moodle")
		moodleSvc.FetchData()
	})

	// 5. ERP База данных
	c.AddFunc(getCron(settings.ERPCron, "@every 24h"), func() {
		log.Println("[Scheduler] Запуск: Снапшот ERP")
		erpSvc.SyncFromDB()
	})

	// // 6. Эмулятор (генерация дрейфа данных)
	// c.AddFunc("@every 1h", func() {
	// 	emulatorSvc.GenerateMockDrift()
	// })

	// Разовый запуск при старте сервера (чтобы не ждать 3 часа первых новостей)
	go func() {
		newsModule.Run(context.Background(), db)
		analyticsSvc.ProcessUnprocessedDatasets(context.Background())
	}()

	c.Start()
	log.Println("[Scheduler] Все сервисы запущены по расписанию из БД")
	return c
}
