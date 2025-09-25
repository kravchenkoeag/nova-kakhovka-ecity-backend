package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Message struct {
	ID      primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	GroupID primitive.ObjectID `bson:"group_id" json:"group_id" validate:"required"`
	UserID  primitive.ObjectID `bson:"user_id" json:"user_id" validate:"required"`

	Content string `bson:"content" json:"content" validate:"required,max=1000"`
	Type    string `bson:"type" json:"type" validate:"required,oneof=text image video file link"`

	// Медиафайлы
	MediaURL  string `bson:"media_url,omitempty" json:"media_url,omitempty"`
	MediaType string `bson:"media_type,omitempty" json:"media_type,omitempty"`
	MediaSize int64  `bson:"media_size,omitempty" json:"media_size,omitempty"`

	// Ответ на сообщение
	ReplyToID *primitive.ObjectID `bson:"reply_to_id,omitempty" json:"reply_to_id,omitempty"`

	// Метаданные
	IsEdited  bool      `bson:"is_edited" json:"is_edited"`
	IsDeleted bool      `bson:"is_deleted" json:"is_deleted"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`

	// Дополнительные поля для реакций и статистики (опционально)
	Reactions []MessageReaction `bson:"reactions,omitempty" json:"reactions,omitempty"`
	ReadBy    []MessageRead     `bson:"read_by,omitempty" json:"read_by,omitempty"`
}

type MessageReaction struct {
	UserID   primitive.ObjectID `bson:"user_id" json:"user_id"`
	Reaction string             `bson:"reaction" json:"reaction"` // emoji или тип реакции
	AddedAt  time.Time          `bson:"added_at" json:"added_at"`
}

type MessageRead struct {
	UserID primitive.ObjectID `bson:"user_id" json:"user_id"`
	ReadAt time.Time          `bson:"read_at" json:"read_at"`
}

// Типы сообщений
const (
	MessageTypeText  = "text"
	MessageTypeImage = "image"
	MessageTypeVideo = "video"
	MessageTypeFile  = "file"
	MessageTypeLink  = "link"
)

// Типы медиа файлов
const (
	MediaTypeImage = "image"
	MediaTypeVideo = "video"
	MediaTypeAudio = "audio"
	MediaTypeDoc   = "document"
)

// Методы для работы с сообщениями

func (m *Message) IsFromUser(userID primitive.ObjectID) bool {
	return m.UserID == userID
}

func (m *Message) CanBeEditedBy(userID primitive.ObjectID) bool {
	// Пользователь может редактировать свои сообщения в течение 15 минут
	if m.UserID != userID {
		return false
	}

	if m.IsDeleted {
		return false
	}

	timeSinceCreation := time.Since(m.CreatedAt)
	return timeSinceCreation < 15*time.Minute
}

func (m *Message) CanBeDeletedBy(userID primitive.ObjectID, isAdmin bool) bool {
	if m.IsDeleted {
		return false
	}

	// Администраторы могут удалять любые сообщения
	if isAdmin {
		return true
	}

	// Пользователи могут удалять свои сообщения
	return m.UserID == userID
}

func (m *Message) IsReply() bool {
	return m.ReplyToID != nil
}

func (m *Message) HasMedia() bool {
	return m.MediaURL != ""
}

func (m *Message) GetPreview() string {
	switch m.Type {
	case MessageTypeText:
		if len(m.Content) > 50 {
			return m.Content[:47] + "..."
		}
		return m.Content
	case MessageTypeImage:
		return "📷 Изображение"
	case MessageTypeVideo:
		return "🎥 Видео"
	case MessageTypeFile:
		return "📎 Файл"
	case MessageTypeLink:
		return "🔗 Ссылка"
	default:
		return m.Content
	}
}

func (m *Message) MarkAsEdited() {
	m.IsEdited = true
	m.UpdatedAt = time.Now()
}

func (m *Message) MarkAsDeleted() {
	m.IsDeleted = true
	m.Content = ""
	m.MediaURL = ""
	m.UpdatedAt = time.Now()
}

func (m *Message) AddReaction(userID primitive.ObjectID, reaction string) bool {
	// Проверяем, не поставил ли пользователь уже эту реакцию
	for i, r := range m.Reactions {
		if r.UserID == userID {
			if r.Reaction == reaction {
				return false // Уже есть такая реакция
			}
			// Обновляем существующую реакцию
			m.Reactions[i].Reaction = reaction
			m.Reactions[i].AddedAt = time.Now()
			return true
		}
	}

	// Добавляем новую реакцию
	m.Reactions = append(m.Reactions, MessageReaction{
		UserID:   userID,
		Reaction: reaction,
		AddedAt:  time.Now(),
	})
	return true
}

func (m *Message) RemoveReaction(userID primitive.ObjectID) bool {
	for i, r := range m.Reactions {
		if r.UserID == userID {
			m.Reactions = append(m.Reactions[:i], m.Reactions[i+1:]...)
			return true
		}
	}
	return false
}

func (m *Message) GetReactionCounts() map[string]int {
	counts := make(map[string]int)
	for _, r := range m.Reactions {
		counts[r.Reaction]++
	}
	return counts
}

func (m *Message) MarkAsRead(userID primitive.ObjectID) bool {
	// Проверяем, не отмечено ли уже как прочитанное
	for _, read := range m.ReadBy {
		if read.UserID == userID {
			return false // Уже прочитано
		}
	}

	m.ReadBy = append(m.ReadBy, MessageRead{
		UserID: userID,
		ReadAt: time.Now(),
	})
	return true
}

func (m *Message) IsReadBy(userID primitive.ObjectID) bool {
	for _, read := range m.ReadBy {
		if read.UserID == userID {
			return true
		}
	}
	return false
}

func (m *Message) GetReadCount() int {
	return len(m.ReadBy)
}

func (m *Message) IsRecent() bool {
	return time.Since(m.CreatedAt) < 24*time.Hour
}

func (m *Message) GetAge() time.Duration {
	return time.Since(m.CreatedAt)
}

// Получение переводов типов сообщений для UI
func GetMessageTypeTranslation(messageType string) string {
	translations := map[string]string{
		MessageTypeText:  "Текст",
		MessageTypeImage: "Изображение",
		MessageTypeVideo: "Видео",
		MessageTypeFile:  "Файл",
		MessageTypeLink:  "Ссылка",
	}
	if translation, exists := translations[messageType]; exists {
		return translation
	}
	return messageType
}

// Валидация размера медиафайла
func (m *Message) ValidateMediaSize() bool {
	if !m.HasMedia() {
		return true
	}

	maxSizes := map[string]int64{
		MessageTypeImage: 10 * 1024 * 1024,  // 10MB для изображений
		MessageTypeVideo: 100 * 1024 * 1024, // 100MB для видео
		MessageTypeFile:  50 * 1024 * 1024,  // 50MB для файлов
	}

	if maxSize, exists := maxSizes[m.Type]; exists {
		return m.MediaSize <= maxSize
	}

	return true
}
