// internal/models/city_issue.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type CityIssue struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	ReporterID primitive.ObjectID `bson:"reporter_id" json:"reporter_id" validate:"required"`

	// Основная информация
	Title       string `bson:"title" json:"title" validate:"required,min=5,max=200"`
	Description string `bson:"description" json:"description" validate:"required,min=10,max=1000"`
	Category    string `bson:"category" json:"category" validate:"required,oneof=road lighting water electricity waste transport building safety other"`
	Priority    string `bson:"priority" json:"priority" validate:"oneof=low medium high critical"`

	// Местоположение
	Location Location `bson:"location" json:"location" validate:"required"`
	Address  string   `bson:"address" json:"address" validate:"required"`

	// Медиафайлы
	Photos []string `bson:"photos" json:"photos"`
	Videos []string `bson:"videos" json:"videos"`

	// Статус и обработка
	Status         string              `bson:"status" json:"status"` // reported, in_progress, resolved, rejected, duplicate
	AssignedTo     *primitive.ObjectID `bson:"assigned_to,omitempty" json:"assigned_to,omitempty"`
	AssignedDept   string              `bson:"assigned_dept" json:"assigned_dept"` // department name
	Resolution     string              `bson:"resolution" json:"resolution"`
	ResolutionNote string              `bson:"resolution_note" json:"resolution_note"`

	// Взаимодействие с пользователями
	Upvotes     []primitive.ObjectID `bson:"upvotes" json:"upvotes"`
	Comments    []IssueComment       `bson:"comments" json:"comments"`
	Subscribers []primitive.ObjectID `bson:"subscribers" json:"subscribers"` // Пользователи, следящие за проблемой

	// Метаданные
	IsVerified  bool                `bson:"is_verified" json:"is_verified"`
	IsPublic    bool                `bson:"is_public" json:"is_public"`
	ViewCount   int                 `bson:"view_count" json:"view_count"`
	CreatedAt   time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time           `bson:"updated_at" json:"updated_at"`
	ResolvedAt  *time.Time          `bson:"resolved_at,omitempty" json:"resolved_at,omitempty"`
	DuplicateOf *primitive.ObjectID `bson:"duplicate_of,omitempty" json:"duplicate_of,omitempty"`
}

type IssueComment struct {
	ID         primitive.ObjectID `bson:"id" json:"id"`
	AuthorID   primitive.ObjectID `bson:"author_id" json:"author_id"`
	Content    string             `bson:"content" json:"content"`
	IsOfficial bool               `bson:"is_official" json:"is_official"` // Комментарий от городских служб
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at" json:"updated_at"`
}

// Категории проблем
const (
	IssueCategoryRoad        = "road"        // Дороги
	IssueCategoryLighting    = "lighting"    // Освещение
	IssueCategoryWater       = "water"       // Водоснабжение
	IssueCategoryElectricity = "electricity" // Электроснабжение
	IssueCategoryWaste       = "waste"       // Отходы
	IssueCategoryTransport   = "transport"   // Транспорт
	IssueCategoryBuilding    = "building"    // Здания
	IssueCategorySafety      = "safety"      // Безопасность
	IssueCategoryOther       = "other"       // Прочее
)

// Статусы проблем
const (
	IssueStatusReported   = "reported"    // Сообщено
	IssueStatusInProgress = "in_progress" // В работе
	IssueStatusResolved   = "resolved"    // Решено
	IssueStatusRejected   = "rejected"    // Отклонено
	IssueStatusDuplicate  = "duplicate"   // Дубликат
)

// Приоритеты
const (
	IssuePriorityLow      = "low"
	IssuePriorityMedium   = "medium"
	IssuePriorityHigh     = "high"
	IssuePriorityCritical = "critical"
)

// Методы для работы с проблемами

func (i *CityIssue) IsResolved() bool {
	return i.Status == IssueStatusResolved
}

func (i *CityIssue) IsInProgress() bool {
	return i.Status == IssueStatusInProgress
}

func (i *CityIssue) HasUserUpvoted(userID primitive.ObjectID) bool {
	for _, upvoterID := range i.Upvotes {
		if upvoterID == userID {
			return true
		}
	}
	return false
}

func (i *CityIssue) HasUserSubscribed(userID primitive.ObjectID) bool {
	for _, subscriberID := range i.Subscribers {
		if subscriberID == userID {
			return true
		}
	}
	return false
}

func (i *CityIssue) GetUpvoteCount() int {
	return len(i.Upvotes)
}

func (i *CityIssue) GetCommentCount() int {
	return len(i.Comments)
}

func (i *CityIssue) GetSubscriberCount() int {
	return len(i.Subscribers)
}

func (i *CityIssue) GetOfficialComments() []IssueComment {
	var officialComments []IssueComment
	for _, comment := range i.Comments {
		if comment.IsOfficial {
			officialComments = append(officialComments, comment)
		}
	}
	return officialComments
}

func (i *CityIssue) GetLatestComment() *IssueComment {
	if len(i.Comments) == 0 {
		return nil
	}
	return &i.Comments[len(i.Comments)-1]
}

func (i *CityIssue) GetPriorityScore() int {
	switch i.Priority {
	case IssuePriorityLow:
		return 1
	case IssuePriorityMedium:
		return 2
	case IssuePriorityHigh:
		return 3
	case IssuePriorityCritical:
		return 4
	default:
		return 1
	}
}

func (i *CityIssue) GetDaysOpen() int {
	if i.IsResolved() && i.ResolvedAt != nil {
		duration := i.ResolvedAt.Sub(i.CreatedAt)
		return int(duration.Hours() / 24)
	}
	duration := time.Since(i.CreatedAt)
	return int(duration.Hours() / 24)
}

func (i *CityIssue) CanBeEditedBy(userID primitive.ObjectID, isModerator bool) bool {
	// Модераторы могут редактировать любые проблемы
	if isModerator {
		return true
	}
	// Обычные пользователи могут редактировать только свои нерешенные проблемы
	return i.ReporterID == userID && !i.IsResolved()
}

func (i *CityIssue) AddUpvote(userID primitive.ObjectID) bool {
	if i.HasUserUpvoted(userID) {
		return false // Уже проголосовал
	}
	i.Upvotes = append(i.Upvotes, userID)
	i.UpdatedAt = time.Now()
	return true
}

func (i *CityIssue) RemoveUpvote(userID primitive.ObjectID) bool {
	for j, upvoterID := range i.Upvotes {
		if upvoterID == userID {
			i.Upvotes = append(i.Upvotes[:j], i.Upvotes[j+1:]...)
			i.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

func (i *CityIssue) AddSubscriber(userID primitive.ObjectID) bool {
	if i.HasUserSubscribed(userID) {
		return false // Уже подписан
	}
	i.Subscribers = append(i.Subscribers, userID)
	i.UpdatedAt = time.Now()
	return true
}

func (i *CityIssue) RemoveSubscriber(userID primitive.ObjectID) bool {
	for j, subscriberID := range i.Subscribers {
		if subscriberID == userID {
			i.Subscribers = append(i.Subscribers[:j], i.Subscribers[j+1:]...)
			i.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

func (c *IssueComment) CanBeEditedBy(userID primitive.ObjectID, isModerator bool) bool {
	// Модераторы могут редактировать любые комментарии
	if isModerator {
		return true
	}
	// Обычные пользователи могут редактировать свои комментарии в течение 15 минут
	if c.AuthorID == userID {
		timeSinceCreation := time.Since(c.CreatedAt)
		return timeSinceCreation < 15*time.Minute
	}
	return false
}

func (c *IssueComment) IsRecent() bool {
	return time.Since(c.CreatedAt) < 24*time.Hour
}

// Получение переводов категорий для UI
func GetCategoryTranslation(category string) string {
	translations := map[string]string{
		IssueCategoryRoad:        "Дороги",
		IssueCategoryLighting:    "Освещение",
		IssueCategoryWater:       "Водоснабжение",
		IssueCategoryElectricity: "Электроснабжение",
		IssueCategoryWaste:       "Отходы",
		IssueCategoryTransport:   "Транспорт",
		IssueCategoryBuilding:    "Здания",
		IssueCategorySafety:      "Безопасность",
		IssueCategoryOther:       "Прочее",
	}
	if translation, exists := translations[category]; exists {
		return translation
	}
	return category
}

// Получение переводов статусов для UI
func GetStatusTranslation(status string) string {
	translations := map[string]string{
		IssueStatusReported:   "Сообщено",
		IssueStatusInProgress: "В работе",
		IssueStatusResolved:   "Решено",
		IssueStatusRejected:   "Отклонено",
		IssueStatusDuplicate:  "Дубликат",
	}
	if translation, exists := translations[status]; exists {
		return translation
	}
	return status
}

// Получение переводов приоритетов для UI
func GetPriorityTranslation(priority string) string {
	translations := map[string]string{
		IssuePriorityLow:      "Низкий",
		IssuePriorityMedium:   "Средний",
		IssuePriorityHigh:     "Высокий",
		IssuePriorityCritical: "Критический",
	}
	if translation, exists := translations[priority]; exists {
		return translation
	}
	return priority
}
