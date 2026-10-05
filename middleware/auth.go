package middleware

import (
	"strings"

	"nimble-voice-backend/utils/jwt"

	"github.com/gin-gonic/gin"
)

func Auth(allowedRoles ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			ctx.AbortWithStatusJSON(401, gin.H{"status": 401, "message": "Authorization header is required"})
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			ctx.AbortWithStatusJSON(401, gin.H{"status": 401, "message": "Invalid token header format, expected: Bearer <token>"})
			return
		}

		claims, err := jwt.ParseToken(parts[1])
		if err != nil {
			ctx.AbortWithStatusJSON(401, gin.H{"status": 401, "message": "Unauthorized: " + err.Error()})
			return
		}

		// Role-based authorization check
		if len(allowedRoles) > 0 {
			permitted := false
			for _, r := range allowedRoles {
				if strings.EqualFold(r, claims.Role) {
					permitted = true
					break
				}
			}
			if !permitted {
				ctx.AbortWithStatusJSON(403, gin.H{"status": 403, "message": "Forbidden: insufficient permissions"})
				return
			}
		}

		// Store user claims in context
		ctx.Set("user_id", claims.UserID)
		ctx.Set("email", claims.Email)
		ctx.Set("role", claims.Role)
		ctx.Set("store_id", claims.StoreID)
		ctx.Set("branch_id", claims.BranchID)

		ctx.Next()
	}
}
