package user

type UpdatePortfolioDto struct {
	Name             *string `json:"name"`
	NativeLanguage   *string `json:"nativeLanguage"`
	LearningLanguage *string `json:"learningLanguage"`
	CEFRLevel        *string `json:"cefrLevel"`
	Location         *string `json:"location"`
}

type UserStatsResponse struct {
	HoursSpokenThisMonth  float64 `json:"hoursSpokenThisMonth"`
	TotalRoomsJoined      int64   `json:"totalRoomsJoined"`
	FrequentPartnersCount int     `json:"frequentPartnersCount"`
	CurrentStreakDays     int     `json:"currentStreakDays"`
	KarmaPoints           int     `json:"karmaPoints"`
}
