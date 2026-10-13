package auth

import _ "nimble-voice-backend/utils/httpx"


// GuestAuth godoc
// @Summary      Create guest session
// @Description  Creates a temporary guest profile for instant audio room testing
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        payload body GuestAuthDto false "Guest preferences"
// @Success      201  {object}  httpx.APIResponse{data=AuthResponse}
// @Failure      500  {object}  httpx.APIResponse
// @Router       /auth/guest [post]
func _() {}

// GetSession godoc
// @Summary      Get current session
// @Description  Retrieves session info via Bearer token or session cookie
// @Tags         Auth
// @Produce      json
// @Success      200  {object}  httpx.APIResponse{data=SessionResponse}
// @Router       /auth/session [get]
func _() {}

// GetProfile godoc
// @Summary      Get user profile
// @Description  Retrieves the authenticated user profile and stats
// @Tags         Auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  httpx.APIResponse
// @Failure      401  {object}  httpx.APIResponse
// @Failure      404  {object}  httpx.APIResponse
// @Router       /auth/profile [get]
func _() {}

// Register godoc
// @Summary      Register a new user
// @Description  Creates a new permanent user account
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        payload body RegisterDto true "User registration details"
// @Success      201  {object}  httpx.APIResponse{data=AuthResponse}
// @Failure      400  {object}  httpx.APIResponse
// @Failure      409  {object}  httpx.APIResponse
// @Router       /auth/register [post]
func _() {}

// Login godoc
// @Summary      Login user
// @Description  Authenticates user credentials and returns a JWT Bearer token
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        payload body LoginDto true "User credentials"
// @Success      200  {object}  httpx.APIResponse{data=AuthResponse}
// @Failure      400  {object}  httpx.APIResponse
// @Failure      401  {object}  httpx.APIResponse
// @Router       /auth/login [post]
func _() {}
