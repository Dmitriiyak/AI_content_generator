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

// GeneratePost генерирует пост в стиле примеров
func (c *YandexGPTClient) GeneratePost(ctx context.Context, keywords string, article ArticleInfo) (string, error) {
	log.Printf("[AI] Генерация поста по теме: %s", keywords)

	prompt := fmt.Sprintf(`
ТЫ: Копирайтер Telegram-канала "Бэкдор". Ты пишешь виральные, цепляющие посты в формате:

⚡️ ЗАГОЛОВОК — кратко, провокационно, с эмодзи в начале
ТЕЛО ПОСТА (2-3 абзаца):
- Первый абзац: суть новости, самые важные детали
- Второй абзац: последствия, цифры, факты
- Третий абзац (опционально): контекст или вывод
В конце: хештеги

СТИЛЬ:
- Используй **жирный** для ключевых моментов и цифр
- Пиши короткими предложениями
- Будь провокационным, но фактологичным
- Добавляй эмодзи для выразительности
- Используй разговорный язык, но без сленга
- Длина: 150-250 слов

ИНФОРМАЦИЯ ДЛЯ ПОСТА:
ТЕМА: %s
ЗАГОЛОВОК ИСТОЧНИКА: %s
ОПИСАНИЕ: %s

ПРИМЕРЫ ХОРОШИХ ПОСТОВ:

⚡️ Кризис ОЗУ привёл к тотальной дурке — Samsung не может купить чипы памяти у самой себя!

Подразделение Samsung Galaxy не смогло заключить долгосрочный контракт с командой, поставляющей чипы HBM и LPDDR. Не помогло даже высшее руководство — настолько быстро растут цены. 

В начале года чип LPDDR5X 12 ГБ стоил $33, а теперь стоит целых $70 — и цена будет только расти. 

Ценник на Galaxy S26 на фоне кризиса может внезапно взлететь до небес — как и стоимость многих других смартфонов.

Спасибо ИИ-пузырю

⚡️ Ozon будет ПРИНУДИТЕЛЬНО списывать чаевые — инфу нашли в руководстве маркетплейса. Если вы хотя бы ОДИН РАЗ оставите чаевые, их будут списывать КАЖДЫЙ раз при посещении ПВЗ.

Отказаться от чаевых можно в течение 15 минут. Если бабки списали — ВСЁ, вернуть их не получится.

Следим за кошельками.

ТВОЯ ЗАДАЧА:
Создать пост в таком же стиле на основе предоставленной информации.

ПРАВИЛА:
1. Начни с эмодзи ⚡️ и короткого провокационного заголовка
2. 2-3 абзаца текста, выдели **жирным** ключевые моменты
3. В конце добавь хештеги как в примерах
4. Не добавляй "Источник:" или подобные фразы в текст поста

ВЕРНИ ТОЛЬКО ГОТОВЫЙ ПОСТ, БЕЗ ДОПОЛНИТЕЛЬНЫХ КОММЕНТАРИЕВ.
`,
		strings.TrimSpace(keywords),
		strings.TrimSpace(article.Title),
		strings.TrimSpace(article.Summary),
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
		MaxTokens:   1000,
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

	// Добавляем ссылку на источник в конце
	postWithSource := fmt.Sprintf("%s\n\nНовость взята с %s", post, article.Source)

	log.Printf("[AI] Успешная генерация поста (длина: %d символов)", len(postWithSource))

	return postWithSource, nil
}

// ClassifyQuery определяет категорию и подкатегорию запроса
func (c *YandexGPTClient) ClassifyQuery(ctx context.Context, query string) (category, subcategory string, err error) {
	prompt := fmt.Sprintf(`
Определи категорию и подкатегорию для запроса: "%s"

Верни ответ ТОЛЬКО в формате JSON:
{
  "category": "название_категории",
  "subcategory": "название_подкатегории"
}

Доступные категории и подкатегории:
1. IT и Технологии:
   - Искусственный интеллект
   - Кибербезопасность
   - Программирование
   - Гаджеты
   - Игры
   - Криптовалюты

2. Бизнес и Финансы:
   - Стартапы
   - Инвестиции
   - Маркетинг
   - Недвижимость
   - Карьера

3. Спорт:
   - Футбол
   - Хоккей
   - Баскетбол
   - Теннис
   - Бокс/MMA
   - Автоспорт

4. Путешествия и Туризм:
   - Авиация
   - Отели
   - Города/Страны
   - Лайфхаки
   - Виза/Документы

5. Наука и Образование:
   - Открытия
   - Медицина
   - Космос
   - Образование
   - История

6. Развлечения и Культура:
   - Кино
   - Музыка
   - Искусство
   - Знаменитости
   - Мемы

7. Общество и Политика:
   - Внутренняя политика
   - Международные отношения
   - Социальные вопросы
   - Законы

8. Здоровье и Спорт:
   - Фитнес
   - Диеты
   - Медицина
   - Психология
`, query)

	request := ChatCompletionRequest{
		Model: c.modelURI,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.3,
		MaxTokens:   500,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Api-Key %s", c.apiKey))
	req.Header.Set("OpenAI-Project", c.folderID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Category    string `json:"category"`
		Subcategory string `json:"subcategory"`
	}

	// Ищем JSON в ответе
	var chatResponse ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResponse); err == nil && len(chatResponse.Choices) > 0 {
		content := chatResponse.Choices[0].Message.Content
		// Парсим JSON из текста
		if err := json.Unmarshal([]byte(content), &result); err == nil {
			return result.Category, result.Subcategory, nil
		}
	}

	// Если не удалось, возвращаем общие категории
	return "Общее", "Новости", nil
}
