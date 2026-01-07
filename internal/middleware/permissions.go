// internal/middleware/permissions.go

package middleware

import (
	"net/http"

	"nova-kakhovka-ecity/internal/models"

	"github.com/gin-gonic/gin"
)

/**
 * RequirePermission - перевіряє чи користувач має конкретний дозвіл
 * 🔒 Використовується для захисту endpoints на рівні Backend
 *
 * Працює через систему Permission (models.Permission)
 * Кожна роль має свій набір дозволів через models.GetRolePermissions()
 *
 * Приклад:
 * router.POST("/announcements",
 *     middleware.AuthMiddleware(jwtManager),
 *     middleware.RequirePermission(string(models.PermissionCreateAnnouncement)),
 *     handler.CreateAnnouncement)
 */
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ✅ ВИПРАВЛЕНО: Отримуємо роль з "user_role" (встановлюється AuthMiddleware)
		roleInterface, exists := c.Get("user_role")
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

/**
 * RequireMinimumRole - перевіряє чи користувач має мінімальну роль
 * 🔒 Використовується для захисту endpoints на рівні Backend
 *
 * Використовує ієрархію ролей: USER < MODERATOR < ADMIN < SUPER_ADMIN
 *
 * Приклад:
 * router.GET("/analytics",
 *     middleware.AuthMiddleware(jwtManager),
 *     middleware.RequireMinimumRole(string(models.RoleModerator)),
 *     handler.GetAnalytics)
 *
 * ПРИМІТКА: Ця функція відрізняється від RequireRole в auth.go
 * - RequireMinimumRole - перевіряє мінімальну роль (з ієрархією)
 * - RequireRole (auth.go) - перевіряє точний список ролей
 */
func RequireMinimumRole(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ✅ ВИПРАВЛЕНО: Отримуємо роль з "user_role"
		roleInterface, exists := c.Get("user_role")
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

/**
 * RequireAnyRole - перевіряє чи користувач має одну з можливих ролей
 * 🔒 Використовується коли endpoint доступний для кількох конкретних ролей
 *
 * На відміну від RequireMinimumRole, перевіряє точну відповідність ролі
 * без урахування ієрархії
 *
 * Приклад:
 * router.POST("/reports",
 *     middleware.AuthMiddleware(jwtManager),
 *     middleware.RequireAnyRole(
 *         string(models.RoleModerator),
 *         string(models.RoleAdmin),
 *     ),
 *     handler.CreateReport)
 */
func RequireAnyRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ✅ ВИПРАВЛЕНО: Отримуємо роль з "user_role"
		roleInterface, exists := c.Get("user_role")
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

/**
 * RequireAnyPermission - перевіряє чи користувач має хоча б один з дозволів
 * 🔒 Використовується коли endpoint доступний при наявності будь-якого з дозволів
 *
 * Приклад:
 * router.PUT("/content/:id",
 *     middleware.AuthMiddleware(jwtManager),
 *     middleware.RequireAnyPermission(
 *         string(models.PermissionEditOwnAnnouncement),
 *         string(models.PermissionModerateAnnouncement),
 *     ),
 *     handler.UpdateContent)
 */
func RequireAnyPermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ✅ ВИПРАВЛЕНО: Отримуємо роль з "user_role"
		roleInterface, exists := c.Get("user_role")
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

/**
 * RequireAllPermissions - перевіряє чи користувач має всі з вказаних дозволів
 * 🔒 Використовується коли endpoint вимагає комбінацію дозволів
 *
 * Приклад:
 * router.DELETE("/users/:id",
 *     middleware.AuthMiddleware(jwtManager),
 *     middleware.RequireAllPermissions(
 *         string(models.PermissionManageUsers),
 *         string(models.PermissionBlockUser),
 *     ),
 *     handler.DeleteUser)
 */
func RequireAllPermissions(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ✅ ВИПРАВЛЕНО: Отримуємо роль з "user_role"
		roleInterface, exists := c.Get("user_role")
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

		// Перевіряємо чи користувач має всі необхідні дозволення
		missingPermissions := []string{}
		for _, permission := range permissions {
			if !userRole.HasPermission(models.Permission(permission)) {
				missingPermissions = append(missingPermissions, permission)
			}
		}

		if len(missingPermissions) > 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"error":                "Insufficient permissions",
				"required_permissions": permissions,
				"missing_permissions":  missingPermissions,
				"user_role":            roleStr,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

/**
 * RequireOwnerOrPermission - перевіряє чи користувач є власником ресурсу
 * або має конкретний дозвіл (наприклад, модератор)
 *
 * 🔒 Використовується для endpoints де власник може редагувати своє,
 * а модератор - будь-яке
 *
 * Приклад:
 * router.PUT("/announcements/:id",
 *     middleware.AuthMiddleware(jwtManager),
 *     middleware.RequireOwnerOrPermission(
 *         "author_id", // поле в базі даних
 *         string(models.PermissionModerateAnnouncement),
 *     ),
 *     handler.UpdateAnnouncement)
 *
 * ПРИМІТКА: Потребує додаткової імплементації в handler для перевірки ownership
 */
func RequireOwnerOrPermission(ownerField string, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ✅ ВИПРАВЛЕНО: Отримуємо роль та user_id з context
		userID, userExists := c.Get("user_id")
		roleInterface, roleExists := c.Get("user_role")

		if !userExists || !roleExists {
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

		// Якщо користувач має потрібний дозвіл - пропускаємо
		if userRole.HasPermission(models.Permission(permission)) {
			c.Next()
			return
		}

		// Якщо немає дозволу - handler має перевірити ownership
		// Встановлюємо flag для handler
		c.Set("require_ownership", true)
		c.Set("owner_user_id", userID)

		c.Next()
	}
}

/**
 * Helper: IsCurrentUserOwner - перевіряє чи поточний користувач є власником
 * Використовується в handlers після RequireOwnerOrPermission
 *
 * Приклад використання в handler:
 *
 * func (h *AnnouncementHandler) UpdateAnnouncement(c *gin.Context) {
 *     requireOwnership, _ := c.Get("require_ownership")
 *
 *     if requireOwnership.(bool) {
 *         ownerUserID, _ := c.Get("owner_user_id")
 *         if announcement.AuthorID != ownerUserID {
 *             c.JSON(403, gin.H{"error": "Not authorized"})
 *             return
 *         }
 *     }
 *
 *     // Продовжуємо оновлення...
 * }
 */
