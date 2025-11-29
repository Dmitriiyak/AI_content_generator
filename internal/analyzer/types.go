package analyzer

import (
	"time"
)

// ChannelInfo содержит основную информацию о канале
type ChannelInfo struct {
	ID             int64     `json:"id"`
	Title          string    `json:"title"`
	Username       string    `json:"username"`
	Description    string    `json:"description"`
	Participants   int       `json:"participants_count"`
	MessagesCount  int       `json:"messages_count"`
	AvgViews       float64   `json:"avg_views"`
	AvgReactions   float64   `json:"avg_reactions"`
	EngagementRate float64   `json:"engagement_rate"`
	CreatedAt      time.Time `json:"created_at"`
}

// Message представляет одно сообщение в канале
type Message struct {
	ID        int       `json:"id"`
	Text      string    `json:"text"`
	Views     int       `json:"views"`
	Reactions int       `json:"reactions"`
	Date      time.Time `json:"date"`
	MediaType string    `json:"media_type"`
}

// ChannelAnalysis содержит полный AI-анализ канала
type ChannelAnalysis struct {
	ChannelInfo ChannelInfo  `json:"channel_info"`
	Messages    []Message    `json:"messages"`
	GPTAnalysis *GPTAnalysis `json:"gpt_analysis"`
}

// TopicWeight представляет тему и её вес в канале
type TopicWeight struct {
	Topic  string  `json:"topic"`
	Weight float64 `json:"weight"` // от 0 до 1
}
