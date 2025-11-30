package news

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"AIGenerator/internal/ai"
	"AIGenerator/internal/analyzer"
	"AIGenerator/internal/categories"
)

// NewsAggregator управляет сбором и фильтрацией новостей
type NewsAggregator struct {
	sources         []NewsSource
	gptClient       *ai.YandexGPTClient
	lastSourceIndex int
	categorySystem  *categories.Category
}

// NewNewsAggregator создает новый агрегатор новостей
func NewNewsAggregator(gptClient *ai.YandexGPTClient) *NewsAggregator {
	return &NewsAggregator{
		sources:         make([]NewsSource, 0),
		gptClient:       gptClient,
		lastSourceIndex: -1,
	}
}

// AddDefaultSources добавляет источники новостей по умолчанию
func (na *NewsAggregator) AddDefaultSources() {
	defaultSources := GetDefaultSources()
	for _, source := range defaultSources {
		na.sources = append(na.sources, &source)
	}
	log.Printf("✅ Добавлено %d источников новостей", len(defaultSources))
}

// FetchAllArticles собирает статьи со всех источников
func (na *NewsAggregator) FetchAllArticles() ([]Article, error) {
	var allArticles []Article

	for _, source := range na.sources {
		articles, err := source.FetchArticles()
		if err != nil {
			log.Printf("⚠️ Ошибка получения статей из %s: %v", source.GetName(), err)
			continue
		}
		allArticles = append(allArticles, articles...)
	}

	// Фильтруем военные темы
	filteredArticles := na.FilterOutMilitaryTopics(allArticles)

	log.Printf("✅ Собрано %d статей (после фильтрации)", len(filteredArticles))
	return filteredArticles, nil
}

// FindRelevantArticlesForKeywords находит релевантные статьи по ключевым словам
func (na *NewsAggregator) FindRelevantArticlesForKeywords(ctx context.Context, articles []Article, keywords string, maxArticles int) []Article {
	if len(articles) == 0 {
		return []Article{}
	}

	// Создаем искусственный анализ для ключевых слов
	analysis := &analyzer.ChannelAnalysis{
		GPTAnalysis: &analyzer.GPTAnalysis{
			MainTopic: keywords,
			Keywords:  strings.Fields(keywords),
		},
	}

	return na.FindRelevantArticles(ctx, articles, analysis, maxArticles)
}

// FilterOutMilitaryTopics фильтрует военные темы из статей
func (na *NewsAggregator) FilterOutMilitaryTopics(articles []Article) []Article {
	var filtered []Article

	militaryKeywords := []string{
		"война", "воен", "боев", "оруж", "атака", "конфликт", "наступление",
		"оборона", "спецоперация", "ВСУ", "ВС РФ", "минобороны", "погиб",
		"ранен", "обстрел", "взрыв", "снаряд", "танк", "артиллерия",
		"авиация", "фронт", "пленных", "удар", "контрнаступление", "ЗСУ",
		"боеприпас", "мина", "ракета", "дрон", "БПЛА", "кадыров", "пригожин",
		"чвк", "мобилизация", "призыв", "окоп", "позиция", "штурм",
	}

	for _, article := range articles {
		if !na.containsMilitaryTopics(article, militaryKeywords) {
			filtered = append(filtered, article)
		}
	}

	return filtered
}

// containsMilitaryTopics проверяет статью на военную тематику
func (na *NewsAggregator) containsMilitaryTopics(article Article, keywords []string) bool {
	text := strings.ToLower(article.Title + " " + article.Summary)

	for _, keyword := range keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}

// FindRelevantArticles улучшенная версия с точными категориями
func (na *NewsAggregator) FindRelevantArticles(ctx context.Context, articles []Article, analysis *analyzer.ChannelAnalysis, maxArticles int) []Article {
	if len(articles) == 0 {
		return []Article{}
	}

	// Определяем категорию канала
	channelCategory := na.determineChannelCategory(analysis)
	log.Printf("🎯 Категория канала: %s", channelCategory)

	// Рассчитываем релевантность для всех статей
	for i := range articles {
		articles[i].Relevance = na.CalculatePreciseRelevance(articles[i], analysis, channelCategory)
	}

	// Сортируем по релевантности
	sort.Slice(articles, func(i, j int) bool {
		return articles[i].Relevance > articles[j].Relevance
	})

	// Фильтруем только релевантные статьи (релевантность > 0.6)
	var relevantArticles []Article
	for _, article := range articles {
		if article.Relevance > 0.6 {
			relevantArticles = append(relevantArticles, article)
		}
		if len(relevantArticles) >= maxArticles*2 {
			break
		}
	}

	// Если релевантных статей мало, добавляем менее релевантные
	if len(relevantArticles) < maxArticles {
		for _, article := range articles {
			if article.Relevance > 0.4 && len(relevantArticles) < maxArticles*2 {
				// Проверяем, нет ли уже этой статьи
				found := false
				for _, relArticle := range relevantArticles {
					if relArticle.URL == article.URL {
						found = true
						break
					}
				}
				if !found {
					relevantArticles = append(relevantArticles, article)
				}
			}
		}
	}

	// Выбираем лучшие статьи из разных источников
	result := na.selectDiverseArticles(relevantArticles, maxArticles)

	log.Printf("🎯 Итоговый выбор: %d статей (из %d релевантных)", len(result), len(relevantArticles))
	return result
}

// determineChannelCategory определяет категорию канала
func (na *NewsAggregator) determineChannelCategory(analysis *analyzer.ChannelAnalysis) string {
	if analysis == nil || analysis.GPTAnalysis == nil {
		return "Общее"
	}

	text := strings.ToLower(analysis.GPTAnalysis.MainTopic + " " +
		strings.Join(analysis.GPTAnalysis.Subtopics, " ") + " " +
		strings.Join(analysis.GPTAnalysis.Keywords, " "))

	categories := categories.GetCategories()
	bestCategory := "Общее"
	maxScore := 0

	for categoryName, category := range categories {
		score := 0
		// Проверяем основную тему
		if strings.Contains(text, strings.ToLower(categoryName)) {
			score += 10
		}

		// Проверяем ключевые слова категории
		for _, keyword := range category.Keywords {
			if strings.Contains(text, strings.ToLower(keyword)) {
				score += 2
			}
		}

		// Проверяем подтемы
		for _, subtopic := range category.Subtopics {
			if strings.Contains(text, strings.ToLower(subtopic)) {
				score += 3
			}
		}

		if score > maxScore {
			maxScore = score
			bestCategory = categoryName
		}
	}

	return bestCategory
}

// CalculatePreciseRelevance вычисляет точную релевантность на основе категорий
func (na *NewsAggregator) CalculatePreciseRelevance(article Article, analysis *analyzer.ChannelAnalysis, channelCategory string) float64 {
	if analysis == nil || analysis.GPTAnalysis == nil {
		return 0.3
	}

	var relevance float64
	text := strings.ToLower(article.Title + " " + article.Summary)

	// Получаем категорию статьи
	articleCategory := categories.DetectCategory(text)

	// БОЛЬШОЙ бонус за совпадение категорий
	if articleCategory == channelCategory {
		relevance += 0.5
		log.Printf("✅ Совпадение категорий: %s == %s", articleCategory, channelCategory)
	}

	// Проверяем ключевые слова из анализа канала
	keywordMatches := 0
	for _, keyword := range analysis.GPTAnalysis.Keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			keywordMatches++
			relevance += 0.15
		}
	}

	// Проверяем основную тему
	if strings.Contains(text, strings.ToLower(analysis.GPTAnalysis.MainTopic)) {
		relevance += 0.3
	}

	// Проверяем подтемы
	for _, subtopic := range analysis.GPTAnalysis.Subtopics {
		if strings.Contains(text, strings.ToLower(subtopic)) {
			relevance += 0.1
		}
	}

	// Учитываем свежесть
	hoursSincePublished := time.Since(article.PublishedAt).Hours()
	if hoursSincePublished < 6 {
		relevance += 0.2
	} else if hoursSincePublished < 12 {
		relevance += 0.15
	} else if hoursSincePublished < 24 {
		relevance += 0.1
	}

	// Бонус за предпочтительные источники для категории
	if cat, exists := categories.GetCategory(channelCategory); exists {
		for _, source := range cat.Sources {
			if article.Source == source {
				relevance += 0.1
				break
			}
		}
	}

	// Ограничиваем максимальную релевантность
	if relevance > 1.0 {
		relevance = 1.0
	}

	return relevance
}

// selectDiverseArticles выбирает статьи из разных источников
func (na *NewsAggregator) selectDiverseArticles(articles []Article, maxArticles int) []Article {
	var result []Article
	usedSources := make(map[string]bool)

	// Сначала берем самые релевантные из разных источников
	for _, article := range articles {
		if len(result) >= maxArticles {
			break
		}
		if !usedSources[article.Source] {
			result = append(result, article)
			usedSources[article.Source] = true
			log.Printf("✅ Выбрана статья из %s: %s (релевантность: %.2f)",
				article.Source, article.Title, article.Relevance)
		}
	}

	// Если не набрали достаточно, добавляем самые релевантные независимо от источника
	if len(result) < maxArticles {
		for _, article := range articles {
			if len(result) >= maxArticles {
				break
			}
			// Проверяем, не добавили ли мы уже эту статью
			alreadyAdded := false
			for _, addedArticle := range result {
				if addedArticle.URL == article.URL {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				result = append(result, article)
			}
		}
	}

	return result
}

// GenerateContentIdeas генерирует идеи для контента
func (na *NewsAggregator) GenerateContentIdeas(articles []Article, analysis *analyzer.ChannelAnalysis) []string {
	var ideas []string

	for i, article := range articles {
		idea := fmt.Sprintf("💡 **Идея %d/%d**\n\n📰 *%s*\n\n🎯 *Угол для вашего канала:*\n%s\n\n🔗 %s",
			i+1, len(articles),
			article.Title,
			na.generateChannelAngle(article, analysis),
			article.URL)
		ideas = append(ideas, idea)
	}

	return ideas
}

// generateChannelAngle создает уникальный угол подачи
func (na *NewsAggregator) generateChannelAngle(article Article, analysis *analyzer.ChannelAnalysis) string {
	if analysis.GPTAnalysis == nil {
		return "Практический подход с пользой для аудитории"
	}

	if analysis.GPTAnalysis.ContentAngle != "" {
		return analysis.GPTAnalysis.ContentAngle
	}

	return "Практический подход с пользой для аудитории"
}
