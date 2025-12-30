// internal/handlers/auth.go

package handlers

import (
	"context"
	"net/http"
	"time"

	"nova-kakhovka-ecity/internal/models"
	"nova-kakhovka-ecity/pkg/auth"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	userCollection *mongo.Collection
	jwtManager     *auth.JWTManager
}

// ✅ Request structures
type RegisterRequest struct {
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=6,max=100"`
	FirstName string `json:"first_name" binding:"required,min=2,max=50"`
	LastName  string `json:"last_name" binding:"required,min=2,max=50"`
	Phone     string `json:"phone,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// ✅ Response structure з правильними JSON tags
type AuthResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

func NewAuthHandler(userCollection *mongo.Collection, jwtManager *auth.JWTManager) *AuthHandler {
	return &AuthHandler{
		userCollection: userCollection,
		jwtManager:     jwtManager,
	}
}

// Register handles user registration
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Перевіряємо чи існує користувач з таким email
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var existingUser models.User
	err := h.userCollection.FindOne(ctx, bson.M{"email": req.Email}).Decode(&existingUser)
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "User with this email already exists",
		})
		return
	} else if err != mongo.ErrNoDocuments {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	// Хешуємо пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error hashing password",
		})
		return
	}

	// Створюємо нового користувача
	now := time.Now()
	user := models.User{
		Email:        req.Email,
		Phone:        req.Phone,
		PasswordHash: string(hashedPassword),
		FirstName:    req.FirstName,
		LastName:     req.LastName,

		// ✅ НОВИЙ КОД: Встановлюємо роль за замовчуванням
		Role:        string(models.RoleUser), // За замовчуванням USER
		IsModerator: false,                   // Legacy support

		IsVerified: false,
		IsBlocked:  false,
		Groups:     []primitive.ObjectID{},
		Interests:  []string{},
		Status: models.UserStatus{
			Message:   "",
			IsVisible: false,
			UpdatedAt: now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Зберігаємо користувача в базу даних
	result, err := h.userCollection.InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating user",
		})
		return
	}

	user.ID = result.InsertedID.(primitive.ObjectID)

	// Генеруємо JWT токен
	// ✅ 4 параметри: userID, email, role, isModerator
	token, err := h.jwtManager.GenerateToken(
		user.ID.Hex(),    // userID: string
		user.Email,       // email: string
		user.Role,        // role: string
		user.IsModerator, // isModerator: bool
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error generating token",
		})
		return
	}

	// ✅ ПРАВИЛЬНО: Використовуємо структуру AuthResponse
	c.JSON(http.StatusCreated, AuthResponse{
		Token: token,
		User:  &user, // User struct має всі поля включно з Role
	})
}

// Login handles user authentication
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Знаходимо користувача
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user models.User
	err := h.userCollection.FindOne(ctx, bson.M{"email": req.Email}).Decode(&user)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	// Перевіряємо чи користувач заблокований
	if user.IsBlocked {
		// Формуємо детальну відповідь для заблокованого користувача
		response := models.BlockedUserResponse{
			Error:     "Account is blocked",
			IsBlocked: true,
			Message:   "Ваш акаунт заблоковано. Будь ласка, зверніться до модератора для отримання додаткової інформації.",
		}

		// Додаємо причину блокування, якщо вона є
		if user.BlockReason != "" {
			response.BlockReason = user.BlockReason
		}

		// Додаємо час блокування, якщо він є
		if user.BlockedAt != nil {
			response.BlockedAt = *user.BlockedAt
		}

		c.JSON(http.StatusForbidden, response)
		return
	}

	// Перевіряємо пароль
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	// ✅ МІГРАЦІЯ: Якщо у користувача немає ролі (legacy users)
	if user.Role == "" {
		// Встановлюємо роль на основі is_moderator
		if user.IsModerator {
			user.Role = string(models.RoleModerator)
		} else {
			user.Role = string(models.RoleUser)
		}

		// Оновлюємо в базі даних
		_, err = h.userCollection.UpdateOne(
			ctx,
			bson.M{"_id": user.ID},
			bson.M{"$set": bson.M{"role": user.Role}},
		)
		if err != nil {
			// Логуємо помилку, але не блокуємо login
			// Користувач все одно зможе увійти
			println("Warning: Failed to migrate user role:", err.Error())
		}
	}

	// Оновлюємо last_login_at
	now := time.Now()
	_, err = h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{"last_login_at": now}},
	)
	// Ігноруємо помилку - не критично

	// Генеруємо JWT токен
	token, err := h.jwtManager.GenerateToken(
		user.ID.Hex(),
		user.Email,
		user.Role,
		user.IsModerator,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error generating token",
		})
		return
	}

	// ✅ ПРАВИЛЬНО: Використовуємо структуру AuthResponse
	c.JSON(http.StatusOK, AuthResponse{
		Token: token,
		User:  &user, // User struct має всі поля включно з Role
	})
}

// GetProfile returns current user profile
func (h *AuthHandler) GetProfile(c *gin.Context) {
	// Отримуємо user_id з JWT claims (встановлюється middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	// Конвертуємо в ObjectID
	objectID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Знаходимо користувача
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

	// Повертаємо профіль (без password_hash)
	c.JSON(http.StatusOK, user)
}

// UpdateProfile updates user profile
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	// Отримуємо user_id з JWT
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	objectID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Парсимо request body
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Видаляємо поля які не можна оновлювати через цей endpoint
	delete(updates, "email")
	delete(updates, "password_hash")
	delete(updates, "role")
	delete(updates, "is_moderator")
	delete(updates, "is_blocked")
	delete(updates, "is_verified")
	delete(updates, "_id")

	// Додаємо updated_at
	updates["updated_at"] = time.Now()

	// Оновлюємо користувача
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": objectID},
		bson.M{"$set": updates},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating profile",
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
		"message": "Profile updated successfully",
	})
}

// ChangePassword змінює пароль користувача
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	type ChangePasswordRequest struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return
	}

	// Отримуємо ID користувача з контексту
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Unauthorized",
		})
		return
	}

	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Знаходимо користувача
	var user models.User
	err := h.userCollection.FindOne(ctx, bson.M{"_id": userIDObj}).Decode(&user)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	// Перевіряємо старий пароль
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Incorrect old password",
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

	// Оновлюємо пароль
	_, err = h.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": userIDObj},
		bson.M{
			"$set": bson.M{
				"password_hash": string(hashedPassword),
				"updated_at":    time.Now(),
			},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating password",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Password changed successfully",
	})
}
