package room

type CreateRoomDto struct {
	Title              string   `json:"title" binding:"required,min=3,max=100"`
	Language           string   `json:"language" binding:"required"`
	CEFRLevel          string   `json:"cefrLevel"`
	LevelLabel         string   `json:"levelLabel"`
	MaxSlots           int      `json:"maxSlots"`
	TopicTag           string   `json:"topicTag"`
	Description        *string  `json:"description"`
	Topic              string   `json:"topic"`
	IsBeginnerFriendly *bool    `json:"isBeginnerFriendly"`
	Tags               []string `json:"tags"`
	RoomKey            *string  `json:"room_key"`
}

type UpdateRoomDto struct {
	Title    *string `json:"title"`
	Topic    *string `json:"topic"`
	Status   *string `json:"status"` // LIVE, WAITING, ENDED
	MaxSlots *int    `json:"maxSlots"`
}

type RoomQuery struct {
	Lang   string `form:"lang"`
	Query  string `form:"query"`
	Filter string `form:"filter"` // all, active, free-seats, beginner, native
	Page   int    `form:"page"`
	Limit  int    `form:"limit"`
}

type JoinRoomDto struct {
	Password *string `json:"password"`
}

type PaginationMeta struct {
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalPages int   `json:"totalPages"`
}

type RoomListResponse struct {
	Rooms      interface{}    `json:"rooms"`
	Pagination PaginationMeta `json:"pagination"`
}

type JoinRoomResponse struct {
	Success      bool   `json:"success"`
	RoomID       string `json:"roomId"`
	PeerToken    string `json:"peerToken"`
	CurrentSlots int    `json:"currentSlots"`
	MaxSlots     int    `json:"maxSlots"`
}

type LeaveRoomResponse struct {
	Success        bool   `json:"success"`
	RoomID         string `json:"roomId"`
	RemainingSlots int    `json:"remainingSlots"`
}
