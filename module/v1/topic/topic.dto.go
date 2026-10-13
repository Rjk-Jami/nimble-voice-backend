package topic

type TopicQuery struct {
	Category string `form:"category" example:"Travel"`
	Level    string `form:"level" example:"B1"`
}

type PromptItem struct {
	Title string `json:"title" example:"Describe your favorite travel destination"`
	Level string `json:"level" example:"Intermediate"`
	Tag   string `json:"tag" example:"Vacations"`
}

type CategoryDeck struct {
	Category string       `json:"category" example:"Travel"`
	Icon     string       `json:"icon" example:"Plane"`
	Prompts  []PromptItem `json:"prompts"`
}

