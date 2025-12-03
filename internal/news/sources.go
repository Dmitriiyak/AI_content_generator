package news

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
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
			Enclosure   []struct {
				URL    string `xml:"url,attr"`
				Type   string `xml:"type,attr"`
				Length string `xml:"length,attr"`
			} `xml:"enclosure"`
			MediaContent struct {
				URL    string `xml:"url,attr"`
				Medium string `xml:"medium,attr"`
				Type   string `xml:"type,attr"`
			} `xml:"content"`
			MediaThumbnail struct {
				URL string `xml:"url,attr"`
			} `xml:"thumbnail"`
		} `xml:"item"`
	} `xml:"channel"`
}

// extractImageFromItem извлекает URL изображения из элемента RSS
func extractImageFromItem(item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Category    string `xml:"category"`
	Enclosure   []struct {
		URL    string `xml:"url,attr"`
		Type   string `xml:"type,attr"`
		Length string `xml:"length,attr"`
	} `xml:"enclosure"`
	MediaContent struct {
		URL    string `xml:"url,attr"`
		Medium string `xml:"medium,attr"`
		Type   string `xml:"type,attr"`
	} `xml:"content"`
	MediaThumbnail struct {
		URL string `xml:"url,attr"`
	} `xml:"thumbnail"`
}) string {
	// 1. Проверяем media:content (обычно для медиа)
	if item.MediaContent.URL != "" && (item.MediaContent.Medium == "image" ||
		strings.Contains(item.MediaContent.Type, "image")) {
		return item.MediaContent.URL
	}

	// 2. Проверяем enclosure (вложение)
	for _, enclosure := range item.Enclosure {
		if strings.Contains(enclosure.Type, "image") {
			return enclosure.URL
		}
	}

	// 3. Проверяем thumbnail
	if item.MediaThumbnail.URL != "" {
		return item.MediaThumbnail.URL
	}

	// 4. Извлекаем из описания HTML
	if item.Description != "" {
		// Ищем теги img
		imgRegex := regexp.MustCompile(`<img[^>]+src="([^">]+)"`)
		matches := imgRegex.FindStringSubmatch(item.Description)
		if len(matches) > 1 {
			return matches[1]
		}

		// Ищем data-src для lazy loading
		dataSrcRegex := regexp.MustCompile(`data-src="([^">]+)"`)
		matches = dataSrcRegex.FindStringSubmatch(item.Description)
		if len(matches) > 1 {
			return matches[1]
		}

		// Ищем в amp-img
		ampImgRegex := regexp.MustCompile(`<amp-img[^>]+src="([^">]+)"`)
		matches = ampImgRegex.FindStringSubmatch(item.Description)
		if len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
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
	log.Printf("[RSS] Загрузка RSS из %s", r.Name)

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

		// Пропускаем старые новости (больше 7 дней)
		if time.Since(pubDate) > 7*24*time.Hour {
			continue
		}

		// Извлекаем изображение
		imageURL := extractImageFromItem(item)

		article := Article{
			Title:       cleanText(item.Title),
			URL:         item.Link,
			Summary:     cleanText(item.Description),
			PublishedAt: pubDate,
			Source:      r.Name,
			Tags:        []string{item.Category},
			ImageURL:    imageURL, // Добавляем URL картинки
		}

		if imageURL != "" {
			log.Printf("[RSS] Найдено изображение для статьи: %s", imageURL)
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
		// Технологии и IT
		{
			Name:     "Хабрахабр",
			URL:      "https://habr.com/ru/rss/articles/?fl=ru",
			Language: "ru",
		},
		{
			Name:     "VC.ru",
			URL:      "https://vc.ru/rss",
			Language: "ru",
		},
		{
			Name:     "Tproger",
			URL:      "https://tproger.ru/feed/",
			Language: "ru",
		},
		{
			Name:     "CNews",
			URL:      "https://www.cnews.ru/inc/rss/news.xml",
			Language: "ru",
		},
		{
			Name:     "IXBT",
			URL:      "https://www.ixbt.com/export/news.rss",
			Language: "ru",
		},
		{
			Name:     "3DNews",
			URL:      "https://3dnews.ru/breaking/rss",
			Language: "ru",
		},

		// Бизнес и финансы
		{
			Name:     "РБК",
			URL:      "https://rssexport.rbc.ru/rbcnews/news/30/full.rss",
			Language: "ru",
		},
		{
			Name:     "Коммерсант",
			URL:      "https://www.kommersant.ru/RSS/news.xml",
			Language: "ru",
		},
		{
			Name:     "Forbes",
			URL:      "https://www.forbes.ru/newrss.xml",
			Language: "ru",
		},

		// Спорт
		{
			Name:     "Спорт-Экспресс",
			URL:      "https://www.sport-express.ru/services/materials/news/se/",
			Language: "ru",
		},
		{
			Name:     "Чемпионат",
			URL:      "https://www.championat.com/rss/news/football",
			Language: "ru",
		},
		{
			Name:     "Sports.ru",
			URL:      "https://www.sports.ru/rss/all_news.xml",
			Language: "ru",
		},

		// Наука и образование
		{
			Name:     "N+1",
			URL:      "https://nplus1.ru/rss",
			Language: "ru",
		},

		// Разное
		{
			Name:     "РИА Новости",
			URL:      "https://ria.ru/export/rss2/index.xml",
			Language: "ru",
		},
		{
			Name:     "ТАСС",
			URL:      "https://tass.ru/rss/v2.xml",
			Language: "ru",
		},
	}
}
