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

// FindRelevantArticles находит релевантные статьи по ключевым словам
func (na *NewsAggregator) FindRelevantArticles(keywords string, maxArticles int) ([]Article, error) {
	log.Printf("[NEWS] Поиск новостей по теме: %s", keywords)

	// Получаем все статьи
	allArticles, err := na.FetchAllArticles()
	if err != nil {
		log.Printf("[NEWS] Ошибка получения статей: %v", err)
		return nil, err
	}

	log.Printf("[NEWS] Получено %d статей", len(allArticles))

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
		score   int
	}

	var scoredArticles []scoredArticle
	keywordsLower := strings.ToLower(keywords)
	keywordList := strings.Fields(keywordsLower)

	// Оцениваем каждую статью
	for _, article := range articles {
		score := na.calculateRelevance(article, keywordList)
		if score > 0 {
			scoredArticles = append(scoredArticles, scoredArticle{
				article: article,
				score:   score,
			})
		}
	}

	// Сортируем по релевантности
	sort.Slice(scoredArticles, func(i, j int) bool {
		return scoredArticles[i].score > scoredArticles[j].score
	})

	// Берем топ статей
	var result []Article
	for i := 0; i < len(scoredArticles) && i < maxArticles; i++ {
		result = append(result, scoredArticles[i].article)
	}

	log.Printf("[NEWS] Найдено %d релевантных статей по теме: %s", len(result), keywords)

	if len(result) == 0 {
		log.Printf("[NEWS] Нет релевантных статей по теме: %s", keywords)
	}

	return result, nil
}

// FetchAllArticles собирает статьи со всех источников
func (na *NewsAggregator) FetchAllArticles() ([]Article, error) {
	var allArticles []Article

	for _, source := range na.sources {
		log.Printf("[NEWS] Получение статей из %s", source.GetName())
		articles, err := source.FetchArticles()
		if err != nil {
			log.Printf("[NEWS] Ошибка получения статей из %s: %v", source.GetName(), err)
			continue
		}
		log.Printf("[NEWS] Получено %d статей из %s", len(articles), source.GetName())
		allArticles = append(allArticles, articles...)
	}

	log.Printf("[NEWS] Итого собрано %d статей", len(allArticles))
	return allArticles, nil
}

// calculateRelevance вычисляет релевантность статьи
func (na *NewsAggregator) calculateRelevance(article Article, keywords []string) int {
	score := 0
	text := strings.ToLower(article.Title + " " + article.Summary)

	// Проверяем ключевые слова
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			score += 5
		}
	}

	// Бонус за свежесть
	hoursSincePublished := time.Since(article.PublishedAt).Hours()
	if hoursSincePublished < 6 {
		score += 10
	} else if hoursSincePublished < 12 {
		score += 7
	} else if hoursSincePublished < 24 {
		score += 4
	}

	// Бонус за совпадение в заголовке
	titleLower := strings.ToLower(article.Title)
	for _, keyword := range keywords {
		if strings.Contains(titleLower, keyword) {
			score += 3
		}
	}

	return score
}

// FilterOutMilitaryTopics фильтрует военные темы
func (na *NewsAggregator) FilterOutMilitaryTopics(articles []Article) []Article {
	var filtered []Article
	militaryKeywords := []string{
		"война", "воен", "боев", "оруж", "атака", "конфликт", "наступление",
		"оборона", "спецоперация", "ВСУ", "ВС РФ", "минобороны", "погиб",
		"ранен", "обстрел", "взрыв", "снаряд", "танк", "артиллерия",
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
