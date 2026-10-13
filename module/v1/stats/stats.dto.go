package stats

type NetworkStatsResponse struct {
	OnlineCount        int   `json:"onlineCount" example:"142"`
	ActiveRoomsCount   int64 `json:"activeRoomsCount" example:"18"`
	LiveLanguagesCount int64 `json:"liveLanguagesCount" example:"6"`
}

