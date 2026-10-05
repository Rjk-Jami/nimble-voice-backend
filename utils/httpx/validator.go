package httpx

import (
	"github.com/gin-gonic/gin"
)

func BindAndValidate(ctx *gin.Context, target interface{}) bool {
	if err := ctx.ShouldBind(target); err != nil {
		SendResponse(ctx, SendData(400, "Validation error", err.Error()))
		return false
	}
	return true
}
