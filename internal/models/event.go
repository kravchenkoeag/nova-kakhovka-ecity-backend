// internal/models/event.go
package models

import (
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
	Location Location `bson:"location" json:"location"`
	Address  string   `bson:"address" json:"address"`
	Venue    string   `bson:"venue" json:"venue"` // Название места проведения
	IsOnline bool     `bson:"is_online" json:"is_online"`
	OnlineURL string  `bson:"online_url,omitempty" json:"online_url,omitempty"`

	// Участники
	Participants    []primitive.ObjectID `bson:"participants" json:"participants"`
	MaxParticipants int                  `bson:"max_participants" json:"max_participants"`
	MinAge          int                  `bson:"min_age,omitempty" json:"min_age,omitempty"`
	MaxAge          int                  `bson:"max_age,omitempty" json:"max_age,omitempty"`

	// Настройки и стоимость
	IsPublic    bool    `bson:"is_public" json:"is_public"`
	IsFree      bool    `bson:"is_free" json:"is_free"`
	Price       float64 `bson:"price,omitempty" json:"price,omitempty"`
	Currency    string  `bson:"currency,omitempty" json:"currency,omitempty"`

	// Требования и ограничения
	Requirements    string   `bson:"requirements,omitempty" json:"requirements,omitempty"`
	ProhibitedItems []string `bson:"prohibited_items,omitempty" json:"prohibited_items,omitempty"`

	// Контактная информация
	ContactInfo []ContactInfo `bson:"contact_info,omitempty" json:"contact_info,omitempty"`

	// Медиа
	Images        []string `bson:"images,omitempty" json:"images,omitempty"`
	CoverImage    string   `bson:"cover_image,omitempty" json:"cover_image,omitempty"`

	// Статус и модерация
	Status      string `bson:"status" json:"status"` // draft, published, cancelled, completed
	IsVerified  bool   `bson:"is_verified" json:"is_verified"`
	IsFeatured  bool   `bson:"is_featured" json:"is_featured"`

	// Статистика
	ViewCount   int `bson:"view_count" json:"view_count"`
	ShareCount  int `bson:"share_count" json:"share_count"`

	// Временные метки
	CreatedAt   time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at" json:"updated_at"`
	PublishedAt *time.Time `bson:"published_at,omitempty" json:"published_at,omitempty"`

	// Теги для поиска
	Tags []string `bson:"tags,omitempty" json:"tags,omitempty"`
}

// Категории событий
const (
	EventCategoryCultural     = "cultural"     // Культурные
	EventCategoryEducational  = "educational"  // Образовательные
	EventCategorySocial       = "social"       // Социальные
	EventCategoryBusiness     = "business"     // Деловые
	EventCategorySports       = "sports"       // Спортивные
	EventCategoryCharity      = "charity"      // Благотворительные
	EventCategoryMeeting      = "meeting"      // Встречи
	EventCategoryWorkshop     = "workshop"     // Мастер-классы
	EventCategoryConference   = "conference"   // Конференции
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