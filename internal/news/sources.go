package news

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RSSSource представляет RSS-ленту как источник новостей
type RSSSource struct {
	Name string
	URL  string
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

func (r *RSSSource) FetchArticles() ([]Article, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", r.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения RSS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка статуса RSS: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения RSS: %w", err)
	}

	var rss RSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, fmt.Errorf("ошибка парсинга RSS: %w", err)
	}

	var articles []Article
	for _, item := range rss.Channel.Item {
		pubDate, err := parseDate(item.PubDate)
		if err != nil {
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
			Category:    item.Category,
		}

		articles = append(articles, article)
	}

	return articles, nil
}

// cleanText очищает текст от HTML тегов и лишних пробелов
func cleanText(text string) string {
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
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Now(), nil
}

// GetDefaultSources возвращает список RSS-лент
func GetDefaultSources() []RSSSource {
	return []RSSSource{
		{
			Name: "РИА Новости",
			URL:  "https://ria.ru/export/rss2/index.xml",
		},
		{
			Name: "Коммерсант",
			URL:  "https://www.kommersant.ru/RSS/news.xml",
		},
		{
			Name: "ТАСС",
			URL:  "https://tass.ru/rss/v2.xml",
		},
		{
			Name: "VC.ru",
			URL:  "https://vc.ru/rss",
		},
		{
			Name: "Хабрахабр",
			URL:  "https://habr.com/ru/rss/articles/?fl=ru",
		},
	}
}
