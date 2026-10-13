package user

type UpdatePortfolioDto struct {
	Name             *string `json:"name" example:"Alex Rivera"`
	NativeLanguage   *string `json:"nativeLanguage" example:"Spanish"`
	LearningLanguage *string `json:"learningLanguage" example:"English"`
	CEFRLevel        *string `json:"cefrLevel" example:"B2"`
	Location         *string `json:"location" example:"Madrid, Spain"`
}

type UserStatsResponse struct {
	HoursSpokenThisMonth  float64 `json:"hoursSpokenThisMonth" example:"14.5"`
	TotalRoomsJoined      int64   `json:"totalRoomsJoined" example:"32"`
	FrequentPartnersCount int     `json:"frequentPartnersCount" example:"8"`
	CurrentStreakDays     int     `json:"currentStreakDays" example:"5"`
	KarmaPoints           int     `json:"karmaPoints" example:"240"`
}

