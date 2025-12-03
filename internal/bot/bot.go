package bot

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"AIGenerator/internal/ai"
	"AIGenerator/internal/database"
	"AIGenerator/internal/news"
	"AIGenerator/internal/payment"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api            *tgbotapi.BotAPI
	newsAggregator *news.NewsAggregator
	gptClient      *ai.YandexGPTClient
	db             *database.Database
	yooMoney       *payment.YooMoneyClient
	mu             sync.Mutex
	adminChatID    int64
}

func New(token string, newsAggregator *news.NewsAggregator, gptClient *ai.YandexGPTClient, db *database.Database, yooMoney *payment.YooMoneyClient, adminChatID int64) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания бота: %w", err)
	}

	log.Printf("[BOT] Бот @%s создан успешно", api.Self.UserName)
	return &Bot{
		api:            api,
		newsAggregator: newsAggregator,
		gptClient:      gptClient,
		db:             db,
		yooMoney:       yooMoney,
		adminChatID:    adminChatID,
	}, nil
}

func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	log.Println("[BOT] Ожидание обновлений...")

	go func() {
		<-ctx.Done()
		log.Println("[BOT] Получен сигнал завершения, останавливаю бота...")
	}()

	for update := range updates {
		if update.CallbackQuery != nil {
			go b.handleCallback(update.CallbackQuery)
			continue
		}

		if update.Message == nil {
			continue
		}

		if update.Message.IsCommand() {
			go b.handleCommand(update.Message)
			continue
		}

		if b.db.IsUserPendingFeedback(update.Message.Chat.ID) {
			go b.handleFeedbackText(update.Message)
			continue
		}

		b.sendMessage(update.Message.Chat.ID,
			"❌ Для генерации поста используйте команду /generate\n"+
				"Пример: /generate искусственный интеллект\n"+
				"Или отправьте ссылку на статью: /generate https://example.com/news\n"+
				"Подробнее: /help")
	}
}

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	b.mu.Lock()
	defer b.mu.Unlock()

	log.Printf("[COMMAND] Получена команда /%s от %d", msg.Command(), msg.Chat.ID)

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
	case "statistics":
		b.handleStatistics(msg)
	case "feedback":
		b.handleFeedbackCommand(msg)
	case "cancel":
		b.handleCancelCommand(msg)
	case "payments":
		b.handlePaymentsCommand(msg)
	case "sendmsg":
		b.handleSendMessageCommand(msg)
	case "addgenerations":
		b.handleAddGenerationsCommand(msg)
	default:
		b.sendMessage(msg.Chat.ID, "❌ Неизвестная команда. Используйте /help для списка команд.")
	}
}

func (b *Bot) handleStart(msg *tgbotapi.Message) {

	text := `🤖 AI Content Generator

Я помогу создавать качественные посты для Telegram каналов на основе актуальных новостей или по ссылке на статью.

✨ Основные команды:
/generate - создать пост по ключевым словам или ссылке
/balance - проверить баланс генераций  
/buy - приобрести дополнительные генерации
/feedback - оставить отзыв о работе бота
/help - показать справку

🎯 Для всех новых пользователей 10 бесплатных генераций!

🚀 Для генерации поста используйте:
• /generate ключевые_слова
• /generate ссылка_на_статью

⚠️ Посты на военную тематику и новости с военной тематикой не обрабатываются.

✨ Примеры:
/generate искусственный интеллект
/generate https://habr.com/ru/news/...`

	b.sendMessage(msg.Chat.ID, text)
}

func (b *Bot) handleHelp(msg *tgbotapi.Message) {
	text := `📖 Справка по командам

🎯 Основные команды:
/generate - создать пост по ключевым словам или ссылке
/balance - проверить баланс
/buy - купить генерации
/feedback - оставить отзыв о работе бота
/help - эта справка

📝 Как использовать:
• Используйте команду /generate ключевые_слова
• Или отправьте ссылку на статью: /generate https://example.com/news

✨ Примеры:
  /generate искусственный интеллект
  /generate https://example.com/ru/news/...

⚠️ Ограничения:
• Посты на военную тематику и новости с военной тематикой не обрабатываются.
• ИИ может отказаться генерировать пост на некоторые темы.
• На ваш запрос может не найтись новости в наших источниках, поэтому пост может быть не точным.
Если вы найдете новость, которую не нашел наш бот, отправьте ссылку на нее и ваш запрос в обратную связь (команда /feedback) и мы вернем вам генерацию!
Сделаем бота лучше вместе!

💎 Тарифы:
• 10 генераций - 99 руб
• 25 генераций - 199 руб  
• 100 генераций - 499 руб

⏰ Лимиты:
• Первые 10 генераций - бесплатно
• Генерация списывается только при успешном создании поста

💳 Оплата:
• Безопасная оплата через ЮKassa
• Мгновенное зачисление
• Поддержка банковских карт и электронных кошельков`

	b.sendMessage(msg.Chat.ID, text)
}

func (b *Bot) handleGenerateCommand(msg *tgbotapi.Message) {
	args := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/generate"))
	if args == "" {
		b.sendMessage(msg.Chat.ID,
			"❌ Не указаны ключевые слова или ссылка\n\n"+
				"📝 Используйте:\n"+
				"/generate ключевые слова\n"+
				"или\n"+
				"/generate https://example.com/news\n\n"+
				"✨ Примеры:\n"+
				"/generate искусственный интеллект\n"+
				"/generate https://habr.com/ru/news/...")
		return
	}

	// Проверяем, является ли аргумент ссылкой
	if b.isURL(args) {
		go b.handleGenerateFromURL(context.Background(), msg, args)
	} else {
		go b.handleGenerateFromKeywords(context.Background(), msg, args)
	}
}

// isURL проверяет, является ли строка URL
func (b *Bot) isURL(text string) bool {
	return strings.HasPrefix(text, "http://") ||
		strings.HasPrefix(text, "https://") ||
		strings.Contains(text, "://")
}

// handleGenerateFromKeywords обрабатывает генерацию по ключевым словам
func (b *Bot) handleGenerateFromKeywords(ctx context.Context, msg *tgbotapi.Message, keywords string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC] Восстановление после паники в handleGenerateFromKeywords: %v", r)
			b.sendMessage(msg.Chat.ID, "❌ Произошла внутренняя ошибка. Попробуйте позже.")
		}
	}()

	userID := msg.Chat.ID

	if keywords == "" {
		b.sendMessage(userID, "❌ Пожалуйста, укажите ключевые слова для генерации поста.\n"+
			"Пример: /generate искусственный интеллект")
		return
	}

	log.Printf("[GENERATE] Начало обработки запроса от %d: %s", userID, keywords)

	// Проверяем доступные генерации
	user := b.db.GetUser(userID)
	log.Printf("[GENERATE] Пользователь %d: доступно %d генераций", userID, user.AvailableGenerations)

	if user.AvailableGenerations <= 0 {
		text := "❌ Закончились генерации!\n\n" +
			"💎 Используйте команду /buy чтобы приобрести дополнительные генерации\n\n" +
			"✨ Доступные пакеты:\n" +
			"• 10 генераций - 99 руб\n" +
			"• 25 генераций - 199 руб\n" +
			"• 100 генераций - 499 руб"
		b.sendMessage(userID, text)
		return
	}

	// Шаг 1: Начало процесса
	step1Msg := b.sendMessage(userID, fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n⏳ Шаг 1/3: Ищу новости по теме...", keywords))

	// Шаг 2: Поиск новостей
	b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
		fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/3: ✓ Готово\n⏳ Шаг 2/3: Анализирую новости...", keywords))

	log.Printf("[GENERATE] Шаг 2/3: Поиск новостей...")

	// Получаем релевантные новости
	articles, err := b.newsAggregator.FindRelevantArticles(keywords, 5)
	if err != nil {
		log.Printf("[GENERATE] ❌ Ошибка при поиске новостей: %v", err)
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ Ошибка генерации\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Ошибка при поиске новостей", keywords))
		return
	}

	log.Printf("[GENERATE] Найдено %d статей", len(articles))

	if len(articles) == 0 {
		log.Printf("[GENERATE] ❌ Не найдено новостей по запросу: %s", keywords)
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ Новости не найдены\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Не найдено подходящих новостей по теме", keywords))
		return
	}

	// Выбираем статью с изображением, если есть
	var selectedArticle news.Article
	for _, article := range articles {
		if article.ImageURL != "" {
			selectedArticle = article
			break
		}
	}

	// Если нет статьи с изображением, берем первую
	if selectedArticle.Title == "" && len(articles) > 0 {
		selectedArticle = articles[0]
	}

	// Шаг 3: Генерация через AI
	b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
		fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/3: ✓ Готово\n✅ Шаг 2/3: ✓ Найдено %d новостей\n⏳ Шаг 3/3: Генерация поста через AI...",
			keywords, len(articles)))

	log.Printf("[GENERATE] Шаг 3/3: Выбрана статья: %s", selectedArticle.Title)

	// Генерируем пост через GPT
	articleInfo := ai.ArticleInfo{
		Title:    selectedArticle.Title,
		Summary:  selectedArticle.Summary,
		URL:      selectedArticle.URL,
		Source:   selectedArticle.Source,
		ImageURL: selectedArticle.ImageURL,
	}

	log.Printf("[GENERATE] Генерация поста через AI...")
	post, err := b.gptClient.GeneratePost(ctx, keywords, articleInfo)
	if err != nil {
		log.Printf("[GENERATE] ❌ Ошибка генерации поста для темы: %s, ошибка: %v", keywords, err)
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ Ошибка генерации\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Ошибка AI при генерации поста", keywords))
		return
	}

	// Проверяем, не отказался ли GPT
	if b.isGPTRefusal(post) {
		log.Printf("[GENERATE] ❌ GPT отказался генерировать пост для темы: %s", keywords)
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ ИИ отказался делать пост на данную тему\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: ИИ отказался обсуждать данную тему\n\n💡 Попробуйте другую тему или выберите другую новость", keywords))
		return
	}

	if strings.TrimSpace(post) == "" {
		log.Printf("[GENERATE] ❌ Получен пустой пост")
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ Ошибка генерации\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: AI вернул пустой пост", keywords))
		return
	}

	log.Printf("[GENERATE] Пост сгенерирован, длина: %d символов", len(post))

	// ТОЛЬКО ЗДЕСЬ списываем генерацию, когда все этапы успешно пройдены
	success, err := b.db.UseGeneration(userID)
	if err != nil || !success {
		log.Printf("[GENERATE] ❌ Ошибка списания генерации: %v", err)
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ Ошибка системы\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Ошибка при списании генерации", keywords))
		return
	}

	b.db.AddGeneration(userID, keywords)

	// Увеличиваем счетчик генераций для напоминания об отзыве
	b.db.IncrementGenerationsCount(userID)

	// Все шаги завершены успешно
	b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
		fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/3: ✓ Готово\n✅ Шаг 2/3: ✓ Найдено %d новостей\n✅ Шаг 3/3: ✓ Генерация завершена\n\n✨ Все этапы завершены! Отправляю результат...",
			keywords, len(articles)))

	// Отправляем результат
	user = b.db.GetUser(userID)

	// 1. Отправляем изображение прямо в пост (если есть)
	if selectedArticle.ImageURL != "" && b.isValidImageURL(selectedArticle.ImageURL) {
		// Создаем сообщение с фото и текстом
		if err := b.sendPhotoWithCaption(userID, selectedArticle.ImageURL, post); err != nil {
			log.Printf("[GENERATE] ❌ Ошибка отправки фото с текстом: %v, отправляю только текст", err)
			// Если не удалось отправить с фото, отправляем только текст
			b.sendMessageWithMarkdown(userID, post)
		} else {
			log.Printf("[GENERATE] ✅ Пост отправлен с изображением")
		}
	} else {
		// Если нет изображения, отправляем только текст
		b.sendMessageWithMarkdown(userID, post)
	}

	// 2. Отправляем метаданные отдельным сообщением
	hashtags := b.generateHashtags(selectedArticle)
	metadata := fmt.Sprintf(
		"📋 *Метаданные для поста (добавьте по желанию):*\n\n"+
			"🔖 *Рекомендуемые хештеги:*\n"+
			"%s\n\n"+
			"📰 *Источник:* [Новость](%s) взята с %s\n\n"+
			"✨ *Осталось генераций:* %d",
		hashtags,
		selectedArticle.URL,
		selectedArticle.Source,
		user.AvailableGenerations)

	b.sendMessageWithMarkdown(userID, metadata)

	// 3. Отправляем кнопки для оценки качества
	b.sendRatingRequest(userID, keywords)

	// 4. Проверяем, нужно ли напомнить об отзыве
	if b.db.ShouldRemindFeedback(userID) {
		b.sendFeedbackReminder(userID)
	}

	log.Printf("[GENERATE] ✅ Завершена обработка запроса от %d", userID)
}

// handleGenerateFromURL обрабатывает генерацию по ссылке
func (b *Bot) handleGenerateFromURL(ctx context.Context, msg *tgbotapi.Message, url string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC] Восстановление после паники в handleGenerateFromURL: %v", r)
			b.sendMessage(msg.Chat.ID, "❌ Произошла внутренняя ошибка. Попробуйте позже.")
		}
	}()

	userID := msg.Chat.ID

	log.Printf("[GENERATE] Начало обработки ссылки от %d: %s", userID, url)

	// Проверяем доступные генерации
	user := b.db.GetUser(userID)
	log.Printf("[GENERATE] Пользователь %d: доступно %d генераций", userID, user.AvailableGenerations)

	if user.AvailableGenerations <= 0 {
		text := "❌ Закончились генерации!\n\n" +
			"💎 Используйте команду /buy чтобы приобрести дополнительные генерации\n\n" +
			"✨ Доступные пакеты:\n" +
			"• 10 генераций - 99 руб\n" +
			"• 25 генераций - 199 руб\n" +
			"• 100 генераций - 499 руб"
		b.sendMessage(userID, text)
		return
	}

	// Шаг 1: Начало процесса
	step1Msg := b.sendMessage(userID, fmt.Sprintf("🔄 Генерация поста по ссылке\n\n🔗 %s\n\n⏳ Шаг 1/3: Получаю содержимое страницы...", b.truncateURL(url)))

	// Шаг 2: Получаем содержимое страницы
	b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
		fmt.Sprintf("🔄 Генерация поста по ссылке\n\n🔗 %s\n\n✅ Шаг 1/3: ✓ Готово\n⏳ Шаг 2/3: Анализирую содержимое...", b.truncateURL(url)))

	title, content, mainImage, err := b.fetchWebContent(url)
	if err != nil {
		log.Printf("[GENERATE] ❌ Ошибка получения содержимого: %v", err)
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ Ошибка генерации\n\n🔗 %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Не удалось получить содержимое страницы", b.truncateURL(url)))
		return
	}

	if title == "" {
		title = "Новость с сайта"
	}

	// Обрезаем контент до 3000 символов (чтобы не тратить много токенов)
	if len(content) > 3000 {
		content = content[:3000] + "..."
	}

	// Шаг 3: Генерация через AI
	b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
		fmt.Sprintf("🔄 Генерация поста по ссылке\n\n🔗 %s\n\n✅ Шаг 1/3: ✓ Готово\n✅ Шаг 2/3: ✓ Содержимое получено\n⏳ Шаг 3/3: Генерация поста через AI...", b.truncateURL(url)))

	log.Printf("[GENERATE] Генерация поста через AI...")
	post, err := b.gptClient.GeneratePostFromURL(ctx, title, content)
	if err != nil {
		log.Printf("[GENERATE] ❌ Ошибка генерации поста для ссылки: %s, ошибка: %v", url, err)
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ Ошибка генерации\n\n🔗 %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Ошибка AI при генерации поста", b.truncateURL(url)))
		return
	}

	// Проверяем, не отказался ли GPT
	if b.isGPTRefusal(post) {
		log.Printf("[GENERATE] ❌ GPT отказался генерировать пост для ссылки: %s", url)
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ ИИ отказался делать пост на данную тему\n\n🔗 %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: ИИ отказался обсуждать данную тему\n\n💡 Попробуйте другую ссылку", b.truncateURL(url)))
		return
	}

	if strings.TrimSpace(post) == "" {
		log.Printf("[GENERATE] ❌ Получен пустой пост")
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ Ошибка генерации\n\n🔗 %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: AI вернул пустой пост", b.truncateURL(url)))
		return
	}

	log.Printf("[GENERATE] Пост сгенерирован, длина: %d символов", len(post))

	// ТОЛЬКО ЗДЕСЬ списываем генерацию, когда все этапы успешно пройдены
	success, err := b.db.UseGeneration(userID)
	if err != nil || !success {
		log.Printf("[GENERATE] ❌ Ошибка списания генерации: %v", err)
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("❌ Ошибка системы\n\n🔗 %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Ошибка при списании генерации", b.truncateURL(url)))
		return
	}

	b.db.AddGeneration(userID, "ссылка: "+b.truncateURL(url))

	// Увеличиваем счетчик генераций для напоминания об отзыве
	b.db.IncrementGenerationsCount(userID)

	// Все шаги завершены успешно
	b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
		fmt.Sprintf("🔄 Генерация поста по ссылке\n\n🔗 %s\n\n✅ Шаг 1/3: ✓ Готово\n✅ Шаг 2/3: ✓ Содержимое получено\n✅ Шаг 3/3: ✓ Генерация завершена\n\n✨ Все этапы завершены! Отправляю результат...", b.truncateURL(url)))

	// Отправляем результат
	user = b.db.GetUser(userID)

	// 1. Отправляем изображение прямо в пост (если есть)
	if mainImage != "" && b.isValidImageURL(mainImage) {
		// Создаем сообщение с фото и текстом
		if err := b.sendPhotoWithCaption(userID, mainImage, post); err != nil {
			log.Printf("[GENERATE] ❌ Ошибка отправки фото с текстом: %v, отправляю только текст", err)
			// Если не удалось отправить с фото, отправляем только текст
			b.sendMessageWithMarkdown(userID, post)
		} else {
			log.Printf("[GENERATE] ✅ Пост отправлен с изображением")
		}
	} else {
		// Если нет изображения, отправляем только текст
		b.sendMessageWithMarkdown(userID, post)
	}

	// 2. Отправляем метаданные отдельным сообщением
	metadata := fmt.Sprintf(
		"📋 *Метаданные для поста (добавьте по желанию):*\n\n"+
			"🔖 *Рекомендуемые хештеги:*\n"+
			"#новости #интересное\n\n"+
			"📰 *Источник:* [Ссылка на статью](%s)\n\n"+
			"✨ *Осталось генераций:* %d",
		url,
		user.AvailableGenerations)

	b.sendMessageWithMarkdown(userID, metadata)

	// 3. Отправляем кнопки для оценки качества
	b.sendRatingRequest(userID, "ссылка")

	log.Printf("[GENERATE] ✅ Завершена обработка ссылки от %d", userID)
}

// sendPhotoWithCaption отправляет фото с текстом поста
func (b *Bot) sendPhotoWithCaption(chatID int64, photoURL, caption string) error {
	// Ограничение Telegram на длину подписи к фото
	maxCaptionLength := 1024
	if len(caption) > maxCaptionLength {
		caption = b.truncateText(caption, maxCaptionLength-3) + "..."
	}

	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(photoURL))
	photo.Caption = caption
	photo.ParseMode = "Markdown"

	_, err := b.api.Send(photo)
	if err != nil {
		log.Printf("[ERROR] Ошибка отправки фото: %v, URL: %s", err, photoURL)
		return err
	}

	log.Printf("[MESSAGE] Отправлено фото с подписью в чат %d", chatID)
	return nil
}

// sendDocumentWithCaption отправляет документ с подписью
func (b *Bot) sendDocumentWithCaption(chatID int64, docURL, caption string) error {
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FileURL(docURL))
	doc.Caption = caption
	doc.ParseMode = "Markdown"

	_, err := b.api.Send(doc)
	if err != nil {
		log.Printf("[ERROR] Ошибка отправки документа: %v, URL: %s", err, docURL)
		return err
	}

	return nil
}

// isValidImageURL проверяет, является ли URL валидным изображением
func (b *Bot) isValidImageURL(url string) bool {
	if url == "" {
		return false
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}

	validExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg"}
	urlLower := strings.ToLower(url)
	for _, ext := range validExtensions {
		if strings.HasSuffix(urlLower, ext) {
			return true
		}
	}

	imageIndicators := []string{"/img/", "/images/", "/photo/", "/pics/", "/assets/", "/media/", "image="}
	for _, indicator := range imageIndicators {
		if strings.Contains(urlLower, indicator) {
			return true
		}
	}

	return true
}

// fetchWebContent получает содержимое веб-страницы
func (b *Bot) fetchWebContent(url string) (string, string, string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("статус код: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", err
	}

	html := string(body)

	// Извлекаем заголовок
	titleRegex := regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
	var title string
	if matches := titleRegex.FindStringSubmatch(html); len(matches) > 1 {
		title = strings.TrimSpace(matches[1])
	}

	// Извлекаем главное изображение
	mainImage := b.extractMainImageFromHTML(html)

	// Извлекаем текст
	content := b.extractTextFromHTML(html)
	content = b.truncateText(content, 5000)

	return title, content, mainImage, nil
}

// extractMainImageFromHTML извлекает URL главного изображения из HTML страницы
func (b *Bot) extractMainImageFromHTML(html string) string {
	// Приоритет 1: Open Graph изображение
	ogImageRegex := regexp.MustCompile(`<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	if matches := ogImageRegex.FindStringSubmatch(html); len(matches) > 1 {
		return matches[1]
	}

	// Приоритет 2: Twitter изображение
	twitterImageRegex := regexp.MustCompile(`<meta[^>]+name=["']twitter:image["'][^>]+content=["']([^"']+)["']`)
	if matches := twitterImageRegex.FindStringSubmatch(html); len(matches) > 1 {
		return matches[1]
	}

	// Приоритет 3: Schema.org изображение
	schemaImageRegex := regexp.MustCompile(`<meta[^>]+itemprop=["']image["'][^>]+content=["']([^"']+)["']`)
	if matches := schemaImageRegex.FindStringSubmatch(html); len(matches) > 1 {
		return matches[1]
	}

	// Приоритет 4: Изображение в статье
	articleImgRegex := regexp.MustCompile(`<article[^>]*>.*?<img[^>]+src=["']([^"']+)["'][^>]*>`)
	if matches := articleImgRegex.FindStringSubmatch(html); len(matches) > 1 {
		return matches[1]
	}

	// Приоритет 5: Первое изображение
	firstImgRegex := regexp.MustCompile(`<img[^>]+src=["']([^"']+)["'][^>]*>`)
	if matches := firstImgRegex.FindStringSubmatch(html); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// extractTextFromHTML извлекает текст из HTML
func (b *Bot) extractTextFromHTML(html string) string {
	// Убираем теги скриптов и стилей
	html = regexp.MustCompile(`<script[^>]*>[\s\S]*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`<style[^>]*>[\s\S]*?</style>`).ReplaceAllString(html, "")

	// Убираем HTML теги
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, " ")

	// Убираем множественные пробелы и переносы строк
	html = regexp.MustCompile(`\s+`).ReplaceAllString(html, " ")

	// Берем первые 1000 слов
	words := strings.Fields(html)
	if len(words) > 1000 {
		words = words[:1000]
	}

	return strings.Join(words, " ")
}

// truncateText обрезает текст до указанной длины
func (b *Bot) truncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}

	truncated := text[:maxLength]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// truncateURL обрезает URL для отображения
func (b *Bot) truncateURL(url string) string {
	if len(url) > 50 {
		return url[:47] + "..."
	}
	return url
}

// isGPTRefusal проверяет, отказался ли GPT генерировать пост
func (b *Bot) isGPTRefusal(post string) bool {
	refusalPhrases := []string{
		"я не могу обсуждать эту тему",
		"не могу обсуждать",
		"отказываюсь обсуждать",
		"это неэтично",
		"это неприемлемо",
		"я не буду",
		"не могу создать",
		"не могу написать",
		"извините, но я не могу",
		"сожалею, но я не могу",
	}

	postLower := strings.ToLower(strings.TrimSpace(post))
	for _, phrase := range refusalPhrases {
		if strings.Contains(postLower, phrase) {
			return true
		}
	}

	return false
}

func (b *Bot) handleBuy(msg *tgbotapi.Message) {
	if b.yooMoney == nil {
		b.sendMessage(msg.Chat.ID,
			"❌ Платежная система временно недоступна\n\n"+
				"💡 Пожалуйста, попробуйте позже или свяжитесь с администратором.")
		return
	}

	pricing := b.db.GetPricing()

	text := fmt.Sprintf("💎 Приобретите дополнительные генерации\n\n"+
		"Выберите пакет:\n\n"+
		"🔹 10 генераций - %d руб.\n"+
		"🔹 25 генераций - %d руб.\n"+
		"🔹 100 генераций - %d руб.\n\n"+
		"💳 Оплата через ЮKassa\n"+
		"✨ Генерация списывается только при успешном создании поста!",
		pricing["10"], pricing["25"], pricing["100"])

	b.sendMessageWithKeyboard(msg.Chat.ID, text, b.createBuyMenu())
}

func (b *Bot) handleBalance(msg *tgbotapi.Message) {
	user := b.db.GetUser(msg.Chat.ID)

	text := fmt.Sprintf(
		"🎯 Ваш баланс\n\n"+
			"✨ Доступно генераций: %d\n"+
			"📊 Всего использовано: %d\n\n"+
			"💡 Генерация списывается только при успешном создании поста\n"+
			"💰 Используйте /buy для покупки дополнительных генераций",
		user.AvailableGenerations,
		user.TotalGenerations)

	b.sendMessage(msg.Chat.ID, text)
}

func (b *Bot) generateHashtags(article news.Article) string {
	hashtags := []string{"новости", "интересное"}

	if len(article.Tags) > 0 {
		for _, tag := range article.Tags {
			if tag != "" {
				cleanTag := strings.ToLower(strings.ReplaceAll(tag, " ", ""))
				if !contains(hashtags, cleanTag) {
					hashtags = append(hashtags, cleanTag)
				}
			}
		}
	}

	var result strings.Builder
	for i, tag := range hashtags {
		if i > 0 {
			result.WriteString(" ")
		}
		result.WriteString("#")
		result.WriteString(tag)
	}

	return result.String()
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// handleStatistics - исправленная функция статистики
func (b *Bot) handleStatistics(msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.sendMessage(msg.Chat.ID, "🔐 Введите пароль для доступа к статистике:\n/statistics пароль")
		return
	}

	stats := b.db.GetStatistics(args)
	if stats == nil {
		b.sendMessage(msg.Chat.ID, "❌ Неверный пароль")
		return
	}

	text := "📊 СТАТИСТИКА БОТА\n\n"

	// Все время
	if allTime, ok := stats["all_time"].(map[string]interface{}); ok {
		text += "⏳ ЗА ВСЕ ВРЕМЯ:\n"
		text += fmt.Sprintf("👥 Всего пользователей: %d\n", safeInt(allTime["users"]))
		text += fmt.Sprintf("🆕 Новых пользователей: %d\n", safeInt(allTime["new_users"]))
		text += fmt.Sprintf("🔄 Генераций: %d\n", safeInt(allTime["generations"]))
		text += fmt.Sprintf("💰 Покупки: 10(%d) 25(%d) 100(%d)\n",
			safeInt(allTime["purchases_10"]), safeInt(allTime["purchases_25"]), safeInt(allTime["purchases_100"]))
		text += fmt.Sprintf("💵 Прибыль: %d руб.\n\n", safeInt(allTime["total_revenue"]))
	}

	// Месяц
	if month, ok := stats["last_month"].(map[string]interface{}); ok {
		text += "📅 ЗА ПОСЛЕДНИЙ МЕСЯЦ:\n"
		text += fmt.Sprintf("👥 Всего пользователей: %d\n", safeInt(month["users"]))
		text += fmt.Sprintf("🆕 Новых пользователей: %d\n", safeInt(month["new_users"]))
		text += fmt.Sprintf("🔄 Генераций: %d\n", safeInt(month["generations"]))
		text += fmt.Sprintf("💰 Покупки: 10(%d) 25(%d) 100(%d)\n",
			safeInt(month["purchases_10"]), safeInt(month["purchases_25"]), safeInt(month["purchases_100"]))
		text += fmt.Sprintf("💵 Прибыль: %d руб.\n\n", safeInt(month["total_revenue"]))
	}

	// День
	if day, ok := stats["last_24h"].(map[string]interface{}); ok {
		text += "🌞 ЗА ПОСЛЕДНИЕ 24 ЧАСА:\n"
		text += fmt.Sprintf("👥 Всего пользователей: %d\n", safeInt(day["users"]))
		text += fmt.Sprintf("🆕 Новых пользователей: %d\n", safeInt(day["new_users"]))
		text += fmt.Sprintf("🔄 Генераций: %d\n", safeInt(day["generations"]))
		text += fmt.Sprintf("💰 Покупки: 10(%d) 25(%d) 100(%d)\n",
			safeInt(day["purchases_10"]), safeInt(day["purchases_25"]), safeInt(day["purchases_100"]))
		text += fmt.Sprintf("💵 Прибыль: %d руб.\n", safeInt(day["total_revenue"]))
	}

	// Топ темы
	topTopics := b.db.GetTopGenerationTopics(time.Time{}, time.Now(), 5)
	if len(topTopics) > 0 {
		text += "\n\n🎯 ТОП-5 ПОПУЛЯРНЫХ ТЕМ:\n"
		i := 1
		for topic, count := range topTopics {
			text += fmt.Sprintf("%d. %s - %d раз\n", i, topic, count)
			i++
			if i > 5 {
				break
			}
		}
	}

	b.sendMessage(msg.Chat.ID, text)
}

// handleSendMessageCommand - команда для отправки сообщений всем пользователям или конкретному
func (b *Bot) handleSendMessageCommand(msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.sendMessage(msg.Chat.ID, "🔐 Использование:\n"+
			"/sendmsg пароль текст_сообщения - отправить всем\n"+
			"/sendmsg пароль chatid текст_сообщения - отправить конкретному пользователю")
		return
	}

	parts := strings.Fields(args)
	if len(parts) < 2 {
		b.sendMessage(msg.Chat.ID, "❌ Недостаточно аргументов. Формат:\n"+
			"/sendmsg пароль текст_сообщения\n"+
			"или\n"+
			"/sendmsg пароль chatid текст_сообщения")
		return
	}

	// Проверяем пароль
	password := parts[0]
	adminPassword := b.getAdminPassword()

	if password != adminPassword {
		b.sendMessage(msg.Chat.ID, "❌ Неверный пароль")
		return
	}

	// Определяем, есть ли chatid
	var chatID int64
	var messageText string
	var sendToAll bool

	if len(parts) >= 3 {
		parsedChatID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			sendToAll = true
			messageText = strings.Join(parts[1:], " ")
		} else {
			chatID = parsedChatID
			messageText = strings.Join(parts[2:], " ")
		}
	} else {
		sendToAll = true
		messageText = strings.Join(parts[1:], " ")
	}

	if sendToAll {
		users := b.db.GetAllUsers()
		totalUsers := len(users)
		successCount := 0
		failCount := 0

		b.sendMessage(msg.Chat.ID, fmt.Sprintf("🔄 Начинаю рассылку сообщения для %d пользователей...", totalUsers))

		for i, userID := range users {
			err := b.sendMessageToUser(userID, messageText)
			if err != nil {
				failCount++
				log.Printf("[SENDMSG] ❌ Ошибка отправки пользователю %d: %v", userID, err)
			} else {
				successCount++
			}

			if i%10 == 0 && i > 0 {
				time.Sleep(1 * time.Second)
			}
		}

		report := fmt.Sprintf("✅ Рассылка завершена!\n\n"+
			"📊 Статистика:\n"+
			"👥 Всего пользователей: %d\n"+
			"✅ Успешно отправлено: %d\n"+
			"❌ Ошибок: %d",
			totalUsers, successCount, failCount)

		b.sendMessage(msg.Chat.ID, report)
	} else {
		err := b.sendMessageToUser(chatID, messageText)
		if err != nil {
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка отправки пользователю %d: %v", chatID, err))
		} else {
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Сообщение успешно отправлено пользователю %d", chatID))
		}
	}
}

// getAdminPassword возвращает пароль админа
func (b *Bot) getAdminPassword() string {
	adminPassword := os.Getenv("STATISTICS_PASSWORD")
	if adminPassword == "" {
		adminPassword = "admin123"
	}
	return adminPassword
}

// sendMessageToUser отправляет сообщение конкретному пользователю
func (b *Bot) sendMessageToUser(chatID int64, message string) error {
	msg := tgbotapi.NewMessage(chatID, message)
	_, err := b.api.Send(msg)
	return err
}

// handleAddGenerationsCommand - команда для добавления генераций пользователю
func (b *Bot) handleAddGenerationsCommand(msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.sendMessage(msg.Chat.ID, "🔐 Использование:\n"+
			"/addgenerations пароль chatid количество_генераций\n\n"+
			"Пример: /addgenerations admin123 123456789 10")
		return
	}

	parts := strings.Fields(args)
	if len(parts) != 3 {
		b.sendMessage(msg.Chat.ID, "❌ Неверное количество аргументов. Формат:\n"+
			"/addgenerations пароль chatid количество_генераций")
		return
	}

	// Проверяем пароль
	password := parts[0]
	adminPassword := b.getAdminPassword()

	if password != adminPassword {
		b.sendMessage(msg.Chat.ID, "❌ Неверный пароль")
		return
	}

	// Парсим chatid
	chatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		b.sendMessage(msg.Chat.ID, "❌ Неверный chatid. Должен быть числом.")
		return
	}

	// Парсим количество генераций
	count, err := strconv.Atoi(parts[2])
	if err != nil {
		b.sendMessage(msg.Chat.ID, "❌ Неверное количество генераций. Должно быть числом.")
		return
	}

	if count <= 0 {
		b.sendMessage(msg.Chat.ID, "❌ Количество генераций должно быть больше 0.")
		return
	}

	if count > 1000 {
		b.sendMessage(msg.Chat.ID, "❌ Слишком большое количество генераций. Максимум 1000.")
		return
	}

	// Добавляем генерации
	err = b.db.AddGenerations(chatID, count)
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка добавления генераций: %v", err))
		return
	}

	// Получаем обновленные данные пользователя
	user := b.db.GetUser(chatID)

	// Отправляем подтверждение админу
	b.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Пользователю %d успешно добавлено %d генераций.\n"+
		"Теперь у него доступно: %d генераций", chatID, count, user.AvailableGenerations))

	// Отправляем уведомление пользователю
	b.sendMessage(chatID, fmt.Sprintf("🎉 Администратор добавил вам %d генераций!\n\n"+
		"✨ Теперь доступно: %d генераций\n"+
		"📊 Всего использовано: %d\n\n"+
		"Спасибо за использование нашего бота! 🚀",
		count, user.AvailableGenerations, user.TotalGenerations))
}

func (b *Bot) handlePaymentsCommand(msg *tgbotapi.Message) {
	userID := msg.Chat.ID

	if b.yooMoney == nil {
		b.sendMessage(userID, "❌ Платежная система временно недоступна.")
		return
	}

	text := `💳 Управление платежами

Здесь вы можете:
• Проверить статус своих платежей
• Получить помощь по оплате
• Отменить ожидающие платежи

Для покупки генераций используйте команду /buy

📞 Если у вас возникли проблемы с оплатой, свяжитесь с администратором.`

	b.sendMessage(userID, text)
}

func (b *Bot) handleFeedbackCommand(msg *tgbotapi.Message) {
	userID := msg.Chat.ID

	b.db.SetPendingFeedback(userID, true)

	text := `📝 Оставьте отзыв о работе бота

Пожалуйста, напишите ваш отзыв, предложения или замечания по работе бота.

Ваш отзыв поможет нам стать лучше!

Если передумали, используйте команду /cancel`

	b.sendMessage(userID, text)
}

func (b *Bot) handleCancelCommand(msg *tgbotapi.Message) {
	userID := msg.Chat.ID

	if !b.db.IsUserPendingFeedback(userID) {
		b.sendMessage(userID, "❌ У вас нет активного запроса на отзыв.")
		return
	}

	b.db.SetPendingFeedback(userID, false)
	b.db.ResetGenerationsCount(userID)

	b.sendMessage(userID, "✅ Отправка отзыва отменена.")
}

func (b *Bot) handleFeedbackText(msg *tgbotapi.Message) {
	userID := msg.Chat.ID
	feedbackText := msg.Text

	if !b.db.IsUserPendingFeedback(userID) {
		return
	}

	username := "Без имени"
	if msg.From != nil && msg.From.UserName != "" {
		username = "@" + msg.From.UserName
	} else if msg.From != nil && msg.From.FirstName != "" {
		username = msg.From.FirstName
		if msg.From.LastName != "" {
			username += " " + msg.From.LastName
		}
	}

	adminMessage := fmt.Sprintf(
		"📨 *НОВЫЙ ОТЗЫВ*\n\n"+
			"👤 Пользователь: %s\n"+
			"🆔 ID: %d\n"+
			"📅 Дата: %s\n\n"+
			"💬 Отзыв:\n%s",
		username,
		userID,
		time.Now().Format("02.01.2006 15:04"),
		feedbackText)

	b.sendMessageWithMarkdown(b.adminChatID, adminMessage)

	b.db.SetPendingFeedback(userID, false)
	b.db.ResetGenerationsCount(userID)

	b.sendMessage(userID, "✅ Спасибо за ваш отзыв! Это очень ценно для нас! 🙏")
}

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	_, _ = b.api.Request(tgbotapi.NewCallback(callback.ID, ""))

	data := callback.Data

	if strings.HasPrefix(data, "buy_") {
		b.handlePurchase(callback.Message.Chat.ID, data)
	} else if strings.HasPrefix(data, "rate_") {
		b.handleRating(callback)
	} else if strings.HasPrefix(data, "check_") {
		b.handleCheckPayment(callback)
	} else if strings.HasPrefix(data, "cancel_") {
		b.handleCancelPayment(callback)
	}
}

func (b *Bot) handleRating(callback *tgbotapi.CallbackQuery) {
	userID := callback.Message.Chat.ID
	data := callback.Data

	parts := strings.SplitN(data, "_", 3)
	if len(parts) != 3 {
		return
	}

	rating, err := strconv.Atoi(parts[1])
	if err != nil || rating < 1 || rating > 5 {
		return
	}

	topic := parts[2]

	username := "Без имени"
	if callback.From != nil && callback.From.UserName != "" {
		username = "@" + callback.From.UserName
	} else if callback.From != nil && callback.From.FirstName != "" {
		username = callback.From.FirstName
		if callback.From.LastName != "" {
			username += " " + callback.From.LastName
		}
	}

	adminMessage := fmt.Sprintf(
		"⭐️ *НОВАЯ ОЦЕНКА*\n\n"+
			"👤 Пользователь: %s\n"+
			"🆔 ID: %d\n"+
			"🎯 Тема генерации: %s\n"+
			"📅 Дата: %s\n\n"+
			"⭐️ Оценка: %d/5",
		username,
		userID,
		topic,
		time.Now().Format("02.01.2006 15:04"),
		rating)

	b.sendMessageWithMarkdown(b.adminChatID, adminMessage)

	b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
		"✅ Спасибо за вашу оценку! Ваше мнение важно для нас! ⭐️")

	b.sendMessage(userID, fmt.Sprintf("✅ Спасибо за оценку %d/5! Ваше мнение помогает нам становиться лучше! 🙌", rating))
}

func (b *Bot) handlePurchase(chatID int64, packageType string) {
	if b.yooMoney == nil {
		b.sendMessage(chatID, "❌ Платежная система временно недоступна. Попробуйте позже.")
		return
	}

	var price, count int
	var description string

	switch packageType {
	case "buy_10":
		price = 99
		count = 10
		description = "Покупка 10 генераций в AI Content Generator"
	case "buy_25":
		price = 199
		count = 25
		description = "Покупка 25 генераций в AI Content Generator"
	case "buy_100":
		price = 499
		count = 100
		description = "Покупка 100 генераций в AI Content Generator"
	default:
		b.sendMessage(chatID, "❌ Неизвестный тип пакета")
		return
	}

	log.Printf("[PAYMENT] Создание платежа для пользователя %d: %s (%d руб, %d генераций)",
		chatID, packageType, price, count)

	// Создаем платеж через ЮKassa
	paymentResp, err := b.yooMoney.CreatePayment(float64(price), description, chatID, packageType, count)
	if err != nil {
		log.Printf("[PAYMENT] ❌ Ошибка создания платежа: %v", err)

		errorMsg := fmt.Sprintf("❌ Ошибка при создании платежа:\n\n%s\n\n💡 Проверьте настройки платежной системы", err.Error())
		b.sendMessage(chatID, errorMsg)
		return
	}

	// Сохраняем информацию о платеже
	purchase := &database.Purchase{
		PaymentID:   paymentResp.ID,
		UserID:      chatID,
		PackageType: packageType,
		Price:       price,
		Status:      "pending",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := b.db.AddPendingPurchase(purchase); err != nil {
		log.Printf("[PAYMENT] ❌ Ошибка сохранения платежа: %v", err)
		b.sendMessage(chatID, "❌ Ошибка при сохранении платежа в базу данных.")
		return
	}

	// Отправляем пользователю ссылку для оплаты
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("💳 Оплатить", paymentResp.Confirmation.ConfirmationURL),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Проверить оплату", fmt.Sprintf("check_%s", paymentResp.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", fmt.Sprintf("cancel_%s", paymentResp.ID)),
		),
	)

	msg := fmt.Sprintf(
		"💎 *Покупка %d генераций*\n\n"+
			"💰 Сумма: *%d руб.*\n"+
			"🎯 Количество: *%d генераций*\n\n"+
			"📋 *Для оплаты:*\n"+
			"1. Нажмите кнопку '💳 Оплатить'\n"+
			"2. Оплатите через ЮKassa\n"+
			"3. После оплаты нажмите '🔄 Проверить оплату'\n\n"+
			"⌛️ *Ссылка действительна 30 минут*\n"+
			"🆔 *ID платежа:* `%s`",
		count, price, count, paymentResp.ID)

	message := tgbotapi.NewMessage(chatID, msg)
	message.ParseMode = "Markdown"
	message.DisableWebPagePreview = true
	message.ReplyMarkup = keyboard

	if _, err := b.api.Send(message); err != nil {
		log.Printf("[PAYMENT] ❌ Ошибка отправки сообщения: %v", err)
	}

	// Запускаем проверку статуса платежа в фоне
	go b.checkPaymentStatus(chatID, paymentResp.ID)
}

// Обработчик проверки платежа
func (b *Bot) handleCheckPayment(callback *tgbotapi.CallbackQuery) {
	paymentID := strings.TrimPrefix(callback.Data, "check_")
	userID := callback.Message.Chat.ID

	// Проверяем статус платежа
	paymentResp, err := b.yooMoney.CheckPayment(paymentID)
	if err != nil {
		b.sendMessage(userID, "❌ Ошибка при проверке платежа. Попробуйте позже.")
		return
	}

	switch paymentResp.Status {
	case "succeeded":
		// Обновляем статус в базе
		b.db.UpdatePurchaseStatus(paymentID, "succeeded")

		// Получаем данные из метаданных
		packageType := paymentResp.Metadata["package_type"]
		count := paymentResp.Metadata["count"]

		var packageCode string
		var generationCount int

		// Извлекаем значения из метаданных
		if pkg, ok := packageType.(string); ok {
			packageCode = strings.TrimPrefix(pkg, "buy_")
		} else {
			packageCode = "10"
		}

		if cnt, ok := count.(float64); ok {
			generationCount = int(cnt)
		} else if cnt, ok := count.(int); ok {
			generationCount = cnt
		} else {
			generationCount = 10
		}

		// Определяем цену по пакету
		var price int
		switch packageCode {
		case "10":
			price = 99
		case "25":
			price = 199
		case "100":
			price = 499
		default:
			price = 99
		}

		// Добавляем покупку в базу
		if err := b.db.AddPurchase(userID, packageCode, price); err != nil {
			b.sendMessage(userID, "❌ Ошибка при зачислении генераций. Обратитесь к администратору.")
			return
		}

		user := b.db.GetUser(userID)

		// Редактируем сообщение
		b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
			fmt.Sprintf("✅ *Оплата успешна!*\n\n"+
				"✨ Добавлено генераций: *%d*\n"+
				"💰 Сумма: *%d руб.*\n"+
				"🎯 Теперь доступно: *%d*\n\n"+
				"Теперь вы можете использовать /generate для создания постов!",
				generationCount, price, user.AvailableGenerations))

		// Отправляем подтверждение
		b.sendMessage(userID, "🎉 Оплата прошла успешно! Генерации зачислены на ваш счет.")

	case "pending":
		b.sendMessage(userID, "⏳ Платеж еще не прошел. Попробуйте проверить позже.")

	case "canceled":
		b.db.UpdatePurchaseStatus(paymentID, "canceled")
		b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
			"❌ Платеж отменен. Если у вас есть вопросы, обратитесь к администратору.")

	default:
		b.sendMessage(userID, "⚠️ Неизвестный статус платежа: "+paymentResp.Status)
	}
}

// Обработчик отмены платежа
func (b *Bot) handleCancelPayment(callback *tgbotapi.CallbackQuery) {
	paymentID := strings.TrimPrefix(callback.Data, "cancel_")
	userID := callback.Message.Chat.ID

	// Обновляем статус в базе
	b.db.UpdatePurchaseStatus(paymentID, "canceled")

	// Редактируем сообщение
	b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
		"❌ Платеж отменен. Вы можете начать заново с помощью команды /buy")

	b.sendMessage(userID, "Платеж отменен. Если вам нужна помощь, используйте /help")
}

// Периодическая проверка статуса платежей
func (b *Bot) checkPaymentStatus(chatID int64, paymentID string) {
	time.Sleep(30 * time.Second)

	for i := 0; i < 10; i++ {
		paymentResp, err := b.yooMoney.CheckPayment(paymentID)
		if err != nil {
			log.Printf("[PAYMENT] Ошибка проверки статуса платежа %s: %v", paymentID, err)
			time.Sleep(30 * time.Second)
			continue
		}

		if paymentResp.Status == "succeeded" {
			packageType := paymentResp.Metadata["package_type"]
			count := paymentResp.Metadata["count"]

			var packageCode string
			var generationCount int

			if pkg, ok := packageType.(string); ok {
				packageCode = strings.TrimPrefix(pkg, "buy_")
			} else {
				packageCode = "10"
			}

			if cnt, ok := count.(float64); ok {
				generationCount = int(cnt)
			} else if cnt, ok := count.(int); ok {
				generationCount = cnt
			} else {
				generationCount = 10
			}

			var price int
			switch packageCode {
			case "10":
				price = 99
			case "25":
				price = 199
			case "100":
				price = 499
			default:
				price = 99
			}

			if err := b.db.AddPurchase(chatID, packageCode, price); err == nil {
				b.sendMessage(chatID,
					fmt.Sprintf("✅ Платеж прошел успешно! Зачислено %d генераций.", generationCount))
				b.db.UpdatePurchaseStatus(paymentID, "succeeded")
			}
			return
		} else if paymentResp.Status == "canceled" {
			b.db.UpdatePurchaseStatus(paymentID, "canceled")
			return
		}

		time.Sleep(30 * time.Second)
	}

	b.sendMessage(chatID,
		"⏳ Ваш платеж все еще в ожидании. Вы можете проверить статус вручную, нажав кнопку '🔄 Проверить оплату' в сообщении о покупке.")
}

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

func (b *Bot) sendRatingRequest(chatID int64, topic string) {
	text := "⭐️ Оцените качество генерации:"

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("1 ⭐", fmt.Sprintf("rate_1_%s", topic)),
			tgbotapi.NewInlineKeyboardButtonData("2 ⭐", fmt.Sprintf("rate_2_%s", topic)),
			tgbotapi.NewInlineKeyboardButtonData("3 ⭐", fmt.Sprintf("rate_3_%s", topic)),
			tgbotapi.NewInlineKeyboardButtonData("4 ⭐", fmt.Sprintf("rate_4_%s", topic)),
			tgbotapi.NewInlineKeyboardButtonData("5 ⭐", fmt.Sprintf("rate_5_%s", topic)),
		),
	)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) sendFeedbackReminder(chatID int64) {
	text := `💬 *Небольшая просьба!*

Вы уже использовали несколько генераций. Пожалуйста, помогите нам стать лучше!

Если у вас есть минутка, оставьте отзыв о работе бота командой /feedback

Ваше мнение очень важно для нас! 🙏`

	b.sendMessageWithMarkdown(chatID, text)
}

// Функция для отправки сообщений с Markdown
func (b *Bot) sendMessageWithMarkdown(chatID int64, text string) tgbotapi.Message {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	message, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Ошибка отправки сообщения с Markdown: %v", err)
		return b.sendMessage(chatID, text)
	}
	log.Printf("[MESSAGE] Отправлено сообщение с Markdown в чат %d, ID: %d", chatID, message.MessageID)
	return message
}

func (b *Bot) sendMessage(chatID int64, text string) tgbotapi.Message {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = ""
	msg.DisableWebPagePreview = true

	message, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Ошибка отправки сообщения в чат %d: %v", chatID, err)
		return tgbotapi.Message{}
	}
	log.Printf("[MESSAGE] Отправлено сообщение в чат %d, ID: %d", chatID, message.MessageID)
	return message
}

func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, replyMarkup tgbotapi.InlineKeyboardMarkup) tgbotapi.Message {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = ""
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = replyMarkup

	message, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Ошибка отправки сообщения с клавиатурой в чат %d: %v", chatID, err)
		return tgbotapi.Message{}
	}
	return message
}

func (b *Bot) editMessage(chatID int64, messageID int, text string) {
	msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	msg.ParseMode = ""
	msg.DisableWebPagePreview = true

	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Ошибка редактирования сообщения %d в чате %d: %v", messageID, chatID, err)
	}
}

func (b *Bot) deleteMessage(chatID int64, messageID int) {
	msg := tgbotapi.NewDeleteMessage(chatID, messageID)
	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Ошибка удаления сообщения %d в чате %d: %v", messageID, chatID, err)
	}
}

func safeInt(value interface{}) int {
	if value == nil {
		return 0
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
		return 0
	default:
		return 0
	}
}
