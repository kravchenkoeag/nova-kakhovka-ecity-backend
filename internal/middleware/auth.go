// internal/middleware/auth.go

package middleware

import (
	"net/http"
	"strings"

	"nova-kakhovka-ecity/pkg/auth"

	"github.com/gin-gonic/gin"
)

/**
 * AuthMiddleware - базова автентифікація через JWT
 * Перевіряє наявність та валідність токена
 * Додає в context: user_id, user_email, user_role, is_moderator
 */
func AuthMiddleware(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Отримуємо Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header is required",
			})
			c.Abort()
			return
		}

		// Перевіряємо формат "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authorization header format. Expected: Bearer <token>",
			})
			c.Abort()
			return
		}

		token := parts[1]

		// Валідуємо токен
		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired token",
			})
			c.Abort()
			return
		}

		// Додаємо інформацію про користувача в контекст
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)

		// Визначаємо is_moderator на основі ролі
		// Модераторами вважаються: MODERATOR, ADMIN, SUPER_ADMIN
		isModerator := claims.Role == "MODERATOR" ||
			claims.Role == "ADMIN" ||
			claims.Role == "SUPER_ADMIN"

		// Також перевіряємо legacy поле (для сумісності)
		if claims.IsModerator {
			isModerator = true
		}

		c.Set("is_moderator", isModerator)

		c.Next()
	}
}

/**
 * ModeratorMiddleware - перевіряє чи користувач є модератором
 * Використовується після AuthMiddleware
 *
 * Приклад використання:
 * protected.Use(middleware.AuthMiddleware(jwtManager))
 * protected.Use(middleware.ModeratorMiddleware())
 * protected.PUT("/petitions/:id/status", handler.UpdateStatus)
 */
func ModeratorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Перевіряємо наявність is_moderator в context
		isModerator, exists := c.Get("is_moderator")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
			})
			c.Abort()
			return
		}

		// Перевіряємо тип і значення
		isModeratorBool, ok := isModerator.(bool)
		if !ok || !isModeratorBool {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Moderator access required",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

/**
 * RequireRole - перевіряє чи користувач має конкретну роль
 * Можна передати одну або декілька ролей
 *
 * Приклад використання:
 * admin := api.Group("")
 * admin.Use(middleware.AuthMiddleware(jwtManager))
 * admin.Use(middleware.RequireRole("ADMIN", "SUPER_ADMIN"))
 */
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Отримуємо роль з context
		userRole, exists := c.Get("user_role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
			})
			c.Abort()
			return
		}

		// Конвертуємо в string
		userRoleStr, ok := userRole.(string)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid user role type",
			})
			c.Abort()
			return
		}

		// Перевіряємо чи роль користувача в списку дозволених
		hasRole := false
		for _, role := range allowedRoles {
			if userRoleStr == role {
				hasRole = true
				break
			}
		}

		if !hasRole {
			c.JSON(http.StatusForbidden, gin.H{
				"error":         "Insufficient permissions",
				"required_role": allowedRoles,
				"current_role":  userRoleStr,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

/**
 * OptionalAuth - опціональна автентифікація
 * Якщо токен присутній - валідує його та додає user_id в context
 * Якщо токена немає - дозволяє продовжити без автентифікації
 *
 * Використовується для публічних endpoints, які можуть працювати
 * по-різному для автентифікованих та неавтентифікованих користувачів
 *
 * Приклад: GET /petitions - показує draft тільки автору
 */
func OptionalAuth(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Отримуємо Authorization header
		authHeader := c.GetHeader("Authorization")

		// Якщо немає токена - просто продовжуємо
		if authHeader == "" {
			c.Next()
			return
		}

		// Перевіряємо формат
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			// Неправильний формат - ігноруємо, не блокуємо запит
			c.Next()
			return
		}

		token := parts[1]

		// Валідуємо токен
		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			// Невалідний токен - ігноруємо, не блокуємо запит
			c.Next()
			return
		}

		// Токен валідний - додаємо інформацію в context
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)

		isModerator := claims.Role == "MODERATOR" ||
			claims.Role == "ADMIN" ||
			claims.Role == "SUPER_ADMIN" ||
			claims.IsModerator

		c.Set("is_moderator", isModerator)

		c.Next()
	}
}

/**
 * RateLimitByUser - обмеження швидкості запитів на основі user_id
 * Використовується після AuthMiddleware
 *
 * TODO: Реалізувати через Redis або in-memory cache
 */
func RateLimitByUser(requestsPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement rate limiting
		// For now, just pass through
		c.Next()
	}
}

/**
 * AdminOnly - швидкий хелпер для admin-only endpoints
 * Комбінація Auth + RequireRole("ADMIN", "SUPER_ADMIN")
 */
func AdminOnly(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Викликаємо AuthMiddleware
		authMiddleware := AuthMiddleware(jwtManager)
		authMiddleware(c)

		// Якщо автентифікація не пройшла - зупиняємо
		if c.IsAborted() {
			return
		}

		// Перевіряємо роль
		roleMiddleware := RequireRole("ADMIN", "SUPER_ADMIN")
		roleMiddleware(c)
	}
}

/**
 * ModeratorOrAdmin - швидкий хелпер для moderator/admin endpoints
 * Комбінація Auth + RequireRole("MODERATOR", "ADMIN", "SUPER_ADMIN")
 */
func ModeratorOrAdmin(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Викликаємо AuthMiddleware
		authMiddleware := AuthMiddleware(jwtManager)
		authMiddleware(c)

		// Якщо автентифікація не пройшла - зупиняємо
		if c.IsAborted() {
			return
		}

		// Перевіряємо роль
		roleMiddleware := RequireRole("MODERATOR", "ADMIN", "SUPER_ADMIN")
		roleMiddleware(c)
	}
}
