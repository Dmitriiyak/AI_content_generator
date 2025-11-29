package analyzer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gotd/td/tg"
)

// ChannelAnalyzer отвечает за анализ Telegram-каналов
type ChannelAnalyzer struct {
	client *tg.Client
}

// NewChannelAnalyzer создает новый экземпляр анализатора
func NewChannelAnalyzer(client *tg.Client) *ChannelAnalyzer {
	return &ChannelAnalyzer{
		client: client,
	}
}

// AnalyzeChannel анализирует канал по username
func (ca *ChannelAnalyzer) AnalyzeChannel(ctx context.Context, username string) (*ChannelAnalysis, error) {
	log.Printf("Начинаем анализ канала: @%s", username)

	// Получаем базовую информацию о канале
	channelInfo, err := ca.getChannelInfo(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения информации о канале: %w", err)
	}

	// Получаем историю сообщений
	messages, err := ca.getChannelMessages(ctx, username, 50)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения сообщений: %w", err)
	}

	// Анализируем сообщения
	analysis := &ChannelAnalysis{
		ChannelInfo: *channelInfo,
		Messages:    messages,
	}

	// Определяем темы канала
	analysis.Topics = ca.analyzeTopics(messages)

	// Анализируем форматы постов
	analysis.PostFormats = ca.analyzePostFormats(messages)

	// Определяем лучшее время для постинга
	analysis.BestPostTime = ca.analyzeBestPostTime(messages)

	// Обновляем статистику на основе реальных данных
	ca.updateChannelStats(analysis)

	log.Printf("✅ Анализ канала @%s завершен. Найдено %d сообщений, %d тем",
		username, len(messages), len(analysis.Topics))

	return analysis, nil
}

// getChannelInfo получает базовую информацию о канале
func (ca *ChannelAnalyzer) getChannelInfo(ctx context.Context, username string) (*ChannelInfo, error) {
	// Разрешаем username в канал
	resolved, err := ca.client.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: username,
	})
	if err != nil {
		return nil, fmt.Errorf("не удалось найти канал @%s: %w", username, err)
	}

	if len(resolved.Chats) == 0 {
		return nil, fmt.Errorf("канал @%s не найден", username)
	}

	chat := resolved.Chats[0]
	channel, ok := chat.(*tg.Channel)
	if !ok {
		return nil, fmt.Errorf("@%s не является каналом", username)
	}

	info := &ChannelInfo{
		ID:           channel.ID,
		Title:        channel.Title,
		Username:     username,
		Participants: 0, // Будем устанавливать после получения полной информации
	}

	// Получаем полную информацию о канале
	fullChannel, err := ca.client.ChannelsGetFullChannel(ctx, &tg.InputChannel{
		ChannelID:  channel.ID,
		AccessHash: channel.AccessHash,
	})
	if err != nil {
		log.Printf("⚠️ Не удалось получить полную информацию о канале: %v", err)
		// Используем базовое количество участников если полная информация недоступна
		info.Participants = channel.ParticipantsCount
		return info, nil
	}

	// Извлекаем информацию из полного описания канала
	if channelFull, ok := fullChannel.FullChat.(*tg.ChannelFull); ok {
		info.Description = channelFull.About

		// ИСПРАВЛЕНИЕ: Правильное получение количества участников
		// У каналов есть ParticipantsCount, у чатов - количество участников может быть в другом поле
		info.Participants = channelFull.ParticipantsCount

		// Если в ChannelFull нет данных, используем данные из базового канала
		if info.Participants == 0 {
			info.Participants = channel.ParticipantsCount
		}
	}

	return info, nil
}

// getChannelMessages получает историю сообщений канала
func (ca *ChannelAnalyzer) getChannelMessages(ctx context.Context, username string, limit int) ([]Message, error) {
	// Получаем информацию о канале
	resolved, err := ca.client.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: username,
	})
	if err != nil {
		return nil, err
	}

	if len(resolved.Chats) == 0 {
		return nil, fmt.Errorf("канал @%s не найден", username)
	}

	channel, ok := resolved.Chats[0].(*tg.Channel)
	if !ok {
		return nil, fmt.Errorf("@%s не является каналом", username)
	}

	// Получаем историю сообщений
	result, err := ca.client.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer: &tg.InputPeerChannel{
			ChannelID:  channel.ID,
			AccessHash: channel.AccessHash,
		},
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}

	var messages []Message

	// Обрабатываем разные типы ответов
	switch resp := result.(type) {
	case *tg.MessagesChannelMessages:
		for _, msg := range resp.Messages {
			message, err := ca.convertMessage(msg)
			if err != nil {
				continue
			}
			if message != nil {
				messages = append(messages, *message)
			}
		}
	case *tg.MessagesMessages:
		for _, msg := range resp.Messages {
			message, err := ca.convertMessage(msg)
			if err != nil {
				continue
			}
			if message != nil {
				messages = append(messages, *message)
			}
		}
	default:
		return nil, fmt.Errorf("неподдерживаемый тип ответа: %T", result)
	}

	return messages, nil
}

// convertMessage преобразует сообщение Telegram в нашу структуру
func (ca *ChannelAnalyzer) convertMessage(msg tg.MessageClass) (*Message, error) {
	message, ok := msg.(*tg.Message)
	if !ok {
		return nil, fmt.Errorf("неподдерживаемый тип сообщения: %T", msg)
	}

	// Пропускаем сообщения без текста
	if message.Message == "" {
		return nil, nil
	}

	msgModel := &Message{
		ID:    message.ID,
		Text:  message.Message,
		Views: message.Views,
		Date:  time.Unix(int64(message.Date), 0),
	}

	// ИСПРАВЛЕНИЕ: Правильный подсчёт реакций
	// В gotd/td реакции могут быть представлены по-разному
	msgModel.Reactions = ca.calculateTotalReactions(message)

	// Определяем тип медиа
	if message.Media != nil {
		msgModel.MediaType = ca.getMediaType(message.Media)
	}

	return msgModel, nil
}

// calculateTotalReactions подсчитывает общее количество реакций
func (ca *ChannelAnalyzer) calculateTotalReactions(message *tg.Message) int {
	totalReactions := 0

	// Проверяем непосредственно поле Results вместо всей структуры
	if message.Reactions.Results != nil {
		// Суммируем количество всех реакций
		for _, result := range message.Reactions.Results {
			totalReactions += result.Count
		}
	}

	return totalReactions
}

// getMediaType определяет тип медиа в сообщении
func (ca *ChannelAnalyzer) getMediaType(media tg.MessageMediaClass) string {
	switch media.(type) {
	case *tg.MessageMediaPhoto:
		return "photo"
	case *tg.MessageMediaDocument:
		return "document"
	case *tg.MessageMediaWebPage:
		return "webpage"
	default:
		return "text"
	}
}

// updateChannelStats обновляет статистику канала на основе реальных данных
func (ca *ChannelAnalyzer) updateChannelStats(analysis *ChannelAnalysis) {
	if len(analysis.Messages) == 0 {
		return
	}

	totalViews := 0
	totalReactions := 0
	messageCount := len(analysis.Messages)

	for _, msg := range analysis.Messages {
		totalViews += msg.Views
		totalReactions += msg.Reactions
	}

	// Обновляем средние значения в ChannelInfo
	analysis.ChannelInfo.MessagesCount = messageCount
	analysis.ChannelInfo.AvgViews = float64(totalViews) / float64(messageCount)
	analysis.ChannelInfo.AvgReactions = float64(totalReactions) / float64(messageCount)

	// Рассчитываем engagement rate (очень упрощённо)
	if analysis.ChannelInfo.Participants > 0 {
		analysis.ChannelInfo.EngagementRate = (analysis.ChannelInfo.AvgViews / float64(analysis.ChannelInfo.Participants)) * 100
	}
}

// analyzeTopics анализирует темы канала на основе сообщений
func (ca *ChannelAnalyzer) analyzeTopics(messages []Message) []string {
	if len(messages) == 0 {
		return []string{"общее"}
	}

	topicKeywords := map[string][]string{
		"технологии":  {"технологии", "гаджеты", "смартфон", "ai", "искусственный интеллект", "робот", "it", "программирование", "софт", "приложение"},
		"новости":     {"новости", "события", "происшествия", "политика", "объявление", "сегодня", "вчера"},
		"бизнес":      {"бизнес", "стартап", "инвестиции", "компания", "рынок", "деньги", "экономика", "продажи"},
		"образование": {"образование", "учеба", "курсы", "обучение", "знания", "университет", "школа", "студент"},
		"развлечения": {"кино", "музыка", "игры", "юмор", "развлечения", "сериал", "фильм", "артист"},
		"спорт":       {"спорт", "футбол", "хоккей", "матч", "игра", "чемпионат", "победа", "соревнования"},
	}

	topicCount := make(map[string]int)
	totalMessages := len(messages)

	for _, msg := range messages {
		text := strings.ToLower(msg.Text)

		for topic, keywords := range topicKeywords {
			for _, keyword := range keywords {
				if strings.Contains(text, keyword) {
					topicCount[topic]++
					break
				}
			}
		}
	}

	var topics []string
	for topic, count := range topicCount {
		if float64(count)/float64(totalMessages) > 0.1 {
			topics = append(topics, topic)
		}
	}

	if len(topics) == 0 {
		topics = []string{"общее"}
	}

	if len(topics) > 3 {
		topics = topics[:3]
	}

	return topics
}

// analyzePostFormats анализирует форматы постов
func (ca *ChannelAnalyzer) analyzePostFormats(messages []Message) []string {
	if len(messages) == 0 {
		return []string{"новость", "обсуждение", "анонс"}
	}

	formatCount := make(map[string]int)

	for _, msg := range messages {
		format := ca.classifyPostFormat(msg)
		formatCount[format]++
	}

	var formats []string
	for i := 0; i < 3 && len(formatCount) > 0; i++ {
		maxFormat := ""
		maxCount := 0

		for format, count := range formatCount {
			if count > maxCount {
				maxCount = count
				maxFormat = format
			}
		}

		if maxFormat != "" {
			formats = append(formats, maxFormat)
			delete(formatCount, maxFormat)
		}
	}

	return formats
}

// classifyPostFormat определяет формат поста
func (ca *ChannelAnalyzer) classifyPostFormat(msg Message) string {
	text := strings.ToLower(msg.Text)

	switch {
	case strings.Contains(text, "?"):
		return "вопрос"
	case len(text) < 100:
		return "анонс"
	case strings.Contains(text, "новость") || strings.Contains(text, "событие"):
		return "новость"
	case strings.Contains(text, "мнение") || strings.Contains(text, "думаю"):
		return "мнение"
	case msg.MediaType != "text":
		return "медиа"
	default:
		return "обсуждение"
	}
}

// analyzeBestPostTime анализирует лучшее время для публикации
func (ca *ChannelAnalyzer) analyzeBestPostTime(messages []Message) []int {
	if len(messages) == 0 {
		return []int{10, 14, 18}
	}

	hourCount := make(map[int]int)

	for _, msg := range messages {
		hour := msg.Date.Hour()
		engagement := msg.Views + msg.Reactions*10
		hourCount[hour] += engagement
	}

	var bestHours []int
	for i := 0; i < 3 && len(hourCount) > 0; i++ {
		maxHour := -1
		maxEngagement := 0

		for hour, engagement := range hourCount {
			if engagement > maxEngagement {
				maxEngagement = engagement
				maxHour = hour
			}
		}

		if maxHour != -1 {
			bestHours = append(bestHours, maxHour)
			delete(hourCount, maxHour)
		}
	}

	defaultHours := []int{10, 14, 18}
	for i := len(bestHours); i < 3; i++ {
		bestHours = append(bestHours, defaultHours[i])
	}

	return bestHours
}
