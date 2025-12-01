package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"AIGenerator/internal/ai"
	"AIGenerator/internal/database"
	"AIGenerator/internal/news"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api            *tgbotapi.BotAPI
	newsAggregator *news.NewsAggregator
	gptClient      *ai.YandexGPTClient
	db             *database.Database
	mu             sync.Mutex
}

func New(token string, newsAggregator *news.NewsAggregator, gptClient *ai.YandexGPTClient, db *database.Database) (*Bot, error) {
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
	}, nil
}

func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	log.Println("[BOT] Ожидание обновлений...")

	// Обработка контекста завершения
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

		// УБРАНО: обработка обычных текстовых сообщений
		// Теперь только команда /generate
		b.sendMessage(update.Message.Chat.ID,
			"❌ Для генерации поста используйте команду /generate\n"+
				"Пример: /generate искусственный интеллект\n"+
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
	default:
		b.sendMessage(msg.Chat.ID, "❌ Неизвестная команда. Используйте /help для списка команд.")
	}
}

func (b *Bot) handleGenerate(ctx context.Context, msg *tgbotapi.Message, keywords string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC] Восстановление после паники в handleGenerate: %v", r)
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
		b.sendMessage(userID, "❌ Закончились генерации!\n\n💎 Используйте команду /buy чтобы приобрести дополнительные генерации")
		return
	}

	// Используем одну генерацию
	success, err := b.db.UseGeneration(userID)
	if err != nil || !success {
		log.Printf("[GENERATE] Ошибка использования генерации: %v", err)
		b.sendMessage(userID, "❌ Ошибка системы. Попробуйте позже.")
		return
	}

	log.Printf("[GENERATE] Генерация использована, осталось: %d", user.AvailableGenerations-1)

	// Шаг 1: Начало процесса - ОТПРАВЛЯЕМ СООБЩЕНИЕ НАВСЕГДА
	step1Msg := b.sendMessage(userID, fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n⏳ Шаг 1/4: Проверяю доступные генерации...", keywords))
	log.Printf("[GENERATE] Отправлено первое сообщение, ID: %d", step1Msg.MessageID)

	// Шаг 2: Поиск новостей
	if step1Msg.MessageID > 0 {
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/4: ✓ Готово\n⏳ Шаг 2/4: Ищу новости по теме...", keywords))
	} else {
		// Если первое сообщение не отправилось, отправляем новое
		step1Msg = b.sendMessage(userID, fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/4: ✓ Готово\n⏳ Шаг 2/4: Ищу новости по теме...", keywords))
	}

	log.Printf("[GENERATE] Шаг 2/4: Поиск новостей...")

	// Получаем релевантные новости
	articles, err := b.newsAggregator.FindRelevantArticles(keywords, 3)
	if err != nil {
		log.Printf("[GENERATE] ❌ Ошибка при поиске новостей: %v", err)
		if step1Msg.MessageID > 0 {
			b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
				fmt.Sprintf("❌ Ошибка генерации\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Ошибка при поиске новостей", keywords))
		} else {
			b.sendMessage(userID, fmt.Sprintf("❌ Ошибка генерации\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Ошибка при поиске новостей", keywords))
		}
		// Возвращаем генерацию
		b.db.AddGenerations(userID, 1)
		return
	}

	log.Printf("[GENERATE] Найдено %d статей", len(articles))

	if len(articles) == 0 {
		log.Printf("[GENERATE] ❌ Не найдено новостей по запросу: %s", keywords)
		if step1Msg.MessageID > 0 {
			b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
				fmt.Sprintf("❌ Новости не найдены\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Не найдено подходящих новостей по теме", keywords))
		} else {
			b.sendMessage(userID, fmt.Sprintf("❌ Новости не найдены\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Не найдено подходящих новостей по теме", keywords))
		}
		// Возвращаем генерацию
		b.db.AddGenerations(userID, 1)
		return
	}

	// Шаг 3: Новости найдены
	if step1Msg.MessageID > 0 {
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/4: ✓ Готово\n✅ Шаг 2/4: ✓ Найдено %d новостей\n⏳ Шаг 3/4: Выбираю лучшую статью...", keywords, len(articles)))
	} else {
		step1Msg = b.sendMessage(userID, fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/4: ✓ Готово\n✅ Шаг 2/4: ✓ Найдено %d новостей\n⏳ Шаг 3/4: Выбираю лучшую статью...", keywords, len(articles)))
	}

	log.Printf("[GENERATE] Шаг 3/4: Выбрана статья: %s", articles[0].Title)

	// Генерируем пост через GPT
	article := articles[0]
	articleInfo := ai.ArticleInfo{
		Title:   article.Title,
		Summary: article.Summary,
		URL:     article.URL,
		Source:  article.Source,
	}

	// Шаг 4: Генерация через AI
	if step1Msg.MessageID > 0 {
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/4: ✓ Готово\n✅ Шаг 2/4: ✓ Найдено %d новостей\n✅ Шаг 3/4: ✓ Статья выбрана\n⏳ Шаг 4/4: Генерация поста через AI...", keywords, len(articles)))
	} else {
		step1Msg = b.sendMessage(userID, fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/4: ✓ Готово\n✅ Шаг 2/4: ✓ Найдено %d новостей\n✅ Шаг 3/4: ✓ Статья выбрана\n⏳ Шаг 4/4: Генерация поста через AI...", keywords, len(articles)))
	}

	log.Printf("[GENERATE] Шаг 4/4: Генерация поста через AI...")
	post, err := b.gptClient.GeneratePost(ctx, keywords, articleInfo)
	if err != nil {
		log.Printf("[GENERATE] ❌ Ошибка генерации поста для темы: %s, ошибка: %v", keywords, err)
		if step1Msg.MessageID > 0 {
			b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
				fmt.Sprintf("❌ Ошибка генерации\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Ошибка AI при генерации поста", keywords))
		} else {
			b.sendMessage(userID, fmt.Sprintf("❌ Ошибка генерации\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: Ошибка AI при генерации поста", keywords))
		}
		// Возвращаем генерацию
		b.db.AddGenerations(userID, 1)
		return
	}

	if strings.TrimSpace(post) == "" {
		log.Printf("[GENERATE] ❌ Получен пустой пост")
		if step1Msg.MessageID > 0 {
			b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
				fmt.Sprintf("❌ Ошибка генерации\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: AI вернул пустой пост", keywords))
		} else {
			b.sendMessage(userID, fmt.Sprintf("❌ Ошибка генерации\n\n🎯 Тема: %s\n\n⏹️ Процесс остановлен\n\n📛 Причина: AI вернул пустой пост", keywords))
		}
		// Возвращаем генерацию
		b.db.AddGenerations(userID, 1)
		return
	}

	log.Printf("[GENERATE] Шаг 4/4: Пост сгенерирован, длина: %d символов", len(post))

	// Все шаги завершены успешно
	if step1Msg.MessageID > 0 {
		b.editMessage(step1Msg.Chat.ID, step1Msg.MessageID,
			fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/4: ✓ Готово\n✅ Шаг 2/4: ✓ Найдено %d новостей\n✅ Шаг 3/4: ✓ Статья выбрана\n✅ Шаг 4/4: ✓ Пост сгенерирован\n\n✨ Все этапы завершены! Отправляю результат...", keywords, len(articles)))
	} else {
		b.sendMessage(userID, fmt.Sprintf("🔄 Генерация поста начата\n\n🎯 Тема: %s\n\n✅ Шаг 1/4: ✓ Готово\n✅ Шаг 2/4: ✓ Найдено %d новостей\n✅ Шаг 3/4: ✓ Статья выбрана\n✅ Шаг 4/4: ✓ Пост сгенерирован\n\n✨ Все этапы завершены! Отправляю результат...", keywords, len(articles)))
	}

	// Логируем успех
	log.Printf("[GENERATE] ✅ Успешная генерация поста для темы: %s, источник: %s, ссылка: %s",
		keywords, article.Source, article.URL)

	// Отправляем результат
	user = b.db.GetUser(userID)
	successText := fmt.Sprintf(
		"✅ Пост готов!\n\n"+
			"🎯 Тема: %s\n"+
			"📰 Источник: %s\n"+
			"🔗 Ссылка: %s\n"+
			"✨ Осталось генераций: %d\n\n"+
			"📋 Сгенерированный пост:",
		keywords, article.Source, article.URL, user.AvailableGenerations)

	b.sendMessage(userID, successText)
	b.sendMessage(userID, post)
	log.Printf("[GENERATE] ✅ Завершена обработка запроса от %d", userID)
}

func (b *Bot) handleStart(msg *tgbotapi.Message) {
	user := b.db.GetUser(msg.Chat.ID)

	text := fmt.Sprintf(`🤖 AI Content Generator

Я помогу создавать качественные посты для Telegram каналов на основе актуальных новостей.

✨ Основные команды:
/generate - создать пост по ключевым словам
/balance - проверить баланс генераций  
/buy - приобрести дополнительные генерации
/help - показать справку

🎯 У вас есть %d бесплатных генераций!

🚀 Для генерации поста используйте команду /generate ключевые_слова
Пример: /generate искусственный интеллект`, user.AvailableGenerations)

	b.sendMessage(msg.Chat.ID, text)
}

func (b *Bot) handleHelp(msg *tgbotapi.Message) {
	text := `📖 Справка по командам

🎯 Основные команды:
/generate - создать пост по ключевым словам
/balance - проверить баланс
/buy - купить генерации
/help - эта справка

📝 Как использовать:
• Используйте команду /generate ключевые_слова
• Примеры:
  /generate искусственный интеллект
  /generate программирование
  /generate новые технологии

💎 Тарифы:
• 10 генераций - 99 руб
• 25 генераций - 199 руб  
• 100 генераций - 499 руб

⏰ Лимиты:
• Первые 10 генераций - бесплатно`

	b.sendMessage(msg.Chat.ID, text)
}

func (b *Bot) handleGenerateCommand(msg *tgbotapi.Message) {
	args := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/generate"))
	if args == "" {
		b.sendMessage(msg.Chat.ID,
			"❌ Не указаны ключевые слова\n\n"+
				"📝 Используйте:\n"+
				"/generate ключевые слова\n\n"+
				"✨ Примеры:\n"+
				"/generate искусственный интеллект\n"+
				"/generate новые технологии")
		return
	}

	go b.handleGenerate(context.Background(), msg, args)
}

func (b *Bot) handleBuy(msg *tgbotapi.Message) {
	pricing := b.db.GetPricing()

	text := fmt.Sprintf("💎 Приобретите дополнительные генерации\n\n"+
		"Выберите пакет:\n\n"+
		"🔹 10 генераций - %d руб.\n"+
		"🔹 25 генераций - %d руб.\n"+
		"🔹 100 генераций - %d руб.\n\n"+
		"💡 Генерации будут добавлены мгновенно!",
		pricing["10"], pricing["25"], pricing["100"])

	b.sendMessageWithKeyboard(msg.Chat.ID, text, b.createBuyMenu())
}

func (b *Bot) handleBalance(msg *tgbotapi.Message) {
	user := b.db.GetUser(msg.Chat.ID)

	text := fmt.Sprintf(
		"🎯 Ваш баланс\n\n"+
			"✨ Доступно генераций: %d\n"+
			"📊 Всего использовано: %d\n\n"+
			"💡 Используйте /buy для покупки дополнительных генераций",
		user.AvailableGenerations,
		user.TotalGenerations)

	b.sendMessage(msg.Chat.ID, text)
}

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
		text += fmt.Sprintf("👥 Пользователей: %d (%d новых)\n",
			safeInt(allTime["users"]), safeInt(allTime["new_users"]))
		text += fmt.Sprintf("🚀 Генераций: %d\n", safeInt(allTime["generates"]))
		text += fmt.Sprintf("💰 Покупки: 10(%d) 25(%d) 100(%d)\n",
			safeInt(allTime["purchases_10"]), safeInt(allTime["purchases_25"]), safeInt(allTime["purchases_100"]))
		text += fmt.Sprintf("💵 Прибыль: %d руб.\n\n", safeInt(allTime["total_revenue"]))
	}

	// Месяц
	if month, ok := stats["last_month"].(map[string]interface{}); ok {
		text += "📅 ЗА ПОСЛЕДНИЙ МЕСЯЦ:\n"
		text += fmt.Sprintf("👥 Пользователей: %d (%d новых)\n",
			safeInt(month["users"]), safeInt(month["new_users"]))
		text += fmt.Sprintf("🚀 Генераций: %d\n", safeInt(month["generates"]))
		text += fmt.Sprintf("💰 Покупки: 10(%d) 25(%d) 100(%d)\n",
			safeInt(month["purchases_10"]), safeInt(month["purchases_25"]), safeInt(month["purchases_100"]))
		text += fmt.Sprintf("💵 Прибыль: %d руб.\n\n", safeInt(month["total_revenue"]))
	}

	// День
	if day, ok := stats["last_24h"].(map[string]interface{}); ok {
		text += "🌞 ЗА ПОСЛЕДНИЕ 24 ЧАСА:\n"
		text += fmt.Sprintf("👥 Пользователей: %d (%d новых)\n",
			safeInt(day["users"]), safeInt(day["new_users"]))
		text += fmt.Sprintf("🚀 Генераций: %d\n", safeInt(day["generates"]))
		text += fmt.Sprintf("💰 Покупки: 10(%d) 25(%d) 100(%d)\n",
			safeInt(day["purchases_10"]), safeInt(day["purchases_25"]), safeInt(day["purchases_100"]))
		text += fmt.Sprintf("💵 Прибыль: %d руб.", safeInt(day["total_revenue"]))
	}

	b.sendMessage(msg.Chat.ID, text)
}

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	// Отвечаем на callback
	_, _ = b.api.Request(tgbotapi.NewCallback(callback.ID, ""))

	if strings.HasPrefix(callback.Data, "buy_") {
		b.handlePurchase(callback.Message.Chat.ID, callback.Data)
	}
}

func (b *Bot) handlePurchase(chatID int64, packageType string) {
	var price, count int

	switch packageType {
	case "buy_10":
		price = 99
		count = 10
	case "buy_25":
		price = 199
		count = 25
	case "buy_100":
		price = 499
		count = 100
	default:
		b.sendMessage(chatID, "❌ Неизвестный тип пакета")
		return
	}

	// Добавляем покупку
	packageCode := strings.TrimPrefix(packageType, "buy_")
	if err := b.db.AddPurchase(chatID, packageCode, price); err != nil {
		b.sendMessage(chatID, "❌ Ошибка при обработке покупки")
		return
	}

	user := b.db.GetUser(chatID)
	text := fmt.Sprintf(
		"✅ Покупка успешна!\n\n"+
			"✨ Добавлено генераций: %d\n"+
			"💰 Стоимость: %d руб.\n"+
			"🎯 Теперь доступно: %d\n\n"+
			"Теперь вы можете использовать /generate для создания постов!",
		count, price, user.AvailableGenerations)

	b.sendMessage(chatID, text)
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
