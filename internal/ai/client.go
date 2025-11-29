package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// YandexGPTClient представляет клиент для работы с Yandex GPT API
type YandexGPTClient struct {
	apiKey     string
	folderID   string
	modelURI   string
	baseURL    string
	httpClient *http.Client
}

// ChatCompletionRequest представляет структуру запроса для chat/completions
type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// Message представляет одно сообщение в диалоге
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse представляет структуру ответа от chat/completions
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

// PostGenerationRequest запрос на генерацию поста
type PostGenerationRequest struct {
	ChannelAnalysis *ChannelAnalysis `json:"channel_analysis"`
	Article         ArticleRelevance `json:"article"`
}

// NewYandexGPTClient создает нового клиента для Yandex GPT
func NewYandexGPTClient() (*YandexGPTClient, error) {
	apiKey := os.Getenv("YANDEX_GPT_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("YANDEX_GPT_API_KEY не установлен в .env файле")
	}

	folderID := os.Getenv("YANDEX_FOLDER_ID")
	if folderID == "" {
		return nil, fmt.Errorf("YANDEX_FOLDER_ID не установлен в .env файле")
	}

	// Используем только проверенную модель rc
	modelURI := fmt.Sprintf("gpt://%s/yandexgpt-lite/rc", folderID)

	fmt.Printf("🔧 Настройка YandexGPT:\n")
	fmt.Printf("   Folder ID: %s\n", folderID)
	fmt.Printf("   Model: yandexgpt-lite/rc\n")
	fmt.Printf("   API Key: %s...\n", apiKey[:min(8, len(apiKey))])

	return &YandexGPTClient{
		apiKey:   apiKey,
		folderID: folderID,
		modelURI: modelURI,
		baseURL:  "https://llm.api.cloud.yandex.net/v1/chat/completions",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// TestConnection проверяет соединение с Yandex GPT API
func (c *YandexGPTClient) TestConnection(ctx context.Context) error {
	fmt.Println("🧪 Тестируем подключение к YandexGPT...")
	response, err := c.AnalyzeText(ctx, "Ответь одним словом: 'работает'")
	if err != nil {
		fmt.Printf("❌ Ошибка подключения к YandexGPT: %v\n", err)
		return fmt.Errorf("ошибка подключения к YandexGPT: %w", err)
	}

	fmt.Printf("✅ Тест соединения: YandexGPT ответил '%s'\n", response)
	return nil
}

// AnalyzeText отправляет текст на анализ в Yandex GPT и возвращает ответ
func (c *YandexGPTClient) AnalyzeText(ctx context.Context, prompt string) (string, error) {
	fmt.Printf("🔧 Используем модель: %s\n", c.modelURI)

	request := ChatCompletionRequest{
		Model: c.modelURI,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.3,
		MaxTokens:   2000,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("ошибка маршалинга запроса: %w", err)
	}

	fmt.Printf("🔧 ДЕТАЛИ ЗАПРОСА:\n")
	fmt.Printf("   URL: %s\n", c.baseURL)
	fmt.Printf("   Model: %s\n", c.modelURI)
	fmt.Printf("   Folder ID: %s\n", c.folderID)
	fmt.Printf("   API Key: %s...\n", c.apiKey[:min(8, len(c.apiKey))])
	fmt.Printf("   Длина промпта: %d символов\n", len(prompt))

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Api-Key %s", c.apiKey))
	req.Header.Set("OpenAI-Project", c.folderID)

	// Выполняем запрос
	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Printf("❌ ОШИБКА HTTP ЗАПРОСА: %v\n", err)
		return "", fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("🔧 ОТВЕТ ОТ API:\n")
	fmt.Printf("   Статус код: %d\n", resp.StatusCode)
	fmt.Printf("   Статус: %s\n", resp.Status)

	// Читаем тело ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ ОШИБКА ЧТЕНИЯ ТЕЛА ОТВЕТА: %v\n", err)
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ ОШИБКА API:\n")
		fmt.Printf("   Код: %d\n", resp.StatusCode)
		fmt.Printf("   Сообщение: %s\n", resp.Status)
		fmt.Printf("   Тело ошибки: %s\n", string(body))

		// Парсим ошибку если это JSON
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			if message, exists := errorResp["message"]; exists {
				fmt.Printf("   Сообщение ошибки: %v\n", message)
			}
			if code, exists := errorResp["code"]; exists {
				fmt.Printf("   Код ошибки: %v\n", code)
			}
		}

		return "", fmt.Errorf("ошибка API: статус %d", resp.StatusCode)
	}

	var chatResponse ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResponse); err != nil {
		fmt.Printf("❌ ОШИБКА ПАРСИНГА JSON ОТВЕТА: %v\n", err)
		return "", fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	if len(chatResponse.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ от GPT")
	}

	responseText := chatResponse.Choices[0].Message.Content
	fmt.Printf("✅ УСПЕШНЫЙ ОТВЕТ:\n")
	fmt.Printf("   Ответ: %s\n", responseText)
	fmt.Printf("   Использовано токенов: %d\n", chatResponse.Usage.TotalTokens)

	return responseText, nil
}

// AnalyzeChannel проводит анализ Telegram канала через YandexGPT
func (c *YandexGPTClient) AnalyzeChannel(ctx context.Context, channelName, description string, messages []string) (string, error) {
	prompt := fmt.Sprintf(`
Проанализируй Telegram канал и верни ответ в формате JSON.

Информация о канале:
- Название: %s
- Описание: %s

Последние сообщения из канала:
%s

Проанализируй и верни JSON:
{
  "main_topic": "основная тема",
  "subtopics": ["подтема1", "подтема2", "подтема3"],
  "target_audience": "описание аудитории", 
  "content_style": "формальный/неформальный",
  "keywords": ["keyword1", "keyword2", "keyword3", "keyword4", "keyword5"],
  "content_angle": "рекомендуемый угол подачи"
}

Верни ТОЛЬКО JSON без дополнительных текстов.
`, channelName, description, formatMessages(messages))

	return c.AnalyzeText(ctx, prompt)
}

// SelectRelevantNews выбирает самые релевантные новости для канала через AI
func (c *YandexGPTClient) SelectRelevantNews(ctx context.Context, channelAnalysis *ChannelAnalysis, articles []ArticleRelevance, maxResults int) ([]NewsRelevance, error) {
	if len(articles) == 0 {
		return []NewsRelevance{}, nil
	}

	// Ограничиваем количество статей для анализа
	if len(articles) > 20 {
		articles = articles[:20]
	}

	prompt := fmt.Sprintf(`
ВЫБОР РЕЛЕВАНТНЫХ НОВОСТЕЙ ДЛЯ TELEGRAM КАНАЛА

ИНФОРМАЦИЯ О КАНАЛЕ:
- Основная тема: %s
- Подтемы: %s
- Целевая аудитория: %s
- Стиль контента: %s
- Ключевые слова: %s
- Рекомендуемый угол подачи: %s

СПИСОК НОВОСТЕЙ ДЛЯ ОЦЕНКИ:
%s

ПРОАНАЛИЗИРУЙ и верни ТОП-%d самых релевантных новостей в формате JSON:

{
  "selected_news": [
    {
      "article": {
        "title": "заголовок новости",
        "summary": "описание новости", 
        "url": "ссылка на новость"
      },
      "relevance": 0.95,
      "explanation": "подробное объяснение релевантности",
      "match_reasons": ["причина 1", "причина 2", "причина 3"]
    }
  ]
}

КРИТЕРИИ ОЦЕНКИ:
1. Соответствие основной теме и подтемам канала
2. Интерес для целевой аудитории  
3. Возможность подачи под рекомендуемым углом
4. Актуальность и информационная ценность

Верни ТОЛЬКО JSON без дополнительных текстов.
`,
		channelAnalysis.MainTopic,
		strings.Join(channelAnalysis.Subtopics, ", "),
		channelAnalysis.TargetAudience,
		channelAnalysis.ContentStyle,
		strings.Join(channelAnalysis.Keywords, ", "),
		channelAnalysis.ContentAngle,
		formatArticlesForPrompt(articles),
		maxResults,
	)

	response, err := c.AnalyzeText(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("ошибка AI-подбора новостей: %w", err)
	}

	var selectionResponse NewsSelectionResponse
	if err := json.Unmarshal([]byte(response), &selectionResponse); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа AI: %w", err)
	}

	return selectionResponse.SelectedNews, nil
}

// formatMessages форматирует сообщения для промпта
func formatMessages(messages []string) string {
	var result string
	for i, msg := range messages {
		if i >= 15 {
			break
		}
		if len(msg) > 10 {
			result += fmt.Sprintf("%d. %s\n", i+1, msg)
		}
	}
	return result
}

// formatArticlesForPrompt форматирует статьи для промпта
func formatArticlesForPrompt(articles []ArticleRelevance) string {
	var result strings.Builder
	for i, article := range articles {
		result.WriteString(fmt.Sprintf("%d. ЗАГОЛОВОК: %s\n   ОПИСАНИЕ: %s\n   ССЫЛКА: %s\n\n",
			i+1, article.Title, article.Summary, article.URL))
	}
	return result.String()
}

// GeneratePost генерирует полноценный Telegram пост
func (c *YandexGPTClient) GeneratePost(ctx context.Context, analysis *ChannelAnalysis, article ArticleRelevance) (string, error) {
	prompt := fmt.Sprintf(`
СГЕНЕРИРУЙ КАЧЕСТВЕННЫЙ TELEGRAM ПОСТ ДЛЯ КАНАЛА

ИНФОРМАЦИЯ О КАНАЛЕ:
- Основная тема: %s
- Подтемы: %s  
- Целевая аудитория: %s
- Стиль контента: %s
- Рекомендуемый угол подачи: %s

НОВОСТЬ ДЛЯ ПОСТА:
- Заголовок: %s
- Описание: %s
- Ссылка: %s

ТРЕБОВАНИЯ К ПОСТУ:
1. **Заголовок**: привлекательный, основан на новости, но может быть творчески переработан
2. **Основной текст**: 
   - Полностью соответствует стилю и формату канала
   - Используй жирный шрифт (**жирный**) для выделения ключевых моментов
   - Можешь намеренно скрыть некоторые детали чтобы подогреть интерес (но не злоупотребляй этим)
   - Текст должен быть естественным и вовлекающим
3. **В конце**: ссылка на источник новости

ВАЖНО: Пост должен выглядеть так, как будто его написал автор канала!

Верни ТОЛЬКО готовый пост без дополнительных комментариев.
`,
		analysis.MainTopic,
		strings.Join(analysis.Subtopics, ", "),
		analysis.TargetAudience,
		analysis.ContentStyle,
		analysis.ContentAngle,
		article.Title,
		article.Summary,
		article.URL,
	)

	return c.AnalyzeText(ctx, prompt)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
