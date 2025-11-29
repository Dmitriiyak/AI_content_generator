package news

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// NewNewsAggregator создает новый агрегатор новостей
func NewNewsAggregator() *NewsAggregator {
	return &NewsAggregator{
		sources: make([]NewsSource, 0),
	}
}

// AddSource добавляет источник новостей
func (na *NewsAggregator) AddSource(source NewsSource) {
	na.sources = append(na.sources, source)
}

// AddDefaultSources добавляет популярные RSS-ленты
func (na *NewsAggregator) AddDefaultSources() {
	for _, rssSource := range GetDefaultSources() {
		na.AddSource(&rssSource)
	}
}

// FetchAllArticles собирает статьи со всех источников
func (na *NewsAggregator) FetchAllArticles() ([]Article, error) {
	var allArticles []Article
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, source := range na.sources {
		wg.Add(1)
		go func(s NewsSource) {
			defer wg.Done()

			articles, err := s.FetchArticles()
			if err != nil {
				log.Printf("⚠️ Ошибка получения новостей из %s: %v", s.GetName(), err)
				return
			}

			mu.Lock()
			allArticles = append(allArticles, articles...)
			mu.Unlock()

			log.Printf("✅ Получено %d новостей из %s", len(articles), s.GetName())
		}(source)
	}

	wg.Wait()

	log.Printf("📰 Всего собрано новостей: %d", len(allArticles))
	return allArticles, nil
}

// FindRelevantArticles находит статьи, релевантные тематике канала
func (na *NewsAggregator) FindRelevantArticles(articles []Article, channelTopics []string, maxArticles int) []Article {
	var relevantArticles []Article

	for _, article := range articles {
		relevance := calculateRelevance(article, channelTopics)
		if relevance > 0.3 { // порог релевантности
			article.Relevance = relevance
			relevantArticles = append(relevantArticles, article)
		}
	}

	// Сортируем по релевантности и свежести
	sortArticlesByRelevance(relevantArticles)

	// Ограничиваем количество
	if len(relevantArticles) > maxArticles {
		relevantArticles = relevantArticles[:maxArticles]
	}

	log.Printf("🎯 Найдено релевантных новостей: %d", len(relevantArticles))
	return relevantArticles
}

// calculateRelevance вычисляет релевантность статьи для канала
func calculateRelevance(article Article, channelTopics []string) float64 {
	var relevance float64

	// Ключевые слова для каждой темы
	topicKeywords := map[string][]string{
		"технологии":  {"технологии", "гаджет", "смартфон", "ai", "искусственный интеллект", "робот", "it", "программирование", "софт"},
		"новости":     {"новость", "событие", "происшествие", "политика", "объявление"},
		"бизнес":      {"бизнес", "стартап", "инвестиция", "компания", "рынок", "деньги", "экономика"},
		"образование": {"образование", "учеба", "курс", "обучение", "знание", "университет", "школа"},
		"развлечения": {"кино", "музыка", "игра", "юмор", "развлечение", "сериал", "фильм"},
		"спорт":       {"спорт", "футбол", "хоккей", "матч", "игра", "чемпионат", "победа"},
	}

	text := strings.ToLower(article.Title + " " + article.Summary)

	for _, topic := range channelTopics {
		if keywords, exists := topicKeywords[topic]; exists {
			for _, keyword := range keywords {
				if strings.Contains(text, keyword) {
					relevance += 0.2
					break // достаточно одного ключевого слова на тему
				}
			}
		}
	}

	// Учитываем свежесть статьи
	hoursSincePublished := time.Since(article.PublishedAt).Hours()
	if hoursSincePublished < 24 {
		relevance += 0.3
	} else if hoursSincePublished < 48 {
		relevance += 0.1
	}

	return relevance
}

// sortArticlesByRelevance сортирует статьи по релевантности
func sortArticlesByRelevance(articles []Article) {
	for i := 0; i < len(articles)-1; i++ {
		for j := i + 1; j < len(articles); j++ {
			// Сначала по релевантности, потом по свежести
			if articles[i].Relevance < articles[j].Relevance ||
				(articles[i].Relevance == articles[j].Relevance &&
					articles[i].PublishedAt.Before(articles[j].PublishedAt)) {
				articles[i], articles[j] = articles[j], articles[i]
			}
		}
	}
}

// GenerateContentIdeas создает идеи для контента на основе новостей
func (na *NewsAggregator) GenerateContentIdeas(articles []Article, channelName string) []string {
	var ideas []string

	for _, article := range articles {
		idea := fmt.Sprintf("📰 %s\n\n%s\n\n🔗 %s",
			article.Title,
			generateDiscussionPrompt(article),
			article.URL)
		ideas = append(ideas, idea)
	}

	return ideas
}

// generateDiscussionPrompt создает промпт для обсуждения новости
func generateDiscussionPrompt(article Article) string {
	prompts := []string{
		"Что вы думаете об этой новости?",
		"Как это повлияет на нашу отрасль?",
		"Ваши прогнозы на этот счет?",
		"Сталкивались ли вы с подобным?",
		"Какие возможности это открывает?",
	}

	// Простая логика выбора промпта на основе категории
	promptIndex := len(article.Category) % len(prompts)
	return prompts[promptIndex]
}
