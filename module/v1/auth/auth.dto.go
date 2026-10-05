package auth

type GuestAuthDto struct {
	Name             string `json:"name"`
	NativeLanguage   string `json:"nativeLanguage"`
	LearningLanguage string `json:"learningLanguage"`
}

type RegisterDto struct {
	Name             string `json:"name" binding:"required,min=2"`
	Email            string `json:"email" binding:"required,email"`
	Password         string `json:"password" binding:"required,min=6"`
	NativeLanguage   string `json:"nativeLanguage"`
	LearningLanguage string `json:"learningLanguage"`
}

type LoginDto struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}

type SessionResponse struct {
	Authenticated bool        `json:"authenticated"`
	User          interface{} `json:"user"`
}
