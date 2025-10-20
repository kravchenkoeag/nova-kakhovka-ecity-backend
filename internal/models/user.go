// internal/models/user.go

package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Location представляє географічні координати
type Location struct {
	Type        string    `bson:"type" json:"type"`
	Coordinates []float64 `bson:"coordinates" json:"coordinates"` // [longitude, latitude]
}

// UserStatus представляє статус користувача
type UserStatus struct {
	Message   string    `bson:"message" json:"message"`
	IsVisible bool      `bson:"is_visible" json:"is_visible"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// BusinessInfo представляє інформацію про бізнес користувача
type BusinessInfo struct {
	Name        string   `bson:"name" json:"name"`
	Description string   `bson:"description" json:"description"`
	Services    []string `bson:"services" json:"services"`
	Category    string   `bson:"category" json:"category"`
}

// User представляє користувача системи
type User struct {
	ID                primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	Email             string               `bson:"email" json:"email"`
	Phone             string               `bson:"phone,omitempty" json:"phone,omitempty"`
	PasswordHash      string               `bson:"password_hash" json:"-"` // Не повертається в JSON
	FirstName         string               `bson:"first_name" json:"first_name"`
	LastName          string               `bson:"last_name" json:"last_name"`
	Profession        string               `bson:"profession,omitempty" json:"profession,omitempty"`
	ProfilePic        string               `bson:"profile_pic,omitempty" json:"profile_pic,omitempty"`
	CurrentLocation   *Location            `bson:"current_location,omitempty" json:"current_location,omitempty"`
	RegisteredAddress string               `bson:"registered_address,omitempty" json:"registered_address,omitempty"`
	IsAddressVisible  bool                 `bson:"is_address_visible" json:"is_address_visible"`
	Interests         []string             `bson:"interests" json:"interests"`
	Status            UserStatus           `bson:"status" json:"status"`
	BusinessInfo      *BusinessInfo        `bson:"business_info,omitempty" json:"business_info,omitempty"`
	Groups            []primitive.ObjectID `bson:"groups" json:"groups"`

	// ✅ НОВИЙ КОД: Система ролей
	Role        string `bson:"role" json:"role"`                 // NEW: USER, MODERATOR, ADMIN, SUPER_ADMIN
	IsModerator bool   `bson:"is_moderator" json:"is_moderator"` // LEGACY: Для зворотної сумісності

	IsVerified      bool       `bson:"is_verified" json:"is_verified"`
	IsBlocked       bool       `bson:"is_blocked" json:"is_blocked"`
	CreatedAt       time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `bson:"updated_at" json:"updated_at"`
	LastLoginAt     *time.Time `bson:"last_login_at,omitempty" json:"last_login_at,omitempty"`
	EmailVerifiedAt *time.Time `bson:"email_verified_at,omitempty" json:"email_verified_at,omitempty"`
	PhoneVerifiedAt *time.Time `bson:"phone_verified_at,omitempty" json:"phone_verified_at,omitempty"`
}

// GetFullName повертає повне ім'я користувача
func (u *User) GetFullName() string {
	return u.FirstName + " " + u.LastName
}

// GetRole повертає роль користувача як UserRole
func (u *User) GetRole() UserRole {
	return UserRole(u.Role)
}

// SetRole встановлює роль користувача
func (u *User) SetRole(role UserRole) {
	u.Role = string(role)

	// Оновлюємо legacy поле для зворотної сумісності
	u.IsModerator = role == RoleModerator ||
		role == RoleAdmin ||
		role == RoleSuperAdmin
}

// HasRole перевіряє чи користувач має певну роль
func (u *User) HasRole(role UserRole) bool {
	return u.GetRole() == role
}

// IsAtLeast перевіряє чи роль користувача не нижча за вказану
func (u *User) IsAtLeast(role UserRole) bool {
	return u.GetRole().IsHigherOrEqual(role)
}

// CanManage перевіряє чи користувач може керувати іншим користувачем
func (u *User) CanManage(target *User) bool {
	return u.GetRole().CanManageUser(target.GetRole())
}
