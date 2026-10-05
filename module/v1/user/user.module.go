package user

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func UserModule(router *gin.RouterGroup, db *gorm.DB) {
	service := NewUserService(db)
	controller := NewUserController(service)
	UserRouter(router, controller)
}
