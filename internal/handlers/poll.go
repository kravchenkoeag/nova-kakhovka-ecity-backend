// internal/handlers/poll.go
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"nova-kakhovka-ecity/internal/models"
	"nova-kakhovka-ecity/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PollHandler struct {
	pollCollection      *mongo.Collection
	userCollection      *mongo.Collection
	notificationService *services.NotificationService
}

type CreatePollRequest struct {
	Title            string                 `json:"title" validate:"required,min=5,max=300"`
	Description      string                 `json:"description" validate:"required,min=10,max=2000"`
	Category         string                 `json:"category" validate:"required,oneof=city_planning transport infrastructure social environment governance budget education healthcare"`
	Questions        []CreatePollQuestion   `json:"questions" validate:"required,min=1"`
	AllowMultiple    bool                   `json:"allow_multiple"`
	IsAnonymous      bool                   `json:"is_anonymous"`
	IsPublic         bool                   `json:"is_public"`
	TargetGroups     []string               `json:"target_groups,omitempty"`
	AgeRestriction   *models.AgeRestriction `json:"age_restriction,omitempty"`
	LocationRequired bool                   `json:"location_required"`
	StartDate        time.Time              `json:"start_date"`
	EndDate          time.Time              `json:"end_date" validate:"required"`
	Tags             []string               `json:"tags"`
}

type CreatePollQuestion struct {
	Text       string             `json:"text" validate:"required,min=5,max=500"`
	Type       string             `json:"type" validate:"required,oneof=single_choice multiple_choice rating text scale yes_no"`
	Options    []CreatePollOption `json:"options,omitempty"`
	IsRequired bool               `json:"is_required"`
	MinRating  int                `json:"min_rating,omitempty"`
	MaxRating  int                `json:"max_rating,omitempty"`
	MaxLength  int                `json:"max_length,omitempty"`
}

type CreatePollOption struct {
	Text  string `json:"text" validate:"required,min=1,max=200"`
	Image string `json:"image,omitempty"`
}

type SubmitPollResponseRequest struct {
	Answers []SubmitPollAnswer `json:"answers" validate:"required,min=1"`
}

type SubmitPollAnswer struct {
	QuestionID   string   `json:"question_id" validate:"required"`
	OptionIDs    []string `json:"option_ids,omitempty"`
	TextAnswer   string   `json:"text_answer,omitempty"`
	NumberAnswer *int     `json:"number_answer,omitempty"`
	BoolAnswer   *bool    `json:"bool_answer,omitempty"`
}

type PollFilters struct {
	Category  string    `form:"category"`
	Status    string    `form:"status"`
	CreatorID string    `form:"creator_id"`
	IsPublic  *bool     `form:"is_public"`
	DateFrom  time.Time `form:"date_from"`
	DateTo    time.Time `form:"date_to"`
	Page      int       `form:"page"`
	Limit     int       `form:"limit"`
	SortBy    string    `form:"sort_by"`
	SortOrder string    `form:"sort_order"`
	Search    string    `form:"search"`
}

func NewPollHandler(pollCollection, userCollection *mongo.Collection, notificationService *services.NotificationService) *PollHandler {
	return &PollHandler{
		pollCollection:      pollCollection,
		userCollection:      userCollection,
		notificationService: notificationService,
	}
}

func (h *PollHandler) validatePollCreation(req CreatePollRequest) error {
	// Проверяем каждый вопрос
	for i, question := range req.Questions {
		switch question.Type {
		case models.QuestionTypeSingleChoice, models.QuestionTypeMultipleChoice:
			if len(question.Options) < 2 {
				return fmt.Errorf("question %d: choice questions must have at least 2 options", i+1)
			}
			if len(question.Options) > 20 {
				return fmt.Errorf("question %d: too many options (max 20)", i+1)
			}

		case models.QuestionTypeRating, models.QuestionTypeScale:
			if question.MinRating >= question.MaxRating {
				return fmt.Errorf("question %d: min_rating must be less than max_rating", i+1)
			}
			if question.MinRating < 1 || question.MaxRating > 10 {
				return fmt.Errorf("question %d: rating must be between 1 and 10", i+1)
			}

		case models.QuestionTypeText:
			if question.MaxLength > 0 && question.MaxLength < 10 {
				return fmt.Errorf("question %d: max_length must be at least 10", i+1)
			}
			if question.MaxLength > 5000 {
				return fmt.Errorf("question %d: max_length cannot exceed 5000", i+1)
			}
		}
	}
	return nil
}

// CreatePoll создает новое опитування
func (h *PollHandler) CreatePoll(c *gin.Context) {
	var req CreatePollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Проверяем права модератора для создания
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	// Валидация специфичная для опросов
	if err := h.validatePollCreation(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid poll configuration",
			"details": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Формируем вопросы
	questions := make([]models.PollQuestion, 0, len(req.Questions))
	for _, q := range req.Questions {
		question := models.PollQuestion{
			ID:         primitive.NewObjectID(),
			Text:       q.Text,
			Type:       q.Type,
			IsRequired: q.IsRequired,
			MinRating:  q.MinRating,
			MaxRating:  q.MaxRating,
			MaxLength:  q.MaxLength,
		}

		// Добавляем опции для вопросов с выбором
		if q.Type == models.QuestionTypeSingleChoice || q.Type == models.QuestionTypeMultipleChoice {
			for _, opt := range q.Options {
				question.Options = append(question.Options, models.PollOption{
					ID:    primitive.NewObjectID(),
					Text:  opt.Text,
					Image: opt.Image,
					Votes: 0,
				})
			}
		}

		questions = append(questions, question)
	}

	now := time.Now()
	poll := models.Poll{
		CreatorID:        userIDObj,
		Title:            req.Title,
		Description:      req.Description,
		Category:         req.Category,
		Questions:        questions,
		Responses:        []models.PollResponse{},
		AllowMultiple:    req.AllowMultiple,
		IsAnonymous:      req.IsAnonymous,
		IsPublic:         req.IsPublic,
		Status:           models.PollStatusActive,
		CreatedAt:        now,
		UpdatedAt:        now,
		StartDate:        req.StartDate,
		EndDate:          req.EndDate,
		Tags:             req.Tags,
		TargetGroups:     []primitive.ObjectID{}, // Конвертировать строки в ObjectID если нужно
		AgeRestriction:   req.AgeRestriction,
		LocationRequired: req.LocationRequired,
		ViewCount:        0,
		ShareCount:       0,
	}

	result, err := h.pollCollection.InsertOne(ctx, poll)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating poll",
		})
		return
	}

	poll.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, poll)
}

// GetPolls возвращает список опросов с фильтрацией
func (h *PollHandler) GetPolls(c *gin.Context) {
	var filters PollFilters
	if err := c.ShouldBindQuery(&filters); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid query parameters",
			"details": err.Error(),
		})
		return
	}

	// Дефолтные значения для пагинации
	if filters.Page < 1 {
		filters.Page = 1
	}
	if filters.Limit < 1 || filters.Limit > 100 {
		filters.Limit = 20
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Построение фильтра запроса
	query := bson.M{}

	if filters.Category != "" {
		query["category"] = filters.Category
	}
	if filters.Status != "" {
		query["status"] = filters.Status
	}
	if filters.CreatorID != "" {
		if creatorID, err := primitive.ObjectIDFromHex(filters.CreatorID); err == nil {
			query["creator_id"] = creatorID
		}
	}
	if filters.IsPublic != nil {
		query["is_public"] = *filters.IsPublic
	}
	if !filters.DateFrom.IsZero() || !filters.DateTo.IsZero() {
		dateQuery := bson.M{}
		if !filters.DateFrom.IsZero() {
			dateQuery["$gte"] = filters.DateFrom
		}
		if !filters.DateTo.IsZero() {
			dateQuery["$lte"] = filters.DateTo
		}
		query["created_at"] = dateQuery
	}
	if filters.Search != "" {
		query["$or"] = []bson.M{
			{"title": bson.M{"$regex": filters.Search, "$options": "i"}},
			{"description": bson.M{"$regex": filters.Search, "$options": "i"}},
		}
	}

	// Настройка сортировки
	sortOptions := options.Find()
	if filters.SortBy != "" {
		sortOrder := 1
		if filters.SortOrder == "desc" {
			sortOrder = -1
		}
		sortOptions.SetSort(bson.D{{filters.SortBy, sortOrder}})
	} else {
		sortOptions.SetSort(bson.D{{"created_at", -1}})
	}

	// Пагинация
	skip := (filters.Page - 1) * filters.Limit
	sortOptions.SetLimit(int64(filters.Limit))
	sortOptions.SetSkip(int64(skip))

	// Выполнение запроса
	cursor, err := h.pollCollection.Find(ctx, query, sortOptions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching polls",
		})
		return
	}
	defer cursor.Close(ctx)

	var polls []models.Poll
	if err := cursor.All(ctx, &polls); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding polls",
		})
		return
	}

	// Подсчет общего количества
	total, _ := h.pollCollection.CountDocuments(ctx, query)

	c.JSON(http.StatusOK, gin.H{
		"polls": polls,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       total,
			"total_pages": (total + int64(filters.Limit) - 1) / int64(filters.Limit),
		},
	})
}

// GetPoll возвращает детальную информацию об опросе
func (h *PollHandler) GetPoll(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var poll models.Poll
	err = h.pollCollection.FindOne(ctx, bson.M{"_id": pollID}).Decode(&poll)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Poll not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching poll",
		})
		return
	}

	// Увеличиваем счетчик просмотров
	h.pollCollection.UpdateOne(
		ctx,
		bson.M{"_id": pollID},
		bson.M{"$inc": bson.M{"view_count": 1}},
	)

	c.JSON(http.StatusOK, poll)
}

// UpdatePoll обновляет информацию об опросе
func (h *PollHandler) UpdatePoll(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	var updateReq map[string]interface{}
	if err := c.ShouldBindJSON(&updateReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем существование опроса и права доступа
	var poll models.Poll
	err = h.pollCollection.FindOne(ctx, bson.M{"_id": pollID}).Decode(&poll)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Poll not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching poll",
		})
		return
	}

	// Проверяем права (только создатель или модератор)
	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)
	isModerator, _ := c.Get("is_moderator")

	if poll.CreatorID != userIDObj && !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You don't have permission to update this poll",
		})
		return
	}

	// Удаляем поля, которые не должны обновляться
	delete(updateReq, "_id")
	delete(updateReq, "creator_id")
	delete(updateReq, "responses")
	delete(updateReq, "created_at")

	updateReq["updated_at"] = time.Now()

	result, err := h.pollCollection.UpdateOne(
		ctx,
		bson.M{"_id": pollID},
		bson.M{"$set": updateReq},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating poll",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Poll not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Poll updated successfully",
	})
}

// DeletePoll удаляет опрос
func (h *PollHandler) DeletePoll(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем существование и права
	var poll models.Poll
	err = h.pollCollection.FindOne(ctx, bson.M{"_id": pollID}).Decode(&poll)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Poll not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching poll",
		})
		return
	}

	// Проверяем права
	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)
	isModerator, _ := c.Get("is_moderator")

	if poll.CreatorID != userIDObj && !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You don't have permission to delete this poll",
		})
		return
	}

	result, err := h.pollCollection.DeleteOne(ctx, bson.M{"_id": pollID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting poll",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Poll not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Poll deleted successfully",
	})
}

// VotePoll позволяет пользователю проголосовать
func (h *PollHandler) VotePoll(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	var req SubmitPollResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Получаем опрос
	var poll models.Poll
	err = h.pollCollection.FindOne(ctx, bson.M{"_id": pollID}).Decode(&poll)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Poll not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching poll",
		})
		return
	}

	// Проверяем статус опроса
	if poll.Status != models.PollStatusActive {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Poll is not active",
		})
		return
	}

	// Проверяем даты
	now := time.Now()
	if now.Before(poll.StartDate) || now.After(poll.EndDate) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Poll is not available at this time",
		})
		return
	}

	// Проверяем, не голосовал ли уже пользователь
	if !poll.AllowMultiple {
		for _, response := range poll.Responses {
			if response.UserID != nil && *response.UserID == userIDObj {
				c.JSON(http.StatusConflict, gin.H{
					"error": "You have already voted in this poll",
				})
				return
			}
		}
	}

	// Создаем ответ
	response := models.PollResponse{
		ID:        primitive.NewObjectID(),
		Answers:   []models.PollAnswer{},
		CreatedAt: now,
	}

	// Если опрос не анонимный, сохраняем ID пользователя
	if !poll.IsAnonymous {
		response.UserID = &userIDObj
	}

	// Обрабатываем каждый ответ
	for _, answer := range req.Answers {
		questionID, err := primitive.ObjectIDFromHex(answer.QuestionID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid question ID",
			})
			return
		}

		// Находим вопрос
		var question *models.PollQuestion
		for i := range poll.Questions {
			if poll.Questions[i].ID == questionID {
				question = &poll.Questions[i]
				break
			}
		}

		if question == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Question not found",
			})
			return
		}

		pollAnswer := models.PollAnswer{
			QuestionID: questionID,
		}

		// Обрабатываем в зависимости от типа вопроса
		switch question.Type {
		case models.QuestionTypeSingleChoice, models.QuestionTypeMultipleChoice:
			if len(answer.OptionIDs) == 0 && question.IsRequired {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Question '%s' is required", question.Text),
				})
				return
			}

			// Проверяем количество выбранных опций
			if question.Type == models.QuestionTypeSingleChoice && len(answer.OptionIDs) > 1 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Question '%s' allows only one option", question.Text),
				})
				return
			}

			// Конвертируем и проверяем опции
			for _, optID := range answer.OptionIDs {
				optionID, err := primitive.ObjectIDFromHex(optID)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": "Invalid option ID",
					})
					return
				}

				// Проверяем существование опции
				found := false
				for _, opt := range question.Options {
					if opt.ID == optionID {
						found = true
						break
					}
				}
				if !found {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": "Option not found",
					})
					return
				}

				pollAnswer.OptionIDs = append(pollAnswer.OptionIDs, optionID)
			}

		case models.QuestionTypeText:
			if answer.TextAnswer == "" && question.IsRequired {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Question '%s' is required", question.Text),
				})
				return
			}
			if question.MaxLength > 0 && len(answer.TextAnswer) > question.MaxLength {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Answer exceeds max length of %d", question.MaxLength),
				})
				return
			}
			pollAnswer.TextAnswer = answer.TextAnswer

		case models.QuestionTypeRating, models.QuestionTypeScale:
			if answer.NumberAnswer == nil && question.IsRequired {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Question '%s' is required", question.Text),
				})
				return
			}
			if answer.NumberAnswer != nil {
				if *answer.NumberAnswer < question.MinRating || *answer.NumberAnswer > question.MaxRating {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": fmt.Sprintf("Rating must be between %d and %d", question.MinRating, question.MaxRating),
					})
					return
				}
			}
			pollAnswer.NumberAnswer = answer.NumberAnswer

		case models.QuestionTypeYesNo:
			if answer.BoolAnswer == nil && question.IsRequired {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Question '%s' is required", question.Text),
				})
				return
			}
			pollAnswer.BoolAnswer = answer.BoolAnswer
		}

		response.Answers = append(response.Answers, pollAnswer)
	}

	// Проверяем, что все обязательные вопросы имеют ответы
	for _, question := range poll.Questions {
		if question.IsRequired {
			found := false
			for _, answer := range response.Answers {
				if answer.QuestionID == question.ID {
					found = true
					break
				}
			}
			if !found {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Question '%s' is required", question.Text),
				})
				return
			}
		}
	}

	// Обновляем счетчики голосов для выбранных опций
	for _, answer := range response.Answers {
		for _, optionID := range answer.OptionIDs {
			// Находим вопрос и опцию
			for i, question := range poll.Questions {
				if question.ID == answer.QuestionID {
					for j, option := range question.Options {
						if option.ID == optionID {
							poll.Questions[i].Options[j].Votes++
							break
						}
					}
					break
				}
			}
		}
	}

	// Добавляем ответ к опросу
	poll.Responses = append(poll.Responses, response)

	// Сохраняем обновленный опрос
	_, err = h.pollCollection.ReplaceOne(
		ctx,
		bson.M{"_id": pollID},
		poll,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error saving vote",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Vote submitted successfully",
	})
}

// GetPollResults возвращает результаты опроса
func (h *PollHandler) GetPollResults(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var poll models.Poll
	err = h.pollCollection.FindOne(ctx, bson.M{"_id": pollID}).Decode(&poll)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Poll not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching poll",
		})
		return
	}

	// Подготавливаем результаты
	results := gin.H{
		"poll_id":         poll.ID,
		"title":           poll.Title,
		"total_responses": len(poll.Responses),
		"questions":       []gin.H{},
	}

	// Обрабатываем каждый вопрос
	for _, question := range poll.Questions {
		questionResult := gin.H{
			"question_id":   question.ID,
			"text":          question.Text,
			"type":          question.Type,
			"total_answers": 0,
		}

		switch question.Type {
		case models.QuestionTypeSingleChoice, models.QuestionTypeMultipleChoice:
			// Подсчет голосов для каждой опции
			options := []gin.H{}
			totalVotes := 0
			for _, option := range question.Options {
				optionVotes := 0
				for _, response := range poll.Responses {
					for _, answer := range response.Answers {
						if answer.QuestionID == question.ID {
							for _, optID := range answer.OptionIDs {
								if optID == option.ID {
									optionVotes++
									break
								}
							}
						}
					}
				}
				totalVotes += optionVotes
				options = append(options, gin.H{
					"option_id":  option.ID,
					"text":       option.Text,
					"votes":      optionVotes,
					"percentage": 0, // Вычислим после
				})
			}

			// Вычисляем проценты
			for i := range options {
				if totalVotes > 0 {
					options[i]["percentage"] = float64(options[i]["votes"].(int)) / float64(totalVotes) * 100
				}
			}

			questionResult["options"] = options
			questionResult["total_answers"] = totalVotes

		case models.QuestionTypeRating, models.QuestionTypeScale:
			// Подсчет средней оценки
			sum := 0
			count := 0
			distribution := make(map[int]int)

			for _, response := range poll.Responses {
				for _, answer := range response.Answers {
					if answer.QuestionID == question.ID && answer.NumberAnswer != nil {
						sum += *answer.NumberAnswer
						count++
						distribution[*answer.NumberAnswer]++
					}
				}
			}

			average := 0.0
			if count > 0 {
				average = float64(sum) / float64(count)
			}

			questionResult["average_rating"] = average
			questionResult["total_answers"] = count
			questionResult["distribution"] = distribution
			questionResult["min_rating"] = question.MinRating
			questionResult["max_rating"] = question.MaxRating

		case models.QuestionTypeYesNo:
			// Подсчет да/нет
			yesCount := 0
			noCount := 0

			for _, response := range poll.Responses {
				for _, answer := range response.Answers {
					if answer.QuestionID == question.ID && answer.BoolAnswer != nil {
						if *answer.BoolAnswer {
							yesCount++
						} else {
							noCount++
						}
					}
				}
			}

			total := yesCount + noCount
			questionResult["yes_count"] = yesCount
			questionResult["no_count"] = noCount
			questionResult["total_answers"] = total
			if total > 0 {
				questionResult["yes_percentage"] = float64(yesCount) / float64(total) * 100
				questionResult["no_percentage"] = float64(noCount) / float64(total) * 100
			}

		case models.QuestionTypeText:
			// Собираем текстовые ответы
			textAnswers := []string{}
			for _, response := range poll.Responses {
				for _, answer := range response.Answers {
					if answer.QuestionID == question.ID && answer.TextAnswer != "" {
						textAnswers = append(textAnswers, answer.TextAnswer)
					}
				}
			}
			questionResult["answers"] = textAnswers
			questionResult["total_answers"] = len(textAnswers)
		}

		results["questions"] = append(results["questions"].([]gin.H), questionResult)
	}

	c.JSON(http.StatusOK, results)
}

// MarkAsRead помечает опрос как прочитанный пользователем
func (h *PollHandler) MarkAsRead(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Добавляем пользователя в список прочитавших
	_, err = h.pollCollection.UpdateOne(
		ctx,
		bson.M{"_id": pollID},
		bson.M{
			"$addToSet": bson.M{"read_by": userIDObj},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error marking poll as read",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Poll marked as read",
	})
}

// RejectAnnouncement отклоняет оголошення (для модераторів)
func (h *PollHandler) RejectAnnouncement(c *gin.Context) {
	announcementID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.pollCollection.UpdateOne(
		ctx,
		bson.M{"_id": announcementID},
		bson.M{
			"$set": bson.M{
				"status":     "rejected",
				"updated_at": time.Now(),
			},
		},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error rejecting announcement",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Announcement not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Announcement rejected successfully",
	})
}

// ApproveAnnouncement схвалює оголошення (для модераторів)
func (h *PollHandler) ApproveAnnouncement(c *gin.Context) {
	announcementID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.pollCollection.UpdateOne(
		ctx,
		bson.M{"_id": announcementID},
		bson.M{
			"$set": bson.M{
				"status":      "approved",
				"is_verified": true,
				"updated_at":  time.Now(),
			},
		},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error approving announcement",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Announcement not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Announcement approved successfully",
	})
}

// DeleteVehicle видаляє транспортний засіб (для адміністраторів)
func (h *PollHandler) DeleteVehicle(c *gin.Context) {
	vehicleID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid vehicle ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.pollCollection.DeleteOne(ctx, bson.M{"_id": vehicleID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting vehicle",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Vehicle not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Vehicle deleted successfully",
	})
}

// UpdateVehicle оновлює інформацію про транспортний засіб
func (h *PollHandler) UpdateVehicle(c *gin.Context) {
	vehicleID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid vehicle ID",
		})
		return
	}

	var updateReq map[string]interface{}
	if err := c.ShouldBindJSON(&updateReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Оновлюємо час останньої модифікації
	updateReq["updated_at"] = time.Now()

	result, err := h.pollCollection.UpdateOne(
		ctx,
		bson.M{"_id": vehicleID},
		bson.M{"$set": updateReq},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating vehicle",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Vehicle not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Vehicle updated successfully",
	})
}

// DeleteRoute видаляє маршрут транспорту
func (h *PollHandler) DeleteRoute(c *gin.Context) {
	routeID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid route ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.pollCollection.DeleteOne(ctx, bson.M{"_id": routeID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting route",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Route not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Route deleted successfully",
	})
}

// StartPollCleanupScheduler запускає фонову задачу для очищення старих опитувань
func (h *PollHandler) StartPollCleanupScheduler() {
	go func() {
		ticker := time.NewTicker(24 * time.Hour) // Раз на добу
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				h.cleanupExpiredPolls()
			}
		}
	}()
}

// cleanupExpiredPolls видаляє або архівує старі опитування
func (h *PollHandler) cleanupExpiredPolls() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	now := time.Now()

	// Змінюємо статус опитувань, які закінчилися
	filter := bson.M{
		"status":   models.PollStatusActive,
		"end_date": bson.M{"$lt": now},
	}

	update := bson.M{
		"$set": bson.M{
			"status":     models.PollStatusCompleted,
			"updated_at": now,
		},
	}

	result, err := h.pollCollection.UpdateMany(ctx, filter, update)
	if err == nil && result.ModifiedCount > 0 {
		// Логуємо кількість завершених опитувань
		// В реальному додатку використовуйте логер
		fmt.Printf("Marked %d polls as completed\n", result.ModifiedCount)
	}

	// Видаляємо дуже старі чернетки (старші 30 днів)
	oldDraftFilter := bson.M{
		"status": models.PollStatusDraft,
		"created_at": bson.M{
			"$lt": now.AddDate(0, 0, -30),
		},
	}

	deleteResult, err := h.pollCollection.DeleteMany(ctx, oldDraftFilter)
	if err == nil && deleteResult.DeletedCount > 0 {
		fmt.Printf("Deleted %d old draft polls\n", deleteResult.DeletedCount)
	}
}
