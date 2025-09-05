package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo/options"
	"net/http"
	"strconv"
	"time"

	"nova-kakhovka-ecity/internal/config"
	"nova-kakhovka-ecity/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type NotificationService struct {
	config                 *config.Config
	userCollection         *mongo.Collection
	notificationCollection *mongo.Collection
	httpClient            *http.Client
}

type FCMMessage struct {
	To           string                 `json:"to,omitempty"`
	RegistrationIDs []string           `json:"registration_ids,omitempty"`
	Notification FCMNotification       `json:"notification"`
	Data         map[string]interface{} `json:"data,omitempty"`
	Priority     string                 `json:"priority"`
	TimeToLive   int                    `json:"time_to_live,omitempty"`
}

type FCMNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Icon  string `json:"icon,omitempty"`
	Sound string `json:"sound,omitempty"`
	Color string `json:"color,omitempty"`
}

type FCMResponse struct {
	MulticastID  int64           `json:"multicast_id"`
	Success      int             `json:"success"`
	Failure      int             `json:"failure"`
	CanonicalIDs int             `json:"canonical_ids"`
	Results      []FCMResult     `json:"results"`
}

type FCMResult struct {
	MessageID      string `json:"message_id,omitempty"`
	RegistrationID string `json:"registration_id,omitempty"`
	Error          string `json:"error,omitempty"`
}

// Добавляем поле FCM токена в модель пользователя
type UserDeviceToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	FCMToken  string            `bson:"fcm_token" json:"fcm_token"`
	Platform  string            `bson:"platform" json:"platform"` // android, ios, web
	IsActive  bool              `bson:"is_active" json:"is_active"`
	CreatedAt time.Time         `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time         `bson:"updated_at" json:"updated_at"`
}

// Модель для хранения уведомлений в базе
type StoredNotification struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	UserID       primitive.ObjectID `bson:"user_id" json:"user_id"`
	Title        string            `bson:"title" json:"title"`
	Body         string            `bson:"body" json:"body"`
	Type         string            `bson:"type" json:"type"` // message, event, announcement, system
	RelatedID    *primitive.ObjectID `bson:"related_id,omitempty" json:"related_id,omitempty"`
	Data         map[string]interface{} `bson:"data,omitempty" json:"data,omitempty"`
	IsRead       bool              `bson:"is_read" json:"is_read"`
	IsSent       bool              `bson:"is_sent" json:"is_sent"`
	CreatedAt    time.Time         `bson:"created_at" json:"created_at"`
	ReadAt       *time.Time        `bson:"read_at,omitempty" json:"read_at,omitempty"`
}

const (
	FCMEndpoint = "https://fcm.googleapis.com/fcm/send"

	// Типы уведомлений
	NotificationTypeMessage      = "message"
	NotificationTypeEvent        = "event"
	NotificationTypeAnnouncement = "announcement"
	NotificationTypeSystem       = "system"
	NotificationTypeEmergency    = "emergency"
)

func NewNotificationService(cfg *config.Config, userCollection, notificationCollection *mongo.Collection) *NotificationService {
	return &NotificationService{
		config:                 cfg,
		userCollection:         userCollection,
		notificationCollection: notificationCollection,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Отправка уведомления одному пользователю
func (ns *NotificationService) SendNotificationToUser(ctx context.Context, userID primitive.ObjectID, title, body, notificationType string, data map[string]interface{}, relatedID *primitive.ObjectID) error {
	// Сохраняем уведомление в базе данных
	notification := StoredNotification{
		UserID:    userID,
		Title:     title,
		Body:      body,
		Type:      notificationType,
		RelatedID: relatedID,
		Data:      data,
		IsRead:    false,
		IsSent:    false,
		CreatedAt: time.Now(),
	}

	result, err := ns.notificationCollection.InsertOne(ctx, notification)
	if err != nil {
		return fmt.Errorf("failed to save notification: %w", err)
	}

	notification.ID = result.InsertedID.(primitive.ObjectID)

	// Получаем FCM токены пользователя
	tokens, err := ns.getUserFCMTokens(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user FCM tokens: %w", err)
	}

	if len(tokens) == 0 {
		// Помечаем как отправленное, даже если нет токенов
		ns.markNotificationAsSent(ctx, notification.ID)
		return nil
	}

	// Отправляем FCM уведомление
	err = ns.sendFCMNotification(tokens, title, body, data)
	if err != nil {
		return fmt.Errorf("failed to send FCM notification: %w", err)
	}

	// Помечаем уведомление как отправленное
	ns.markNotificationAsSent(ctx, notification.ID)

	return nil
}

// Отправка уведомления группе пользователей
func (ns *NotificationService) SendNotificationToUsers(ctx context.Context, userIDs []primitive.ObjectID, title, body, notificationType string, data map[string]interface{}, relatedID *primitive.ObjectID) error {
	var allTokens []string

	// Сохраняем уведомления для всех пользователей
	for _, userID := range userIDs {
		notification := StoredNotification{
			UserID:    userID,
			Title:     title,
			Body:      body,
			Type:      notificationType,
			RelatedID: relatedID,
			Data:      data,
			IsRead:    false,
			IsSent:    false,
			CreatedAt: time.Now(),
		}

		_, err := ns.notificationCollection.InsertOne(ctx, notification)
		if err != nil {
			continue // Продолжаем даже если не удалось сохранить одно уведомление
		}

		// Получаем токены для каждого пользователя
		tokens, err := ns.getUserFCMTokens(ctx, userID)
		if err != nil {
			continue
		}
		allTokens = append(allTokens, tokens...)
	}

	if len(allTokens) == 0 {
		return nil
	}

	// Отправляем FCM уведомление всем токенам
	err := ns.sendFCMNotification(allTokens, title, body, data)
	if err != nil {
		return fmt.Errorf("failed to send batch FCM notification: %w", err)
	}

	// Помечаем все уведомления как отправленные
	ns.notificationCollection.UpdateMany(ctx, bson.M{
		"user_id": bson.M{"$in": userIDs},
		"type":    notificationType,
		"is_sent": false,
		"created_at": bson.M{"$gte": time.Now().Add(-5 * time.Minute)}, // Последние 5 минут
	}, bson.M{
		"$set": bson.M{"is_sent": true},
	})

	return nil
}

// Отправка экстренного уведомления всем пользователям
func (ns *NotificationService) SendEmergencyNotification(ctx context.Context, title, body string, data map[string]interface{}) error {
	// Получаем всех активных пользователей
	cursor, err := ns.userCollection.Find(ctx, bson.M{
		"is_blocked": false,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}
	defer cursor.Close(ctx)

	var userIDs []primitive.ObjectID
	for cursor.Next(ctx) {
		var user models.User
		if err := cursor.Decode(&user); err != nil {
			continue
		}
		userIDs = append(userIDs, user.ID)
	}

	return ns.SendNotificationToUsers(ctx, userIDs, title, body, NotificationTypeEvent, data, &eventID)
}

func (ns *NotificationService) SendAnnouncementModerationNotification(ctx context.Context, userID primitive.ObjectID, announcementTitle string, announcementID primitive.ObjectID, approved bool) error {
	data := map[string]interface{}{
		"type":            NotificationTypeAnnouncement,
		"announcement_id": announcementID.Hex(),
		"approved":        approved,
		"action":          "open_announcement",
	}

	var title, body string
	if approved {
		title = "Объявление одобрено"
		body = fmt.Sprintf("Ваше объявление '%s' было одобрено и опубликовано", announcementTitle)
	} else {
		title = "Объявление отклонено"
		body = fmt.Sprintf("Ваше объявление '%s' было отклонено модератором", announcementTitle)
	}

	return ns.SendNotificationToUser(ctx, userID, title, body, NotificationTypeAnnouncement, data, &announcementID)
}

func (ns *NotificationService) SendSystemMaintenanceNotification(ctx context.Context, message string, maintenanceDate time.Time) error {
	data := map[string]interface{}{
		"type":             NotificationTypeSystem,
		"maintenance_date": maintenanceDate.Format(time.RFC3339),
		"action":           "none",
	}

	title := "Техническое обслуживание"
	body := fmt.Sprintf("Плановые работы %s. %s", maintenanceDate.Format("02.01.2006 15:04"), message)

	// Получаем всех пользователей
	cursor, err := ns.userCollection.Find(context.Background(), bson.M{
		"is_blocked": false,
	})
	if err != nil {
		return err
	}
	defer cursor.Close(context.Background())

	var userIDs []primitive.ObjectID
	for cursor.Next(context.Background()) {
		var user models.User
		if err := cursor.Decode(&user); err != nil {
			continue
		}
		userIDs = append(userIDs, user.ID)
	}

	return ns.SendNotificationToUsers(ctx, userIDs, title, body, NotificationTypeSystem, data, nil)
}

// internal/handlers/notification.go
package handlers

import (
"context"
"net/http"
"strconv"
"time"

"nova-kakhovka-ecity/internal/services"

"github.com/gin-gonic/gin"
"go.mongodb.org/mongo-driver/bson"
"go.mongodb.org/mongo-driver/bson/primitive"
"go.mongodb.org/mongo-driver/mongo"
"go.mongodb.org/mongo-driver/mongo/options"
)

type NotificationHandler struct {
	notificationService    *services.NotificationService
	notificationCollection *mongo.Collection
	deviceTokenCollection  *mongo.Collection
}

type RegisterDeviceTokenRequest struct {
	FCMToken string `json:"fcm_token" validate:"required"`
	Platform string `json:"platform" validate:"required,oneof=android ios web"`
}

type SendNotificationRequest struct {
	UserIDs []string               `json:"user_ids" validate:"required"`
	Title   string                 `json:"title" validate:"required,max=100"`
	Body    string                 `json:"body" validate:"required,max=500"`
	Type    string                 `json:"type" validate:"required,oneof=message event announcement system emergency"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

func NewNotificationHandler(notificationService *services.NotificationService, notificationCollection, deviceTokenCollection *mongo.Collection) *NotificationHandler {
	return &NotificationHandler{
		notificationService:    notificationService,
		notificationCollection: notificationCollection,
		deviceTokenCollection:  deviceTokenCollection,
	}
}

func (h *NotificationHandler) RegisterDeviceToken(c *gin.Context) {
	var req RegisterDeviceTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем, существует ли уже этот токен для пользователя
	filter := bson.M{
		"user_id":   userIDObj,
		"fcm_token": req.FCMToken,
	}

	var existingToken services.UserDeviceToken
	err := h.deviceTokenCollection.FindOne(ctx, filter).Decode(&existingToken)

	now := time.Now()

	if err == mongo.ErrNoDocuments {
		// Токен не существует, создаем новый
		deviceToken := services.UserDeviceToken{
			UserID:    userIDObj,
			FCMToken:  req.FCMToken,
			Platform:  req.Platform,
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		}

		_, err := h.deviceTokenCollection.InsertOne(ctx, deviceToken)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Error saving device token",
			})
			return
		}
	} else if err == nil {
		// Токен существует, обновляем его
		_, err := h.deviceTokenCollection.UpdateOne(ctx, filter, bson.M{
			"$set": bson.M{
				"is_active":  true,
				"platform":   req.Platform,
				"updated_at": now,
			},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Error updating device token",
			})
			return
		}
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	// Деактивируем старые токены того же пользователя и платформы
	h.deviceTokenCollection.UpdateMany(ctx, bson.M{
		"user_id":   userIDObj,
		"platform":  req.Platform,
		"fcm_token": bson.M{"$ne": req.FCMToken},
	}, bson.M{
		"$set": bson.M{
			"is_active":  false,
			"updated_at": now,
		},
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "Device token registered successfully",
	})
}

func (h *NotificationHandler) UnregisterDeviceToken(c *gin.Context) {
	fcmToken := c.Query("fcm_token")
	if fcmToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "FCM token is required",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.deviceTokenCollection.UpdateOne(ctx, bson.M{
		"user_id":   userIDObj,
		"fcm_token": fcmToken,
	}, bson.M{
		"$set": bson.M{
			"is_active":  false,
			"updated_at": time.Now(),
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error unregistering device token",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Device token not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Device token unregistered successfully",
	})
}

func (h *NotificationHandler) GetUserNotifications(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	unreadOnly := c.DefaultQuery("unread_only", "false") == "true"

	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	filter := bson.M{"user_id": userIDObj}
	if unreadOnly {
		filter["is_read"] = false
	}

	skip := (page - 1) * limit
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{"created_at", -1}})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.notificationCollection.Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching notifications",
		})
		return
	}
	defer cursor.Close(ctx)

	var notifications []services.StoredNotification
	if err := cursor.All(ctx, &notifications); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding notifications",
		})
		return
	}

	// Получаем количество непрочитанных уведомлений
	unreadCount, err := h.notificationCollection.CountDocuments(ctx, bson.M{
		"user_id": userIDObj,
		"is_read": false,
	})
	if err != nil {
		unreadCount = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"notifications": notifications,
		"unread_count":  unreadCount,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
		},
	})
}

func (h *NotificationHandler) MarkNotificationAsRead(c *gin.Context) {
	notificationID := c.Param("id")
	notificationIDObj, err := primitive.ObjectIDFromHex(notificationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid notification ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	result, err := h.notificationCollection.UpdateOne(ctx, bson.M{
		"_id":     notificationIDObj,
		"user_id": userIDObj,
	}, bson.M{
		"$set": bson.M{
			"is_read": true,
			"read_at": now,
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error marking notification as read",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Notification not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Notification marked as read",
	})
}

func (h *NotificationHandler) MarkAllNotificationsAsRead(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	result, err := h.notificationCollection.UpdateMany(ctx, bson.M{
		"user_id": userIDObj,
		"is_read": false,
	}, bson.M{
		"$set": bson.M{
			"is_read": true,
			"read_at": now,
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error marking notifications as read",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "All notifications marked as read",
		"updated_count": result.ModifiedCount,
	})
}

// Админские функции для отправки уведомлений
func (h *NotificationHandler) SendNotification(c *gin.Context) {
	var req SendNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Проверяем права модератора
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	// Преобразуем строки в ObjectID
	var userIDs []primitive.ObjectID
	for _, userIDStr := range req.UserIDs {
		userID, err := primitive.ObjectIDFromHex(userIDStr)
		if err != nil {
			continue
		}
		userIDs = append(userIDs, userID)
	}

	if len(userIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No valid user IDs provided",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := h.notificationService.SendNotificationToUsers(ctx, userIDs, req.Title, req.Body, req.Type, req.Data, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error sending notification",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Notification sent successfully",
		"user_count": len(userIDs),
	})
}

func (h *NotificationHandler) SendEmergencyNotification(c *gin.Context) {
	var req struct {
		Title string                 `json:"title" validate:"required,max=100"`
		Body  string                 `json:"body" validate:"required,max=500"`
		Data  map[string]interface{} `json:"data,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Проверяем права модератора
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := h.notificationService.SendEmergencyNotification(ctx, req.Title, req.Body, req.Data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error sending emergency notification",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Emergency notification sent to all users",
	})
}IDs, title, body, NotificationTypeEmergency, data, nil)
}

func (ns *NotificationService) getUserFCMTokens(ctx context.Context, userID primitive.ObjectID) ([]string, error) {
	// Здесь должна быть коллекция device_tokens
	deviceTokenCollection := ns.userCollection.Database().Collection("device_tokens")

	cursor, err := deviceTokenCollection.Find(ctx, bson.M{
		"user_id":   userID,
		"is_active": true,
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var tokens []string
	for cursor.Next(ctx) {
		var deviceToken UserDeviceToken
		if err := cursor.Decode(&deviceToken); err != nil {
			continue
		}
		tokens = append(tokens, deviceToken.FCMToken)
	}

	return tokens, nil
}

func (ns *NotificationService) sendFCMNotification(tokens []string, title, body string, data map[string]interface{}) error {
	if ns.config.FirebaseKey == "" {
		return fmt.Errorf("Firebase key is not configured")
	}

	// Разбиваем на батчи по 1000 токенов (лимит FCM)
	batchSize := 1000
	for i := 0; i < len(tokens); i += batchSize {
		end := i + batchSize
		if end > len(tokens) {
			end = len(tokens)
		}

		batch := tokens[i:end]
		err := ns.sendFCMBatch(batch, title, body, data)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ns *NotificationService) sendFCMBatch(tokens []string, title, body string, data map[string]interface{}) error {
	message := FCMMessage{
		RegistrationIDs: tokens,
		Notification: FCMNotification{
			Title: title,
			Body:  body,
			Icon:  "ic_notification",
			Sound: "default",
			Color: "#2196F3",
		},
		Data:       data,
		Priority:   "high",
		TimeToLive: 3600, // 1 час
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal FCM message: %w", err)
	}

	req, err := http.NewRequest("POST", FCMEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create FCM request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+ns.config.FirebaseKey)

	resp, err := ns.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send FCM request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("FCM request failed with status: %d", resp.StatusCode)
	}

	var fcmResp FCMResponse
	if err := json.NewDecoder(resp.Body).Decode(&fcmResp); err != nil {
		return fmt.Errorf("failed to decode FCM response: %w", err)
	}

	// Обрабатываем результат и удаляем неактивные токены
	ns.handleFCMResponse(fcmResp, tokens)

	return nil
}

func (ns *NotificationService) handleFCMResponse(response FCMResponse, tokens []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deviceTokenCollection := ns.userCollection.Database().Collection("device_tokens")

	for i, result := range response.Results {
		if i >= len(tokens) {
			break
		}

		token := tokens[i]

		// Если токен недействителен, помечаем его как неактивный
		if result.Error == "NotRegistered" || result.Error == "InvalidRegistration" {
			deviceTokenCollection.UpdateOne(ctx, bson.M{
				"fcm_token": token,
			}, bson.M{
				"$set": bson.M{
					"is_active":  false,
					"updated_at": time.Now(),
				},
			})
		}

		// Если есть новый canonical ID, обновляем токен
		if result.RegistrationID != "" {
			deviceTokenCollection.UpdateOne(ctx, bson.M{
				"fcm_token": token,
			}, bson.M{
				"$set": bson.M{
					"fcm_token":  result.RegistrationID,
					"updated_at": time.Now(),
				},
			})
		}
	}
}

func (ns *NotificationService) markNotificationAsSent(ctx context.Context, notificationID primitive.ObjectID) {
	ns.notificationCollection.UpdateOne(ctx, bson.M{"_id": notificationID}, bson.M{
		"$set": bson.M{"is_sent": true},
	})
}

// Специализированные методы для разных типов уведомлений

func (ns *NotificationService) SendNewMessageNotification(ctx context.Context, userIDs []primitive.ObjectID, senderName, groupName, messagePreview string, groupID primitive.ObjectID) error {
	data := map[string]interface{}{
		"type":           NotificationTypeMessage,
		"group_id":       groupID.Hex(),
		"sender_name":    senderName,
		"group_name":     groupName,
		"action":         "open_chat",
	}

	title := fmt.Sprintf("Новое сообщение в %s", groupName)
	body := fmt.Sprintf("%s: %s", senderName, messagePreview)

	return ns.SendNotificationToUsers(ctx, userIDs, title, body, NotificationTypeMessage, data, &groupID)
}

func (ns *NotificationService) SendEventInviteNotification(ctx context.Context, userIDs []primitive.ObjectID, eventTitle, organizerName string, eventID primitive.ObjectID, eventDate time.Time) error {
	data := map[string]interface{}{
		"type":           NotificationTypeEvent,
		"event_id":       eventID.Hex(),
		"organizer_name": organizerName,
		"event_date":     eventDate.Format(time.RFC3339),
		"action":         "open_event",
	}

	title := "Приглашение на событие"
	body := fmt.Sprintf("%s приглашает вас на '%s' %s", organizerName, eventTitle, eventDate.Format("02.01.2006 15:04"))

	return ns.SendNotificationToUsers(ctx, user
