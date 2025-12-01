package news

import (
	"time"
)

// Article представляет одну новостную статью
type Article struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Summary     string    `json:"summary"`
	Content     string    `json:"content"`
	PublishedAt time.Time `json:"published_at"`
	Source      string    `json:"source"`
	Category    string    `json:"category"`
}

// NewsSource представляет источник новостей
type NewsSource interface {
	FetchArticles() ([]Article, error)
	GetName() string
}
