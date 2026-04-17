package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/sashabaranov/go-openai"
)

// Кастомный HTTP-транспорт для добавления специфичных заголовков OpenRouter
type openRouterTransport struct {
	Base        http.RoundTripper
	HTTPReferer string
	XTitle      string
}

func (t *openRouterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.HTTPReferer != "" {
		req.Header.Set("HTTP-Referer", t.HTTPReferer)
	}
	if t.XTitle != "" {
		req.Header.Set("X-Title", t.XTitle)
	}
	// Важно для OpenRouter
	req.Header.Set("Content-Type", "application/json")
	return t.Base.RoundTrip(req)
}

type OpenRouterService struct {
	client *openai.Client
	model  string
}

func NewOpenRouterService(apiKey, model, appName, httpReferer string) *OpenRouterService {
	if model == "" {
		model = "google/gemini-2.0-flash-lite-preview-02-05:free"
	}

	// Настраиваем конфигурацию OpenAI для работы с OpenRouter
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = "https://openrouter.ai/api/v1"

	// Подключаем наш кастомный транспорт для заголовков
	baseTransport := http.DefaultTransport
	config.HTTPClient = &http.Client{
		Transport: &openRouterTransport{
			Base:        baseTransport,
			HTTPReferer: httpReferer,
			XTitle:      appName,
		},
	}

	return &OpenRouterService{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

// System Prompt
const systemPrompt = `
Ты - профессиональный бизнес-аналитик данных и эксперт по Python.
Твоя задача: анализировать предоставленные данные в формате JSON.
Правила:
1. Если данных много, анализируй тренды и статистику.
2. Используй Markdown.
3. Если пользователь просит код, пиши только код.
4. Будь точен в типах данных (числа в JSON могут быть строками).
`

func (s *OpenRouterService) AnalyzeData(ctx context.Context, question string, dataPreview interface{}) (string, error) {
	var contextContent string

	// 1. Безопасный парсинг данных (с обработкой ошибок)
	if dataPreview != nil {
		dataBytes, err := json.Marshal(dataPreview)
		if err != nil {
			return "", fmt.Errorf("failed to marshal dataPreview: %w", err)
		}
		contextContent = fmt.Sprintf("Данные (JSON контекст): %s\n\n", string(dataBytes))
	}

	userPrompt := contextContent + "Вопрос пользователя: " + question

	// 2. Формируем запрос используя структуры библиотеки
	req := openai.ChatCompletionRequest{
		Model: s.model,
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
		Temperature: 0.1, // Низкая температура для точности
	}

	// 3. Отправляем запрос
	resp, err := s.client.CreateChatCompletion(ctx, req)
	if err != nil {
		// Библиотека сама распарсит сообщение об ошибке от OpenRouter
		return "", fmt.Errorf("openrouter API error: %w", err)
	}

	// 4. Проверяем наличие ответа
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	content := resp.Choices[0].Message.Content

	// Логируем успешный ответ и количество потраченных токенов (библиотека парсит Usage автоматически)
	log.Printf("[OpenRouter] Success. Tokens used: %d (Prompt: %d, Completion: %d)",
		resp.Usage.TotalTokens, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	return content, nil
}
