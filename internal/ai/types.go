package ai

// ArticleInfo представляет информацию о новости для генерации поста
type ArticleInfo struct {
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	URL         string `json:"url"`
	Source      string `json:"source"`
	Category    string `json:"category"`    // Добавим
	Subcategory string `json:"subcategory"` // Добавим
}

// PostGenerationRequest запрос на генерацию поста
type PostGenerationRequest struct {
	Keywords string      `json:"keywords"`
	Article  ArticleInfo `json:"article"`
}
