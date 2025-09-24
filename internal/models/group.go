package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Group struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Name        string             `bson:"name" json:"name" validate:"required,min=3,max=100"`
	Description string             `bson:"description" json:"description" validate:"max=500"`
	Type        string             `bson:"type" json:"type" validate:"required,oneof=country region city interest"`

	// Фильтры для автодобавления
	LocationFilter string   `bson:"location_filter" json:"location_filter"`
	InterestFilter []string `bson:"interest_filter" json:"interest_filter"`

	// Участники и администраторы
	Members    []primitive.ObjectID `bson:"members" json:"members"`
	Admins     []primitive.ObjectID `bson:"admins" json:"admins"`
	Moderators []primitive.ObjectID `bson:"moderators" json:"moderators"`

	// Настройки
	IsPublic   bool `bson:"is_public" json:"is_public"`
	AutoJoin   bool `bson:"auto_join" json:"auto_join"`
	MaxMembers int  `bson:"max_members" json:"max_members"`

	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
	CreatedBy primitive.ObjectID `bson:"created_by" json:"created_by"`
}

// Типы групп
const (
	GroupTypeCountry  = "country"
	GroupTypeRegion   = "region"
	GroupTypeCity     = "city"
	GroupTypeInterest = "interest"
)

// Методы для работы с группами

func (g *Group) IsMember(userID primitive.ObjectID) bool {
	for _, memberID := range g.Members {
		if memberID == userID {
			return true
		}
	}
	return false
}

func (g *Group) IsAdmin(userID primitive.ObjectID) bool {
	for _, adminID := range g.Admins {
		if adminID == userID {
			return true
		}
	}
	return false
}

func (g *Group) IsModerator(userID primitive.ObjectID) bool {
	for _, moderatorID := range g.Moderators {
		if moderatorID == userID {
			return true
		}
	}
	return false
}

func (g *Group) CanUserJoin(userID primitive.ObjectID) bool {
	if g.IsMember(userID) {
		return false // Уже участник
	}

	if !g.IsPublic {
		return false // Частная группа
	}

	if g.MaxMembers > 0 && len(g.Members) >= g.MaxMembers {
		return false // Достигнут лимит участников
	}

	return true
}

func (g *Group) CanUserPost(userID primitive.ObjectID) bool {
	return g.IsMember(userID) || g.IsAdmin(userID) || g.IsModerator(userID)
}

func (g *Group) GetMemberCount() int {
	return len(g.Members)
}

func (g *Group) AddMember(userID primitive.ObjectID) bool {
	if g.IsMember(userID) {
		return false
	}

	g.Members = append(g.Members, userID)
	g.UpdatedAt = time.Now()
	return true
}

func (g *Group) RemoveMember(userID primitive.ObjectID) bool {
	for i, memberID := range g.Members {
		if memberID == userID {
			g.Members = append(g.Members[:i], g.Members[i+1:]...)
			g.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

func (g *Group) PromoteToAdmin(userID primitive.ObjectID) bool {
	if !g.IsMember(userID) || g.IsAdmin(userID) {
		return false
	}

	g.Admins = append(g.Admins, userID)
	g.UpdatedAt = time.Now()
	return true
}

func (g *Group) DemoteFromAdmin(userID primitive.ObjectID) bool {
	for i, adminID := range g.Admins {
		if adminID == userID {
			g.Admins = append(g.Admins[:i], g.Admins[i+1:]...)
			g.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// Получение переводов типов групп для UI
func GetGroupTypeTranslation(groupType string) string {
	translations := map[string]string{
		GroupTypeCountry:  "Страна",
		GroupTypeRegion:   "Регион",
		GroupTypeCity:     "Город",
		GroupTypeInterest: "По интересам",
	}
	if translation, exists := translations[groupType]; exists {
		return translation
	}
	return groupType
}
