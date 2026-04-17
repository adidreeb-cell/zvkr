package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"strings"
	"time"

	"exeldoctor/internal/models"
	"exeldoctor/internal/services/excel"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"gorm.io/gorm"
)

type IMAPService struct {
	DB    *gorm.DB
	Excel *excel.Service
}

func NewIMAPService(db *gorm.DB, xl *excel.Service) *IMAPService {
	return &IMAPService{DB: db, Excel: xl}
}

func (s *IMAPService) FetchUnreadExcel() {
	var cfg models.SystemSetting
	s.DB.First(&cfg, 1)

	if !cfg.EnableIMAP || cfg.IMAPHost == "" {
		return
	}

	log.Println("[IMAP] Подключение к почте...")
	c, err := client.DialTLS(fmt.Sprintf("%s:%d", cfg.IMAPHost, cfg.IMAPPort), nil)
	if err != nil {
		log.Printf("[IMAP Error] Ошибка подключения: %v", err)
		return
	}
	defer c.Logout()

	if err := c.Login(cfg.IMAPUsername, cfg.IMAPPassword); err != nil {
		log.Printf("[IMAP Error] Ошибка логина: %v", err)
		return
	}

	// Обязательно проверяем ошибку тут
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		log.Printf("[IMAP Error] Ошибка выбора папки INBOX: %v", err)
		return
	}
	if mbox.Messages == 0 {
		return
	}

	// Ищем непрочитанные сообщения
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}
	seqNums, err := c.Search(criteria)
	if err != nil {
		log.Printf("[IMAP Error] Ошибка поиска писем: %v", err)
		return
	}
	if len(seqNums) == 0 {
		return
	}

	log.Printf("[IMAP] Найдено непрочитанных писем: %d", len(seqNums))

	seqset := new(imap.SeqSet)
	seqset.AddNum(seqNums...)
	messages := make(chan *imap.Message, 10)

	// ИСПРАВЛЕНИЕ 1: Запрашиваем правильную секцию письма (всё тело целиком)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{section.FetchItem()}

	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, items, messages)
	}()

	for msg := range messages {
		s.processMessage(msg, section) // передаем section сюда
	}

	if err := <-done; err != nil {
		log.Printf("[IMAP Error] Ошибка при загрузке писем: %v", err)
	}

	// ИСПРАВЛЕНИЕ 3: Помечаем письма как прочитанные, чтобы не парсить их снова
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.SeenFlag}
	if err := c.Store(seqset, item, flags, nil); err != nil {
		log.Printf("[IMAP Error] Ошибка при пометке писем как прочитанных: %v", err)
	}

	log.Println("[IMAP] Проверка почты завершена.")
}

func (s *IMAPService) processMessage(msg *imap.Message, section *imap.BodySectionName) {
	r := msg.GetBody(section)
	if r == nil {
		log.Printf("[IMAP Warn] Не удалось получить тело письма (ID: %d)", msg.SeqNum)
		return
	}

	mr, err := mail.CreateReader(r)
	if err != nil {
		log.Printf("[IMAP Error] Ошибка чтения письма: %v", err)
		return
	}

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			continue
		}

		var filename string

		// 1. Ищем имя файла в Content-Disposition (стандарт для вложений)
		cd := p.Header.Get("Content-Disposition")
		_, cdParams, _ := mime.ParseMediaType(cd)
		if name := cdParams["filename"]; name != "" {
			filename = name
		} else {
			// 2. Иногда файлы шлют без Disposition, ищем в Content-Type
			ct := p.Header.Get("Content-Type")
			_, ctParams, _ := mime.ParseMediaType(ct)
			if name := ctParams["name"]; name != "" {
				filename = name
			}
		}

		// 3. Если файл найден, декодируем его имя (если оно =?UTF-8?Q?...)
		if filename != "" {
			dec := new(mime.WordDecoder)
			if decoded, err := dec.DecodeHeader(filename); err == nil {
				filename = decoded
			}
		}

		if filename == "" {
			continue // Это не файл
		}

		filenameLow := strings.ToLower(filename)
		if strings.HasSuffix(filenameLow, ".xlsx") || strings.HasSuffix(filenameLow, ".xls") || strings.HasSuffix(filenameLow, ".csv") {

			log.Printf("[IMAP] Найден Excel файл: %s. Начинаю обработку...", filename)

			buf := new(bytes.Buffer)
			_, err := buf.ReadFrom(p.Body)
			if err != nil {
				log.Printf("[IMAP Error] Ошибка чтения файла %s: %v", filename, err)
				continue
			}

			headers, data, err := s.Excel.Parse(buf, "email_dataset")
			if err == nil {
				hJSON, _ := json.Marshal(headers)
				dJSON, _ := json.Marshal(data)

				dataset := models.Dataset{
					Name:      fmt.Sprintf("Email: %s (%s)", filename, time.Now().Format("02.01 15:04")),
					Source:    "email",
					Headers:   hJSON,
					Data:      dJSON,
					CreatedAt: time.Now(),
				}
				if err := s.DB.Create(&dataset).Error; err != nil {
					log.Printf("[IMAP Error] Ошибка сохранения датасета в БД: %v", err)
				} else {
					log.Printf("[IMAP] Успешно сохранен файл из почты: %s", filename)
				}
			} else {
				log.Printf("[IMAP Error] Ошибка парсинга Excel (%s): %v", filename, err)
			}
		}
	}
}
