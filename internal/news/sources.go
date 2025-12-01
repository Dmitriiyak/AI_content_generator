package news

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// RSSSource представляет RSS-ленту как источник новостей с категориями
type RSSSource struct {
	Name        string
	URL         string
	Category    string
	Subcategory string
	Language    string
}

// RSS структура для парсинга RSS-лент
type RSS struct {
	Channel struct {
		Title string `xml:"title"`
		Item  []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
			Category    string `xml:"category"`
		} `xml:"item"`
	} `xml:"channel"`
}

func (r *RSSSource) GetName() string {
	return r.Name
}

func (r *RSSSource) GetCategory() string {
	return r.Category
}

func (r *RSSSource) GetSubcategory() string {
	return r.Subcategory
}

func (r *RSSSource) FetchArticles() ([]Article, error) {
	log.Printf("[RSS] Загрузка RSS из %s (%s/%s)", r.Name, r.Category, r.Subcategory)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", r.URL, nil)
	if err != nil {
		log.Printf("[RSS] ❌ Ошибка создания запроса: %v", err)
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[RSS] ❌ Ошибка получения RSS: %v", err)
		return nil, fmt.Errorf("ошибка получения RSS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[RSS] ❌ Ошибка статуса RSS: %d", resp.StatusCode)
		return nil, fmt.Errorf("ошибка статуса RSS: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[RSS] ❌ Ошибка чтения RSS: %v", err)
		return nil, fmt.Errorf("ошибка чтения RSS: %w", err)
	}

	var rss RSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		log.Printf("[RSS] ❌ Ошибка парсинга RSS: %v", err)
		return nil, fmt.Errorf("ошибка парсинга RSS: %w", err)
	}

	var articles []Article
	log.Printf("[RSS] Найдено %d элементов в RSS", len(rss.Channel.Item))

	for i, item := range rss.Channel.Item {
		pubDate, err := parseDate(item.PubDate)
		if err != nil {
			log.Printf("[RSS] ❌ Ошибка парсинга даты для элемента %d: %v", i, err)
			pubDate = time.Now()
		}

		// Пропускаем старые новости (больше 3 дней)
		if time.Since(pubDate) > 72*time.Hour {
			continue
		}

		article := Article{
			Title:       cleanText(item.Title),
			URL:         item.Link,
			Summary:     cleanText(item.Description),
			PublishedAt: pubDate,
			Source:      r.Name,
			Category:    r.Category,
			Subcategory: r.Subcategory,
			Tags:        []string{item.Category},
		}

		articles = append(articles, article)
	}

	log.Printf("[RSS] Загружено %d статей из %s", len(articles), r.Name)
	return articles, nil
}

// cleanText очищает текст от HTML тегов и лишних пробелов
func cleanText(text string) string {
	if text == "" {
		return ""
	}

	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<br />", "\n")

	// Убираем HTML теги
	var result strings.Builder
	inTag := false
	for _, ch := range text {
		if ch == '<' {
			inTag = true
		} else if ch == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(ch)
		}
	}

	text = result.String()

	// Убираем множественные пробелы
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	return strings.TrimSpace(text)
}

// parseDate пытается распарсить различные форматы дат
func parseDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Now(), nil
	}

	formats := []string{
		time.RFC1123,
		time.RFC1123Z,
		time.RFC822,
		time.RFC822Z,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
		"02.01.2006 15:04",
		"Mon, 02 Jan 2006 15:04:05 GMT",
		"Mon, 2 Jan 2006 15:04:05 MST",
		"2 Jan 2006 15:04:05 -0700",
		"2006-01-02",
		"02.01.2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	log.Printf("[DATE] Не удалось распарсить дату: %s", dateStr)
	return time.Now(), fmt.Errorf("не удалось распарсить дату: %s", dateStr)
}

// GetDefaultSources возвращает список RSS-лент с категориями
func GetDefaultSources() []RSSSource {
	return []RSSSource{
		// IT и Технологии
		{
			Name:        "Хабрахабр",
			URL:         "https://habr.com/ru/rss/articles/?fl=ru",
			Category:    "IT и Технологии",
			Subcategory: "Программирование",
			Language:    "ru",
		},
		{
			Name:        "VC.ru",
			URL:         "https://vc.ru/rss",
			Category:    "IT и Технологии",
			Subcategory: "Стартапы",
			Language:    "ru",
		},
		{
			Name:        "Tproger",
			URL:         "https://tproger.ru/feed/",
			Category:    "IT и Технологии",
			Subcategory: "Программирование",
			Language:    "ru",
		},
		{
			Name:        "3DNews",
			URL:         "https://3dnews.ru/breaking/rss",
			Category:    "IT и Технологии",
			Subcategory: "Гаджеты",
			Language:    "ru",
		},
		{
			Name:        "IXBT",
			URL:         "https://www.ixbt.com/export/news.rss",
			Category:    "IT и Технологии",
			Subcategory: "Гаджеты",
			Language:    "ru",
		},
		{
			Name:        "CNews",
			URL:         "https://www.cnews.ru/inc/rss/news.xml",
			Category:    "IT и Технологии",
			Subcategory: "Искусственный интеллект",
			Language:    "ru",
		},
		{
			Name:        "SecurityLab",
			URL:         "https://www.securitylab.ru/_ssi/rss/news.xml",
			Category:    "IT и Технологии",
			Subcategory: "Кибербезопасность",
			Language:    "ru",
		},

		// Бизнес и Финансы
		{
			Name:        "РБК",
			URL:         "https://rssexport.rbc.ru/rbcnews/news/30/full.rss",
			Category:    "Бизнес и Финансы",
			Subcategory: "Экономика",
			Language:    "ru",
		},
		{
			Name:        "Коммерсант",
			URL:         "https://www.kommersant.ru/RSS/news.xml",
			Category:    "Бизнес и Финансы",
			Subcategory: "Бизнес",
			Language:    "ru",
		},
		{
			Name:        "Forbes",
			URL:         "https://www.forbes.ru/newrss.xml",
			Category:    "Бизнес и Финансы",
			Subcategory: "Бизнес",
			Language:    "ru",
		},
		{
			Name:        "Инвест-Форсайт",
			URL:         "https://invest-forecast.ru/feed/",
			Category:    "Бизнес и Финансы",
			Subcategory: "Инвестиции",
			Language:    "ru",
		},

		// Спорт
		{
			Name:        "Спорт-Экспресс",
			URL:         "https://www.sport-express.ru/services/materials/news/se/",
			Category:    "Спорт",
			Subcategory: "Футбол",
			Language:    "ru",
		},
		{
			Name:        "Чемпионат",
			URL:         "https://www.championat.com/rss/news/football.html",
			Category:    "Спорт",
			Subcategory: "Футбол",
			Language:    "ru",
		},
		{
			Name:        "Sports.ru",
			URL:         "https://www.sports.ru/rss/all_news.xml",
			Category:    "Спорт",
			Subcategory: "Общее",
			Language:    "ru",
		},
		{
			Name:        "Матч ТВ",
			URL:         "https://matchtv.ru/rss",
			Category:    "Спорт",
			Subcategory: "Разное",
			Language:    "ru",
		},

		// Путешествия
		{
			Name:        "Турпром",
			URL:         "https://www.tourprom.ru/export/news/",
			Category:    "Путешествия и Туризм",
			Subcategory: "Новости",
			Language:    "ru",
		},
		{
			Name:        "Travel.ru",
			URL:         "https://www.travel.ru/rss/",
			Category:    "Путешествия и Туризм",
			Subcategory: "Новости",
			Language:    "ru",
		},
		{
			Name:        "Aviasales Блог",
			URL:         "https://www.aviasales.ru/blog/feed",
			Category:    "Путешествия и Туризм",
			Subcategory: "Авиация",
			Language:    "ru",
		},

		// Наука и Образование
		{
			Name:        "N+1",
			URL:         "https://nplus1.ru/rss",
			Category:    "Наука и Образование",
			Subcategory: "Наука",
			Language:    "ru",
		},
		{
			Name:        "ПостНаука",
			URL:         "https://postnauka.ru/feed",
			Category:    "Наука и Образование",
			Subcategory: "Наука",
			Language:    "ru",
		},
		{
			Name:        "Indicator",
			URL:         "https://indicator.ru/rss",
			Category:    "Наука и Образование",
			Subcategory: "Наука",
			Language:    "ru",
		},

		// Развлечения и Культура
		{
			Name:        "Кинопоиск",
			URL:         "https://www.kinopoisk.ru/rss/feed.xml",
			Category:    "Развлечения и Культура",
			Subcategory: "Кино",
			Language:    "ru",
		},
		{
			Name:        "Афиша",
			URL:         "https://www.afisha.ru/rss/news.xml",
			Category:    "Развлечения и Культура",
			Subcategory: "События",
			Language:    "ru",
		},

		// Общество и Политика
		{
			Name:        "РИА Новости",
			URL:         "https://ria.ru/export/rss2/index.xml",
			Category:    "Общество и Политика",
			Subcategory: "Политика",
			Language:    "ru",
		},
		{
			Name:        "ТАСС",
			URL:         "https://tass.ru/rss/v2.xml",
			Category:    "Общество и Политика",
			Subcategory: "Общее",
			Language:    "ru",
		},
		{
			Name:        "Интерфакс",
			URL:         "https://www.interfax.ru/rss.asp",
			Category:    "Общество и Политика",
			Subcategory: "Политика",
			Language:    "ru",
		},
		{
			Name:        "Лента.ру",
			URL:         "https://lenta.ru/rss",
			Category:    "Общество и Политика",
			Subcategory: "Общее",
			Language:    "ru",
		},

		// Здоровье
		{
			Name:        "МедПортал",
			URL:         "https://medportal.ru/ru/rss/rss.xml",
			Category:    "Здоровье и Спорт",
			Subcategory: "Медицина",
			Language:    "ru",
		},
		{
			Name:        "Здоровье Mail.ru",
			URL:         "https://health.mail.ru/rss/all/",
			Category:    "Здоровье и Спорт",
			Subcategory: "Здоровье",
			Language:    "ru",
		},
		// Автомобили
		{
			Name:        "Auto.ru",
			URL:         "https://auto.ru/rss/news/",
			Category:    "Автомобили",
			Subcategory: "Новинки",
			Language:    "ru",
		},
		{
			Name:        "За рулем",
			URL:         "https://www.zr.ru/rss/",
			Category:    "Автомобили",
			Subcategory: "Тест-драйвы",
			Language:    "ru",
		},

		// Еда
		{
			Name:        "Gastronom.ru",
			URL:         "https://www.gastronom.ru/rss",
			Category:    "Еда и Рестораны",
			Subcategory: "Рецепты",
			Language:    "ru",
		},

		// Мода
		{
			Name:        "Vogue",
			URL:         "https://www.vogue.ru/rss",
			Category:    "Мода и Стиль",
			Subcategory: "Одежда",
			Language:    "ru",
		},
	}
}
