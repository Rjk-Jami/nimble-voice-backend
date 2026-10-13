package chat

type SendMessageDto struct {
	Content string `json:"content" binding:"required,min=1,max=500" example:"Hello everyone, great to practice speaking here!"`
	Type    string `json:"type" example:"TEXT"` // TEXT, SYSTEM, IDIOM, REACTION
}

type ChatQuery struct {
	Limit  int    `form:"limit" example:"50"`
	Before string `form:"before" example:"2026-10-13T12:00:00Z"`
}

type ChatMessagesResponse struct {
	Messages interface{} `json:"messages"`
}

