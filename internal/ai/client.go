package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type YandexGPTClient struct {
	apiKey     string
	folderID   string
	modelURI   string
	baseURL    string
	httpClient *http.Client
}

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func NewYandexGPTClient() (*YandexGPTClient, error) {
	apiKey := os.Getenv("YANDEX_GPT_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("YANDEX_GPT_API_KEY не установлен")
	}

	folderID := os.Getenv("YANDEX_FOLDER_ID")
	if folderID == "" {
		return nil, fmt.Errorf("YANDEX_FOLDER_ID не установлен")
	}

	modelURI := fmt.Sprintf("gpt://%s/yandexgpt-lite", folderID)

	return &YandexGPTClient{
		apiKey:   apiKey,
		folderID: folderID,
		modelURI: modelURI,
		baseURL:  "https://llm.api.cloud.yandex.net/v1/chat/completions",
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

func (c *YandexGPTClient) GeneratePost(ctx context.Context, keywords string, article ArticleInfo) (string, error) {
	log.Printf("[AI] Начало генерации поста по теме: %s", keywords)

	prompt := fmt.Sprintf(`
СОЗДАЙ КАЧЕСТВЕННЫЙ TELEGRAM ПОСТ

ТЕМА: %s

ИНФОРМАЦИЯ О НОВОСТИ:
Заголовок: %s
Описание: %s
Источник: %s

ТРЕБОВАНИЯ К ПОСТУ:
1. Напиши привлекательный заголовок
2. Основной текст должен быть интересным и информативным
3. Используй эмодзи для выразительности
4. В конце укажи источник новости
5. Сделай текст естественным и читабельным

ПОСТ ДОЛЖЕН БЫТЬ ГОТОВ К ПУБЛИКАЦИИ В TELEGRAM.

Верни ТОЛЬКО готовый пост без дополнительных комментариев.
`,
		strings.TrimSpace(keywords),
		strings.TrimSpace(article.Title),
		strings.TrimSpace(article.Summary),
		strings.TrimSpace(article.Source),
	)

	request := ChatCompletionRequest{
		Model: c.modelURI,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.7,
		MaxTokens:   2000,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		log.Printf("[AI] Ошибка маршалинга запроса: %v", err)
		return "", fmt.Errorf("ошибка маршалинга: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("[AI] Ошибка создания запроса: %v", err)
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Api-Key %s", c.apiKey))
	req.Header.Set("OpenAI-Project", c.folderID)

	log.Printf("[AI] Отправка запроса к YandexGPT...")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[AI] Ошибка HTTP запроса: %v", err)
		return "", fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[AI] Ошибка API: статус %d, тело: %s", resp.StatusCode, string(body))
		return "", fmt.Errorf("ошибка API: статус %d", resp.StatusCode)
	}

	var chatResponse ChatCompletionResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[AI] Ошибка чтения ответа: %v", err)
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if err := json.Unmarshal(body, &chatResponse); err != nil {
		log.Printf("[AI] Ошибка парсинга: %v", err)
		return "", fmt.Errorf("ошибка парсинга: %w", err)
	}

	if len(chatResponse.Choices) == 0 {
		log.Printf("[AI] Пустой ответ от GPT")
		return "", fmt.Errorf("пустой ответ от GPT")
	}

	post := strings.TrimSpace(chatResponse.Choices[0].Message.Content)
	log.Printf("[AI] Успешная генерация поста (длина: %d символов)", len(post))

	return post, nil
}
