package user

type RegisterUserDto struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role"`
	StoreID  string `json:"store_id"`
	BranchID string `json:"branch_id"`
}

type LoginUserDto struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponseDto struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}
