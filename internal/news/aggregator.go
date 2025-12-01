package news

import (
	"log"
	"sort"
	"strings"
	"time"
)

// NewsAggregator управляет сбором и фильтрацией новостей
type NewsAggregator struct {
	sources []NewsSource
}

// NewNewsAggregator создает новый агрегатор новостей
func NewNewsAggregator() *NewsAggregator {
	return &NewsAggregator{
		sources: make([]NewsSource, 0),
	}
}

// AddDefaultSources добавляет источники новостей по умолчанию
func (na *NewsAggregator) AddDefaultSources() {
	defaultSources := GetDefaultSources()
	for _, source := range defaultSources {
		na.sources = append(na.sources, &source)
	}
	log.Printf("[NEWS] Добавлено %d источников новостей", len(defaultSources))
}

// FindRelevantArticles находит релевантные статьи по ключевым словам и категориям
func (na *NewsAggregator) FindRelevantArticles(keywords, category, subcategory string, maxArticles int) ([]Article, error) {
	log.Printf("[NEWS] Поиск новостей по теме: %s (Категория: %s/%s)", keywords, category, subcategory)

	// Получаем все статьи из релевантных источников
	allArticles, err := na.FetchArticlesByCategory(category, subcategory)
	if err != nil {
		log.Printf("[NEWS] Ошибка получения статей: %v", err)
		return nil, err
	}

	log.Printf("[NEWS] Получено %d статей из категории %s/%s", len(allArticles), category, subcategory)

	if len(allArticles) == 0 {
		log.Printf("[NEWS] ⚠️ Не получено ни одной статьи из категории, ищем во всех источниках")
		allArticles, err = na.FetchAllArticles()
		if err != nil {
			return nil, err
		}
	}

	// Фильтруем военные темы
	articles := na.FilterOutMilitaryTopics(allArticles)
	log.Printf("[NEWS] После фильтрации осталось %d статей", len(articles))

	if len(articles) == 0 {
		log.Printf("[NEWS] Нет статей после фильтрации")
		return []Article{}, nil
	}

	// Создаем структуру для сортировки
	type scoredArticle struct {
		article Article
		score   float64
	}

	var scoredArticles []scoredArticle
	keywordsLower := strings.ToLower(keywords)
	keywordList := strings.Fields(keywordsLower)

	log.Printf("[NEWS] Ключевые слова для поиска: %v", keywordList)

	// Оцениваем каждую статью
	for _, article := range articles {
		score := na.calculateRelevance(article, keywordList, category, subcategory)
		if score > 0 {
			scoredArticles = append(scoredArticles, scoredArticle{
				article: article,
				score:   score,
			})
		}
	}

	log.Printf("[NEWS] Найдено %d статей с релевантностью > 0", len(scoredArticles))

	if len(scoredArticles) == 0 {
		log.Printf("[NEWS] Нет релевантных статей")
		return []Article{}, nil
	}

	// Сортируем по релевантности
	sort.Slice(scoredArticles, func(i, j int) bool {
		return scoredArticles[i].score > scoredArticles[j].score
	})

	// Берем топ статей
	var result []Article
	for i := 0; i < len(scoredArticles) && i < maxArticles; i++ {
		result = append(result, scoredArticles[i].article)
		log.Printf("[NEWS] Статья %d: %s (релевантность: %.2f)",
			i+1, scoredArticles[i].article.Title, scoredArticles[i].score)
	}

	log.Printf("[NEWS] Найдено %d релевантных статей по теме: %s", len(result), keywords)
	return result, nil
}

// FetchArticlesByCategory собирает статьи из определенной категории
func (na *NewsAggregator) FetchArticlesByCategory(category, subcategory string) ([]Article, error) {
	var allArticles []Article

	for _, source := range na.sources {
		// Проверяем, подходит ли источник под категорию
		if rssSource, ok := source.(*RSSSource); ok {
			if category != "" && rssSource.Category != category {
				continue
			}
			if subcategory != "" && rssSource.Subcategory != subcategory {
				continue
			}
		}

		log.Printf("[NEWS] Получение статей из %s", source.GetName())
		articles, err := source.FetchArticles()
		if err != nil {
			log.Printf("[NEWS] ❌ Ошибка получения статей из %s: %v", source.GetName(), err)
			continue
		}
		log.Printf("[NEWS] Получено %d статей из %s", len(articles), source.GetName())
		allArticles = append(allArticles, articles...)
	}

	log.Printf("[NEWS] Итого собрано %d статей из категории %s/%s", len(allArticles), category, subcategory)
	return allArticles, nil
}

// FetchAllArticles собирает статьи со всех источников
func (na *NewsAggregator) FetchAllArticles() ([]Article, error) {
	var allArticles []Article

	for _, source := range na.sources {
		log.Printf("[NEWS] Получение статей из %s", source.GetName())
		articles, err := source.FetchArticles()
		if err != nil {
			log.Printf("[NEWS] ❌ Ошибка получения статей из %s: %v", source.GetName(), err)
			continue
		}
		log.Printf("[NEWS] Получено %d статей из %s", len(articles), source.GetName())
		allArticles = append(allArticles, articles...)
	}

	log.Printf("[NEWS] Итого собрано %d статей", len(allArticles))
	return allArticles, nil
}

// calculateRelevance вычисляет релевантность статьи (0-100)
func (na *NewsAggregator) calculateRelevance(article Article, keywords []string, targetCategory, targetSubcategory string) float64 {
	score := 0.0
	text := strings.ToLower(article.Title + " " + article.Summary)

	// 1. Совпадение ключевых слов (40%)
	keywordScore := 0.0
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			keywordScore += 1.0
		}
	}
	if len(keywords) > 0 {
		keywordScore = (keywordScore / float64(len(keywords))) * 40.0
	}
	score += keywordScore

	// 2. Совпадение категорий (30%)
	categoryScore := 0.0
	if targetCategory != "" && article.Category == targetCategory {
		categoryScore += 15.0
	}
	if targetSubcategory != "" && article.Subcategory == targetSubcategory {
		categoryScore += 15.0
	}
	score += categoryScore

	// 3. Свежесть (20%)
	if !article.PublishedAt.IsZero() {
		hoursSincePublished := time.Since(article.PublishedAt).Hours()
		if hoursSincePublished < 6 {
			score += 20.0
		} else if hoursSincePublished < 12 {
			score += 15.0
		} else if hoursSincePublished < 24 {
			score += 10.0
		} else if hoursSincePublished < 48 {
			score += 5.0
		}
	}

	// 4. Качество статьи (10%)
	qualityScore := na.calculateArticleQuality(article)
	score += qualityScore

	return score
}

// calculateArticleQuality оценивает качество статьи
func (na *NewsAggregator) calculateArticleQuality(article Article) float64 {
	score := 0.0

	// Проверка длины
	titleLength := len(article.Title)
	summaryLength := len(article.Summary)

	if titleLength > 20 && titleLength < 120 {
		score += 3.0 // Оптимальная длина заголовка
	}

	if summaryLength > 200 && summaryLength < 1000 {
		score += 5.0 // Оптимальная длина описания
	} else if summaryLength >= 1000 {
		score += 3.0 // Длинное, но может быть полезно
	}

	// Проверка наличия ключевых элементов
	text := strings.ToLower(article.Title + " " + article.Summary)

	if strings.Contains(text, "эксперт") || strings.Contains(text, "аналитик") {
		score += 1.0
	}
	if strings.Contains(text, "данные") || strings.Contains(text, "исследование") {
		score += 1.0
	}
	if strings.Contains(text, "новый") || strings.Contains(text, "впервые") {
		score += 1.0
	}

	// Ограничиваем максимальный балл качества
	if score > 10.0 {
		score = 10.0
	}

	return score
}

// FilterOutMilitaryTopics фильтрует военные темы
func (na *NewsAggregator) FilterOutMilitaryTopics(articles []Article) []Article {
	var filtered []Article
	militaryKeywords := []string{
		"война", "воен", "боев", "оруж", "атака", "конфликт", "наступление",
		"оборона", "спецоперация", "ВСУ", "ВС РФ", "минобороны", "погиб",
		"ранен", "обстрел", "взрыв", "снаряд", "танк", "артиллерия", "залп",
	}

	for _, article := range articles {
		if !na.containsMilitaryTopics(article, militaryKeywords) {
			filtered = append(filtered, article)
		}
	}

	return filtered
}

func (na *NewsAggregator) containsMilitaryTopics(article Article, keywords []string) bool {
	text := strings.ToLower(article.Title + " " + article.Summary)

	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}

	return false
}
