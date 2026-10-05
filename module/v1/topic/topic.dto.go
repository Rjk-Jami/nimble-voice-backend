package topic

type TopicQuery struct {
	Category string `form:"category"`
	Level    string `form:"level"`
}

type PromptItem struct {
	Title string `json:"title"`
	Level string `json:"level"`
	Tag   string `json:"tag"`
}

type CategoryDeck struct {
	Category string       `json:"category"`
	Icon     string       `json:"icon"`
	Prompts  []PromptItem `json:"prompts"`
}
