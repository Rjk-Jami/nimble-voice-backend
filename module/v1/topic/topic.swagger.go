package topic

import _ "nimble-voice-backend/utils/httpx"


// GetPrompts godoc
// @Summary      Get conversation topic prompts
// @Description  Returns categorized conversation starter cards and icebreaker decks
// @Tags         Topics
// @Produce      json
// @Param        category query string false "Filter by topic category (e.g. Travel, Business, Tech)"
// @Param        level    query string false "Filter by CEFR difficulty level (e.g. A2, B1, B2, C1)"
// @Success      200      {object} httpx.APIResponse{data=[]CategoryDeck}
// @Router       /topics/prompts [get]
func _() {}
