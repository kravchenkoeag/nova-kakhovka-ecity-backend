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

type RegisterRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=6,max=100"`
	FirstName string `json:"first_name" validate:"required,min=2,max=50"`
	LastName  string `json:"last_name" validate:"required,min=2,max=50"`
	Phone     string `json:"phone,omitempty" validate:"omitempty,min=10,max=15"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

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

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Проверяем, существует ли пользователь с таким email
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

	// Хешируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error hashing password",
		})
		return
	}

	// Создаем нового пользователя
	now := time.Now()
	user := models.User{
		Email:        req.Email,
		Phone:        req.Phone,
		PasswordHash: string(hashedPassword),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		IsVerified:   false,
		IsModerator:  false,
		IsBlocked:    false,
		Groups:       []primitive.ObjectID{},
		Interests:    []string{},
		Status: models.UserStatus{
			Message:   "",
			IsVisible: false,
			UpdatedAt: now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Сохраняем пользователя в базу данных
	result, err := h.userCollection.InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating user",
		})
		return
	}

	user.ID = result.InsertedID.(primitive.ObjectID)

	// Генерируем JWT токен
	token, err := h.jwtManager.GenerateToken(user.ID, user.Email, user.IsModerator)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error generating token",
		})
		return
	}

	// Убираем пароль из ответа
	user.PasswordHash = ""

	c.JSON(http.StatusCreated, AuthResponse{
		Token: token,
		User:  &user,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ищем пользователя по email
	var user models.User
	err := h.userCollection.FindOne(ctx, bson.M{"email": req.Email}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid email or password",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Проверяем, не заблокирован ли пользователь
	if user.IsBlocked {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Account is blocked",
		})
		return
	}

	// Проверяем пароль
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid email or password",
		})
		return
	}

	// Обновляем время последнего входа
	now := time.Now()
	_, err = h.userCollection.UpdateOne(ctx, bson.M{"_id": user.ID}, bson.M{
		"$set": bson.M{
			"last_login_at": now,
			"updated_at":    now,
		},
	})
	if err != nil {
		// Логируем ошибку, но не прерываем процесс входа
		// log.Printf("Error updating last login: %v", err)
	}

	// Генерируем JWT токен
	token, err := h.jwtManager.GenerateToken(user.ID, user.Email, user.IsModerator)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error generating token",
		})
		return
	}

	// Убираем пароль из ответа
	user.PasswordHash = ""

	c.JSON(http.StatusOK, AuthResponse{
		Token: token,
		User:  &user,
	})
}

func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user models.User
	err := h.userCollection.FindOne(ctx, bson.M{"_id": userIDObj}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Убираем пароль из ответа
	user.PasswordHash = ""

	c.JSON(http.StatusOK, user)
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	var updateData bson.M
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request data",
		})
		return
	}

	// Удаляем поля, которые нельзя обновлять через этот эндпоинт
	delete(updateData, "_id")
	delete(updateData, "password_hash")
	delete(updateData, "email")
	delete(updateData, "is_verified")
	delete(updateData, "is_moderator")
	delete(updateData, "is_blocked")
	delete(updateData, "created_at")

	// Добавляем временную метку обновления
	updateData["updated_at"] = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.userCollection.UpdateOne(ctx, bson.M{"_id": userIDObj}, bson.M{"$set": updateData})
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
