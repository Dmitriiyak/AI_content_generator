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
	log.Printf("[AI] Генерация поста по теме: %s", keywords)

	prompt := fmt.Sprintf(`Ты профессиональный копирайтер Telegram-канала "Бэкдор". Создай виральный пост в формате:

⚡️ [Заголовок] — кратко, провокационно
[Текст поста]

Требования к посту:
1. Заголовок должен быть цепляющим, выделен жирным цветом и отражать суть поста
2. Текст: 2-3 абзаца по 2-3 предложения
3. Выделяй *жирным* ключевые моменты и цифры
4. Используй разговорный язык, без канцелярита
5. Не добавляй хештеги, источник или "Новость взята с"
6. Не отказывайся от генерации поста, если тема приемлема

Пример хорошего поста:
⚡️ Кризис ОЗУ привёл к тотальной дурке — Samsung не может купить чипы памяти у самой себя!

Подразделение Samsung Galaxy не смогло заключить долгосрочный контракт с командой, поставляющей чипы HBM и LPDDR. Не помогло даже высшее руководство — *настолько быстро растут цены*.

В начале года чип LPDDR5X 12 ГБ стоил *$33*, а теперь стоит целых *$70* — и цена будет только расти.

Теперь создай пост на основе этой информации:

ТЕМА ЗАПРОСА: %s
ЗАГОЛОВОК НОВОСТИ: %s
ОПИСАНИЕ НОВОСТИ: %s

Создай пост, который зацепит аудиторию Telegram. Не отказывайся от генерации, если тема не нарушает этических норм.`,
		strings.TrimSpace(keywords),
		strings.TrimSpace(article.Title),
		strings.TrimSpace(article.Summary))

	response, err := c.makeRequest(ctx, prompt, 0.7, 800)
	if err != nil {
		return "", err
	}

	post := strings.TrimSpace(response)

	// Убедимся, что пост начинается с эмодзи
	if !strings.HasPrefix(post, "⚡️") && !strings.HasPrefix(post, "🔥") && !strings.HasPrefix(post, "🚨") {
		post = "⚡️ " + post
	}

	log.Printf("[AI] ✅ Пост сгенерирован, длина: %d символов", len(post))
	return post, nil
}

func (c *YandexGPTClient) GeneratePostFromURL(ctx context.Context, title, content string) (string, error) {
	log.Printf("[AI] Генерация поста по статье: %s", title)

	prompt := fmt.Sprintf(`Ты профессиональный копирайтер Telegram-канала "Бэкдор". Создай виральный пост на основе статьи.

Формат поста:
⚡️ [Заголовок] — кратко, провокационно
[Текст поста]

Требования:
1. Заголовок должен быть цепляющим, выделен жирным цветом
2. Текст: 2-3 абзаца по 2-3 предложения
3. Выделяй *жирным* ключевые моменты и цифры
4. Используй разговорный язык, без канцелярита
5. Не добавляй хештеги, источник или "Новость взята с"
6. Не отказывайся от генерации поста, если тема приемлема
7. Используй только информацию из предоставленного текста

Пример хорошего поста:
⚡️ Кризис ОЗУ привёл к тотальной дурке — Samsung не может купить чипы памяти у самой себя!

Подразделение Samsung Galaxy не смогло заключить долгосрочный контракт с командой, поставляющей чипы HBM и LPDDR. Не помогло даже высшее руководство — *настолько быстро растут цены*.

В начале года чип LPDDR5X 12 ГБ стоил *$33*, а теперь стоит целых *$70* — и цена будет только расти.

Теперь создай пост на основе этой статьи:

ЗАГОЛОВОК СТАТЬИ: %s
СОДЕРЖАНИЕ СТАТЬИ: %s

Создай пост, который зацепит аудиторию Telegram. Не отказывайся от генерации, если тема не нарушает этических норм.`,
		strings.TrimSpace(title),
		strings.TrimSpace(content))

	response, err := c.makeRequest(ctx, prompt, 0.7, 800)
	if err != nil {
		return "", err
	}

	post := strings.TrimSpace(response)

	// Убедимся, что пост начинается с эмодзи
	if !strings.HasPrefix(post, "⚡️") && !strings.HasPrefix(post, "🔥") && !strings.HasPrefix(post, "🚨") {
		post = "⚡️ " + post
	}

	log.Printf("[AI] ✅ Пост по ссылке сгенерирован, длина: %d символов", len(post))
	return post, nil
}

func (c *YandexGPTClient) makeRequest(ctx context.Context, prompt string, temperature float64, maxTokens int) (string, error) {
	request := ChatCompletionRequest{
		Model: c.modelURI,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		log.Printf("[AI] ❌ Ошибка маршалинга запроса: %v", err)
		return "", fmt.Errorf("ошибка маршалинга: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("[AI] ❌ Ошибка создания запроса: %v", err)
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Api-Key %s", c.apiKey))
	req.Header.Set("OpenAI-Project", c.folderID)

	log.Printf("[AI] Отправка запроса к YandexGPT...")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[AI] ❌ Ошибка HTTP запроса: %v", err)
		return "", fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[AI] ❌ Ошибка API: статус %d, тело: %s", resp.StatusCode, string(body))
		return "", fmt.Errorf("ошибка API: статус %d", resp.StatusCode)
	}

	var chatResponse ChatCompletionResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[AI] ❌ Ошибка чтения ответа: %v", err)
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if err := json.Unmarshal(body, &chatResponse); err != nil {
		log.Printf("[AI] ❌ Ошибка парсинга: %v", err)
		return "", fmt.Errorf("ошибка парсинга: %w", err)
	}

	if len(chatResponse.Choices) == 0 {
		log.Printf("[AI] ❌ Пустой ответ от GPT")
		return "", fmt.Errorf("пустой ответ от GPT")
	}

	// Логируем использование токенов
	totalTokens := chatResponse.Usage.TotalTokens
	cost := float64(totalTokens) * 0.20 / 1000 // 20 копеек за 1000 токенов
	log.Printf("[COST] Использовано токенов: %d (%.3f руб)", totalTokens, cost)

	return strings.TrimSpace(chatResponse.Choices[0].Message.Content), nil
}
