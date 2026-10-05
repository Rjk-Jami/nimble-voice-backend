package route_v1

import (
	"nimble-voice-backend/module/v1/user"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterV1Routes(router *gin.RouterGroup, db *gorm.DB) {
	user.UserModule(router.Group("/user"), db)
}
