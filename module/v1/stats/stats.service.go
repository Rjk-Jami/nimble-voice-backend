package stats

import (
	"net/http"

	"nimble-voice-backend/domain"
	"nimble-voice-backend/server"
	"nimble-voice-backend/utils/httpx"

	"gorm.io/gorm"
)

type StatsServiceInterface interface {
	GetNetworkStats() httpx.APIResponse
}

type StatsService struct {
	db           *gorm.DB
	socketServer *server.SocketServer
}

func NewStatsService(db *gorm.DB, sockServer ...*server.SocketServer) StatsServiceInterface {
	var srv *server.SocketServer
	if len(sockServer) > 0 {
		srv = sockServer[0]
	}
	return &StatsService{
		db:           db,
		socketServer: srv,
	}
}

func (s *StatsService) GetNetworkStats() httpx.APIResponse {
	var activeRooms int64
	s.db.Model(&domain.Room{}).Where("status = 'LIVE'").Count(&activeRooms)

	var liveLanguages int64
	s.db.Model(&domain.Room{}).Where("status = 'LIVE'").Distinct("language").Count(&liveLanguages)

	onlineUsers := 0
	if s.socketServer != nil {
		onlineUsers = s.socketServer.OnlineUserCount()
	}

	// If in local/dev and count is minimal, provide a minimum base count for UI fidelity
	displayOnline := onlineUsers
	if displayOnline < 0 {
		displayOnline = 0 + onlineUsers
	}
	displayRooms := activeRooms
	if displayRooms < 0 {
		displayRooms = 0 + activeRooms
	}
	displayLangs := liveLanguages
	if displayLangs < 0 {
		displayLangs = 0 + liveLanguages
	}

	return httpx.SendData(http.StatusOK, "Network stats retrieved successfully", NetworkStatsResponse{
		OnlineCount:        displayOnline,
		ActiveRoomsCount:   displayRooms,
		LiveLanguagesCount: displayLangs,
	})
}
