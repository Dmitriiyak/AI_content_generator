package ai

// ArticleInfo представляет информацию о новости для генерации поста
type ArticleInfo struct {
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	URL      string `json:"url"`
	Source   string `json:"source"`
	ImageURL string `json:"image_url"` // Добавлено поле для картинки
}

// PostGenerationRequest запрос на генерацию поста
type PostGenerationRequest struct {
	Keywords string      `json:"keywords"`
	Article  ArticleInfo `json:"article"`
}
