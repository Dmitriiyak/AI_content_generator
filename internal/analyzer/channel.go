package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"AIGenerator/internal/ai"

	"github.com/gotd/td/tg"
)

// ChannelAnalyzer анализирует Telegram каналы с помощью AI
type ChannelAnalyzer struct {
	client    *tg.Client
	gptClient *ai.YandexGPTClient
}

// GPTAnalysis содержит структурированный анализ от YandexGPT
type GPTAnalysis struct {
	MainTopic      string         `json:"main_topic"`
	Subtopics      []string       `json:"subtopics"`
	ContentStyle   ContentStyle   `json:"content_style"`
	TargetAudience TargetAudience `json:"target_audience"`
	ContentTypes   []string       `json:"content_types"`
	UniqueFeatures []string       `json:"unique_features"`
	Keywords       []string       `json:"keywords"`
	ContentAngle   string         `json:"content_angle"`
}

// ContentStyle описывает стиль контента канала
type ContentStyle struct {
	Formality        int  `json:"formality"`
	Professionalism  int  `json:"professionalism"`
	Entertainment    int  `json:"entertainment"`
	AvgMessageLength int  `json:"avg_message_length"`
	UsesEmojis       bool `json:"uses_emojis"`
}

// TargetAudience описывает целевую аудиторию канала
type TargetAudience struct {
	AgeRange              string   `json:"age_range"`
	ProfessionalInterests []string `json:"professional_interests"`
	PainPoints            []string `json:"pain_points"`
}

// NewChannelAnalyzer создает новый анализатор каналов
func NewChannelAnalyzer(client *tg.Client, gptClient *ai.YandexGPTClient) *ChannelAnalyzer {
	return &ChannelAnalyzer{
		client:    client,
		gptClient: gptClient,
	}
}

// AnalyzeChannel анализирует Telegram канал с помощью AI
func (ca *ChannelAnalyzer) AnalyzeChannel(ctx context.Context, username string) (*ChannelAnalysis, error) {
	log.Printf("🤖 Начинаем AI-анализ канала: @%s", username)

	// Получаем базовую информацию о канале
	channelInfo, err := ca.getChannelInfo(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения информации о канале: %w", err)
	}

	// Получаем историю сообщений для анализа
	messages, err := ca.getChannelMessages(ctx, username, 30)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения сообщений: %w", err)
	}

	// Анализируем канал через YandexGPT
	gptAnalysis, err := ca.analyzeWithGPT(ctx, messages, channelInfo)
	if err != nil {
		log.Printf("⚠️ AI-анализ не удался, используем упрощенный анализ: %v", err)
		gptAnalysis = ca.fallbackAnalysis(messages, channelInfo)
	}

	analysis := &ChannelAnalysis{
		ChannelInfo: *channelInfo,
		Messages:    messages,
		GPTAnalysis: gptAnalysis,
	}

	log.Printf("✅ Анализ канала @%s завершен. Тема: %s", username, gptAnalysis.MainTopic)
	return analysis, nil
}

// analyzeWithGPT выполняет глубокий анализ через YandexGPT
func (ca *ChannelAnalyzer) analyzeWithGPT(ctx context.Context, messages []Message, channelInfo *ChannelInfo) (*GPTAnalysis, error) {
	if len(messages) == 0 {
		return &GPTAnalysis{
			MainTopic: "общее",
			Subtopics: []string{"разное"},
		}, nil
	}

	// Если GPT клиент не доступен, используем fallback
	if ca.gptClient == nil {
		return ca.fallbackAnalysis(messages, channelInfo), nil
	}

	// Подготавливаем сообщения для анализа
	var messageTexts []string
	for i, msg := range messages {
		if i >= 15 { // Ограничиваем количество сообщений
			break
		}
		if msg.Text != "" && len(msg.Text) > 10 { // Пропускаем пустые и очень короткие
			messageTexts = append(messageTexts, msg.Text)
		}
	}

	// Вызываем YandexGPT для анализа канала
	response, err := ca.gptClient.AnalyzeChannel(ctx, channelInfo.Title, channelInfo.Description, messageTexts)
	if err != nil {
		return nil, fmt.Errorf("ошибка YandexGPT: %w", err)
	}

	// Парсим JSON ответ
	var analysis GPTAnalysis
	if err := json.Unmarshal([]byte(response), &analysis); err != nil {
		log.Printf("⚠️ Ошибка парсинга GPT ответа, используем fallback: %v", err)
		return ca.fallbackAnalysis(messages, channelInfo), nil
	}

	return &analysis, nil
}

// fallbackAnalysis упрощенный анализ когда GPT недоступен
func (ca *ChannelAnalyzer) fallbackAnalysis(messages []Message, channelInfo *ChannelInfo) *GPTAnalysis {
	log.Printf("🔄 Используем упрощенный анализ без AI")

	// Базовый анализ на основе сообщений
	mainTopic := ca.detectMainTopic(messages)

	return &GPTAnalysis{
		MainTopic:      mainTopic,
		Subtopics:      ca.extractSubtopics(mainTopic),
		ContentStyle:   ca.analyzeContentStyle(messages),
		TargetAudience: ca.analyzeAudience(mainTopic),
		ContentTypes:   []string{"информационный контент"},
		UniqueFeatures: []string{"экспертное мнение"},
		Keywords:       ca.extractKeywords(mainTopic),
		ContentAngle:   "практический подход с пользой для аудитории",
	}
}

// detectMainTopic определяет основную тему канала
func (ca *ChannelAnalyzer) detectMainTopic(messages []Message) string {
	if len(messages) == 0 {
		return "общая тематика"
	}

	// Простой анализ ключевых слов
	techKeywords := []string{"техно", "it", "программир", "код", "ai", "ии", "гаджет", "смартфон"}
	businessKeywords := []string{"бизнес", "стартап", "компани", "рынок", "экономик"}
	newsKeywords := []string{"новост", "событи", "политик", "обществ"}

	keywordCount := map[string]int{
		"технологии и IT":   0,
		"бизнес и стартапы": 0,
		"новости и события": 0,
	}

	for _, msg := range messages {
		text := strings.ToLower(msg.Text)
		for _, word := range techKeywords {
			if strings.Contains(text, word) {
				keywordCount["технологии и IT"]++
			}
		}
		for _, word := range businessKeywords {
			if strings.Contains(text, word) {
				keywordCount["бизнес и стартапы"]++
			}
		}
		for _, word := range newsKeywords {
			if strings.Contains(text, word) {
				keywordCount["новости и события"]++
			}
		}
	}

	// Находим тему с наибольшим количеством упоминаний
	maxCount := 0
	detectedTopic := "общая тематика"
	for topic, count := range keywordCount {
		if count > maxCount {
			maxCount = count
			detectedTopic = topic
		}
	}

	return detectedTopic
}

// extractSubtopics извлекает подтемы
func (ca *ChannelAnalyzer) extractSubtopics(mainTopic string) []string {
	// Базовые подтемы для разных категорий
	topicMap := map[string][]string{
		"технологии и IT":   {"программирование", "искусственный интеллект", "гаджеты", "кибербезопасность"},
		"бизнес и стартапы": {"финансы", "маркетинг", "управление", "инвестиции"},
		"новости и события": {"политика", "экономика", "общество", "технологии"},
	}

	if topics, exists := topicMap[mainTopic]; exists {
		return topics
	}

	return []string{"актуальные тренды", "практические советы", "экспертные мнения"}
}

// analyzeContentStyle анализирует стиль контента
func (ca *ChannelAnalyzer) analyzeContentStyle(messages []Message) ContentStyle {
	if len(messages) == 0 {
		return ContentStyle{
			Formality:        5,
			Professionalism:  5,
			Entertainment:    5,
			AvgMessageLength: 200,
			UsesEmojis:       true,
		}
	}

	totalLength := 0
	emojiCount := 0
	formalWords := []string{"компания", "рынок", "инвестиции", "разработка", "анализ"}
	formalCount := 0

	for _, msg := range messages {
		totalLength += len(msg.Text)

		// Проверяем эмодзи
		if strings.ContainsAny(msg.Text, "😂😊👍🎯🔥❤️✨") {
			emojiCount++
		}

		// Проверяем формальные слова
		text := strings.ToLower(msg.Text)
		for _, word := range formalWords {
			if strings.Contains(text, word) {
				formalCount++
				break
			}
		}
	}

	avgLength := totalLength / len(messages)

	// Определяем формальность на основе количества формальных слов
	formality := 5
	if formalCount > len(messages)/2 {
		formality = 8
	} else if formalCount < len(messages)/4 {
		formality = 3
	}

	return ContentStyle{
		Formality:        formality,
		Professionalism:  formality,      // Упрощенно связываем с формальностью
		Entertainment:    10 - formality, // Обратная зависимость
		AvgMessageLength: avgLength,
		UsesEmojis:       emojiCount > len(messages)/3,
	}
}

// analyzeAudience анализирует целевую аудиторию
func (ca *ChannelAnalyzer) analyzeAudience(mainTopic string) TargetAudience {
	switch mainTopic {
	case "технологии и IT":
		return TargetAudience{
			AgeRange:              "20-35",
			ProfessionalInterests: []string{"IT", "разработка", "технологии", "инновации"},
			PainPoints:            []string{"нехватка времени", "информационная перегрузка", "быстрое устаревание знаний"},
		}
	case "бизнес и стартапы":
		return TargetAudience{
			AgeRange:              "25-45",
			ProfessionalInterests: []string{"предпринимательство", "менеджмент", "финансы", "маркетинг"},
			PainPoints:            []string{"конкуренция", "поиск инвестиций", "управление ростом", "маркетинг"},
		}
	case "новости и события":
		return TargetAudience{
			AgeRange:              "18-60",
			ProfessionalInterests: []string{"аналитика", "политика", "экономика", "общество"},
			PainPoints:            []string{"информационный шум", "фейковые новости", "нехватка времени"},
		}
	default:
		return TargetAudience{
			AgeRange:              "18-45",
			ProfessionalInterests: []string{"саморазвитие", "карьера", "образование"},
			PainPoints:            []string{"поиск качественного контента", "нехватка времени", "информационный шум"},
		}
	}
}

// extractKeywords извлекает ключевые слова
func (ca *ChannelAnalyzer) extractKeywords(mainTopic string) []string {
	keywordMap := map[string][]string{
		"технологии и IT":   {"технологии", "IT", "программирование", "AI", "гаджеты", "инновации", "разработка"},
		"бизнес и стартапы": {"бизнес", "стартапы", "финансы", "маркетинг", "управление", "инвестиции"},
		"новости и события": {"новости", "события", "аналитика", "тренды", "прогнозы", "политика"},
	}

	if keywords, exists := keywordMap[mainTopic]; exists {
		return keywords
	}

	return []string{"актуальное", "полезное", "экспертное", "информация", "развитие"}
}

// getChannelInfo получает базовую информацию о канале
func (ca *ChannelAnalyzer) getChannelInfo(ctx context.Context, username string) (*ChannelInfo, error) {
	// TODO: Реализовать получение информации о канале через MTProto
	// Временная реализация для тестирования

	// Симулируем разные каналы для тестирования
	var title, description string
	var participants int

	switch username {
	case "tproger":
		title = "TProger"
		description = "Канал о программировании и IT"
		participants = 150000
	case "vcru":
		title = "VC.ru"
		description = "Сообщество предпринимателей и стартапов"
		participants = 120000
	case "habr":
		title = "Хабрахабр"
		description = "IT-сообщество и технический блог"
		participants = 200000
	default:
		title = "Тестовый канал"
		description = "Канал для тестирования AI-анализа"
		participants = 10000
	}

	return &ChannelInfo{
		ID:           generateChannelID(username),
		Title:        title,
		Username:     username,
		Description:  description,
		Participants: participants,
		CreatedAt:    time.Now().Add(-365 * 24 * time.Hour),
	}, nil
}

// getChannelMessages получает сообщения канала
func (ca *ChannelAnalyzer) getChannelMessages(ctx context.Context, username string, limit int) ([]Message, error) {
	// TODO: Реализовать получение сообщений через MTProto
	// Временная реализация с тестовыми сообщениями

	var messages []Message

	// Генерируем тестовые сообщения в зависимости от тематики канала
	switch username {
	case "tproger":
		messages = []Message{
			{
				ID:    1,
				Text:  "Новости IT: Искусственный интеллект продолжает развиваться быстрыми темпами. Новые модели GPT показывают впечатляющие результаты в обработке естественного языка.",
				Views: 1500,
				Date:  time.Now().Add(-24 * time.Hour),
			},
			{
				ID:    2,
				Text:  "Советы по программированию: Используйте чистый код и следите за производительностью ваших приложений. Оптимизация алгоритмов может значительно ускорить работу.",
				Views: 1200,
				Date:  time.Now().Add(-48 * time.Hour),
			},
			{
				ID:    3,
				Text:  "Обзор новых технологий: Последние гаджеты и устройства, которые стоит попробовать в 2024 году. От смартфонов до умных часов.",
				Views: 1800,
				Date:  time.Now().Add(-72 * time.Hour),
			},
		}
	case "vcru":
		messages = []Message{
			{
				ID:    1,
				Text:  "Бизнес-новости: Российский рынок стартапов показывает рост несмотря на экономические вызовы. Инвестиции в IT-сектор увеличились на 15%.",
				Views: 2000,
				Date:  time.Now().Add(-24 * time.Hour),
			},
			{
				ID:    2,
				Text:  "Советы предпринимателям: Как эффективно управлять удаленной командой и повышать продуктивность сотрудников.",
				Views: 1500,
				Date:  time.Now().Add(-48 * time.Hour),
			},
		}
	default:
		messages = []Message{
			{
				ID:    1,
				Text:  "Добро пожаловать в наш канал! Здесь мы делимся интересными новостями и полезными советами.",
				Views: 1000,
				Date:  time.Now().Add(-24 * time.Hour),
			},
			{
				ID:    2,
				Text:  "Не забывайте подписываться на канал и делиться нашими публикациями с друзьями!",
				Views: 800,
				Date:  time.Now().Add(-48 * time.Hour),
			},
		}
	}

	return messages, nil
}

// generateChannelID генерирует ID канала на основе username
func generateChannelID(username string) int64 {
	var hash int64
	for _, char := range username {
		hash = hash*31 + int64(char)
	}
	return hash
}
