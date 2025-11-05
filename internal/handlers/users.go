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

// GetUserStats отримує статистику користувачів
// 🔒 Вимагає права: Permission.VIEW_ANALYTICS або Permission.USERS_MANAGE
// Метод: GET /api/v1/users/stats
func (h *UsersHandler) GetUserStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Загальна кількість користувачів
	total, err := h.userCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get total users",
		})
		return
	}

	// Активні користувачі (не заблоковані)
	active, err := h.userCollection.CountDocuments(ctx, bson.M{"is_blocked": false})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get active users",
		})
		return
	}

	// Заблоковані користувачі
	blocked, err := h.userCollection.CountDocuments(ctx, bson.M{"is_blocked": true})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get blocked users",
		})
		return
	}

	// Верифіковані користувачі
	verified, err := h.userCollection.CountDocuments(ctx, bson.M{"is_verified": true})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get verified users",
		})
		return
	}

	// Модератори
	moderators, err := h.userCollection.CountDocuments(ctx, bson.M{
		"role": bson.M{"$in": []string{
			string(models.RoleModerator),
		}},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get moderators",
		})
		return
	}

	// Адміністратори (ADMIN + SUPER_ADMIN)
	admins, err := h.userCollection.CountDocuments(ctx, bson.M{
		"role": bson.M{"$in": []string{
			string(models.RoleAdmin),
			string(models.RoleSuperAdmin),
		}},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get admins",
		})
		return
	}

	response := UserStatsResponse{
		Data: UserStatsData{
			Total:         total,
			Active:        active,
			Blocked:       blocked,
			Admins:        admins,
			VerifiedUsers: verified,
			Moderators:    moderators,
		},
	}

	c.JSON(http.StatusOK, response)
}
