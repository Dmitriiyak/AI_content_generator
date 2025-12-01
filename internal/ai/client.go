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

func (c *YandexGPTClient) ClassifyQuery(ctx context.Context, query string) (category, subcategory string, err error) {
	log.Printf("[AI] Классификация запроса: %s", query)

	prompt := fmt.Sprintf(`Проанализируй запрос и определи наиболее подходящую категорию новостей. Учитывай названия людей, компаний, брендов, продуктов.

ЗАПРОС: "%s"

Важные правила:
• "Макс Ферстаппен", "Формула 1" → категория: "Спорт", подкатегория: "Формула 1"
• "Hyundai", "Tesla", "BMW" → категория: "Автомобили", подкатегория: "Автопроизводители"
• "ЦСКА", "Спартак", "Барселона" → категория: "Спорт", подкатегория: "Футбол"
• "Apple", "Samsung" → категория: "IT и Технологии", подкатегория: "Гаджеты"
• "Искусственный интеллект", "ChatGPT" → категория: "IT и Технологии", подкатегория: "Искусственный интеллект"
• "Билайн", "МТС" → категория: "Телекоммуникации", подкатегория: "Сотовые операторы"
• "Сбербанк", "Тинькофф" → категория: "Бизнес и Финансы", подкатегория: "Банки"

Верни ответ ТОЛЬКО в формате JSON:
{
  "category": "название_категории",
  "subcategory": "название_подкатегории"
}

Доступные категории:
1. IT и Технологии: Искусственный интеллект, Кибербезопасность, Программирование, Гаджеты, Игры, Криптовалюты, Соцсети
2. Бизнес и Финансы: Стартапы, Инвестиции, Маркетинг, Недвижимость, Карьера, Банки, Криптовалюта
3. Спорт: Футбол, Хоккей, Баскетбол, Теннис, Бокс/MMA, Автоспорт, Формула 1, Зимние виды
4. Путешествия и Туризм: Авиация, Отели, Города/Страны, Лайфхаки, Виза/Документы, ЖД билеты
5. Наука и Образование: Открытия, Медицина, Космос, Образование, История, Биология, Физика
6. Развлечения и Культура: Кино, Музыка, Искусство, Знаменитости, Мемы, Сериалы, Литература
7. Общество и Политика: Внутренняя политика, Международные отношения, Социальные вопросы, Законы, Экономика
8. Здоровье: Фитнес, Диеты, Медицина, Психология, ЗОЖ, Болезни
9. Автомобили: Новинки, Технологии, Автопроизводители, Тест-драйвы, Электромобили
10. Еда и Рестораны: Рестораны, Рецепты, Доставка, Фастфуд, Здоровое питание
11. Мода и Стиль: Одежда, Обувь, Аксессуары, Бьюти, Косметика
12. Телекоммуникации: Сотовые операторы, Интернет-провайдеры, Тарифы, Связь
13. Недвижимость: Квартиры, Дома, Ипотека, Аренда, Коммерческая

Если не уверен, используй: {"category": "Общее", "subcategory": "Новости"}`, query)

	response, err := c.makeRequest(ctx, prompt, 0.3, 300)
	if err != nil {
		log.Printf("[AI] ❌ Ошибка классификации: %v", err)
		return "Общее", "Новости", nil
	}

	// Извлекаем JSON из ответа
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")

	if start == -1 || end == -1 {
		log.Printf("[AI] ❌ Некорректный JSON в ответе: %s", response)
		return "Общее", "Новости", nil
	}

	jsonStr := response[start : end+1]

	var result struct {
		Category    string `json:"category"`
		Subcategory string `json:"subcategory"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		log.Printf("[AI] ❌ Ошибка парсинга JSON: %v, ответ: %s", err, jsonStr)
		return "Общее", "Новости", nil
	}

	// Проверяем, что категория не пустая
	if result.Category == "" {
		result.Category = "Общее"
	}
	if result.Subcategory == "" {
		result.Subcategory = "Новости"
	}

	log.Printf("[AI] ✅ Категория определена: %s/%s", result.Category, result.Subcategory)
	return result.Category, result.Subcategory, nil
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

Пример хорошего поста:
⚡️ Кризис ОЗУ привёл к тотальной дурке — Samsung не может купить чипы памяти у самой себя!

Подразделение Samsung Galaxy не смогло заключить долгосрочный контракт с командой, поставляющей чипы HBM и LPDDR. Не помогло даже высшее руководство — *настолько быстро растут цены*.

В начале года чип LPDDR5X 12 ГБ стоил *$33*, а теперь стоит целых *$70* — и цена будет только расти.

Теперь создай пост на основе этой информации:

ТЕМА ЗАПРОСА: %s
ЗАГОЛОВОК НОВОСТИ: %s
ОПИСАНИЕ НОВОСТИ: %s

Создай пост, который зацепит аудиторию Telegram.`,
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
