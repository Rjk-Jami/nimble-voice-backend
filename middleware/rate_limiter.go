package middleware

import (
	"time"

	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth/v7/limiter"
	"github.com/gin-gonic/gin"
)

func RateLimiter() gin.HandlerFunc {
	// Maximum 20 requests per second per IP
	lmt := tollbooth.NewLimiter(20, &limiter.ExpirableOptions{
		DefaultExpirationTTL: time.Minute,
	})

	return func(c *gin.Context) {
		// Bypass rate limiter for socket.io polling and streaming
		if c.Request.URL.Path == "/socket.io" || (len(c.Request.URL.Path) > 10 && c.Request.URL.Path[:11] == "/socket.io/") {
			c.Next()
			return
		}

		httpError := tollbooth.LimitByRequest(lmt, c.Writer, c.Request)
		if httpError != nil {
			c.AbortWithStatusJSON(httpError.StatusCode, gin.H{
				"status":  httpError.StatusCode,
				"message": "Too many requests. Please slow down.",
			})
			return
		}
		c.Next()
	}
}
