package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type Notification struct {
	ID        primitive.ObjectID     `bson:"_id,omitempty" json:"id,omitempty"`
	UserID    primitive.ObjectID     `bson:"user_id" json:"user_id"`
	Type      string                 `bson:"type" json:"type"` // announcement, event, poll, etc
	Title     string                 `bson:"title" json:"title"`
	Message   string                 `bson:"message" json:"message"`
	Data      map[string]interface{} `bson:"data,omitempty" json:"data,omitempty"` // Додаткові дані
	IsRead    bool                   `bson:"is_read" json:"is_read"`
	ReadAt    *time.Time             `bson:"read_at,omitempty" json:"read_at,omitempty"`
	CreatedAt time.Time              `bson:"created_at" json:"created_at"`
}

// Типи сповіщень
const (
	NotificationTypeAnnouncement = "announcement"
	NotificationTypeEvent        = "event"
	NotificationTypePoll         = "poll"
	NotificationTypePetition     = "petition"
	NotificationTypeCityIssue    = "city_issue"
	NotificationTypeMessage      = "message"
	NotificationTypeSystem       = "system"
)
