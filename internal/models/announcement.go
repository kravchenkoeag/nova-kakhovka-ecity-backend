// internal/models/announcement.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Announcement struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	AuthorID primitive.ObjectID `bson:"author_id" json:"author_id" validate:"required"`

	Title       string `bson:"title" json:"title" validate:"required,min=5,max=200"`
	Description string `bson:"description" json:"description" validate:"required,min=10,max=2000"`
	Category    string `bson:"category" json:"category" validate:"required,oneof=work help services housing transport"`

	// Местоположение и тип работы
	Location   Location `bson:"location" json:"location"`
	Address    string   `bson:"address" json:"address"`
	Employment string   `bson:"employment" json:"employment" validate:"oneof=once permanent partial"`

	// Контакты и медиа
	ContactInfo []ContactInfo `bson:"contact_info" json:"contact_info"`
	MediaFiles  []string      `bson:"media_files" json:"media_files"`

	// Статус и модерация
	IsActive    bool `bson:"is_active" json:"is_active"`
	IsModerated bool `bson:"is_moderated" json:"is_moderated"`
	IsBlocked   bool `bson:"is_blocked" json:"is_blocked"`

	// Статистика
	Views    int `bson:"views" json:"views"`
	Contacts int `bson:"contacts" json:"contacts"`

	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
	ExpiresAt time.Time `bson:"expires_at" json:"expires_at"`
}

type ContactInfo struct {
	Type  string `bson:"type" json:"type" validate:"required,oneof=phone email telegram viber whatsapp"`
	Value string `bson:"value" json:"value" validate:"required"`
	Label string `bson:"label,omitempty" json:"label,omitempty"` // Дополнительная подпись
}

// Категории объявлений
const (
	AnnouncementCategoryWork      = "work"      // Работа
	AnnouncementCategoryHelp      = "help"      // Помощь
	AnnouncementCategoryServices  = "services"  // Услуги
	AnnouncementCategoryHousing   = "housing"   // Жилье
	AnnouncementCategoryTransport = "transport" // Транспорт
)

// Типы занятости
const (
	EmploymentOnce      = "once"      // Разовая работа
	EmploymentPermanent = "permanent" // Постоянная работа
	EmploymentPartial   = "partial"   // Частичная занятость
)

// Типы контактов
const (
	ContactTypePhone    = "phone"
	ContactTypeEmail    = "email"
	ContactTypeTelegram = "telegram"
	ContactTypeViber    = "viber"
	ContactTypeWhatsApp = "whatsapp"
)

// Методы для работы с объявлениями

func (a *Announcement) IsExpired() bool {
	return time.Now().After(a.ExpiresAt)
}

func (a *Announcement) IsVisible() bool {
	return a.IsActive && a.IsModerated && !a.IsBlocked && !a.IsExpired()
}

func (a *Announcement) CanBeEditedBy(userID primitive.ObjectID, isModerator bool) bool {
	// Модераторы могут редактировать любые объявления
	if isModerator {
		return true
	}
	// Обычные пользователи могут редактировать свои активные объявления
	return a.AuthorID == userID && a.IsActive
}

func (a *Announcement) CanBeDeletedBy(userID primitive.ObjectID, isModerator bool) bool {
	// Модераторы могут удалять любые объявления
	if isModerator {
		return true
	}
	// Обычные пользователи могут удалять свои объявления
	return a.AuthorID == userID
}

func (a *Announcement) GetDaysUntilExpiry() int {
	if a.IsExpired() {
		return 0
	}
	duration := a.ExpiresAt.Sub(time.Now())
	return int(duration.Hours() / 24)
}

func (a *Announcement) GetTimeUntilExpiry() time.Duration {
	if a.IsExpired() {
		return 0
	}
	return a.ExpiresAt.Sub(time.Now())
}

func (a *Announcement) IncrementViews() {
	a.Views++
	a.UpdatedAt = time.Now()
}

func (a *Announcement) IncrementContacts() {
	a.Contacts++
	a.UpdatedAt = time.Now()
}

func (a *Announcement) GetPrimaryContact() *ContactInfo {
	if len(a.ContactInfo) == 0 {
		return nil
	}

	// Приоритет: phone > telegram > email > остальные
	priorities := map[string]int{
		ContactTypePhone:    1,
		ContactTypeTelegram: 2,
		ContactTypeEmail:    3,
		ContactTypeViber:    4,
		ContactTypeWhatsApp: 5,
	}

	var primary *ContactInfo
	minPriority := 999

	for i, contact := range a.ContactInfo {
		if priority, exists := priorities[contact.Type]; exists {
			if priority < minPriority {
				minPriority = priority
				primary = &a.ContactInfo[i]
			}
		}
	}

	if primary == nil && len(a.ContactInfo) > 0 {
		primary = &a.ContactInfo[0]
	}

	return primary
}

func (a *Announcement) GetContactsByType(contactType string) []ContactInfo {
	var contacts []ContactInfo
	for _, contact := range a.ContactInfo {
		if contact.Type == contactType {
			contacts = append(contacts, contact)
		}
	}
	return contacts
}

func (a *Announcement) HasContactType(contactType string) bool {
	for _, contact := range a.ContactInfo {
		if contact.Type == contactType {
			return true
		}
	}
	return false
}

func (a *Announcement) GetMediaCount() int {
	return len(a.MediaFiles)
}

func (a *Announcement) HasMedia() bool {
	return len(a.MediaFiles) > 0
}

func (a *Announcement) IsRecent() bool {
	return time.Since(a.CreatedAt) < 7*24*time.Hour // Считается новым в течение недели
}

func (a *Announcement) IsPopular() bool {
	// Считаем популярным если много просмотров относительно времени существования
	daysOld := int(time.Since(a.CreatedAt).Hours() / 24)
	if daysOld == 0 {
		daysOld = 1
	}
	avgViewsPerDay := a.Views / daysOld
	return avgViewsPerDay > 10 // Более 10 просмотров в день
}

func (a *Announcement) GetEngagementRate() float64 {
	if a.Views == 0 {
		return 0
	}
	return float64(a.Contacts) / float64(a.Views) * 100
}

// Валидация контактной информации
func (c *ContactInfo) IsValid() bool {
	if c.Value == "" {
		return false
	}

	switch c.Type {
	case ContactTypePhone:
		// Простая проверка номера телефона (минимум 10 цифр)
		digitCount := 0
		for _, char := range c.Value {
			if char >= '0' && char <= '9' {
				digitCount++
			}
		}
		return digitCount >= 10

	case ContactTypeEmail:
		// Простая проверка email (содержит @ и точку)
		hasAt := false
		hasDot := false
		for _, char := range c.Value {
			if char == '@' {
				hasAt = true
			}
			if char == '.' {
				hasDot = true
			}
		}
		return hasAt && hasDot

	case ContactTypeTelegram:
		// Telegram username должен начинаться с @
		return len(c.Value) > 1 && (c.Value[0] == '@' || len(c.Value) > 4)

	default:
		return len(c.Value) > 0
	}
}

func (c *ContactInfo) GetDisplayValue() string {
	switch c.Type {
	case ContactTypeTelegram:
		if len(c.Value) > 0 && c.Value[0] != '@' {
			return "@" + c.Value
		}
		return c.Value
	default:
		return c.Value
	}
}

func (c *ContactInfo) GetFormattedValue() string {
	display := c.GetDisplayValue()
	if c.Label != "" {
		return c.Label + ": " + display
	}
	return display
}

// Получение переводов категорий для UI
func GetAnnouncementCategoryTranslation(category string) string {
	translations := map[string]string{
		AnnouncementCategoryWork:      "Работа",
		AnnouncementCategoryHelp:      "Помощь",
		AnnouncementCategoryServices:  "Услуги",
		AnnouncementCategoryHousing:   "Жилье",
		AnnouncementCategoryTransport: "Транспорт",
	}
	if translation, exists := translations[category]; exists {
		return translation
	}
	return category
}

// Получение переводов типов занятости для UI
func GetEmploymentTranslation(employment string) string {
	translations := map[string]string{
		EmploymentOnce:      "Разовая",
		EmploymentPermanent: "Постоянная",
		EmploymentPartial:   "Частичная",
	}
	if translation, exists := translations[employment]; exists {
		return translation
	}
	return employment
}

// Получение переводов типов контактов для UI
func GetContactTypeTranslation(contactType string) string {
	translations := map[string]string{
		ContactTypePhone:    "Телефон",
		ContactTypeEmail:    "Email",
		ContactTypeTelegram: "Telegram",
		ContactTypeViber:    "Viber",
		ContactTypeWhatsApp: "WhatsApp",
	}
	if translation, exists := translations[contactType]; exists {
		return translation
	}
	return contactType
}
