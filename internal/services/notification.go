package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	httpClient             *http.Client
}

type FCMMessage struct {
	To              string                 `json:"to,omitempty"`
	RegistrationIDs []string               `json:"registration_ids,omitempty"`
	Notification    FCMNotification        `json:"notification"`
	Data            map[string]interface{} `json:"data,omitempty"`
	Priority        string                 `json:"priority"`
	TimeToLive      int                    `json:"time_to_live,omitempty"`
}

type FCMNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Icon  string `json:"icon,omitempty"`
	Sound string `json:"sound,omitempty"`
	Color string `json:"color,omitempty"`
}

type FCMResponse struct {
	MulticastID  int64       `json:"multicast_id"`
	Success      int         `json:"success"`
	Failure      int         `json:"failure"`
	CanonicalIDs int         `json:"canonical_ids"`
	Results      []FCMResult `json:"results"`
}

type FCMResult struct {
	MessageID      string `json:"message_id,omitempty"`
	RegistrationID string `json:"registration_id,omitempty"`
	Error          string `json:"error,omitempty"`
}

// Модель для токенов устройств
type UserDeviceToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	FCMToken  string             `bson:"fcm_token" json:"fcm_token"`
	Platform  string             `bson:"platform" json:"platform"` // android, ios, web
	IsActive  bool               `bson:"is_active" json:"is_active"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

// Модель для хранения уведомлений в базе
type StoredNotification struct {
	ID        primitive.ObjectID     `bson:"_id,omitempty" json:"id,omitempty"`
	UserID    primitive.ObjectID     `bson:"user_id" json:"user_id"`
	Title     string                 `bson:"title" json:"title"`
	Body      string                 `bson:"body" json:"body"`
	Type      string                 `bson:"type" json:"type"` // message, event, announcement, system
	RelatedID *primitive.ObjectID    `bson:"related_id,omitempty" json:"related_id,omitempty"`
	Data      map[string]interface{} `bson:"data,omitempty" json:"data,omitempty"`
	IsRead    bool                   `bson:"is_read" json:"is_read"`
	IsSent    bool                   `bson:"is_sent" json:"is_sent"`
	CreatedAt time.Time              `bson:"created_at" json:"created_at"`
	ReadAt    *time.Time             `bson:"read_at,omitempty" json:"read_at,omitempty"`
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
	var notificationIDs []primitive.ObjectID

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

		result, err := ns.notificationCollection.InsertOne(ctx, notification)
		if err != nil {
			continue // Продолжаем даже если не удалось сохранить одно уведомление
		}

		notificationIDs = append(notificationIDs, result.InsertedID.(primitive.ObjectID))

		// Получаем токены для каждого пользователя
		tokens, err := ns.getUserFCMTokens(ctx, userID)
		if err != nil {
			continue
		}
		allTokens = append(allTokens, tokens...)
	}

	if len(allTokens) == 0 {
		// Помечаем все уведомления как отправленные
		for _, notificationID := range notificationIDs {
			ns.markNotificationAsSent(ctx, notificationID)
		}
		return nil
	}

	// Отправляем FCM уведомление всем токенам
	err := ns.sendFCMNotification(allTokens, title, body, data)
	if err != nil {
		return fmt.Errorf("failed to send batch FCM notification: %w", err)
	}

	// Помечаем все уведомления как отправленные
	for _, notificationID := range notificationIDs {
		ns.markNotificationAsSent(ctx, notificationID)
	}

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

	return ns.SendNotificationToUsers(ctx, userIDs, title, body, NotificationTypeEmergency, data, nil)
}

// Специализированные методы для разных типов уведомлений

func (ns *NotificationService) SendNewMessageNotification(ctx context.Context, userIDs []primitive.ObjectID, senderName, groupName, messagePreview string, groupID primitive.ObjectID) error {
	data := map[string]interface{}{
		"type":        NotificationTypeMessage,
		"group_id":    groupID.Hex(),
		"sender_name": senderName,
		"group_name":  groupName,
		"action":      "open_chat",
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
	cursor, err := ns.userCollection.Find(ctx, bson.M{
		"is_blocked": false,
	})
	if err != nil {
		return err
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

	return ns.SendNotificationToUsers(ctx, userIDs, title, body, NotificationTypeSystem, data, nil)
}

// Вспомогательные функции

func (ns *NotificationService) getUserFCMTokens(ctx context.Context, userID primitive.ObjectID) ([]string, error) {
	// Коллекция device_tokens
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
