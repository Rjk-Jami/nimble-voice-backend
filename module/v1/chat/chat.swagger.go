package chat

import _ "nimble-voice-backend/utils/httpx"


// GetMessages godoc
// @Summary      Get room chat messages
// @Description  Fetches the backchannel text chat history for a voice room
// @Tags         Chat
// @Produce      json
// @Param        id     path  string true  "Room ID"
// @Param        limit  query int    false "Number of messages to fetch (default 50)"
// @Param        before query string false "Cursor ISO timestamp to fetch messages prior to"
// @Success      200    {object} httpx.APIResponse{data=ChatMessagesResponse}
// @Failure      404    {object} httpx.APIResponse
// @Router       /rooms/{id}/messages [get]
func _() {}

// SendMessage godoc
// @Summary      Send backchannel chat message
// @Description  Sends a text message, reaction, or idiom prompt to the room's backchannel
// @Tags         Chat
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path string         true "Room ID"
// @Param        payload body SendMessageDto true "Message payload"
// @Success      201     {object} httpx.APIResponse
// @Failure      400     {object} httpx.APIResponse
// @Failure      401     {object} httpx.APIResponse
// @Failure      403     {object} httpx.APIResponse
// @Failure      404     {object} httpx.APIResponse
// @Router       /rooms/{id}/messages [post]
func _() {}
