package auth

type GuestAuthDto struct {
	Name             string `json:"name" example:"Alex Guest"`
	NativeLanguage   string `json:"nativeLanguage" example:"Spanish"`
	LearningLanguage string `json:"learningLanguage" example:"English"`
}

type RegisterDto struct {
	Name             string `json:"name" binding:"required,min=2" example:"John Doe"`
	Email            string `json:"email" binding:"required,email" example:"john@example.com"`
	Password         string `json:"password" binding:"required,min=6" example:"secret123"`
	NativeLanguage   string `json:"nativeLanguage" example:"Spanish"`
	LearningLanguage string `json:"learningLanguage" example:"English"`
}

type LoginDto struct {
	Email    string `json:"email" binding:"required,email" example:"john@example.com"`
	Password string `json:"password" binding:"required" example:"secret123"`
}

type AuthResponse struct {
	Token string      `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	User  interface{} `json:"user"`
}

type SessionResponse struct {
	Authenticated bool        `json:"authenticated" example:"true"`
	User          interface{} `json:"user"`
}

