package room

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RoomModule(router *gin.RouterGroup, db *gorm.DB) {
	service := NewRoomService(db)
	controller := NewRoomController(service)
	RoomRouter(router, controller)
}
