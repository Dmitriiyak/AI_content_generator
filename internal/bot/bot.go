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
	"AIGenerator/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api             *tgbotapi.BotAPI
	channelAnalyzer *analyzer.ChannelAnalyzer
	newsAggregator  *news.NewsAggregator
	gptClient       *ai.YandexGPTClient
	storage         *storage.Storage
	userLastRequest map[int64]time.Time
}

func New(token string, analyzer *analyzer.ChannelAnalyzer, newsAggregator *news.NewsAggregator, gptClient *ai.YandexGPTClient, storage *storage.Storage) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания бота: %w", err)
	}

	return &Bot{
		api:             api,
		channelAnalyzer: analyzer,
		newsAggregator:  newsAggregator,
		gptClient:       gptClient,
		storage:         storage,
		userLastRequest: make(map[int64]time.Time),
	}, nil
}

func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	log.Printf("🤖 Бот запущен: @%s", b.api.Self.UserName)

	for update := range updates {
		if update.CallbackQuery != nil {
			b.handleCallback(update.CallbackQuery)
			continue
		}

		if update.Message == nil {
			continue
		}

		if update.Message.IsCommand() {
			b.handleCommand(update.Message)
			continue
		}

		// Текстовые сообщения считаем запросами на генерацию
		b.handleGenerate(context.Background(), update.Message)
	}
}

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	log.Printf("🔧 Обработка команды: %s от %d", msg.Command(), msg.Chat.ID)

	switch msg.Command() {
	case "start":
		b.handleStart(msg)
	case "help":
		b.handleHelp(msg)
	case "generate":
		b.handleGenerateCommand(msg)
	case "buy":
		b.handleBuy(msg)
	case "balance":
		b.handleBalance(msg)
	default:
		b.sendMessage(msg.Chat.ID, "❌ Неизвестная команда. Используйте /help для списка команд.")
	}
}

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID
	data := callback.Data

	log.Printf("🔧 Обработка callback: %s от %d", data, chatID)

	switch data {
	case "buy_10", "buy_25", "buy_100":
		b.showPaymentInfo(chatID, data)
	default:
		b.answerCallback(callback.ID, "❌ Неизвестная команда")
	}
}

func (b *Bot) handleStart(msg *tgbotapi.Message) {
	user := b.storage.GetUser(msg.Chat.ID)

	text := `🤖 *AI Content Generator*

Я помогу создавать качественные посты для Telegram каналов на основе актуальных новостей.

✨ *Основные команды:*
/generate - создать пост (по ключевым словам или каналу)
/balance - проверить баланс генераций  
/buy - приобрести дополнительные генерации
/help - показать справку

🎯 *У вас есть %d бесплатных генераций!*

🚀 *Начните с команды /generate*`

	b.sendMessage(msg.Chat.ID, fmt.Sprintf(text, user.AvailableGenerations))
}

func (b *Bot) handleHelp(msg *tgbotapi.Message) {
	text := `📖 *Справка по командам*

🎯 *Основные команды:*
/generate - создать пост
/balance - проверить баланс
/buy - купить генерации
/help - эта справка

📝 *Как использовать /generate:*
• Просто отправьте команду /generate и ключевые слова
• Или укажите @username канала для анализа
• Примеры:
  /generate искусственный интеллект
  /generate @techchannel

💎 *Тарифы:*
• 10 генераций - 99 руб
• 25 генераций - 199 руб  
• 100 генераций - 499 руб

⏰ *Лимиты:*
• 30 секунд между запросами
• Первые 10 генераций - бесплатно`

	b.sendMessage(msg.Chat.ID, text)
}

func (b *Bot) handleGenerateCommand(msg *tgbotapi.Message) {
	log.Printf("🔧 Обработка команды /generate от %d", msg.Chat.ID)

	// Получаем аргументы команды
	args := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/generate"))
	log.Printf("📝 Аргументы команды: '%s'", args)

	if args == "" {
		log.Printf("❌ Пустые аргументы команды /generate")
		b.sendMessage(msg.Chat.ID,
			"❌ *Не указаны параметры для генерации*\n\n"+
				"📝 *Используйте:*\n"+
				"`/generate ключевые слова` - для генерации по теме\n"+
				"`/generate @username` - для анализа канала\n\n"+
				"✨ *Примеры:*\n"+
				"`/generate искусственный интеллект`\n"+
				"`/generate @techchannel`")
		return
	}

	// Создаем fake сообщение с аргументами команды
	fakeMsg := *msg
	fakeMsg.Text = args
	log.Printf("🔧 Создан fakeMsg с текстом: '%s'", fakeMsg.Text)
	b.handleGenerate(context.Background(), &fakeMsg)
}

func (b *Bot) handleBuy(msg *tgbotapi.Message) {
	pricing := b.storage.GetPricing()

	text := "💎 *Приобретите дополнительные генерации*\n\n" +
		"Выберите пакет:\n\n" +
		fmt.Sprintf("🔹 10 генераций - %d руб.\n", pricing["10 генераций"]) +
		fmt.Sprintf("🔹 25 генераций - %d руб.\n", pricing["25 генераций"]) +
		fmt.Sprintf("🔹 100 генераций - %d руб.\n\n", pricing["100 генераций"]) +
		"💡 *После оплаты отправьте скриншот чека @admin*"

	b.sendMessageWithKeyboard(msg.Chat.ID, text, b.createBuyMenu())
}

func (b *Bot) handleBalance(msg *tgbotapi.Message) {
	user := b.storage.GetUser(msg.Chat.ID)

	text := fmt.Sprintf(
		"🎯 *Ваш баланс*\n\n"+
			"✨ *Доступно генераций:* %d\n"+
			"📊 *Всего использовано:* %d",
		user.AvailableGenerations,
		user.TotalGenerations)

	b.sendMessage(msg.Chat.ID, text)
}

func (b *Bot) showPaymentInfo(chatID int64, packageType string) {
	pricing := b.storage.GetPricing()
	var count int
	var price int

	switch packageType {
	case "buy_10":
		count = 10
		price = pricing["10 генераций"]
	case "buy_25":
		count = 25
		price = pricing["25 генераций"]
	case "buy_100":
		count = 100
		price = pricing["100 генераций"]
	}

	text := fmt.Sprintf(
		"💳 *Оформление заказа*\n\n"+
			"📦 *Пакет:* %d генераций\n"+
			"💰 *Стоимость:* %d рублей\n\n"+
			"📞 *Для оплаты свяжитесь с @admin*",
		count, price)

	b.sendMessage(chatID, text)
}

func (b *Bot) handleGenerate(ctx context.Context, msg *tgbotapi.Message) {
	log.Printf("🚀 НАЧАЛО handleGenerate для пользователя %d", msg.Chat.ID)
	log.Printf("📝 Текст сообщения: '%s'", msg.Text)

	user := b.storage.GetUser(msg.Chat.ID)
	log.Printf("👤 Пользователь: %+v", user)

	// Проверяем доступные генерации
	if user.AvailableGenerations <= 0 {
		log.Printf("❌ У пользователя %d закончились генерации", msg.Chat.ID)
		b.sendMessage(msg.Chat.ID,
			"❌ *Закончились генерации!*\n\n"+
				"Используйте команду /buy чтобы приобрести дополнительные генерации 💫")
		return
	}
	log.Printf("✅ Генерации доступны: %d", user.AvailableGenerations)

	// Проверяем анти-спам с показом оставшегося времени
	if timeLeft := b.getTimeLeftSeconds(msg.Chat.ID); timeLeft > 0 {
		log.Printf("⏳ Слишком частый запрос от пользователя %d, осталось секунд: %d", msg.Chat.ID, timeLeft)
		b.sendMessage(msg.Chat.ID,
			fmt.Sprintf("⏳ Пожалуйста, подождите %d секунд перед следующим запросом", timeLeft))
		return
	}
	log.Printf("✅ Анти-спам проверка пройдена")

	// Обновляем время запроса сразу после проверки анти-спама
	b.updateRequestTime(msg.Chat.ID)

	// Используем одну генерацию
	log.Printf("🔧 Вызов storage.UseGeneration")
	success, err := b.storage.UseGeneration(msg.Chat.ID)
	if err != nil || !success {
		log.Printf("❌ Ошибка использования генерации: %v", err)
		b.sendMessage(msg.Chat.ID, "❌ Ошибка системы. Попробуйте позже.")
		return
	}

	// Получаем обновленного пользователя после использования генерации
	user = b.storage.GetUser(msg.Chat.ID)
	log.Printf("✅ Генерация использована, осталось: %d", user.AvailableGenerations)

	// Определяем тип запроса
	input := strings.TrimSpace(msg.Text)
	var isChannel bool
	var username string
	var keywords string

	if strings.HasPrefix(input, "@") {
		isChannel = true
		username = strings.TrimPrefix(input, "@")
		log.Printf("🔍 Запрос на анализ канала: @%s", username)
	} else {
		isChannel = false
		keywords = input
		log.Printf("🔍 Запрос по ключевым словам: %s", keywords)
	}

	// Показываем сообщение о процессе
	var processingText string
	if isChannel {
		processingText = fmt.Sprintf("🔄 *Анализирую канал @%s...*", username)
	} else {
		processingText = fmt.Sprintf("🔄 *Генерирую пост по теме: %s...*", keywords)
	}

	log.Printf("📤 Отправка сообщения о процессе: %s", processingText)
	processingMsg := b.sendMessage(msg.Chat.ID, processingText)
	if processingMsg.MessageID == 0 {
		log.Printf("❌ Не удалось отправить сообщение о процессе")
		return
	}
	log.Printf("✅ Сообщение о процессе отправлено, ID: %d", processingMsg.MessageID)

	// Эмулируем процесс для визуальной обратной связи
	log.Printf("📡 Этап: Получение новостей")
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "📡 *Получаю свежие новости...*")
	time.Sleep(1 * time.Second)

	// Получаем новости
	log.Printf("🎯 Этап: Подбор релевантных новостей")
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "🎯 *Подбираю релевантные новости...*")

	log.Printf("🔧 Вызов newsAggregator.FetchAllArticles()")
	articles, err := b.newsAggregator.FetchAllArticles()
	if err != nil {
		log.Printf("❌ Ошибка получения новостей: %v", err)
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Ошибка получения новостей*")
		time.Sleep(2 * time.Second)
		b.deleteMessage(processingMsg.Chat.ID, processingMsg.MessageID)
		b.sendMessage(msg.Chat.ID, "❌ Не удалось получить новости. Попробуйте позже.")
		// Возвращаем генерацию
		b.storage.AddGenerations(msg.Chat.ID, 1)
		return
	}

	log.Printf("✅ Получено статей: %d", len(articles))

	if len(articles) == 0 {
		log.Printf("❌ Нет статей после фильтрации")
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Нет свежих новостей*")
		time.Sleep(2 * time.Second)
		b.deleteMessage(processingMsg.Chat.ID, processingMsg.MessageID)
		b.sendMessage(msg.Chat.ID, "❌ Нет доступных новостей. Попробуйте позже.")
		// Возвращаем генерацию
		b.storage.AddGenerations(msg.Chat.ID, 1)
		return
	}

	// Создаем анализ
	log.Printf("🔍 Этап: Анализ контента")
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "🔍 *Анализирую контент...*")

	var analysis *analyzer.ChannelAnalysis
	if isChannel {
		log.Printf("🔧 Вызов channelAnalyzer.AnalyzeChannel для @%s", username)
		analysis, err = b.channelAnalyzer.AnalyzeChannel(ctx, username)
		if err != nil {
			log.Printf("❌ Ошибка анализа канала: %v", err)
			// Используем fallback анализ
			log.Printf("🔧 Используем fallback анализ")
			analysis = b.createAnalysisFromKeywords(username)
		} else {
			log.Printf("✅ Анализ канала успешен")
		}
	} else {
		log.Printf("🔧 Создаем анализ из ключевых слов")
		analysis = b.createAnalysisFromKeywords(keywords)
	}

	// Подбираем релевантные новости
	log.Printf("🎯 Этап: Выбор лучших новостей")
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "🎯 *Выбираю лучшие новости...*")

	log.Printf("🔧 Вызов newsAggregator.FindRelevantArticles")
	relevantArticles := b.newsAggregator.FindRelevantArticles(ctx, articles, analysis, 3)
	log.Printf("✅ Найдено релевантных статей: %d", len(relevantArticles))

	if len(relevantArticles) == 0 {
		log.Printf("❌ Нет релевантных статей")
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Нет подходящих новостей*")
		time.Sleep(2 * time.Second)
		b.deleteMessage(processingMsg.Chat.ID, processingMsg.MessageID)
		b.sendMessage(msg.Chat.ID, "❌ Не найдено релевантных новостей для вашего запроса.")
		// Возвращаем генерацию
		b.storage.AddGenerations(msg.Chat.ID, 1)
		return
	}

	// Генерируем пост
	log.Printf("✍️ Этап: Генерация поста")
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "✍️ *Генерирую пост...*")

	log.Printf("🔧 Вызов tryGeneratePost")
	generatedPost, usedArticle := b.tryGeneratePost(ctx, analysis, relevantArticles)
	log.Printf("✅ Результат генерации: post=%s, article=%+v",
		func() string {
			if generatedPost == "" {
				return "EMPTY"
			}
			return fmt.Sprintf("LEN:%d", len(generatedPost))
		}(),
		usedArticle)

	// Удаляем сообщение о процессе
	log.Printf("🗑️ Удаление сообщения о процессе")
	b.deleteMessage(processingMsg.Chat.ID, processingMsg.MessageID)

	// Отправляем результат
	if generatedPost != "" {
		var successText string
		if isChannel {
			successText = fmt.Sprintf("✅ *Пост для @%s готов!*\n\n📰 *Источник:* %s",
				username, usedArticle.Source)
		} else {
			successText = fmt.Sprintf("✅ *Пост по теме '%s' готов!*\n\n📰 *Источник:* %s",
				keywords, usedArticle.Source)
		}

		log.Printf("📤 Отправка успешного результата")
		b.sendMessage(msg.Chat.ID, successText)
		b.sendMessage(msg.Chat.ID, generatedPost)
		log.Printf("🎉 Генерация завершена успешно")
	} else {
		log.Printf("❌ Генерация не удалась")
		b.sendMessage(msg.Chat.ID,
			"❌ *Не удалось сгенерировать пост*\n\n"+
				"Попробуйте другой запрос.")
		// Возвращаем генерацию
		b.storage.AddGenerations(msg.Chat.ID, 1)
	}
}

// НОВАЯ ФУНКЦИЯ: возвращает количество секунд до возможности следующего запроса
func (b *Bot) getTimeLeftSeconds(chatID int64) int {
	lastRequest, exists := b.userLastRequest[chatID]
	if !exists {
		return 0
	}

	timePassed := time.Since(lastRequest)
	timeLeft := 30 - int(timePassed.Seconds())

	if timeLeft < 0 {
		return 0
	}
	return timeLeft
}

// ОБНОВЛЕНО: Добавлено логирование для updateRequestTime
func (b *Bot) updateRequestTime(chatID int64) {
	oldTime := b.userLastRequest[chatID]
	b.userLastRequest[chatID] = time.Now()
	log.Printf("🕒 Обновлено время запроса для %d: %v -> %v",
		chatID, oldTime.Format("15:04:05"), b.userLastRequest[chatID].Format("15:04:05"))
}

// Вспомогательные методы
func (b *Bot) createBuyMenu() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("10 генераций - 99р", "buy_10"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("25 генераций - 199р", "buy_25"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("100 генераций - 499р", "buy_100"),
		),
	)
}

func (b *Bot) createAnalysisFromKeywords(keywords string) *analyzer.ChannelAnalysis {
	log.Printf("🔧 Создание анализа из ключевых слов: %s", keywords)
	return &analyzer.ChannelAnalysis{
		GPTAnalysis: &analyzer.GPTAnalysis{
			MainTopic:    keywords,
			Subtopics:    []string{keywords},
			Keywords:     strings.Fields(keywords),
			ContentAngle: "информационный пост с практической пользой",
		},
	}
}

func (b *Bot) tryGeneratePost(ctx context.Context, analysis *analyzer.ChannelAnalysis, articles []news.Article) (string, news.Article) {
	log.Printf("🔧 tryGeneratePost: начало, статей: %d", len(articles))

	if len(articles) == 0 {
		log.Printf("❌ tryGeneratePost: нет статей")
		return "", news.Article{}
	}

	channelAnalysis := &ai.ChannelAnalysis{
		MainTopic:    analysis.GPTAnalysis.MainTopic,
		Subtopics:    analysis.GPTAnalysis.Subtopics,
		Keywords:     analysis.GPTAnalysis.Keywords,
		ContentAngle: analysis.GPTAnalysis.ContentAngle,
	}

	log.Printf("🔧 tryGeneratePost: анализ: %+v", channelAnalysis)

	for i, article := range articles {
		log.Printf("🔧 tryGeneratePost: обработка статьи %d/%d: %s", i+1, len(articles), article.Title)

		articleForAI := ai.ArticleRelevance{
			Title:   article.Title,
			Summary: article.Summary,
			URL:     article.URL,
		}

		log.Printf("🔧 tryGeneratePost: вызов gptClient.GeneratePost")
		post, err := b.gptClient.GeneratePost(ctx, channelAnalysis, articleForAI)
		if err != nil {
			log.Printf("❌ tryGeneratePost: ошибка генерации: %v", err)
			continue
		}

		log.Printf("🔧 tryGeneratePost: получен пост длиной %d", len(post))

		if !b.isRejectedPost(post) && len(strings.TrimSpace(post)) >= 100 {
			log.Printf("✅ tryGeneratePost: пост принят")
			formattedPost := b.formatPostForChannel(post, article)
			return formattedPost, article
		} else {
			log.Printf("❌ tryGeneratePost: пост отклонен - rejected: %v, length: %d",
				b.isRejectedPost(post), len(strings.TrimSpace(post)))
		}

		log.Printf("⚠️ tryGeneratePost: пост отклонен, пробуем следующую статью")
	}

	log.Printf("❌ tryGeneratePost: все статьи обработаны, подходящий пост не найден")
	return "", news.Article{}
}

func (b *Bot) formatPostForChannel(post string, article news.Article) string {
	cleanedPost := strings.TrimSpace(post)
	sourceLine := fmt.Sprintf("\n\n📰 [Новость](%s) взята с *%s*", article.URL, article.Source)
	return cleanedPost + sourceLine
}

func (b *Bot) isRejectedPost(post string) bool {
	rejectionPhrases := []string{
		"не могу обсуждать", "не могу написать", "отказываюсь", "не буду",
		"это не в моей компетенции", "давайте поговорим", "не могу помочь",
	}

	postLower := strings.ToLower(post)
	for _, phrase := range rejectionPhrases {
		if strings.Contains(postLower, strings.ToLower(phrase)) {
			return true
		}
	}
	return false
}

// sendMessage отправляет сообщение без клавиатуры
func (b *Bot) sendMessage(chatID int64, text string) tgbotapi.Message {
	log.Printf("📤 sendMessage: chatID=%d, text=%s", chatID, text)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	message, err := b.api.Send(msg)
	if err != nil {
		log.Printf("❌ Ошибка отправки сообщения: %v", err)
		return tgbotapi.Message{}
	}
	log.Printf("✅ sendMessage: сообщение отправлено, ID=%d", message.MessageID)
	return message
}

// sendMessageWithKeyboard отправляет сообщение с клавиатурой
func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, replyMarkup tgbotapi.InlineKeyboardMarkup) tgbotapi.Message {
	log.Printf("📤 sendMessageWithKeyboard: chatID=%d, text=%s", chatID, text)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = replyMarkup

	message, err := b.api.Send(msg)
	if err != nil {
		log.Printf("❌ Ошибка отправки сообщения с клавиатурой: %v", err)
		return tgbotapi.Message{}
	}
	log.Printf("✅ sendMessageWithKeyboard: сообщение отправлено, ID=%d", message.MessageID)
	return message
}

// editMessage редактирует существующее сообщение
func (b *Bot) editMessage(chatID int64, messageID int, text string) {
	log.Printf("✏️ editMessage: chatID=%d, messageID=%d, text=%s", chatID, messageID, text)

	msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("❌ Ошибка редактирования сообщения: %v", err)
	}
}

func (b *Bot) deleteMessage(chatID int64, messageID int) {
	log.Printf("🗑️ deleteMessage: chatID=%d, messageID=%d", chatID, messageID)

	msg := tgbotapi.NewDeleteMessage(chatID, messageID)
	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("❌ Ошибка удаления сообщения: %v", err)
	}
}

func (b *Bot) answerCallback(callbackID string, text string) {
	log.Printf("🔧 answerCallback: callbackID=%s, text=%s", callbackID, text)

	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := b.api.Request(callback); err != nil {
		log.Printf("❌ Ошибка ответа на callback: %v", err)
	}
}
