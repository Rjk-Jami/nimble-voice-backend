package socket

import (
	"log"

	"github.com/gin-gonic/gin"
	socketio "github.com/googollee/go-socket.io"
)

func InitSocketServer() (*socketio.Server, error) {
	server := socketio.NewServer(nil)

	server.OnConnect("/", func(s socketio.Conn) error {
		s.SetContext("")
		log.Println("Socket connected:", s.ID())
		return nil
	})

	server.OnError("/", func(s socketio.Conn, e error) {
		log.Println("Socket error:", e)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("Socket disconnected:", reason)
	})

	return server, nil
}

func SocketHandler(server *socketio.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		server.ServeHTTP(c.Writer, c.Request)
	}
}
