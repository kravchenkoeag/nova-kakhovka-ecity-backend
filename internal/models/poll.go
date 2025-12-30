// internal/models/poll.go
package models

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Poll struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	CreatorID primitive.ObjectID `bson:"creator_id" json:"creator_id" validate:"required"`

	// Основная информация
	Title       string `bson:"title" json:"title" validate:"required,min=5,max=300"`
	Description string `bson:"description" json:"description" validate:"required,min=10,max=2000"`
	Category    string `bson:"category" json:"category" validate:"required,oneof=city_planning transport infrastructure social environment governance budget education healthcare"`

	// Настройки опроса
	Questions     []PollQuestion `bson:"questions" json:"questions" validate:"required,min=1"`
	AllowMultiple bool           `bson:"allow_multiple" json:"allow_multiple"` // Можно ли отвечать несколько раз
	IsAnonymous   bool           `bson:"is_anonymous" json:"is_anonymous"`     // Анонимный опрос
	IsPublic      bool           `bson:"is_public" json:"is_public"`           // Публичный или только для определенной группы

	// Ограничения участия
	TargetGroups     []primitive.ObjectID `bson:"target_groups,omitempty" json:"target_groups,omitempty"` // Конкретные группы
	AgeRestriction   *AgeRestriction      `bson:"age_restriction,omitempty" json:"age_restriction,omitempty"`
	LocationRequired bool                 `bson:"location_required" json:"location_required"` // Требуется ли быть в определенной локации

	// Временные рамки
	StartDate time.Time `bson:"start_date" json:"start_date"`
	EndDate   time.Time `bson:"end_date" json:"end_date"`

	// Статистика и результаты
	TotalResponses int            `bson:"total_responses" json:"total_responses"`
	Responses      []PollResponse `bson:"responses" json:"responses"`
	ResponseCount  int            `bson:"response_count" json:"response_count"`
	Results        PollResults    `bson:"results" json:"results"`

	// Статус и модерация
	Status        string `bson:"status" json:"status"` // draft, active, completed, cancelled
	IsVerified    bool   `bson:"is_verified" json:"is_verified"`
	ModeratorNote string `bson:"moderator_note,omitempty" json:"moderator_note,omitempty"`

	// Метаданные
	ViewCount   int        `bson:"view_count" json:"view_count"`
	ShareCount  int        `bson:"share_count" json:"share_count"`
	Tags        []string   `bson:"tags" json:"tags"`
	CreatedAt   time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at" json:"updated_at"`
	PublishedAt *time.Time `bson:"published_at,omitempty" json:"published_at,omitempty"`
}

type PollQuestion struct {
	ID         primitive.ObjectID `bson:"id" json:"id"`
	Text       string             `bson:"text" json:"text" validate:"required,min=5,max=500"`
	Type       string             `bson:"type" json:"type" validate:"required,oneof=single_choice multiple_choice rating text scale yes_no"`
	Options    []PollOption       `bson:"options,omitempty" json:"options,omitempty"`
	IsRequired bool               `bson:"is_required" json:"is_required"`
	MinRating  int                `bson:"min_rating,omitempty" json:"min_rating,omitempty"` // Для rating/scale
	MaxRating  int                `bson:"max_rating,omitempty" json:"max_rating,omitempty"` // Для rating/scale
	MaxLength  int                `bson:"max_length,omitempty" json:"max_length,omitempty"` // Для text
}

type PollOption struct {
	ID    primitive.ObjectID `bson:"id" json:"id"`
	Text  string             `bson:"text" json:"text" validate:"required,min=1,max=200"`
	Image string             `bson:"image,omitempty" json:"image,omitempty"`
}

type PollResponse struct {
	ID          primitive.ObjectID `bson:"id" json:"id"`
	PollID      primitive.ObjectID `bson:"poll_id" json:"poll_id"`
	UserID      primitive.ObjectID `bson:"user_id" json:"user_id"`
	Answers     []PollAnswer       `bson:"answers" json:"answers"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
	SubmittedAt time.Time          `bson:"submitted_at" json:"submitted_at"`
	UserAgent   string             `bson:"user_agent,omitempty" json:"user_agent,omitempty"`
	IPAddress   string             `bson:"ip_address,omitempty" json:"ip_address,omitempty"`
}

type PollAnswer struct {
	QuestionID   primitive.ObjectID   `bson:"question_id" json:"question_id"`
	OptionIDs    []primitive.ObjectID `bson:"option_ids,omitempty" json:"option_ids,omitempty"`
	TextAnswer   string               `bson:"text_answer,omitempty" json:"text_answer,omitempty"`
	NumberAnswer *int                 `bson:"number_answer,omitempty" json:"number_answer,omitempty"`
	BoolAnswer   *bool                `bson:"bool_answer,omitempty" json:"bool_answer,omitempty"`
}

type PollResults struct {
	QuestionResults []QuestionResult `bson:"question_results" json:"question_results"`
	Demographics    Demographics     `bson:"demographics,omitempty" json:"demographics,omitempty"`
	UpdatedAt       time.Time        `bson:"updated_at" json:"updated_at"`
}

type QuestionResult struct {
	QuestionID    primitive.ObjectID `bson:"question_id" json:"question_id"`
	QuestionText  string             `bson:"question_text" json:"question_text"`
	QuestionType  string             `bson:"question_type" json:"question_type"`
	OptionResults []OptionResult     `bson:"option_results,omitempty" json:"option_results,omitempty"`
	TextAnswers   []string           `bson:"text_answers,omitempty" json:"text_answers,omitempty"`
	AverageRating *float64           `bson:"average_rating,omitempty" json:"average_rating,omitempty"`
	TotalAnswers  int                `bson:"total_answers" json:"total_answers"`
	YesCount      int                `bson:"yes_count,omitempty" json:"yes_count,omitempty"`
	NoCount       int                `bson:"no_count,omitempty" json:"no_count,omitempty"`
	MinValue      *int               `bson:"min_value,omitempty" json:"min_value,omitempty"`
	MaxValue      *int               `bson:"max_value,omitempty" json:"max_value,omitempty"`
	MedianValue   *float64           `bson:"median_value,omitempty" json:"median_value,omitempty"`
}

type OptionResult struct {
	OptionID   primitive.ObjectID `bson:"option_id" json:"option_id"`
	OptionText string             `bson:"option_text" json:"option_text"`
	Count      int                `bson:"count" json:"count"`
	Percentage float64            `bson:"percentage" json:"percentage"`
}

type Demographics struct {
	AgeGroups      map[string]int `bson:"age_groups,omitempty" json:"age_groups,omitempty"`
	LocationGroups map[string]int `bson:"location_groups,omitempty" json:"location_groups,omitempty"`
	GenderGroups   map[string]int `bson:"gender_groups,omitempty" json:"gender_groups,omitempty"`
}

type AgeRestriction struct {
	MinAge int `bson:"min_age" json:"min_age" validate:"min=0,max=120"`
	MaxAge int `bson:"max_age" json:"max_age" validate:"min=0,max=120"`
}

type Answer struct {
	QuestionID      primitive.ObjectID   `bson:"question_id" json:"question_id"`
	SelectedOptions []primitive.ObjectID `bson:"selected_options,omitempty" json:"selected_options,omitempty"`
	TextAnswer      string               `bson:"text_answer,omitempty" json:"text_answer,omitempty"` // ← ЗМІНЕНО з *string на string
	Rating          *int                 `bson:"rating,omitempty" json:"rating,omitempty"`
}

// Статусы опросов
const (
	PollStatusDraft     = "draft"
	PollStatusActive    = "active"
	PollStatusCompleted = "completed"
	PollStatusCancelled = "cancelled"
)

// Типы вопросов
const (
	QuestionTypeSingleChoice   = "single_choice"
	QuestionTypeMultipleChoice = "multiple_choice"
	QuestionTypeRating         = "rating"
	QuestionTypeText           = "text"
	QuestionTypeScale          = "scale"
	QuestionTypeYesNo          = "yes_no"
)

// Категории опросов
const (
	PollCategoryCityPlanning   = "city_planning"
	PollCategoryTransport      = "transport"
	PollCategoryInfrastructure = "infrastructure"
	PollCategorySocial         = "social"
	PollCategoryEnvironment    = "environment"
	PollCategoryGovernance     = "governance"
	PollCategoryBudget         = "budget"
	PollCategoryEducation      = "education"
	PollCategoryHealthcare     = "healthcare"
)

// Методы для работы с опросами

func (p *Poll) IsExpired() bool {
	return time.Now().After(p.EndDate)
}

func (p *Poll) CanUserParticipate(user User) bool {
	if p.Status != PollStatusActive {
		return false
	}

	now := time.Now()
	if now.Before(p.StartDate) || now.After(p.EndDate) {
		return false
	}

	if !p.IsPublic && len(p.TargetGroups) > 0 {
		hasAccess := false
		for _, targetGroupID := range p.TargetGroups {
			for _, userGroupID := range user.Groups {
				if userGroupID == targetGroupID {
					hasAccess = true
					break
				}
			}
			if hasAccess {
				break
			}
		}
		if !hasAccess {
			return false
		}
	}

	return true
}

func (p *Poll) HasUserResponded(userID primitive.ObjectID) bool {
	for _, response := range p.Responses {
		if response.UserID == userID {
			return true
		}
	}
	return false
}

func (p *Poll) GetResponseByUser(userID primitive.ObjectID) *PollResponse {
	for i, response := range p.Responses {
		if response.UserID == userID {
			return &p.Responses[i]
		}
	}
	return nil
}

func (q *PollQuestion) ValidateQuestion() error {
	switch q.Type {
	case QuestionTypeSingleChoice, QuestionTypeMultipleChoice:
		if len(q.Options) < 2 {
			return fmt.Errorf("choice questions must have at least 2 options")
		}
		if len(q.Options) > 20 {
			return fmt.Errorf("too many options (max 20)")
		}

	case QuestionTypeRating, QuestionTypeScale:
		if q.MinRating == 0 {
			q.MinRating = 1
		}
		if q.MaxRating == 0 {
			if q.Type == QuestionTypeRating {
				q.MaxRating = 5
			} else {
				q.MaxRating = 10
			}
		}
		if q.MinRating >= q.MaxRating {
			return fmt.Errorf("min_rating must be less than max_rating")
		}
		if q.MinRating < 1 || q.MaxRating > 10 {
			return fmt.Errorf("rating must be between 1 and 10")
		}

	case QuestionTypeText:
		if q.MaxLength == 0 {
			q.MaxLength = 1000
		}
		if q.MaxLength > 5000 {
			return fmt.Errorf("max text length cannot exceed 5000 characters")
		}

	case QuestionTypeYesNo:
		if len(q.Options) > 0 {
			return fmt.Errorf("yes/no questions should not have options")
		}
	}

	return nil
}

func (q *PollQuestion) ValidateAnswer(answer PollAnswer) error {
	switch q.Type {
	case QuestionTypeSingleChoice:
		if len(answer.OptionIDs) != 1 {
			return fmt.Errorf("single choice question requires exactly 1 option")
		}
		if !q.isValidOptionID(answer.OptionIDs[0]) {
			return fmt.Errorf("invalid option selected")
		}

	case QuestionTypeMultipleChoice:
		if len(answer.OptionIDs) == 0 {
			return fmt.Errorf("multiple choice question requires at least 1 option")
		}
		for _, optionID := range answer.OptionIDs {
			if !q.isValidOptionID(optionID) {
				return fmt.Errorf("invalid option selected")
			}
		}

	case QuestionTypeRating, QuestionTypeScale:
		if answer.NumberAnswer == nil {
			return fmt.Errorf("rating/scale question requires a number answer")
		}
		if *answer.NumberAnswer < q.MinRating || *answer.NumberAnswer > q.MaxRating {
			return fmt.Errorf("rating must be between %d and %d", q.MinRating, q.MaxRating)
		}

	case QuestionTypeText:
		if answer.TextAnswer == "" && q.IsRequired {
			return fmt.Errorf("text question requires a text answer")
		}
		if len(answer.TextAnswer) > q.MaxLength {
			return fmt.Errorf("text answer exceeds maximum length of %d", q.MaxLength)
		}

	case QuestionTypeYesNo:
		if answer.BoolAnswer == nil && q.IsRequired {
			return fmt.Errorf("yes/no question requires a boolean answer")
		}
	}

	return nil
}

func (q *PollQuestion) isValidOptionID(optionID primitive.ObjectID) bool {
	for _, option := range q.Options {
		if option.ID == optionID {
			return true
		}
	}
	return false
}

func (q *PollQuestion) GetOptionByID(optionID primitive.ObjectID) *PollOption {
	for i, option := range q.Options {
		if option.ID == optionID {
			return &q.Options[i]
		}
	}
	return nil
}
