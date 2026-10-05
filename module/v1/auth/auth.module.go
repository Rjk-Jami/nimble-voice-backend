package auth

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func AuthModule(router *gin.RouterGroup, db *gorm.DB) {
	service := NewAuthService(db)
	controller := NewAuthController(service)
	AuthRouter(router, controller)
}
