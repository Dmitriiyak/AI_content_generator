package bot

import (
	"context"
	"fmt"
	"log"
	"strings"

	"AIGenerator/internal/ai"
	"AIGenerator/internal/analyzer"
	"AIGenerator/internal/news"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot представляет Telegram бота
type Bot struct {
	api             *tgbotapi.BotAPI
	channelAnalyzer *analyzer.ChannelAnalyzer
	newsAggregator  *news.NewsAggregator
	gptClient       *ai.YandexGPTClient
}

// New создает нового бота
func New(token string, analyzer *analyzer.ChannelAnalyzer, newsAggregator *news.NewsAggregator, gptClient *ai.YandexGPTClient) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания бота: %w", err)
	}

	return &Bot{
		api:             api,
		channelAnalyzer: analyzer,
		newsAggregator:  newsAggregator,
		gptClient:       gptClient,
	}, nil
}

// Start запускает бота
func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	log.Printf("Бот запущен: @%s", b.api.Self.UserName)

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
				b.sendMessage(update.Message.Chat.ID, "Неизвестная команда. Используйте /help для списка команд.")
			}
		}
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
"generate @example" - создать пост для канала @example

Я проанализирую канал, подберу релевантную новость и сгенерирую готовый пост в стиле вашего канала!

ВАЖНО: Бот не предлагает посты на военную тематику!`

	b.sendMessage(msg.Chat.ID, welcomeText)
}

// handleHelp обрабатывает команду /help
func (b *Bot) handleHelp(msg *tgbotapi.Message) {
	helpText := `📖 *Справка по командам*

*/start* - начать работу с ботом
*/help* - показать эту справку  
*/generate <канал>* - создать пост для указанного канала

🔧 *Как использовать /generate:*
Формат: /generate @username

🤖 *Что делает бот:*
1. Анализирует стиль и тематику вашего канала
2. Подбирает самую релевантную новость
3. Генерирует готовый пост в вашем стиле
4. Возвращает оформленный текст для публикации

⚠️ *Важно:* Убедитесь, что канал публичный и доступен для анализа.`

	b.sendMessage(msg.Chat.ID, helpText)
}

// handleGenerate обрабатывает команду /generate
func (b *Bot) handleGenerate(ctx context.Context, msg *tgbotapi.Message) {
	// Проверяем формат команды
	args := strings.Fields(msg.Text)
	if len(args) < 2 {
		b.sendMessage(msg.Chat.ID, "❌ *Неверный формат команды*\n\nИспользуйте: `/generate @username`\nПример: `/generate @tproger`")
		return
	}

	channelUsername := args[1]

	// Проверяем формат username
	if !strings.HasPrefix(channelUsername, "@") {
		b.sendMessage(msg.Chat.ID, "❌ *Неверный формат username*\n\nКанал должен начинаться с @\nПример: `/generate @tproger`")
		return
	}

	// Убираем @ для анализа
	username := strings.TrimPrefix(channelUsername, "@")

	if username == "" {
		b.sendMessage(msg.Chat.ID, "❌ *Не указан username канала*\n\nПример: `/generate @tproger`")
		return
	}

	// Отправляем сообщение о начале обработки
	processingMsg := b.sendMessage(msg.Chat.ID, "🔄 *Начинаем анализ...*\n\nАнализирую канал, подбираю новости и генерирую пост. Это займет 1-2 минуты.")

	// 1. Анализируем канал
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "🔍 *Анализирую канал...*")

	analysis, err := b.channelAnalyzer.AnalyzeChannel(ctx, username)
	if err != nil {
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Ошибка анализа канала*\n\nУбедитесь, что канал существует и является публичным.")
		return
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

	// 3. Фильтруем военные темы и подбираем релевантные новости
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "🎯 *Подбираю релевантные новости...*")

	relevantArticles := b.newsAggregator.FindRelevantArticles(ctx, articles, analysis, 3) // Берем больше для резерва

	if len(relevantArticles) == 0 {
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Не найдено релевантных новостей*\n\nВозможные причины:\n• Все новости отфильтрованы (военные темы)\n• Нет подходящих новостей для тематики канала\n• Попробуйте другой канал или повторите позже")
		return
	}

	// 4. Генерируем пост
	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "✍️ *Генерирую пост...*")

	// Конвертируем анализ для AI
	channelAnalysis := &ai.ChannelAnalysis{
		MainTopic:      analysis.GPTAnalysis.MainTopic,
		Subtopics:      analysis.GPTAnalysis.Subtopics,
		TargetAudience: analysis.GPTAnalysis.TargetAudience.AgeRange,
		ContentStyle:   fmt.Sprintf("Формальность: %d/10", analysis.GPTAnalysis.ContentStyle.Formality),
		Keywords:       analysis.GPTAnalysis.Keywords,
		ContentAngle:   analysis.GPTAnalysis.ContentAngle,
	}

	// Пробуем сгенерировать пост для каждой релевантной новости пока не получится
	var generatedPost string
	var usedArticle news.Article

	for i, article := range relevantArticles {
		articleForAI := ai.ArticleRelevance{
			Title:   article.Title,
			Summary: article.Summary,
			URL:     article.URL,
		}

		post, err := b.gptClient.GeneratePost(ctx, channelAnalysis, articleForAI)
		if err != nil {
			log.Printf("⚠️ Ошибка генерации поста для новости %d: %v", i+1, err)
			continue
		}

		// Проверяем что пост не содержит отказ
		if !b.isRejectedPost(post) {
			generatedPost = post
			usedArticle = article
			break
		} else {
			log.Printf("⚠️ AI отказался генерировать пост для новости: %s", article.Title)
		}
	}

	if generatedPost == "" {
		b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, "❌ *Не удалось сгенерировать пост*\n\nYandexGPT отказался обрабатывать все подобранные новости. Это может быть связано с:\n• Ограничениями контент-политики\n• Слишком сложными темами\n• Попробуйте позже или другой канал")
		return
	}

	// 5. Отправляем готовый пост
	resultText := fmt.Sprintf("✅ *Пост для %s готов!*\n\n%s\n\n📊 *Детали:*\n- Канал: %s\n- Тема: %s\n- Релевантность новости: %.2f/1.0\n- Источник: %s",
		channelUsername,
		generatedPost,
		analysis.ChannelInfo.Title,
		analysis.GPTAnalysis.MainTopic,
		usedArticle.Relevance,
		usedArticle.Source,
	)

	b.editMessage(processingMsg.Chat.ID, processingMsg.MessageID, resultText)
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
