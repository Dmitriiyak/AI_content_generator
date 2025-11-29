// internal/ai/types.go
package ai

// ChannelAnalysis представляет упрощенный анализ канала для AI
type ChannelAnalysis struct {
	MainTopic      string   `json:"main_topic"`
	Subtopics      []string `json:"subtopics"`
	TargetAudience string   `json:"target_audience"`
	ContentStyle   string   `json:"content_style"`
	Keywords       []string `json:"keywords"`
	ContentAngle   string   `json:"content_angle"`
}

// ArticleRelevance представляет упрощенную информацию о новости для AI-анализа
type ArticleRelevance struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	URL     string `json:"url"`
}

// NewsRelevance представляет релевантность новости для канала
type NewsRelevance struct {
	Article      ArticleRelevance `json:"article"`
	Relevance    float64          `json:"relevance"`
	Explanation  string           `json:"explanation"`
	MatchReasons []string         `json:"match_reasons"`
}

// NewsSelectionResponse представляет ответ от AI по подбору новостей
type NewsSelectionResponse struct {
	SelectedNews []NewsRelevance `json:"selected_news"`
}
