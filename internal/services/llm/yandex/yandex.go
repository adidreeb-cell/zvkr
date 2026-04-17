package yandex

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/neuron-nexus/yandexgpt/v2"
)

type YandexService struct {
	apiKey   string
	folderID string
	model    string
}

func NewYandexService(apiKey, folderID, model string) *YandexService {
	return &YandexService{
		apiKey:   apiKey,
		folderID: folderID,
		model:    model,
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

type GPTModel struct {
	ModelName string
}

func (s *YandexService) AnalyzeData(ctx context.Context, question string, dataPreview interface{}) (string, error) {
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

	modelS := GPTModel{
		ModelName: s.model,
	}

	// 2. Инициализация приложения YandexGPT
	app := yandexgpt.NewYandexGPTSyncApp(
		s.apiKey,
		yandexgpt.API_KEY,
		s.folderID,
		yandexgpt.GPTModel(modelS),
	)

	// Настройка параметров (аналог Temperature в OpenRouter)
	app.Configure([]yandexgpt.GPTParameter{
		{Name: yandexgpt.ParameterTemperature, Value: "0.1"},
	}...)

	// 3. Формируем сообщения
	app.AddMessage(yandexgpt.GPTMessage{
		Role: yandexgpt.RoleAssistant,
		Text: systemPrompt,
	})
	app.AddMessage(yandexgpt.GPTMessage{
		Role: yandexgpt.RoleUser,
		Text: userPrompt,
	})

	// 4. Отправка запроса
	res, err := app.SendRequest()
	if err != nil {
		return "", fmt.Errorf("yandexgpt API error: %w", err)
	}

	log.Printf("[YandexGPT] Success. Response received.")
	return res.Text, nil
}
