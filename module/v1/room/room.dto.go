package room

type CreateRoomDto struct {
	Title              string   `json:"title" binding:"required,min=3,max=100" example:"Daily English Practice"`
	Language           string   `json:"language" binding:"required" example:"en"`
	CEFRLevel          string   `json:"cefrLevel" example:"B1"`
	LevelLabel         string   `json:"levelLabel" example:"Intermediate"`
	MaxSlots           int      `json:"maxSlots" example:"6"`
	TopicTag           string   `json:"topicTag" example:"Casual Chat"`
	Description        *string  `json:"description" example:"Practice casual conversation and fluency"`
	Topic              string   `json:"topic" example:"Weekend plans and hobbies"`
	IsBeginnerFriendly *bool    `json:"isBeginnerFriendly" example:"true"`
	Tags               []string `json:"tags" example:"english,speaking,fluency"`
	RoomKey            *string  `json:"room_key" example:""`
}

type UpdateRoomDto struct {
	Title    *string `json:"title" example:"Updated English Chat"`
	Topic    *string `json:"topic" example:"Travel and culture"`
	Status   *string `json:"status" example:"LIVE"` // LIVE, WAITING, ENDED
	MaxSlots *int    `json:"maxSlots" example:"8"`
}

type RoomQuery struct {
	Lang   string `form:"lang" example:"en"`
	Query  string `form:"query" example:"daily"`
	Filter string `form:"filter" example:"all"` // all, active, free-seats, beginner, native
	Page   int    `form:"page" example:"1"`
	Limit  int    `form:"limit" example:"20"`
}

type JoinRoomDto struct {
	Password *string `json:"password" example:""`
}

type PaginationMeta struct {
	Total      int64 `json:"total" example:"25"`
	Page       int   `json:"page" example:"1"`
	Limit      int   `json:"limit" example:"20"`
	TotalPages int   `json:"totalPages" example:"2"`
}

type RoomListResponse struct {
	Rooms      interface{}    `json:"rooms"`
	Pagination PaginationMeta `json:"pagination"`
}

type JoinRoomResponse struct {
	Success      bool   `json:"success" example:"true"`
	RoomID       string `json:"roomId" example:"ckz1234567890"`
	PeerToken    string `json:"peerToken" example:"pt_ckz1234567890"`
	CurrentSlots int    `json:"currentSlots" example:"3"`
	MaxSlots     int    `json:"maxSlots" example:"6"`
}

type LeaveRoomResponse struct {
	Success        bool   `json:"success" example:"true"`
	RoomID         string `json:"roomId" example:"ckz1234567890"`
	RemainingSlots int    `json:"remainingSlots" example:"2"`
}

