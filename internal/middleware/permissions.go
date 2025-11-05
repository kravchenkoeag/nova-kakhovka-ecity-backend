// internal/middleware/permissions.go

package middleware

import (
	"net/http"

	"nova-kakhovka-ecity/internal/models"

	"github.com/gin-gonic/gin"
)

// RequirePermission створює middleware для перевірки конкретного дозволення
// 🔒 Використовується для захисту ендпоінтів на рівні Backend
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Отримуємо роль з контексту (встановлюється AuthMiddleware)
		roleInterface, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not authenticated",
			})
			c.Abort()
			return
		}

		roleStr, ok := roleInterface.(string)
		if !ok || roleStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid user role",
			})
			c.Abort()
			return
		}

		// Конвертуємо string в UserRole
		userRole := models.UserRole(roleStr)

		// Перевіряємо чи роль валідна
		if !userRole.IsValid() {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Invalid role",
			})
			c.Abort()
			return
		}

		// Конвертуємо permission string в Permission enum
		requiredPermission := models.Permission(permission)

		// Перевіряємо чи користувач має необхідне дозволення
		if !userRole.HasPermission(requiredPermission) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":     "Insufficient permissions",
				"required":  permission,
				"user_role": roleStr,
			})
			c.Abort()
			return
		}

		// Користувач має необхідне дозволення - продовжуємо
		c.Next()
	}
}

// RequireRole створює middleware для перевірки мінімальної ролі
// 🔒 Використовується для захисту ендпоінтів на рівні Backend
func RequireRole(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Отримуємо роль з контексту (встановлюється AuthMiddleware)
		roleInterface, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not authenticated",
			})
			c.Abort()
			return
		}

		roleStr, ok := roleInterface.(string)
		if !ok || roleStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid user role",
			})
			c.Abort()
			return
		}

		// Конвертуємо strings в UserRole
		userRole := models.UserRole(roleStr)
		requiredRole := models.UserRole(minRole)

		// Перевіряємо чи ролі валідні
		if !userRole.IsValid() || !requiredRole.IsValid() {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Invalid role",
			})
			c.Abort()
			return
		}

		// Перевіряємо чи роль користувача вища або рівна необхідній
		if !userRole.IsHigherOrEqual(requiredRole) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":         "Insufficient permissions",
				"required_role": minRole,
				"user_role":     roleStr,
			})
			c.Abort()
			return
		}

		// Користувач має необхідну роль - продовжуємо
		c.Next()
	}
}

// RequireAnyRole створює middleware для перевірки однієї з можливих ролей
// 🔒 Використовується коли endpoint доступний для кількох ролей
func RequireAnyRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Отримуємо роль з контексту
		roleInterface, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not authenticated",
			})
			c.Abort()
			return
		}

		roleStr, ok := roleInterface.(string)
		if !ok || roleStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid user role",
			})
			c.Abort()
			return
		}

		userRole := models.UserRole(roleStr)

		// Перевіряємо чи користувач має одну з дозволених ролей
		hasRole := false
		for _, allowedRole := range roles {
			if userRole == models.UserRole(allowedRole) {
				hasRole = true
				break
			}
		}

		if !hasRole {
			c.JSON(http.StatusForbidden, gin.H{
				"error":          "Insufficient permissions",
				"required_roles": roles,
				"user_role":      roleStr,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAnyPermission створює middleware для перевірки одного з дозволів
// 🔒 Використовується коли endpoint доступний при наявності будь-якого з дозволів
func RequireAnyPermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Отримуємо роль з контексту
		roleInterface, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not authenticated",
			})
			c.Abort()
			return
		}

		roleStr, ok := roleInterface.(string)
		if !ok || roleStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid user role",
			})
			c.Abort()
			return
		}

		userRole := models.UserRole(roleStr)

		// Перевіряємо чи користувач має хоча б одне з дозволень
		hasPermission := false
		for _, permission := range permissions {
			if userRole.HasPermission(models.Permission(permission)) {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{
				"error":                "Insufficient permissions",
				"required_permissions": permissions,
				"user_role":            roleStr,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
