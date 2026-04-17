package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

type OllamaService struct {
	client  *resty.Client
	baseURL string
	model   string
}

func NewOllamaService(url, model string) *OllamaService {
	return &OllamaService{
		client:  resty.New(),
		baseURL: url,
		model:   model,
	}
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message message `json:"message"`
}

// System Prompt задает поведение аналитика
const systemPrompt = `
Ты - профессиональный бизнес-аналитик данных.
Твоя задача: анализировать предоставленные данные в формате JSON и отвечать на вопросы пользователя.
Правила:
1. Если данных много, анализируй тренды, аномалии и общую статистику.
2. Используй Markdown для форматирования (таблицы, жирный шрифт).
3. Будь краток и точен.
4. Если пользователь просит код для Python, генерируй только код без лишних объяснений.
`

func (s *OllamaService) AnalyzeData(ctx context.Context, question string, dataPreview interface{}) (string, error) {
	dataBytes, _ := json.Marshal(dataPreview)

	userPrompt := fmt.Sprintf("Данные (JSON): %s\n\nВопрос пользователя: %s", string(dataBytes), question)

	payload := chatRequest{
		Model:  s.model,
		Stream: false,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	resp, err := s.client.R().
		SetContext(ctx).
		SetBody(payload).
		SetResult(&chatResponse{}).
		Post(s.baseURL + "/api/chat")

	if err != nil {
		return "", err
	}

	if resp.IsError() {
		return "", fmt.Errorf("ollama error: %s", resp.String())
	}

	result := resp.Result().(*chatResponse)
	return result.Message.Content, nil
}
