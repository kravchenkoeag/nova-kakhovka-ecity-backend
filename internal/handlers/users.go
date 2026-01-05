// internal/handlers/users.go

package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"nova-kakhovka-ecity/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

// UsersHandler обробляє запити для управління користувачами
// 🔒 Всі методи вимагають автентифікації та відповідних прав доступу
type UsersHandler struct {
	userCollection *mongo.Collection
}

// Request/Response структури

// UpdatePasswordRequest - запит на зміну пароля
type UpdatePasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required,min=8,max=100"`
}

// BlockUserRequest - запит на блокування користувача
type BlockUserRequest struct {
	IsBlocked bool   `json:"is_blocked" binding:"required"`
	Reason    string `json:"reason,omitempty"`
}

// BlockUserResponse - відповідь на блокування користувача
type BlockUserResponse struct {
	Message   string `json:"message"`
	UserID    string `json:"user_id"`
	IsBlocked bool   `json:"is_blocked"`
}

// UsersListResponse - відповідь зі списком користувачів
type UsersListResponse struct {
	Data       []models.User `json:"data"`  // ✅ Основні дані в полі data
	Users      []models.User `json:"users"` // Legacy підтримка
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	Limit      int           `json:"limit"`
	TotalPages int           `json:"total_pages"`
}

// UserStatsData - вкладений об'єкт зі статистикою
type UserStatsData struct {
	Total         int64 `json:"total"`
	Active        int64 `json:"active"`
	Blocked       int64 `json:"blocked"`
	Admins        int64 `json:"admins"`
	VerifiedUsers int64 `json:"verified_users"`
	Moderators    int64 `json:"moderators"`
}

// UserStatsResponse - відповідь зі статистикою користувачів
type UserStatsResponse struct {
	Data UserStatsData `json:"data"` // ✅ Всі дані в полі data
}

// NewUsersHandler створює новий обробник користувачів
func NewUsersHandler(userCollection *mongo.Collection) *UsersHandler {
	return &UsersHandler{
		userCollection: userCollection,
	}
}

// GetAllUsers отримує список всіх користувачів з пагінацією та фільтрацією
// 🔒 Вимагає права: Permission.USERS_MANAGE або Permission.MANAGE_USERS
// Метод: GET /api/v1/users
func (h *UsersHandler) GetAllUsers(c *gin.Context) {
	// Отримуємо параметри запиту
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "20")
	search := c.Query("search")
	role := c.Query("role")
	isBlockedStr := c.Query("is_blocked")

	page, _ := strconv.Atoi(pageStr)
	limit, _ := strconv.Atoi(limitStr)

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// Будуємо фільтр
	filter := bson.M{}

	// Пошук за email або ім'ям
	if search != "" {
		filter["$or"] = []bson.M{
			{"email": bson.M{"$regex": search, "$options": "i"}},
			{"first_name": bson.M{"$regex": search, "$options": "i"}},
			{"last_name": bson.M{"$regex": search, "$options": "i"}},
		}
	}

	// Фільтр за роллю
	if role != "" {
		filter["role"] = role
	}

	// Фільтр за статусом блокування
	if isBlockedStr != "" {
		isBlocked, _ := strconv.ParseBool(isBlockedStr)
		filter["is_blocked"] = isBlocked
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Підраховуємо загальну кількість
	total, err := h.userCollection.CountDocuments(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to count users",
		})
		return
	}

	// Отримуємо користувачів з пагінацією
	skip := (page - 1) * limit
	findOptions := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := h.userCollection.Find(ctx, filter, findOptions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch users",
		})
		return
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err = cursor.All(ctx, &users); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to decode users",
		})
		return
	}

	// Обчислюємо загальну кількість сторінок
	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}

	response := UsersListResponse{
		Data:       users, // ✅ Основні дані
		Users:      users, // Legacy підтримка
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}

	c.JSON(http.StatusOK, response)
}

// GetUserByID отримує користувача за ID
// 🔒 Вимагає права: Permission.USERS_MANAGE або Permission.MANAGE_USERS
// Метод: GET /api/v1/users/:id
func (h *UsersHandler) GetUserByID(c *gin.Context) {
	userID := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user models.User
	err = h.userCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// UpdateUserPassword змінює пароль користувача
// 🔒 Вимагає права: Permission.MANAGE_USERS (тільки ADMIN+)
// Метод: PUT /api/v1/users/:id/password
func (h *UsersHandler) UpdateUserPassword(c *gin.Context) {
	userID := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	var req UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Хешуємо новий пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error hashing password",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Оновлюємо пароль
	result, err := h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": objectID},
		bson.M{
			"$set": bson.M{
				"password_hash": string(hashedPassword),
				"updated_at":    time.Now(),
			},
		},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update password",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Password updated successfully",
	})
}

// BlockUser блокує або розблоковує користувача
// 🔒 Вимагає права: Permission.BLOCK_USER (ADMIN+)
// Метод: PUT /api/v1/users/:id/block
func (h *UsersHandler) BlockUser(c *gin.Context) {
	userID := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	var req BlockUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Підготовка оновлення
	update := bson.M{
		"is_blocked": req.IsBlocked,
		"updated_at": time.Now(),
	}

	if req.IsBlocked {
		// Блокуємо користувача
		update["block_reason"] = req.Reason
		update["blocked_at"] = time.Now()
	} else {
		// Розблоковуємо користувача
		update["block_reason"] = nil
		update["blocked_at"] = nil
	}

	// Оновлюємо користувача
	result, err := h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": objectID},
		bson.M{"$set": update},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update user",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	response := BlockUserResponse{
		Message:   "User status updated successfully",
		UserID:    userID,
		IsBlocked: req.IsBlocked,
	}

	c.JSON(http.StatusOK, response)
}

// GetUser повертає детальну інформацію про користувача
func (h *UsersHandler) GetUser(c *gin.Context) {
	userID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid user ID",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user models.User
	err = h.userCollection.FindOne(
		ctx,
		bson.M{"_id": userID},
		options.FindOne().SetProjection(bson.M{"password_hash": 0}),
	).Decode(&user)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error fetching user",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateUser оновлює інформацію про користувача
func (h *UsersHandler) UpdateUser(c *gin.Context) {
	userID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid user ID",
			"details": err.Error(),
		})
		return
	}

	type UpdateUserRequest struct {
		FullName    string `json:"full_name,omitempty"`
		Phone       string `json:"phone,omitempty"`
		DateOfBirth string `json:"date_of_birth,omitempty"`
		Gender      string `json:"gender,omitempty"`
		Address     string `json:"address,omitempty"`
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Перевіряємо чи користувач існує
	var existingUser models.User
	err = h.userCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&existingUser)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching user",
		})
		return
	}

	// Формуємо оновлення
	update := bson.M{
		"updated_at": time.Now(),
	}

	if req.FullName != "" {
		update["full_name"] = req.FullName
	}
	if req.Phone != "" {
		update["phone"] = req.Phone
	}
	if req.DateOfBirth != "" {
		update["date_of_birth"] = req.DateOfBirth
	}
	if req.Gender != "" {
		update["gender"] = req.Gender
	}
	if req.Address != "" {
		update["address"] = req.Address
	}

	// Оновлюємо користувача
	_, err = h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{"$set": update},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error updating user",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User updated successfully",
	})
}

// DeleteUser видаляє користувача (м'яке видалення)
func (h *UsersHandler) DeleteUser(c *gin.Context) {
	userID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid user ID",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Перевіряємо чи користувач існує
	var user models.User
	err = h.userCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching user",
		})
		return
	}

	// М'яке видалення - встановлюємо прапорець is_deleted
	// Альтернативно можна використовувати DeleteOne для повного видалення
	_, err = h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"is_deleted": true,
				"deleted_at": time.Now(),
				"is_blocked": true, // Також блокуємо
			},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error deleting user",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User deleted successfully",
	})
}

// ========================================
// УПРАВЛІННЯ СТАТУСОМ КОРИСТУВАЧА
// ========================================

// UnblockUser розблоковує користувача
func (h *UsersHandler) UnblockUser(c *gin.Context) {
	userID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid user ID",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Розблоковуємо користувача
	result, err := h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"is_blocked":   false,
				"block_reason": "",
				"blocked_at":   nil,
				"updated_at":   time.Now(),
			},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error unblocking user",
			"details": err.Error(),
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User unblocked successfully",
	})
}

// VerifyUser верифікує користувача
func (h *UsersHandler) VerifyUser(c *gin.Context) {
	userID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid user ID",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Верифікуємо користувача
	result, err := h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"is_verified": true,
				"verified_at": time.Now(),
				"updated_at":  time.Now(),
			},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error verifying user",
			"details": err.Error(),
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User verified successfully",
	})
}

// UpdateUserRole оновлює роль користувача
func (h *UsersHandler) UpdateUserRole(c *gin.Context) {
	userID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid user ID",
			"details": err.Error(),
		})
		return
	}

	type UpdateRoleRequest struct {
		Role string `json:"role" binding:"required,oneof=USER MODERATOR ADMIN"`
	}

	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid role",
			"details": "Role must be USER, MODERATOR, or ADMIN",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Оновлюємо роль
	result, err := h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"role":       req.Role,
				"updated_at": time.Now(),
			},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error updating role",
			"details": err.Error(),
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User role updated successfully",
		"role":    req.Role,
	})
}

// GetUserStats отримує статистику користувачів
// 🔒 Вимагає права: Permission.VIEW_ANALYTICS або Permission.USERS_MANAGE
// Метод: GET /api/v1/users/stats
func (h *UsersHandler) GetUserStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Загальна кількість користувачів
	totalUsers, _ := h.userCollection.CountDocuments(ctx, bson.M{})

	// Верифіковані користувачі
	verifiedUsers, _ := h.userCollection.CountDocuments(ctx, bson.M{"is_verified": true})

	// Заблоковані користувачі
	blockedUsers, _ := h.userCollection.CountDocuments(ctx, bson.M{"is_blocked": true})

	// Користувачі за ролями
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":   "$role",
			"count": bson.M{"$sum": 1},
		}}},
	}

	cursor, err := h.userCollection.Aggregate(ctx, pipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching role statistics",
		})
		return
	}
	defer cursor.Close(ctx)

	var roleStats []bson.M
	if err := cursor.All(ctx, &roleStats); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding statistics",
		})
		return
	}

	// Нові користувачі за останній місяць
	oneMonthAgo := time.Now().AddDate(0, -1, 0)
	newUsersLastMonth, _ := h.userCollection.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": oneMonthAgo},
	})

	// Нові користувачі за останній тиждень
	oneWeekAgo := time.Now().AddDate(0, 0, -7)
	newUsersLastWeek, _ := h.userCollection.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": oneWeekAgo},
	})

	c.JSON(http.StatusOK, gin.H{
		"total_users":          totalUsers,
		"verified_users":       verifiedUsers,
		"blocked_users":        blockedUsers,
		"users_by_role":        roleStats,
		"new_users_last_month": newUsersLastMonth,
		"new_users_last_week":  newUsersLastWeek,
		"timestamp":            time.Now(),
	})
}

// SearchUsers выполняет поиск пользователей (упрощенная версия для публичного использования)
func (h *UsersHandler) SearchUsers(c *gin.Context) {
	query := c.Query("q")
	limitStr := c.DefaultQuery("limit", "20")

	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{}

	// Текстовый поиск по email, имени и фамилии
	if query != "" {
		filter["$or"] = []bson.M{
			{"email": bson.M{"$regex": query, "$options": "i"}},
			{"first_name": bson.M{"$regex": query, "$options": "i"}},
			{"last_name": bson.M{"$regex": query, "$options": "i"}},
		}
	}

	opts := options.Find().
		SetLimit(int64(limit)).
		SetProjection(bson.M{"password_hash": 0}). // Исключаем пароль
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := h.userCollection.Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error searching users",
		})
		return
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding users",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"count": len(users),
	})
}

// BanUser блокирует пользователя (для модераторов, упрощенная версия)
func (h *UsersHandler) BanUser(c *gin.Context) {
	userID := c.Param("id")
	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Блокируем пользователя
	update := bson.M{
		"is_blocked":  true,
		"blocked_at":  time.Now(),
		"updated_at":  time.Now(),
	}

	result, err := h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": objectID},
		bson.M{"$set": update},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to ban user",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User banned successfully",
		"user_id": userID,
	})
}

// UnbanUser разблокирует пользователя (для модераторов, упрощенная версия)
func (h *UsersHandler) UnbanUser(c *gin.Context) {
	userID := c.Param("id")
	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Разблокируем пользователя
	update := bson.M{
		"is_blocked":   false,
		"block_reason": "",
		"blocked_at":   nil,
		"updated_at":   time.Now(),
	}

	result, err := h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": objectID},
		bson.M{"$set": update},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to unban user",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User unbanned successfully",
		"user_id": userID,
	})
}
