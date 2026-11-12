package chat

type SendMessageDto struct {
	Content         string `json:"content" binding:"omitempty,max=500" example:"Hello everyone, great to practice speaking here!"`
	Type            string `json:"type" example:"TEXT"` // TEXT, IMAGE, IDIOM, REACTION
	ClientMessageID string `json:"clientMessageId" binding:"omitempty,max=128"`
	MediaURL        string `json:"mediaUrl" binding:"omitempty,max=1000"`
	MediaType       string `json:"mediaType" binding:"omitempty,max=100"`
}

type ChatQuery struct {
	Limit  int    `form:"limit" example:"50"`
	Before string `form:"before" example:"2026-10-13T12:00:00Z"`
}

type ChatMessagesResponse struct {
	Messages interface{} `json:"messages"`
}
