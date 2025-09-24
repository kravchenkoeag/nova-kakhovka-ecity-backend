package middleware

import (
	"net/http"
	"strings"

	"nova-kakhovka-ecity/pkg/auth"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header is required",
			})
			c.Abort()
			return
		}

		// Проверяем формат "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{ // Исправлена опечатка здесь
				"error": "Invalid authorization header format",
			})
			c.Abort()
			return
		}

		token := parts[1]
		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid token",
			})
			c.Abort()
			return
		}

		// Добавляем информацию о пользователе в контекст
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("is_moderator", claims.IsModerator)

		c.Next()
	}
}

func ModeratorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		isModerator, exists := c.Get("is_moderator")
		if !exists || !isModerator.(bool) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Moderator access required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
