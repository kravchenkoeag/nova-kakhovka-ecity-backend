package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Location struct {
	Type        string    `bson:"type" json:"type" validate:"required"` // "Point"
	Coordinates []float64 `bson:"coordinates" json:"coordinates" validate:"required,len=2"`
}

type UserStatus struct {
	Message   string    `bson:"message" json:"message"`
	IsVisible bool      `bson:"is_visible" json:"is_visible"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

type BusinessInfo struct {
	Name        string   `bson:"name" json:"name"`
	Description string   `bson:"description" json:"description"`
	Services    []string `bson:"services" json:"services"`
	Category    string   `bson:"category" json:"category"`
}

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Email        string             `bson:"email" json:"email" validate:"required,email"`
	Phone        string             `bson:"phone" json:"phone" validate:"omitempty,min=10,max=15"`
	PasswordHash string             `bson:"password_hash" json:"-"`

	// Личная информация
	FirstName  string `bson:"first_name" json:"first_name" validate:"required,min=2,max=50"`
	LastName   string `bson:"last_name" json:"last_name" validate:"required,min=2,max=50"`
	Profession string `bson:"profession" json:"profession"`
	ProfilePic string `bson:"profile_pic" json:"profile_pic"`

	// Местоположение и настройки приватности
	CurrentLocation   Location `bson:"current_location" json:"current_location"`
	RegisteredAddress string   `bson:"registered_address" json:"registered_address"`
	IsAddressVisible  bool     `bson:"is_address_visible" json:"is_address_visible"`

	// Интересы и статус
	Interests []string   `bson:"interests" json:"interests"`
	Status    UserStatus `bson:"status" json:"status"`

	// Бизнес информация
	BusinessInfo *BusinessInfo `bson:"business_info,omitempty" json:"business_info,omitempty"`

	// Группы и разрешения
	Groups      []primitive.ObjectID `bson:"groups" json:"groups"`
	IsVerified  bool                 `bson:"is_verified" json:"is_verified"`
	IsModerator bool                 `bson:"is_moderator" json:"is_moderator"`
	IsBlocked   bool                 `bson:"is_blocked" json:"is_blocked"`

	// Временные метки
	CreatedAt       time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `bson:"updated_at" json:"updated_at"`
	LastLoginAt     *time.Time `bson:"last_login_at,omitempty" json:"last_login_at,omitempty"`
	EmailVerifiedAt *time.Time `bson:"email_verified_at,omitempty" json:"email_verified_at,omitempty"`
	PhoneVerifiedAt *time.Time `bson:"phone_verified_at,omitempty" json:"phone_verified_at,omitempty"`
}
