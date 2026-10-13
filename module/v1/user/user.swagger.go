package user

import _ "nimble-voice-backend/utils/httpx"


// UpdatePortfolio godoc
// @Summary      Update user portfolio
// @Description  Updates learning languages, CEFR level, location, and name for the user
// @Tags         Users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body UpdatePortfolioDto true "Portfolio update fields"
// @Success      200     {object} httpx.APIResponse
// @Failure      400     {object} httpx.APIResponse
// @Failure      401     {object} httpx.APIResponse
// @Router       /users/portfolio [patch]
func _() {}

// GetUserStats godoc
// @Summary      Get user statistics
// @Description  Returns monthly spoken hours, streak days, karma points, and room history
// @Tags         Users
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} httpx.APIResponse{data=UserStatsResponse}
// @Failure      401 {object} httpx.APIResponse
// @Router       /users/stats [get]
func _() {}

// GetPublicProfile godoc
// @Summary      Get public learner card
// @Description  Returns public profile details of a speaker by user ID
// @Tags         Users
// @Produce      json
// @Param        userId path string true "User ID"
// @Success      200    {object} httpx.APIResponse
// @Failure      404    {object} httpx.APIResponse
// @Router       /users/{userId} [get]
func _() {}
