package integrations

import (
	"context"
	"encoding/json"
	"exeldoctor/internal/models"
	"log"
	"sort"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
	"go.etcd.io/bbolt"
	"gorm.io/gorm"
)

type NewsItem struct {
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	PublishedAt time.Time `json:"published_at"`
	Description string    `json:"description"`
	Source      string    `json:"source"`
}

type OfficialNewsModule struct {
	db      *bbolt.DB
	sources map[string]string
}

func NewOfficialNewsModule(db *bbolt.DB) *OfficialNewsModule {
	return &OfficialNewsModule{
		db: db,
		sources: map[string]string{
			"Минпросвещения РФ": "https://edu.gov.ru/press/_rss/",
			"Рособрнадзор":      "https://obrnadzor.gov.ru/feed/",
		},
	}
}

func (m *OfficialNewsModule) Name() string {
	return "Russian Official Education News Parser"
}

func (m *OfficialNewsModule) Interval() time.Duration {
	return 3 * time.Hour
}

func (m *OfficialNewsModule) Run(ctx context.Context, gormDB *gorm.DB) error {
	// 1. Получаем источники из настроек БД
	var cfg models.SystemSetting
	gormDB.First(&cfg, 1)

	var sources map[string]string
	if cfg.RSSSources != "" {
		// Парсим JSON из БД в карту
		var sourceList []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		json.Unmarshal([]byte(cfg.RSSSources), &sourceList)

		sources = make(map[string]string)
		for _, s := range sourceList {
			sources[s.Name] = s.URL
		}
	} else {
		// Дефолтные значения, если в БД пусто
		sources = map[string]string{"Минпросвещения РФ": "https://edu.gov.ru/press/_rss/"}
	}

	fp := gofeed.NewParser()
	var allNews []NewsItem

	for sourceName, feedURL := range m.sources {
		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

		feed, err := fp.ParseURLWithContext(feedURL, fetchCtx)
		cancel()

		if err != nil {
			log.Printf("[News Parser] Ошибка получения данных от %s (%s): %v\n", sourceName, feedURL, err)
			continue
		}

		// Берем только последние 10 новостей от каждого министерства
		limit := 10
		if len(feed.Items) < limit {
			limit = len(feed.Items)
		}

		for _, item := range feed.Items[:limit] {
			pubDate := time.Now()
			if item.PublishedParsed != nil {
				pubDate = *item.PublishedParsed
			}

			allNews = append(allNews, NewsItem{
				Title:       item.Title,
				Link:        item.Link,
				PublishedAt: pubDate,
				Description: stripHTMLTags(item.Description), // Очищаем от HTML-мусора
				Source:      sourceName,
			})
		}
		log.Printf("[News Parser] Успешно загружено %d новостей от: %s\n", limit, sourceName)
	}

	// Сортируем все собранные новости по дате (самые свежие сверху)
	sort.Slice(allNews, func(i, j int) bool {
		return allNews[i].PublishedAt.After(allNews[j].PublishedAt)
	})

	// Сериализуем в JSON для сохранения в БД
	data, err := json.Marshal(allNews)
	if err != nil {
		return err
	}

	// Сохраняем в BoltDB
	return m.db.Update(func(tx *bbolt.Tx) error {
		// Создаем бакет (таблицу), если её нет
		b, err := tx.CreateBucketIfNotExists([]byte("DashboardCache"))
		if err != nil {
			return err
		}
		// Записываем JSON по ключу "official_news"
		err = b.Put([]byte("official_news"), data)
		if err == nil {
			log.Println("[News Parser] Новости успешно обновлены в локальном кеше (BoltDB)")
		}
		return err
	})
}

func stripHTMLTags(content string) string {
	p := bluemonday.UGCPolicy()
	return p.Sanitize(content)
}
