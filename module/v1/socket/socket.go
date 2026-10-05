package socket

import (
	"nimble-voice-backend/server"

	"github.com/gin-gonic/gin"
	"github.com/shishir1290/gsocketio"
)

// Re-export core types from server package for seamless access
type SocketServer = server.SocketServer
type AuthContext = server.AuthContext
type RoomEventPayload = server.RoomEventPayload

// InitSocketServer initializes the pure Go Socket.IO v4 server instance
func InitSocketServer(opts ...*gsocketio.Options) (*SocketServer, error) {
	return server.NewSocketServer(opts...)
}

// SocketHandler returns a Gin-compatible handler function
func SocketHandler(srv *SocketServer) gin.HandlerFunc {
	return srv.GinHandler()
}
