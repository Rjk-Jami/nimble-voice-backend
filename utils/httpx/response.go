package httpx

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type APIResponse struct {
	StatusCode int         `json:"status_code"`
	Message    string      `json:"message"`
	Data       interface{} `json:"data,omitempty"`
}

func SendData(statusCode int, message string, data ...interface{}) APIResponse {
	var d interface{}
	if len(data) > 0 {
		d = data[0]
	}
	return APIResponse{StatusCode: statusCode, Message: message, Data: d}
}

// InternalError logs an internal error server-side via Zap and returns a sanitized HTTP 500 APIResponse with nil Data.
func InternalError(operation string, err error, publicMsg string) APIResponse {
	if publicMsg == "" {
		publicMsg = "Internal server error"
	}
	zap.L().Error("internal error",
		zap.String("operation", operation),
		zap.Error(err),
	)
	return SendData(http.StatusInternalServerError, publicMsg)
}

func SendResponse(ctx *gin.Context, res APIResponse) {
	ctx.JSON(res.StatusCode, gin.H{
		"status":    res.StatusCode,
		"message":   res.Message,
		"data":      res.Data,
		"timestamp": time.Now().UTC(),
	})
}
