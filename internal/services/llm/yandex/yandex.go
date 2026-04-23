package yandex

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/sashabaranov/go-openai"
)

// customTransport позволяет добавлять кастомные HTTP-заголовки во все запросы
type customTransport struct {
	Transport http.RoundTripper
	headers   map[string]string
}

// RoundTrip перехватывает запрос и добавляет в него наши заголовки
func (c *customTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Клонируем запрос, чтобы безопасно менять заголовки
	reqClone := req.Clone(req.Context())
	for k, v := range c.headers {
		reqClone.Header.Set(k, v)
	}
	return c.Transport.RoundTrip(reqClone)
}

type YandexService struct {
	client   *openai.Client
	folderID string
	model    string
}

// NewYandexService инициализирует клиента YandexGPT через OpenAI-совместимый API
func NewYandexService(apiKey, folderID, model string) *YandexService {
	// Создаем базовый конфиг
	config := openai.DefaultConfig(apiKey)

	// Указываем базовый URL Yandex Cloud
	config.BaseURL = "https://ai.api.cloud.yandex.net/v1"

	// Переопределяем HTTP-клиент, чтобы прокидывать Folder ID (аналог project в Python)
	config.HTTPClient = &http.Client{
		Transport: &customTransport{
			Transport: http.DefaultTransport,
			headers: map[string]string{
				"x-folder-id": folderID,
			},
		},
	}

	return &YandexService{
		client:   openai.NewClientWithConfig(config),
		folderID: folderID,
		model:    model, // например: "yandexgpt-5-lite/latest"
	}
}

const systemPrompt = `
Ты - профессиональный бизнес-аналитик данных и эксперт по Python.
Твоя задача: анализировать предоставленные данные в формате JSON.
Правила:
1. Если данных много, анализируй тренды и статистику.
2. Используй Markdown.
3. Если пользователь просит код, пиши только код.
4. Будь точен в типах данных (числа в JSON могут быть строками).
`

// AnalyzeData отправляет данные и вопрос в YandexGPT
func (s *YandexService) AnalyzeData(ctx context.Context, question string, dataPreview interface{}) (string, error) {
	var contextContent string

	// 1. Безопасный парсинг данных
	if dataPreview != nil {
		dataBytes, err := json.MarshalIndent(dataPreview, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal dataPreview: %w", err)
		}
		contextContent = fmt.Sprintf("Данные (JSON контекст):\n%s\n\n", string(dataBytes))
	}

	userPrompt := contextContent + "Вопрос пользователя: " + question

	// 2. Формируем URI модели по правилам Яндекса: gpt://<folder_id>/<model>
	modelURI := fmt.Sprintf("gpt://%s/%s", s.folderID, s.model)

	// 3. Формируем запрос
	req := openai.ChatCompletionRequest{
		Model:       modelURI,
		Temperature: 0.3,
		MaxTokens:   500,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userPrompt,
			},
		},
	}

	// 4. Отправка запроса
	resp, err := s.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("yandexgpt API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("received empty response from yandexgpt")
	}

	log.Printf("[YandexGPT] Success. Response received. Tokens used: %d", resp.Usage.TotalTokens)

	return resp.Choices[0].Message.Content, nil
}
