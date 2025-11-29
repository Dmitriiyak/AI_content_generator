package news

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RSSSource представляет RSS-ленту как источник новостей
type RSSSource struct {
	Name       string
	URL        string
	Categories []string
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

func (r *RSSSource) GetCategories() []string {
	return r.Categories
}

func (r *RSSSource) FetchArticles() ([]Article, error) {
	resp, err := http.Get(r.URL)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения RSS (см. sources.go): %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения RSS (см. sources.go): %w", err)
	}

	var rss RSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, fmt.Errorf("ошибка парсинга RSS (см. sources.go): %w", err)
	}

	var articles []Article
	for _, item := range rss.Channel.Item {
		// Парсим дату публикации
		pubDate, err := parseDate(item.PubDate)
		if err != nil {
			// Если дату не распарсить, используем текущую
			pubDate = time.Now()
		}

		// Пропускаем старые новости (больше 3 дней)
		if time.Since(pubDate) > 3*24*time.Hour {
			continue
		}

		article := Article{
			Title:       item.Title,
			URL:         item.Link,
			Summary:     item.Description,
			PublishedAt: pubDate,
			Source:      r.Name,
			Category:    item.Category,
		}

		articles = append(articles, article)
	}

	return articles, nil
}

// parseDate пытается распарсить различные форматы дат
func parseDate(dateStr string) (time.Time, error) {
	formats := []string{
		time.RFC1123,
		time.RFC1123Z,
		time.RFC822,
		time.RFC822Z,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2006-01-02T15:04:05Z",
		"02.01.2006 15:04",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("неизвестный формат даты (см. sources.go): %s", dateStr)
}

// GetDefaultSources возвращает список популярных RSS-лент
func GetDefaultSources() []RSSSource {
	return []RSSSource{
		{
			Name:       "РИА Новости",
			URL:        "https://ria.ru/export/rss2/index.xml",
			Categories: []string{"новости", "политика", "экономика"},
		},
		{
			Name:       "Коммерсант",
			URL:        "https://www.kommersant.ru/RSS/news.xml",
			Categories: []string{"новости", "бизнес", "экономика"},
		},
		{
			Name:       "ТАСС",
			URL:        "https://tass.ru/rss/v2.xml",
			Categories: []string{"новости", "политика", "общество"},
		},
		{
			Name:       "VC.ru",
			URL:        "https://vc.ru/rss",
			Categories: []string{"технологии", "бизнес", "стартапы"},
		},
		{
			Name:       "Хабрахабр",
			URL:        "https://habr.com/ru/rss/all/all/",
			Categories: []string{"технологии", "программирование", "it"},
		},
		{
			Name:       "РБК",
			URL:        "https://rssexport.rbc.ru/rbcnews/news/30/full.rss",
			Categories: []string{"новости", "бизнес", "финансы"},
		},
		{
			Name:       "CNews",
			URL:        "https://www.cnews.ru/inc/rss/news.xml",
			Categories: []string{"технологии", "it", "гаджеты"},
		},
		{
			Name:       "3DNews",
			URL:        "https://3dnews.ru/news/rss/",
			Categories: []string{"технологии", "железо", "гаджеты"},
		},
	}
}
