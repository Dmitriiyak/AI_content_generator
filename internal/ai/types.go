package ai

// ChannelAnalysis представляет результат анализа канала
type ChannelAnalysis struct {
	MainTopic      string   `json:"main_topic"`
	Subtopics      []string `json:"subtopics"`
	TargetAudience string   `json:"target_audience"`
	ContentStyle   string   `json:"content_style"`
	Keywords       []string `json:"keywords"`
	ContentAngle   string   `json:"content_angle"`
}
