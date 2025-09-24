// internal/models/event.go
package models

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Event struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	OrganizerID primitive.ObjectID `bson:"organizer_id" json:"organizer_id" validate:"required"`

	Title       string `bson:"title" json:"title" validate:"required,min=5,max=200"`
	Description string `bson:"description" json:"description" validate:"required,min=10,max=2000"`
	Category    string `bson:"category" json:"category" validate:"oneof=cultural educational social business sports charity meeting workshop conference"`

	// Дата и время
	StartDate time.Time  `bson:"start_date" json:"start_date" validate:"required"`
	EndDate   *time.Time `bson:"end_date,omitempty" json:"end_date,omitempty"`

	// Местоположение
	Location  Location `bson:"location" json:"location"`
	Address   string   `bson:"address" json:"address"`
	Venue     string   `bson:"venue" json:"venue"` // Название места проведения
	IsOnline  bool     `bson:"is_online" json:"is_online"`
	OnlineURL string   `bson:"online_url,omitempty" json:"online_url,omitempty"`

	// Участники
	Participants    []primitive.ObjectID `bson:"participants" json:"participants"`
	MaxParticipants int                  `bson:"max_participants" json:"max_participants"`
	MinAge          int                  `bson:"min_age,omitempty" json:"min_age,omitempty"`
	MaxAge          int                  `bson:"max_age,omitempty" json:"max_age,omitempty"`

	// Настройки и стоимость
	IsPublic bool    `bson:"is_public" json:"is_public"`
	IsFree   bool    `bson:"is_free" json:"is_free"`
	Price    float64 `bson:"price,omitempty" json:"price,omitempty"`
	Currency string  `bson:"currency,omitempty" json:"currency,omitempty"`

	// Требования и ограничения
	Requirements    string   `bson:"requirements,omitempty" json:"requirements,omitempty"`
	ProhibitedItems []string `bson:"prohibited_items,omitempty" json:"prohibited_items,omitempty"`

	// Контактная информация
	ContactInfo []ContactInfo `bson:"contact_info,omitempty" json:"contact_info,omitempty"`

	// Медиа
	Images     []string `bson:"images,omitempty" json:"images,omitempty"`
	CoverImage string   `bson:"cover_image,omitempty" json:"cover_image,omitempty"`

	// Статус и модерация
	Status     string `bson:"status" json:"status"` // draft, published, cancelled, completed
	IsVerified bool   `bson:"is_verified" json:"is_verified"`
	IsFeatured bool   `bson:"is_featured" json:"is_featured"`

	// Статистика
	ViewCount  int `bson:"view_count" json:"view_count"`
	ShareCount int `bson:"share_count" json:"share_count"`

	// Временные метки
	CreatedAt   time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at" json:"updated_at"`
	PublishedAt *time.Time `bson:"published_at,omitempty" json:"published_at,omitempty"`

	// Теги для поиска
	Tags []string `bson:"tags,omitempty" json:"tags,omitempty"`
}

// Категории событий
const (
	EventCategoryCultural    = "cultural"    // Культурные
	EventCategoryEducational = "educational" // Образовательные
	EventCategorySocial      = "social"      // Социальные
	EventCategoryBusiness    = "business"    // Деловые
	EventCategorySports      = "sports"      // Спортивные
	EventCategoryCharity     = "charity"     // Благотворительные
	EventCategoryMeeting     = "meeting"     // Встречи
	EventCategoryWorkshop    = "workshop"    // Мастер-классы
	EventCategoryConference  = "conference"  // Конференции
)

// Статусы событий
const (
	EventStatusDraft     = "draft"
	EventStatusPublished = "published"
	EventStatusCancelled = "cancelled"
	EventStatusCompleted = "completed"
)

// Методы для работы с событиями

func (e *Event) IsUpcoming() bool {
	return time.Now().Before(e.StartDate)
}

func (e *Event) IsOngoing() bool {
	now := time.Now()
	if e.EndDate != nil {
		return now.After(e.StartDate) && now.Before(*e.EndDate)
	}
	// Если нет конечной даты, считаем что событие длится весь день
	return now.After(e.StartDate) && now.Before(e.StartDate.Add(24*time.Hour))
}

func (e *Event) IsPast() bool {
	now := time.Now()
	if e.EndDate != nil {
		return now.After(*e.EndDate)
	}
	return now.After(e.StartDate.Add(24 * time.Hour))
}

func (e *Event) GetTimeUntilStart() time.Duration {
	if e.IsPast() || e.IsOngoing() {
		return 0
	}
	return e.StartDate.Sub(time.Now())
}

func (e *Event) GetDuration() time.Duration {
	if e.EndDate != nil {
		return e.EndDate.Sub(e.StartDate)
	}
	return 0
}

func (e *Event) IsVisible() bool {
	return e.Status == EventStatusPublished && e.IsPublic
}

func (e *Event) CanBeEditedBy(userID primitive.ObjectID, isModerator bool) bool {
	// Модераторы могут редактировать любые события
	if isModerator {
		return true
	}
	// Организаторы могут редактировать свои события до начала
	return e.OrganizerID == userID && e.IsUpcoming()
}

func (e *Event) CanBeDeletedBy(userID primitive.ObjectID, isModerator bool) bool {
	// Модераторы могут удалять любые события
	if isModerator {
		return true
	}
	// Организаторы могут удалять свои события
	return e.OrganizerID == userID
}

func (e *Event) CanUserJoin(userID primitive.ObjectID) bool {
	if e.Status != EventStatusPublished {
		return false
	}

	if e.IsPast() {
		return false
	}

	// Проверяем, не является ли пользователь уже участником
	if e.IsParticipant(userID) {
		return false
	}

	// Проверяем лимит участников
	if e.MaxParticipants > 0 && len(e.Participants) >= e.MaxParticipants {
		return false
	}

	return true
}

func (e *Event) IsParticipant(userID primitive.ObjectID) bool {
	for _, participantID := range e.Participants {
		if participantID == userID {
			return true
		}
	}
	return false
}

func (e *Event) IsOrganizer(userID primitive.ObjectID) bool {
	return e.OrganizerID == userID
}

func (e *Event) GetParticipantCount() int {
	return len(e.Participants)
}

func (e *Event) GetAvailableSpots() int {
	if e.MaxParticipants <= 0 {
		return -1 // Безлимит
	}
	available := e.MaxParticipants - len(e.Participants)
	if available < 0 {
		return 0
	}
	return available
}

func (e *Event) IsFull() bool {
	return e.MaxParticipants > 0 && len(e.Participants) >= e.MaxParticipants
}

func (e *Event) AddParticipant(userID primitive.ObjectID) bool {
	if e.IsParticipant(userID) || !e.CanUserJoin(userID) {
		return false
	}

	e.Participants = append(e.Participants, userID)
	e.UpdatedAt = time.Now()
	return true
}

func (e *Event) RemoveParticipant(userID primitive.ObjectID) bool {
	for i, participantID := range e.Participants {
		if participantID == userID {
			e.Participants = append(e.Participants[:i], e.Participants[i+1:]...)
			e.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

func (e *Event) GetPrimaryContact() *ContactInfo {
	if len(e.ContactInfo) == 0 {
		return nil
	}
	return &e.ContactInfo[0]
}

func (e *Event) HasRequirements() bool {
	return e.Requirements != ""
}

func (e *Event) IsRecent() bool {
	return time.Since(e.CreatedAt) < 7*24*time.Hour
}

func (e *Event) IsPopular() bool {
	// Считаем популярным если много участников относительно времени существования
	if e.MaxParticipants <= 0 {
		return len(e.Participants) > 50 // Абсолютное значение для безлимитных событий
	}

	occupancyRate := float64(len(e.Participants)) / float64(e.MaxParticipants)
	return occupancyRate > 0.7 // Более 70% заполненности
}

func (e *Event) GetFormattedDateTime() string {
	if e.EndDate != nil {
		return e.StartDate.Format("02.01.2006 15:04") + " - " + e.EndDate.Format("02.01.2006 15:04")
	}
	return e.StartDate.Format("02.01.2006 15:04")
}

func (e *Event) GetFormattedPrice() string {
	if e.IsFree {
		return "Бесплатно"
	}

	currency := e.Currency
	if currency == "" {
		currency = "грн"
	}

	return fmt.Sprintf("%.2f %s", e.Price, currency)
}

// Получение переводов категорий для UI
func GetEventCategoryTranslation(category string) string {
	translations := map[string]string{
		EventCategoryCultural:    "Культурные",
		EventCategoryEducational: "Образовательные",
		EventCategorySocial:      "Социальные",
		EventCategoryBusiness:    "Деловые",
		EventCategorySports:      "Спортивные",
		EventCategoryCharity:     "Благотворительные",
		EventCategoryMeeting:     "Встречи",
		EventCategoryWorkshop:    "Мастер-классы",
		EventCategoryConference:  "Конференции",
	}
	if translation, exists := translations[category]; exists {
		return translation
	}
	return category
}

// Получение переводов статусов для UI
func GetEventStatusTranslation(status string) string {
	translations := map[string]string{
		EventStatusDraft:     "Черновик",
		EventStatusPublished: "Опубликовано",
		EventStatusCancelled: "Отменено",
		EventStatusCompleted: "Завершено",
	}
	if translation, exists := translations[status]; exists {
		return translation
	}
	return status
}
