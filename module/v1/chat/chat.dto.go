package chat

type SendMessageDto struct {
	Content string `json:"content" binding:"required,min=1,max=500"`
	Type    string `json:"type"` // TEXT, SYSTEM, IDIOM, REACTION
}

type ChatQuery struct {
	Limit  int    `form:"limit"`
	Before string `form:"before"`
}

type ChatMessagesResponse struct {
	Messages interface{} `json:"messages"`
}
