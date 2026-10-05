package stats

type NetworkStatsResponse struct {
	OnlineCount        int   `json:"onlineCount"`
	ActiveRoomsCount   int64 `json:"activeRoomsCount"`
	LiveLanguagesCount int64 `json:"liveLanguagesCount"`
}
