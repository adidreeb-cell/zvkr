package gigachat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/evgensoft/gigachat"
)

type GigaChatService struct {
	clientID     string
	clientSecret string
}

func NewGigaChatService(clientID, clientSecret string) *GigaChatService {
	return &GigaChatService{
		clientID:     clientID,
		clientSecret: clientSecret,
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

func (s *GigaChatService) AnalyzeData(ctx context.Context, question string, dataPreview interface{}) (string, error) {
	var contextContent string

	// 1. Безопасный парсинг данных
	if dataPreview != nil {
		dataBytes, err := json.Marshal(dataPreview)
		if err != nil {
			return "", fmt.Errorf("failed to marshal dataPreview: %w", err)
		}
		contextContent = fmt.Sprintf("Данные (JSON контекст): %s\n\n", string(dataBytes))
	}

	userPrompt := contextContent + "Вопрос пользователя: " + question

	// 2. Инициализация клиента GigaChat
	client := gigachat.NewClient(s.clientID, s.clientSecret)

	// 3. Формируем запрос
	req := &gigachat.ChatRequest{
		Model: gigachat.ModelGigaChat,
		Messages: []gigachat.Message{
			{
				Role:    gigachat.RoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    gigachat.RoleUser,
				Content: userPrompt,
			},
		},
	}

	// 4. Отправка запроса
	resp, err := client.Chat(req)
	if err != nil {
		return "", fmt.Errorf("gigachat API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from GigaChat")
	}

	content := resp.Choices[0].Message.Content

	// Логируем использование токенов
	log.Printf("[GigaChat] Success. Tokens used: %d (Prompt: %d, Completion: %d)",
		resp.Usage.TotalTokens, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	return content, nil
}
