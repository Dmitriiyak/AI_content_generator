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
)

// NewsAggregator управляет сбором и фильтрацией новостей
type NewsAggregator struct {
	sources   []NewsSource
	gptClient *ai.YandexGPTClient
}

// NewNewsAggregator создает новый агрегатор новостей
func NewNewsAggregator(gptClient *ai.YandexGPTClient) *NewsAggregator {
	return &NewsAggregator{
		sources:   make([]NewsSource, 0),
		gptClient: gptClient,
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
		log.Printf("📥 Получено %d статей из %s", len(articles), source.GetName())
	}

	log.Printf("✅ Всего собрано %d статей", len(allArticles))
	return allArticles, nil
}

// FindRelevantArticles улучшенная версия с AI-подбором
func (na *NewsAggregator) FindRelevantArticles(ctx context.Context, articles []Article, analysis *analyzer.ChannelAnalysis, maxArticles int) []Article {
	if analysis == nil || analysis.GPTAnalysis == nil || na.gptClient == nil {
		log.Printf("⚠️ AI-анализ недоступен, используем базовую фильтрацию")
		return na.findRelevantBasic(articles, analysis, maxArticles)
	}

	return na.findRelevantWithAI(ctx, articles, analysis, maxArticles)
}

// findRelevantWithAI интеллектуальный подбор новостей через AI
func (na *NewsAggregator) findRelevantWithAI(ctx context.Context, articles []Article, analysis *analyzer.ChannelAnalysis, maxArticles int) []Article {
	// Конвертируем данные в формат для AI
	channelAnalysis := na.convertAnalysisForAI(analysis)
	articlesForAI := na.convertArticlesForAI(articles)

	if len(articlesForAI) == 0 {
		log.Printf("⚠️ Нет свежих новостей для AI-подбора")
		return []Article{}
	}

	if channelAnalysis == nil {
		log.Printf("⚠️ Нет анализа канала для AI-подбора")
		return na.findRelevantBasic(articles, analysis, maxArticles)
	}

	// Используем AI для подбора новостей
	relevantNews, err := na.gptClient.SelectRelevantNews(ctx, channelAnalysis, articlesForAI, maxArticles)
	if err != nil {
		log.Printf("⚠️ AI-подбор не удался, используем базовую фильтрацию: %v", err)
		return na.findRelevantBasic(articles, analysis, maxArticles)
	}

	// Сопоставляем выбранные AI новости с исходными статьями
	var result []Article
	for _, newsItem := range relevantNews {
		for _, originalArticle := range articles {
			if originalArticle.URL == newsItem.Article.URL {
				originalArticle.Relevance = newsItem.Relevance
				result = append(result, originalArticle)
				break
			}
		}

		if len(result) >= maxArticles {
			break
		}
	}

	log.Printf("🎯 AI-подбор: выбрано %d релевантных новостей из %d", len(result), len(articles))

	// Сортируем по релевантности
	na.sortArticlesByRelevance(result)

	return result
}

// convertAnalysisForAI конвертирует анализ канала в формат для AI
func (na *NewsAggregator) convertAnalysisForAI(analysis *analyzer.ChannelAnalysis) *ai.ChannelAnalysis {
	if analysis == nil || analysis.GPTAnalysis == nil {
		return nil
	}

	return &ai.ChannelAnalysis{
		MainTopic:      analysis.GPTAnalysis.MainTopic,
		Subtopics:      analysis.GPTAnalysis.Subtopics,
		TargetAudience: analysis.GPTAnalysis.TargetAudience.AgeRange,
		ContentStyle:   na.formatContentStyle(analysis.GPTAnalysis.ContentStyle),
		Keywords:       analysis.GPTAnalysis.Keywords,
		ContentAngle:   analysis.GPTAnalysis.ContentAngle,
	}
}

// formatContentStyle форматирует стиль контента для промпта
func (na *NewsAggregator) formatContentStyle(style analyzer.ContentStyle) string {
	return fmt.Sprintf("Формальность: %d/10, Профессионализм: %d/10, Развлекательность: %d/10",
		style.Formality, style.Professionalism, style.Entertainment)
}

// convertArticlesForAI конвертирует статьи в формат для AI
func (na *NewsAggregator) convertArticlesForAI(articles []Article) []ai.ArticleRelevance {
	var result []ai.ArticleRelevance
	for _, article := range articles {
		// Пропускаем старые новости
		if time.Since(article.PublishedAt) > 48*time.Hour {
			continue
		}

		result = append(result, ai.ArticleRelevance{
			Title:   article.Title,
			Summary: article.Summary,
			URL:     article.URL,
		})
	}
	return result
}

// findRelevantBasic базовая фильтрация (fallback)
func (na *NewsAggregator) findRelevantBasic(articles []Article, analysis *analyzer.ChannelAnalysis, maxArticles int) []Article {
	var relevantArticles []Article

	for _, article := range articles {
		// Пропускаем старые новости
		if time.Since(article.PublishedAt) > 72*time.Hour {
			continue
		}

		relevance := na.calculateBasicRelevance(article, analysis)
		if relevance > 0.3 {
			article.Relevance = relevance
			relevantArticles = append(relevantArticles, article)
		}
	}

	na.sortArticlesByRelevance(relevantArticles)

	if len(relevantArticles) > maxArticles {
		relevantArticles = relevantArticles[:maxArticles]
	}

	return relevantArticles
}

// calculateBasicRelevance вычисляет базовую релевантность
func (na *NewsAggregator) calculateBasicRelevance(article Article, analysis *analyzer.ChannelAnalysis) float64 {
	if analysis == nil || analysis.GPTAnalysis == nil {
		return 0.5 // Средняя релевантность если анализа нет
	}

	var relevance float64
	text := strings.ToLower(article.Title + " " + article.Summary)

	// Проверяем ключевые слова из AI-анализа
	for _, keyword := range analysis.GPTAnalysis.Keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			relevance += 0.2
		}
	}

	// Проверяем основную тему
	if strings.Contains(text, strings.ToLower(analysis.GPTAnalysis.MainTopic)) {
		relevance += 0.3
	}

	// Учитываем свежесть
	hoursSincePublished := time.Since(article.PublishedAt).Hours()
	if hoursSincePublished < 24 {
		relevance += 0.3
	} else if hoursSincePublished < 48 {
		relevance += 0.1
	}

	return min(relevance, 1.0)
}

// sortArticlesByRelevance сортирует статьи по релевантности
func (na *NewsAggregator) sortArticlesByRelevance(articles []Article) {
	sort.Slice(articles, func(i, j int) bool {
		return articles[i].Relevance > articles[j].Relevance
	})
}

// GenerateContentIdeas улучшенная генерация идей
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

// generateChannelAngle создает уникальный угол подачи для конкретного канала
func (na *NewsAggregator) generateChannelAngle(article Article, analysis *analyzer.ChannelAnalysis) string {
	if analysis.GPTAnalysis == nil {
		return na.generateBasicDiscussionPrompt(article)
	}

	// Используем content_angle из AI-анализа если доступен
	if analysis.GPTAnalysis.ContentAngle != "" {
		return analysis.GPTAnalysis.ContentAngle
	}

	// Выбираем угол в зависимости от стиля контента
	if analysis.GPTAnalysis.ContentStyle.Professionalism >= 7 {
		return "Аналитический подход с экспертным мнением"
	} else if analysis.GPTAnalysis.ContentStyle.Entertainment >= 6 {
		return "Интерактивный и вовлекающий стиль"
	}

	return "Практический подход с пользой для аудитории"
}

// generateBasicDiscussionPrompt создает базовый промпт для обсуждения
func (na *NewsAggregator) generateBasicDiscussionPrompt(article Article) string {
	return fmt.Sprintf("Обсудите эту новость с вашей аудиторией. Какие мысли и мнения у вас возникают по этому поводу? %s", article.Title)
}

// Вспомогательная функция
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
