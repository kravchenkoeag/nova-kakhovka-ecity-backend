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

type SendEmergencyNotificationRequest struct {
	Title string                 `json:"title" validate:"required,max=100"`
	Body  string                 `json:"body" validate:"required,max=500"`
	Data  map[string]interface{} `json:"data,omitempty"`
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
		"message":       "All notifications marked as read",
		"updated_count": result.ModifiedCount,
	})
}

func (h *NotificationHandler) DeleteNotification(c *gin.Context) {
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

	result, err := h.notificationCollection.DeleteOne(ctx, bson.M{
		"_id":     notificationIDObj,
		"user_id": userIDObj,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting notification",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Notification not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Notification deleted successfully",
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
			"error":   "Error sending notification",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Notification sent successfully",
		"user_count": len(userIDs),
	})
}

func (h *NotificationHandler) SendEmergencyNotification(c *gin.Context) {
	var req SendEmergencyNotificationRequest
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
			"error":   "Error sending emergency notification",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Emergency notification sent to all users",
	})
}

func (h *NotificationHandler) GetNotificationTypes(c *gin.Context) {
	types := []string{
		services.NotificationTypeMessage,
		services.NotificationTypeEvent,
		services.NotificationTypeAnnouncement,
		services.NotificationTypeSystem,
		services.NotificationTypeEmergency,
	}

	c.JSON(http.StatusOK, gin.H{
		"notification_types": types,
	})
}

// Дополнительные методы для работы с уведомлениями

func (h *NotificationHandler) GetNotificationStats(c *gin.Context) {
	// Проверяем права модератора
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Статистика по типам уведомлений
	typePipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$type",
				"count": bson.M{"$sum": 1},
				"sent":  bson.M{"$sum": bson.M{"$cond": []interface{}{"$is_sent", 1, 0}}},
				"read":  bson.M{"$sum": bson.M{"$cond": []interface{}{"$is_read", 1, 0}}},
			},
		},
	}

	typeCursor, err := h.notificationCollection.Aggregate(ctx, typePipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error getting notification stats",
		})
		return
	}
	defer typeCursor.Close(ctx)

	typeStats := make(map[string]interface{})
	for typeCursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int    `bson:"count"`
			Sent  int    `bson:"sent"`
			Read  int    `bson:"read"`
		}
		if err := typeCursor.Decode(&result); err != nil {
			continue
		}
		typeStats[result.ID] = gin.H{
			"total": result.Count,
			"sent":  result.Sent,
			"read":  result.Read,
		}
	}

	// Общая статистика
	totalCount, _ := h.notificationCollection.CountDocuments(ctx, bson.M{})
	sentCount, _ := h.notificationCollection.CountDocuments(ctx, bson.M{"is_sent": true})
	readCount, _ := h.notificationCollection.CountDocuments(ctx, bson.M{"is_read": true})

	c.JSON(http.StatusOK, gin.H{
		"total_notifications": totalCount,
		"sent_notifications":  sentCount,
		"read_notifications":  readCount,
		"type_stats":          typeStats,
		"updated_at":          time.Now(),
	})
}

func (h *NotificationHandler) CleanupOldNotifications(c *gin.Context) {
	// Проверяем права модератора
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Удаляем уведомления старше 90 дней
	cutoffDate := time.Now().AddDate(0, 0, -90)

	result, err := h.notificationCollection.DeleteMany(ctx, bson.M{
		"created_at": bson.M{"$lt": cutoffDate},
		"is_read":    true,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error cleaning up notifications",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Old notifications cleaned up successfully",
		"deleted_count": result.DeletedCount,
		"cutoff_date":   cutoffDate.Format("2006-01-02"),
	})
}

func (h *NotificationHandler) SendTestNotification(c *gin.Context) {
	// Проверяем права модератора
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data := map[string]interface{}{
		"type":   "test",
		"action": "none",
	}

	err := h.notificationService.SendNotificationToUser(
		ctx,
		userIDObj,
		"Тестовое уведомление",
		"Это тестовое уведомление для проверки работы системы",
		services.NotificationTypeSystem,
		data,
		nil,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error sending test notification",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Test notification sent successfully",
	})
}
