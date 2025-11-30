package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"AIGenerator/internal/ai"
	"AIGenerator/internal/analyzer"
	"AIGenerator/internal/news"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot представляет Telegram бота
type Bot struct {
	api              *tgbotapi.BotAPI
	channelAnalyzer  *analyzer.ChannelAnalyzer
	newsAggregator   *news.NewsAggregator
	gptClient        *ai.YandexGPTClient
	userFirstRequest map[int64]bool      // Отслеживаем первый запрос пользователя
	userLastRequest  map[int64]time.Time // Время последнего запроса
}

// New создает нового бота
func New(token string, analyzer *analyzer.ChannelAnalyzer, newsAggregator *news.NewsAggregator, gptClient *ai.YandexGPTClient) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания бота: %w", err)
	}

	return &Bot{
		api:              api,
		channelAnalyzer:  analyzer,
		newsAggregator:   newsAggregator,
		gptClient:        gptClient,
		userFirstRequest: make(map[int64]bool),
		userLastRequest:  make(map[int64]time.Time),
	}, nil
}

// Start запускает бота
func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	log.Printf("🤖 Бот запущен: @%s", b.api.Self.UserName)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Обрабатываем команды
		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				b.handleStart(update.Message)
			case "help", "рудз":
				b.handleHelp(update.Message)
			case "generate":
				b.handleGenerate(ctx, update.Message)
			default:
				b.sendMessage(update.Message.Chat.ID, "❌ Неизвестная команда. Используйте /help для списка команд.")
			}
		}
	}
}

// handleGenerate обрабатывает команду /generate
func (b *Bot) handleGenerate(ctx context.Context, msg *tgbotapi.Message) {
	// Проверяем анти-спам (кроме первого запроса)
	if !b.isFirstRequest(msg.Chat.ID) && b.isTooFrequent(msg.Chat.ID) {
		timeLeft := b.getTimeLeft(msg.Chat.ID)
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("⏳ Пожалуйста, подождите %d секунд перед следующим запросом", timeLeft))
		return
	}

	// Обновляем время запроса
	b.updateRequestTime(msg.Chat.ID)

	// Проверяем формат команды
	args := strings.Fields(msg.Text)
	if len(args) < 2 {
		b.sendMessage(msg.Chat.ID, "❌ *Неверный формат команды*\n\nИспользуйте:\n`/generate @username` - для анализа канала\n`/generate ключевые слова` - для генерации по теме\n\nПримеры:\n`/generate @test`\n`/generate IT технологии AI`")
		return
	}

	// Определяем тип запроса: канал или ключевые слова
	input := strings.Join(args[1:], " ")
	var isChannel bool
	var username string
	var keywords string

	if strings.HasPrefix(input, "@") {
		// Это запрос для канала
		isChannel = true
		username = strings.TrimPrefix(input, "@")
		if username == "" {
			b.sendMessage(msg.Chat.ID, "❌ *Не указан username канала*\n\nПример: `/generate @test`")
			return
		}
	} else {
		// Это запрос по ключевым словам
		isChannel = false
		keywords = input
		if len(keywords) < 3 {
			b.sendMessage(msg.Chat.ID, "❌ *Слишком короткие ключевые слова*\n\nУкажите более конкретную тему для генерации.\nПример: `/generate искусственный интеллект IT`")
			return
		}
	}

	// Отправляем сообщение о начале обработки
	var processingMsg tgbotapi.Message
	if isChannel {
		processingMsg = b.sendMessage(msg.Chat.ID, fmt.Sprintf("🔄 *Начинаем анализ канала @%s...*\n\nАнализирую канал и подбираю новости...", username))
	} else {
		processingMsg = b.sendMessage(msg.Chat.ID, fmt.Sprintf("🔄 *Генерирую пост по теме: %s...*\n\nИщу релевантные новости...", keywords))
	}

	var analysis *analyzer.ChannelAnalysis
	var err error

	if isChannel {
		// 1. Анализируем канал
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "🔍 *Анализирую канал...*")
		analysis, err = b.channelAnalyzer.AnalyzeChannel(ctx, username)
		if err != nil {
			b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Ошибка анализа канала*\n\nУбедитесь, что канал существует и является публичным.")
			return
		}
	} else {
		// 1. Создаем анализ на основе ключевых слов
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "🔍 *Анализирую тему...*")
		analysis = b.createAnalysisFromKeywords(keywords)
	}

	// 2. Получаем новости
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "📰 *Ищу свежие новости...*")
	articles, err := b.newsAggregator.FetchAllArticles()
	if err != nil {
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Ошибка получения новостей*\n\nПопробуйте позже.")
		return
	}

	if len(articles) == 0 {
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Нет свежих новостей*\n\nПопробуйте позже, когда появятся новые статьи.")
		return
	}

	// 3. Подбираем релевантные новости
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "🎯 *Подбираю релевантные новости...*")
	relevantArticles := b.newsAggregator.FindRelevantArticles(ctx, articles, analysis, 3)

	if len(relevantArticles) == 0 {
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Не найдено релевантных новостей*\n\nПопробуйте другую тему или повторите позже.")
		return
	}

	// Логируем выбранные новости
	for i, article := range relevantArticles {
		log.Printf("📋 Кандидат %d: %s (источник: %s, релевантность: %.2f)",
			i+1, article.Title, article.Source, article.Relevance)
	}

	// 4. Генерируем пост
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "✍️ *Генерирую пост...*")
	generatedPost, usedArticle := b.tryGeneratePost(ctx, analysis, relevantArticles)

	if generatedPost == "" {
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Не удалось сгенерировать пост*\n\nYandexGPT отказался обрабатывать новости. Попробуйте позже.")
		return
	}

	// 5. Удаляем сообщение о процессе
	b.deleteMessage(processingMsg.Chat.ID, processingMsg.MessageID)

	// 6. Отправляем результат
	var successText string
	if isChannel {
		successText = fmt.Sprintf("✅ *Пост для @%s готов!*\n\n📰 *Источник:* %s\n\nСкопируйте текст ниже для публикации в канале:",
			username, usedArticle.Source)
	} else {
		successText = fmt.Sprintf("✅ *Пост по теме '%s' готов!*\n\n📰 *Источник:* %s\n\nСкопируйте текст ниже для публикации:",
			keywords, usedArticle.Source)
	}

	b.sendMessage(msg.Chat.ID, successText)
	b.sendMessage(msg.Chat.ID, generatedPost)

	if isChannel {
		log.Printf("✅ Успешная генерация поста для @%s", username)
	} else {
		log.Printf("✅ Успешная генерация поста по теме: %s", keywords)
	}
}

// createAnalysisFromKeywords создает анализ канала на основе ключевых слов
func (b *Bot) createAnalysisFromKeywords(keywords string) *analyzer.ChannelAnalysis {
	// Создаем базовый анализ на основе ключевых слов
	return &analyzer.ChannelAnalysis{
		ChannelInfo: analyzer.ChannelInfo{
			Title:    "Генерация по ключевым словам",
			Username: "keywords",
		},
		GPTAnalysis: &analyzer.GPTAnalysis{
			MainTopic:    keywords,
			Subtopics:    []string{keywords},
			Keywords:     strings.Fields(keywords),
			ContentAngle: "информационный пост с практической пользой",
			ContentStyle: analyzer.ContentStyle{
				Formality:        6,
				Professionalism:  7,
				Entertainment:    5,
				AvgMessageLength: 250,
				UsesEmojis:       true,
			},
			TargetAudience: analyzer.TargetAudience{
				AgeRange:              "18-45",
				ProfessionalInterests: strings.Fields(keywords),
				PainPoints:            []string{"нехватка времени", "информационный шум"},
			},
		},
	}
}

// isFirstRequest проверяет, является ли это первым запросом пользователя
func (b *Bot) isFirstRequest(chatID int64) bool {
	if _, exists := b.userFirstRequest[chatID]; !exists {
		b.userFirstRequest[chatID] = true
		return true
	}
	return false
}

// isTooFrequent проверяет, не слишком ли часто пользователь отправляет запросы
func (b *Bot) isTooFrequent(chatID int64) bool {
	lastRequest, exists := b.userLastRequest[chatID]
	if !exists {
		return false
	}
	return time.Since(lastRequest) < 30*time.Second
}

// getTimeLeft возвращает оставшееся время до возможности нового запроса
func (b *Bot) getTimeLeft(chatID int64) int {
	lastRequest, exists := b.userLastRequest[chatID]
	if !exists {
		return 0
	}
	timePassed := time.Since(lastRequest)
	timeLeft := 30 - int(timePassed.Seconds())
	if timeLeft < 0 {
		timeLeft = 0
	}
	return timeLeft
}

// updateRequestTime обновляет время последнего запроса
func (b *Bot) updateRequestTime(chatID int64) {
	b.userLastRequest[chatID] = time.Now()
}

// tryGeneratePost пытается сгенерировать пост для каждой новости по очереди
func (b *Bot) tryGeneratePost(ctx context.Context, analysis *analyzer.ChannelAnalysis, articles []news.Article) (string, news.Article) {
	// Конвертируем анализ для AI
	channelAnalysis := &ai.ChannelAnalysis{
		MainTopic:      analysis.GPTAnalysis.MainTopic,
		Subtopics:      analysis.GPTAnalysis.Subtopics,
		TargetAudience: analysis.GPTAnalysis.TargetAudience.AgeRange,
		ContentStyle:   fmt.Sprintf("Формальность: %d/10", analysis.GPTAnalysis.ContentStyle.Formality),
		Keywords:       analysis.GPTAnalysis.Keywords,
		ContentAngle:   analysis.GPTAnalysis.ContentAngle,
	}

	// Пробуем по очереди для каждой новости
	for i, article := range articles {
		log.Printf("🔄 Попытка генерации %d/%d: %s", i+1, len(articles), article.Title)

		articleForAI := ai.ArticleRelevance{
			Title:   article.Title,
			Summary: article.Summary,
			URL:     article.URL,
		}

		post, err := b.gptClient.GeneratePost(ctx, channelAnalysis, articleForAI)
		if err != nil {
			log.Printf("⚠️ Ошибка генерации: %v", err)
			continue
		}

		// Проверяем что пост не содержит отказ и не слишком короткий
		if !b.isRejectedPost(post) && len(strings.TrimSpace(post)) >= 100 {
			formattedPost := b.formatPostForChannel(post, article)
			log.Printf("✅ Успешная генерация для: %s", article.Title)
			return formattedPost, article
		}

		log.Printf("⚠️ Отклонен пост для: %s", article.Title)
	}

	return "", news.Article{}
}

// formatPostForChannel форматирует пост для публикации в канале
func (b *Bot) formatPostForChannel(post string, article news.Article) string {
	// Убираем лишние надписи из поста
	cleanedPost := strings.TrimSpace(post)

	// Добавляем источник в формате: [Новость](ссылка) взята с НазваниеИсточника
	sourceLine := fmt.Sprintf("\n\n📰 [Новость](%s) взята с *%s*", article.URL, article.Source)

	return cleanedPost + sourceLine
}

// isRejectedPost проверяет, отказался ли GPT генерировать пост
func (b *Bot) isRejectedPost(post string) bool {
	rejectionPhrases := []string{
		"не могу обсуждать",
		"не могу написать",
		"отказываюсь",
		"не буду",
		"это не в моей компетенции",
		"давайте поговорим",
		"не могу помочь",
		"я не могу",
		"как искусственный интеллект",
	}

	postLower := strings.ToLower(post)
	for _, phrase := range rejectionPhrases {
		if strings.Contains(postLower, strings.ToLower(phrase)) {
			return true
		}
	}

	return false
}

// sendMessage отправляет сообщение
func (b *Bot) sendMessage(chatID int64, text string) tgbotapi.Message {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = false

	message, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
	}

	return message
}

// editMessage редактирует существующее сообщение
func (b *Bot) editMessage(chatID int64, messageID int, text string) {
	msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = false

	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Ошибка редактирования сообщения: %v", err)
	}
}

// deleteMessage удаляет сообщение
func (b *Bot) deleteMessage(chatID int64, messageID int) {
	msg := tgbotapi.NewDeleteMessage(chatID, messageID)
	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Ошибка удаления сообщения: %v", err)
	}
}

// handleStart обрабатывает команду /start
func (b *Bot) handleStart(msg *tgbotapi.Message) {
	welcomeText := `👋 *Добро пожаловать в AI Content Generator!*

Я помогу вам создавать качественные посты для вашего Telegram канала на основе актуальных новостей.

📋 *Доступные команды:*
/start - начать работу
/help - показать справку
/generate - создать пост для канала

💡 *Пример использования:*
/generate @test - создать пост для канала @test

⚡ *Первая генерация* - выполняется сразу
⏳ *Следующие запросы* - с интервалом 30 секунд

Я проанализирую канал, подберу релевантную новость и сгенерирую готовый пост в стиле вашего канала!`

	b.sendMessage(msg.Chat.ID, welcomeText)
}

// handleHelp обрабатывает команду /help
func (b *Bot) handleHelp(msg *tgbotapi.Message) {
	helpText := `📖 *Справка по командам*

*/start* - начать работу с ботом
*/help* - показать эту справку  
*/generate <канал>* - создать пост для указанного канала

🔧 *Как использовать /generate:*
Формат: /generate @test

⚡ *Особенности работы:*
- Первая генерация выполняется сразу
- Следующие запросы - с интервалом 30 секунд
- Бот запоминает ваш первый запрос

🤖 *Что делает бот:*
1. Анализирует стиль и тематику вашего канала
2. Подбирает самую релевантную новость
3. Генерирует готовый пост в вашем стиле
4. Возвращает оформленный текст для публикации

⚠️ *Важно:* Убедитесь, что канал публичный и доступен для анализа.
⚠️ *Важно:* Бот не предлагает посты на военную тематику`

	b.sendMessage(msg.Chat.ID, helpText)
}
