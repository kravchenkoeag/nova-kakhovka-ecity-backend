// internal/models/petition.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Petition struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	AuthorID primitive.ObjectID `bson:"author_id" json:"author_id" validate:"required"`

	// Основная информация
	Title       string `bson:"title" json:"title" validate:"required,min=10,max=300"`
	Description string `bson:"description" json:"description" validate:"required,min=50,max=5000"`
	Category    string `bson:"category" json:"category" validate:"required,oneof=infrastructure social environment economy governance safety transport education healthcare"`

	// Цели и требования
	RequiredSignatures int    `bson:"required_signatures" json:"required_signatures" validate:"min=100"`
	Demands            string `bson:"demands" json:"demands" validate:"required,min=20,max=2000"`

	// Подписи и поддержка
	Signatures     []PetitionSignature `bson:"signatures" json:"signatures"`
	SignatureCount int                 `bson:"signature_count" json:"signature_count"`

	// Статус и обработка
	Status           string            `bson:"status" json:"status"` // draft, active, completed, expired, under_review, accepted, rejected
	IsVerified       bool              `bson:"is_verified" json:"is_verified"`
	ModeratorNote    string            `bson:"moderator_note" json:"moderator_note"`
	OfficialResponse *OfficialResponse `bson:"official_response,omitempty" json:"official_response,omitempty"`

	// Временные рамки
	StartDate   time.Time  `bson:"start_date" json:"start_date"`
	EndDate     time.Time  `bson:"end_date" json:"end_date"`
	CreatedAt   time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at" json:"updated_at"`
	CompletedAt *time.Time `bson:"completed_at,omitempty" json:"completed_at,omitempty"`

	// Дополнительные поля
	Tags           []string `bson:"tags" json:"tags"`
	ViewCount      int      `bson:"view_count" json:"view_count"`
	ShareCount     int      `bson:"share_count" json:"share_count"`
	AttachmentURLs []string `bson:"attachment_urls" json:"attachment_urls"`
}

type PetitionSignature struct {
	UserID     primitive.ObjectID `bson:"user_id" json:"user_id"`
	FullName   string             `bson:"full_name" json:"full_name"`
	DiiaKeyID  *string            `bson:"diia_key_id,omitempty" json:"diia_key_id,omitempty"` // Ключ ДІЯ для верификации
	IsVerified bool               `bson:"is_verified" json:"is_verified"`
	SignedAt   time.Time          `bson:"signed_at" json:"signed_at"`
	Comment    string             `bson:"comment,omitempty" json:"comment,omitempty"`
}

type OfficialResponse struct {
	ResponderID   primitive.ObjectID `bson:"responder_id" json:"responder_id"`
	ResponderName string             `bson:"responder_name" json:"responder_name"`
	Position      string             `bson:"position" json:"position"`
	Response      string             `bson:"response" json:"response"`
	Decision      string             `bson:"decision" json:"decision"` // accepted, rejected, partially_accepted
	ActionPlan    string             `bson:"action_plan,omitempty" json:"action_plan,omitempty"`
	RespondedAt   time.Time          `bson:"responded_at" json:"responded_at"`
	Documents     []string           `bson:"documents,omitempty" json:"documents,omitempty"`
}

// Статусы петиций
const (
	PetitionStatusDraft       = "draft"
	PetitionStatusActive      = "active"
	PetitionStatusCompleted   = "completed"
	PetitionStatusExpired     = "expired"
	PetitionStatusUnderReview = "under_review"
	PetitionStatusAccepted    = "accepted"
	PetitionStatusRejected    = "rejected"
)

// Категории петиций
const (
	PetitionCategoryInfrastructure = "infrastructure"
	PetitionCategorySocial         = "social"
	PetitionCategoryEnvironment    = "environment"
	PetitionCategoryEconomy        = "economy"
	PetitionCategoryGovernance     = "governance"
	PetitionCategorySafety         = "safety"
	PetitionCategoryTransport      = "transport"
	PetitionCategoryEducation      = "education"
	PetitionCategoryHealthcare     = "healthcare"
)

// Решения по петициям
const (
	PetitionDecisionAccepted          = "accepted"
	PetitionDecisionRejected          = "rejected"
	PetitionDecisionPartiallyAccepted = "partially_accepted"
)

// Методы для работы с петициями

func (p *Petition) IsExpired() bool {
	return time.Now().After(p.EndDate)
}

func (p *Petition) CanBeSigned() bool {
	return p.Status == PetitionStatusActive && !p.IsExpired()
}

func (p *Petition) HasUserSigned(userID primitive.ObjectID) bool {
	for _, signature := range p.Signatures {
		if signature.UserID == userID {
			return true
		}
	}
	return false
}

func (p *Petition) GetSignatureByUser(userID primitive.ObjectID) *PetitionSignature {
	for i, signature := range p.Signatures {
		if signature.UserID == userID {
			return &p.Signatures[i]
		}
	}
	return nil
}

func (p *Petition) GetProgressPercentage() float64 {
	if p.RequiredSignatures == 0 {
		return 0
	}
	return float64(p.SignatureCount) / float64(p.RequiredSignatures) * 100
}

func (p *Petition) IsGoalReached() bool {
	return p.SignatureCount >= p.RequiredSignatures
}

func (p *Petition) GetVerifiedSignaturesCount() int {
	count := 0
	for _, signature := range p.Signatures {
		if signature.IsVerified {
			count++
		}
	}
	return count
}

func (p *Petition) CanReceiveOfficialResponse() bool {
	return p.Status == PetitionStatusCompleted || p.Status == PetitionStatusUnderReview
}

func (p *Petition) GetDaysLeft() int {
	if p.IsExpired() {
		return 0
	}
	duration := p.EndDate.Sub(time.Now())
	return int(duration.Hours() / 24)
}

func (p *Petition) GetTimeLeft() time.Duration {
	if p.IsExpired() {
		return 0
	}
	return p.EndDate.Sub(time.Now())
}
