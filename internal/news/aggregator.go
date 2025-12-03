package news

import (
	"log"
	"sort"
	"strings"
	"time"
)

// Синонимы для расширения поиска
var synonyms = map[string][]string{
	// Технологии
	"ии":       {"искусственный интеллект", "нейросеть", "машинное обучение", "AI", "artificial intelligence"},
	"айти":     {"IT", "информационные технологии", "программирование", "разработка"},
	"гаджет":   {"устройство", "девайс", "техника", "электроника"},
	"смартфон": {"телефон", "мобильный", "андроид", "айфон"},
	"ноутбук":  {"лэптоп", "компьютер", "ПК"},

	// Бизнес
	"стартап":      {"компания", "бизнес", "предприятие", "проект"},
	"криптовалюта": {"биткоин", "эфириум", "блокчейн", "крипта"},
	"инвестиция":   {"вложение", "финансирование", "капитал"},

	// Наука
	"космос":       {"космонавтика", "астрономия", "вселенная", "галактика"},
	"исследование": {"эксперимент", "изучение", "научная работа"},

	// Спорт
	"футбол": {"футбольный", "соккер", "чемпионат"},
	"хоккей": {"хоккейный", "КХЛ", "НХЛ"},
	"теннис": {"большой шлем", "Уимблдон"},

	// Автомобили
	"электромобиль": {"электроавто", "тесла", "EV", "electric vehicle"},
	"авто":          {"автомобиль", "машина", "транспорт"},
}

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

	// Получаем все статьи из всех источников
	allArticles, err := na.FetchAllArticles()
	if err != nil {
		log.Printf("[NEWS] Ошибка получения статей: %v", err)
		return nil, err
	}

	log.Printf("[NEWS] Получено %d статей", len(allArticles))

	if len(allArticles) == 0 {
		log.Printf("[NEWS] ⚠️ Не получено ни одной статьи")
		return []Article{}, nil
	}

	// Фильтруем военные темы
	articles := na.FilterOutMilitaryTopics(allArticles)
	log.Printf("[NEWS] После фильтрации осталось %d статей", len(articles))

	if len(articles) == 0 {
		log.Printf("[NEWS] Нет статей после фильтрации")
		return []Article{}, nil
	}

	// Расширяем ключевые слова синонимами
	expandedKeywords := na.expandKeywords(keywords)
	log.Printf("[NEWS] Расширенные ключевые слова: %v", expandedKeywords)

	// Создаем структуру для сортировки
	type scoredArticle struct {
		article Article
		score   float64
	}

	var scoredArticles []scoredArticle

	// Оцениваем каждую статью
	for _, article := range articles {
		score := na.calculateRelevance(article, expandedKeywords)
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

// expandKeywords расширяет ключевые слова синонимами
func (na *NewsAggregator) expandKeywords(keywords string) []string {
	keywords = strings.ToLower(strings.TrimSpace(keywords))
	words := strings.Fields(keywords)

	expanded := make([]string, 0, len(words)*2)
	seen := make(map[string]bool)

	for _, word := range words {
		// Добавляем оригинальное слово
		if !seen[word] {
			expanded = append(expanded, word)
			seen[word] = true
		}

		// Добавляем синонимы
		if syns, ok := synonyms[word]; ok {
			for _, syn := range syns {
				if !seen[syn] {
					expanded = append(expanded, syn)
					seen[syn] = true
				}
			}
		}
	}

	return expanded
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
func (na *NewsAggregator) calculateRelevance(article Article, keywords []string) float64 {
	score := 0.0
	text := strings.ToLower(article.Title + " " + article.Summary)

	// 1. Совпадение ключевых слов (60%)
	keywordScore := 0.0
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			keywordScore += 1.0
		}
	}
	if len(keywords) > 0 {
		keywordScore = (keywordScore / float64(len(keywords))) * 60.0
	}
	score += keywordScore

	// 2. Свежесть (30%)
	if !article.PublishedAt.IsZero() {
		hoursSincePublished := time.Since(article.PublishedAt).Hours()
		if hoursSincePublished < 6 {
			score += 30.0
		} else if hoursSincePublished < 12 {
			score += 25.0
		} else if hoursSincePublished < 24 {
			score += 20.0
		} else if hoursSincePublished < 48 {
			score += 15.0
		} else if hoursSincePublished < 72 {
			score += 10.0
		}
	}

	// 3. Качество статьи (10%)
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
		// Военные темы
		"война", "воен", "боев", "оруж", "атака", "конфликт", "наступление",
		"оборона", "спецоперация", "минобороны", "погиб", "ранен", "обстрел",
		"взрыв", "снаряд", "танк", "артиллерия", "залп", "мин", "осколок",
		"сражение", "битва", "убит", "убийств", "убийство", "смерть", "погибш",
		"стрельб", "перестрелк", "террорист", "теракт", "диверсант", "диверсия",
		"противостояние", "противоречие", "столкновение", "эскалация", "насилие",
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
