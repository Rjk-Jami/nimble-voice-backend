package httpx

import (
	"time"

	"github.com/gin-gonic/gin"
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

func SendResponse(ctx *gin.Context, res APIResponse) {
	ctx.JSON(res.StatusCode, gin.H{
		"status":    res.StatusCode,
		"message":   res.Message,
		"data":      res.Data,
		"timestamp": time.Now().UTC(),
	})
}
