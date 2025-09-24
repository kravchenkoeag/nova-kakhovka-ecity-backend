// internal/handlers/poll.go
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
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
			if question.MaxLength <= 0 {
				question.MaxLength = 1000 // Значение по умолчанию
			}
			if question.MaxLength > 5000 {
				return fmt.Errorf("question %d: max text length cannot exceed 5000 characters", i+1)
			}

		case models.QuestionTypeYesNo:
			// Для yes/no вопросов опции не нужны
			if len(question.Options) > 0 {
				return fmt.Errorf("question %d: yes/no questions should not have options", i+1)
			}
		}
	}

	// Проверяем целевые группы
	if len(req.TargetGroups) > 0 && !req.IsPublic {
		// Проверяем существование групп
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for _, groupIDStr := range req.TargetGroups {
			groupID, err := primitive.ObjectIDFromHex(groupIDStr)
			if err != nil {
				return fmt.Errorf("invalid group ID: %s", groupIDStr)
			}

			count, err := h.userCollection.Database().Collection("groups").CountDocuments(ctx, bson.M{"_id": groupID})
			if err != nil || count == 0 {
				return fmt.Errorf("group not found: %s", groupIDStr)
			}
		}
	}

	return nil
}

func (h *PollHandler) validatePollResponse(poll models.Poll, answers []SubmitPollAnswer) error {
	// Создаем карту вопросов для быстрого поиска
	questionMap := make(map[primitive.ObjectID]models.PollQuestion)
	for _, question := range poll.Questions {
		questionMap[question.ID] = question
	}

	// Проверяем каждый ответ
	answeredQuestions := make(map[primitive.ObjectID]bool)

	for i, answer := range answers {
		questionID, err := primitive.ObjectIDFromHex(answer.QuestionID)
		if err != nil {
			return fmt.Errorf("answer %d: invalid question ID", i+1)
		}

		question, exists := questionMap[questionID]
		if !exists {
			return fmt.Errorf("answer %d: question not found", i+1)
		}

		answeredQuestions[questionID] = true

		// Валидация в зависимости от типа вопроса
		switch question.Type {
		case models.QuestionTypeSingleChoice:
			if len(answer.OptionIDs) != 1 {
				return fmt.Errorf("answer %d: single choice question requires exactly 1 option", i+1)
			}
			if !h.isValidOption(question, answer.OptionIDs[0]) {
				return fmt.Errorf("answer %d: invalid option selected", i+1)
			}

		case models.QuestionTypeMultipleChoice:
			if len(answer.OptionIDs) == 0 {
				return fmt.Errorf("answer %d: multiple choice question requires at least 1 option", i+1)
			}
			for _, optionIDStr := range answer.OptionIDs {
				if !h.isValidOption(question, optionIDStr) {
					return fmt.Errorf("answer %d: invalid option selected", i+1)
				}
			}

		case models.QuestionTypeRating, models.QuestionTypeScale:
			if answer.NumberAnswer == nil {
				return fmt.Errorf("answer %d: rating question requires a number answer", i+1)
			}
			if *answer.NumberAnswer < question.MinRating || *answer.NumberAnswer > question.MaxRating {
				return fmt.Errorf("answer %d: rating must be between %d and %d", i+1, question.MinRating, question.MaxRating)
			}

		case models.QuestionTypeText:
			if answer.TextAnswer == "" {
				return fmt.Errorf("answer %d: text question requires a text answer", i+1)
			}
			if len(answer.TextAnswer) > question.MaxLength {
				return fmt.Errorf("answer %d: text answer exceeds maximum length of %d", i+1, question.MaxLength)
			}

		case models.QuestionTypeYesNo:
			if answer.BoolAnswer == nil {
				return fmt.Errorf("answer %d: yes/no question requires a boolean answer", i+1)
			}
		}
	}

	// Проверяем, что все обязательные вопросы отвечены
	for _, question := range poll.Questions {
		if question.IsRequired && !answeredQuestions[question.ID] {
			return fmt.Errorf("required question not answered: %s", question.Text)
		}
	}

	return nil
}

func (h *PollHandler) isValidOption(question models.PollQuestion, optionIDStr string) bool {
	optionID, err := primitive.ObjectIDFromHex(optionIDStr)
	if err != nil {
		return false
	}

	for _, option := range question.Options {
		if option.ID == optionID {
			return true
		}
	}
	return false
}

func (h *PollHandler) CreatePoll(c *gin.Context) {
	var req CreatePollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	// Проверяем права создания опросов (только модераторы)
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Only moderators can create polls",
		})
		return
	}

	// Валидация создания опроса
	if err := h.validatePollCreation(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Валидация дат
	if req.EndDate.Before(time.Now().Add(time.Hour)) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "End date must be at least 1 hour from now",
		})
		return
	}

	if req.StartDate.IsZero() {
		req.StartDate = time.Now()
	}

	if req.EndDate.Before(req.StartDate) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "End date must be after start date",
		})
		return
	}

	// Проверяем лимит на количество активных опросов от одного пользователя
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	activeCount, err := h.pollCollection.CountDocuments(ctx, bson.M{
		"creator_id": userIDObj,
		"status":     bson.M{"$in": []string{models.PollStatusDraft, models.PollStatusActive}},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	if activeCount >= 5 { // Лимит 5 активных опросов
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "Too many active polls. Please complete or delete existing polls first.",
		})
		return
	}

	// Преобразуем группы в ObjectID
	var targetGroups []primitive.ObjectID
	for _, groupIDStr := range req.TargetGroups {
		if groupID, err := primitive.ObjectIDFromHex(groupIDStr); err == nil {
			targetGroups = append(targetGroups, groupID)
		}
	}

	// Создаем вопросы
	var questions []models.PollQuestion
	for _, reqQuestion := range req.Questions {
		questionID := primitive.NewObjectID()

		var options []models.PollOption
		for _, reqOption := range reqQuestion.Options {
			option := models.PollOption{
				ID:    primitive.NewObjectID(),
				Text:  reqOption.Text,
				Image: reqOption.Image,
			}
			options = append(options, option)
		}

		question := models.PollQuestion{
			ID:         questionID,
			Text:       reqQuestion.Text,
			Type:       reqQuestion.Type,
			Options:    options,
			IsRequired: reqQuestion.IsRequired,
			MinRating:  reqQuestion.MinRating,
			MaxRating:  reqQuestion.MaxRating,
			MaxLength:  reqQuestion.MaxLength,
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
		AllowMultiple:    req.AllowMultiple,
		IsAnonymous:      req.IsAnonymous,
		IsPublic:         req.IsPublic,
		TargetGroups:     targetGroups,
		AgeRestriction:   req.AgeRestriction,
		LocationRequired: req.LocationRequired,
		StartDate:        req.StartDate,
		EndDate:          req.EndDate,
		TotalResponses:   0,
		Responses:        []models.PollResponse{},
		Results:          models.PollResults{UpdatedAt: now},
		Status:           models.PollStatusDraft,
		IsVerified:       false,
		ViewCount:        0,
		ShareCount:       0,
		Tags:             req.Tags,
		CreatedAt:        now,
		UpdatedAt:        now,
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

func (h *PollHandler) GetPolls(c *gin.Context) {
	var filters PollFilters
	if err := c.ShouldBindQuery(&filters); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid query parameters",
			"details": err.Error(),
		})
		return
	}

	// Устанавливаем значения по умолчанию
	if filters.Page <= 0 {
		filters.Page = 1
	}
	if filters.Limit <= 0 || filters.Limit > 50 {
		filters.Limit = 20
	}
	if filters.SortBy == "" {
		filters.SortBy = "created_at"
	}
	if filters.SortOrder == "" {
		filters.SortOrder = "desc"
	}

	// Строим фильтр для запроса
	filter := bson.M{}

	if filters.IsPublic == nil || *filters.IsPublic {
		filter["is_public"] = true
	}

	if filters.Category != "" {
		filter["category"] = filters.Category
	}
	if filters.Status != "" {
		filter["status"] = filters.Status
	} else {
		// По умолчанию показываем активные опросы
		filter["status"] = models.PollStatusActive
	}

	if filters.CreatorID != "" {
		creatorID, err := primitive.ObjectIDFromHex(filters.CreatorID)
		if err == nil {
			filter["creator_id"] = creatorID
		}
	}

	if !filters.DateFrom.IsZero() || !filters.DateTo.IsZero() {
		dateFilter := bson.M{}
		if !filters.DateFrom.IsZero() {
			dateFilter["$gte"] = filters.DateFrom
		}
		if !filters.DateTo.IsZero() {
			dateFilter["$lte"] = filters.DateTo
		}
		filter["created_at"] = dateFilter
	}

	// Поиск по тексту
	if filters.Search != "" {
		filter["$or"] = []bson.M{
			{"title": bson.M{"$regex": filters.Search, "$options": "i"}},
			{"description": bson.M{"$regex": filters.Search, "$options": "i"}},
		}
	}

	// Настройки сортировки
	sortOrder := 1
	if filters.SortOrder == "desc" {
		sortOrder = -1
	}

	// Параметры пагинации
	skip := (filters.Page - 1) * filters.Limit
	opts := options.Find().
		SetLimit(int64(filters.Limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{Key: filters.SortBy, Value: sortOrder}})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.pollCollection.Find(ctx, filter, opts)
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

	// Получаем общее количество для пагинации
	totalCount, err := h.pollCollection.CountDocuments(ctx, filter)
	if err != nil {
		totalCount = 0
	}

	totalPages := (totalCount + int64(filters.Limit) - 1) / int64(filters.Limit)

	c.JSON(http.StatusOK, gin.H{
		"polls": polls,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

func (h *PollHandler) GetPoll(c *gin.Context) {
	pollID := c.Param("id")
	pollIDObj, err := primitive.ObjectIDFromHex(pollID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var poll models.Poll
	err = h.pollCollection.FindOne(ctx, bson.M{
		"_id":       pollIDObj,
		"is_public": true,
	}).Decode(&poll)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Poll not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Увеличиваем счетчик просмотров
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.pollCollection.UpdateOne(ctx, bson.M{"_id": pollIDObj}, bson.M{
			"$inc": bson.M{"view_count": 1},
		})
	}()

	c.JSON(http.StatusOK, poll)
}

func (h *PollHandler) SubmitPollResponse(c *gin.Context) {
	pollID := c.Param("id")
	pollIDObj, err := primitive.ObjectIDFromHex(pollID)
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
	err = h.pollCollection.FindOne(ctx, bson.M{
		"_id":      pollIDObj,
		"status":   models.PollStatusActive,
		"end_date": bson.M{"$gt": time.Now()},
	}).Decode(&poll)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Poll not found or not active",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Проверяем, не отвечал ли пользователь уже
	if !poll.AllowMultiple && poll.HasUserResponded(userIDObj) {
		c.JSON(http.StatusConflict, gin.H{
			"error": "User has already responded to this poll",
		})
		return
	}

	// Валидация ответов
	if err := h.validatePollResponse(poll, req.Answers); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Создаем ответ
	now := time.Now()
	var pollAnswers []models.PollAnswer

	for _, submitAnswer := range req.Answers {
		questionID, _ := primitive.ObjectIDFromHex(submitAnswer.QuestionID)

		var optionIDs []primitive.ObjectID
		for _, optionIDStr := range submitAnswer.OptionIDs {
			if optionID, err := primitive.ObjectIDFromHex(optionIDStr); err == nil {
				optionIDs = append(optionIDs, optionID)
			}
		}

		pollAnswer := models.PollAnswer{
			QuestionID:   questionID,
			OptionIDs:    optionIDs,
			TextAnswer:   submitAnswer.TextAnswer,
			NumberAnswer: submitAnswer.NumberAnswer,
			BoolAnswer:   submitAnswer.BoolAnswer,
		}
		pollAnswers = append(pollAnswers, pollAnswer)
	}

	response := models.PollResponse{
		ID:          primitive.NewObjectID(),
		UserID:      userIDObj,
		Answers:     pollAnswers,
		SubmittedAt: now,
		UserAgent:   c.GetHeader("User-Agent"),
		IPAddress:   c.ClientIP(),
	}

	// Добавляем ответ в опрос
	update := bson.M{
		"$push": bson.M{"responses": response},
		"$inc":  bson.M{"total_responses": 1},
		"$set":  bson.M{"updated_at": now},
	}

	result, err := h.pollCollection.UpdateOne(ctx, bson.M{"_id": pollIDObj}, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error submitting poll response",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Poll not found",
		})
		return
	}

	// Пересчитываем результаты в фоновом режиме
	go h.recalculatePollResults(pollIDObj)

	c.JSON(http.StatusCreated, gin.H{
		"message":     "Poll response submitted successfully",
		"response_id": response.ID.Hex(),
	})
}

func (h *PollHandler) GetPollResults(c *gin.Context) {
	pollID := c.Param("id")
	pollIDObj, err := primitive.ObjectIDFromHex(pollID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var poll models.Poll
	err = h.pollCollection.FindOne(ctx, bson.M{
		"_id": pollIDObj,
	}).Decode(&poll)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Poll not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Проверяем права доступа к результатам
	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)
	isModerator, _ := c.Get("is_moderator")

	canViewResults := isModerator.(bool) || poll.CreatorID == userIDObj || poll.Status == models.PollStatusCompleted

	if !canViewResults {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Access denied to poll results",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"poll_id":         poll.ID,
		"title":           poll.Title,
		"total_responses": poll.TotalResponses,
		"results":         poll.Results,
		"status":          poll.Status,
	})
}

func (h *PollHandler) GetUserPolls(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	skip := (page - 1) * limit
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{"created_at", -1}})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.pollCollection.Find(ctx, bson.M{
		"creator_id": userIDObj,
	}, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching user polls",
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

	c.JSON(http.StatusOK, polls)
}

func (h *PollHandler) PublishPoll(c *gin.Context) {
	pollID := c.Param("id")
	pollIDObj, err := primitive.ObjectIDFromHex(pollID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	// Проверяем права модератора
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	result, err := h.pollCollection.UpdateOne(ctx, bson.M{
		"_id":    pollIDObj,
		"status": models.PollStatusDraft,
	}, bson.M{
		"$set": bson.M{
			"status":       models.PollStatusActive,
			"published_at": now,
			"updated_at":   now,
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error publishing poll",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Poll not found or already published",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Poll published successfully",
	})
}

func (h *PollHandler) ClosePoll(c *gin.Context) {
	pollID := c.Param("id")
	pollIDObj, err := primitive.ObjectIDFromHex(pollID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	// Проверяем права модератора
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	result, err := h.pollCollection.UpdateOne(ctx, bson.M{
		"_id":    pollIDObj,
		"status": models.PollStatusActive,
	}, bson.M{
		"$set": bson.M{
			"status":     models.PollStatusCompleted,
			"end_date":   now,
			"updated_at": now,
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error closing poll",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Poll not found or not active",
		})
		return
	}

	// Пересчитываем результаты
	go h.recalculatePollResults(pollIDObj)

	c.JSON(http.StatusOK, gin.H{
		"message": "Poll closed successfully",
	})
}

func (h *PollHandler) DeletePoll(c *gin.Context) {
	pollID := c.Param("id")
	pollIDObj, err := primitive.ObjectIDFromHex(pollID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid poll ID",
		})
		return
	}

	// Проверяем права модератора
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Можно удалить только черновики или завершенные опросы
	result, err := h.pollCollection.DeleteOne(ctx, bson.M{
		"_id": pollIDObj,
		"status": bson.M{"$in": []string{
			models.PollStatusDraft,
			models.PollStatusCompleted,
			models.PollStatusCancelled,
		}},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting poll",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Poll not found or cannot be deleted (active polls cannot be deleted)",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Poll deleted successfully",
	})
}

func (h *PollHandler) GetPollStats(c *gin.Context) {
	// Проверяем права модератора
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Статистика по статусам
	statusPipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$status",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	statusCursor, err := h.pollCollection.Aggregate(ctx, statusPipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error getting poll stats",
		})
		return
	}
	defer statusCursor.Close(ctx)

	statusStats := make(map[string]int)
	for statusCursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int    `bson:"count"`
		}
		if err := statusCursor.Decode(&result); err != nil {
			continue
		}
		statusStats[result.ID] = result.Count
	}

	// Статистика по категориям
	categoryPipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$category",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	categoryCursor, err := h.pollCollection.Aggregate(ctx, categoryPipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error getting category stats",
		})
		return
	}
	defer categoryCursor.Close(ctx)

	categoryStats := make(map[string]int)
	for categoryCursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int    `bson:"count"`
		}
		if err := categoryCursor.Decode(&result); err != nil {
			continue
		}
		categoryStats[result.ID] = result.Count
	}

	// Статистика ответов
	responsePipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":               nil,
				"total_responses":   bson.M{"$sum": "$total_responses"},
				"average_responses": bson.M{"$avg": "$total_responses"},
				"max_responses":     bson.M{"$max": "$total_responses"},
			},
		},
	}

	responseCursor, err := h.pollCollection.Aggregate(ctx, responsePipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error getting response stats",
		})
		return
	}
	defer responseCursor.Close(ctx)

	var responseStats map[string]interface{}
	if responseCursor.Next(ctx) {
		responseCursor.Decode(&responseStats)
	}

	c.JSON(http.StatusOK, gin.H{
		"status_stats":   statusStats,
		"category_stats": categoryStats,
		"response_stats": responseStats,
		"updated_at":     time.Now(),
	})
}

func (h *PollHandler) recalculatePollResults(pollID primitive.ObjectID) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Получаем опрос с ответами
	var poll models.Poll
	err := h.pollCollection.FindOne(ctx, bson.M{"_id": pollID}).Decode(&poll)
	if err != nil {
		return
	}

	// Пересчитываем результаты для каждого вопроса
	var questionResults []models.QuestionResult
	for _, question := range poll.Questions {
		questionResult := models.QuestionResult{
			QuestionID:   question.ID,
			QuestionText: question.Text,
			QuestionType: question.Type,
			TotalAnswers: 0,
		}

		// Собираем все ответы на этот вопрос
		questionAnswers := []models.PollAnswer{}
		for _, response := range poll.Responses {
			for _, answer := range response.Answers {
				if answer.QuestionID == question.ID {
					questionAnswers = append(questionAnswers, answer)
					break
				}
			}
		}

		questionResult.TotalAnswers = len(questionAnswers)

		switch question.Type {
		case models.QuestionTypeSingleChoice, models.QuestionTypeMultipleChoice:
			// Подсчитываем голоса по опциям
			optionCounts := make(map[primitive.ObjectID]int)
			for _, answer := range questionAnswers {
				for _, optionID := range answer.OptionIDs {
					optionCounts[optionID]++
				}
			}

			// Создаем результаты по опциям
			for _, option := range question.Options {
				count := optionCounts[option.ID]
				percentage := float64(0)
				if questionResult.TotalAnswers > 0 {
					percentage = float64(count) / float64(questionResult.TotalAnswers) * 100
				}

				optionResult := models.OptionResult{
					OptionID:   option.ID,
					OptionText: option.Text,
					Count:      count,
					Percentage: percentage,
				}
				questionResult.OptionResults = append(questionResult.OptionResults, optionResult)
			}

		case models.QuestionTypeRating, models.QuestionTypeScale:
			// Вычисляем статистику рейтинга
			var ratings []int
			totalRating := 0
			validAnswers := 0
			for _, answer := range questionAnswers {
				if answer.NumberAnswer != nil {
					rating := *answer.NumberAnswer
					ratings = append(ratings, rating)
					totalRating += rating
					validAnswers++
				}
			}
			if validAnswers > 0 {
				averageRating := float64(totalRating) / float64(validAnswers)
				questionResult.AverageRating = &averageRating

				// Сортируем для медианы
				sort.Ints(ratings)
				minValue := ratings[0]
				maxValue := ratings[len(ratings)-1]
				questionResult.MinValue = &minValue
				questionResult.MaxValue = &maxValue

				// Медиана
				var median float64
				n := len(ratings)
				if n%2 == 0 {
					median = float64(ratings[n/2-1]+ratings[n/2]) / 2
				} else {
					median = float64(ratings[n/2])
				}
				questionResult.MedianValue = &median
			}

		case models.QuestionTypeText:
			// Собираем текстовые ответы (ограничиваем количество для экономии места)
			maxTextAnswers := 100
			for i, answer := range questionAnswers {
				if i >= maxTextAnswers {
					break
				}
				if answer.TextAnswer != "" {
					questionResult.TextAnswers = append(questionResult.TextAnswers, answer.TextAnswer)
				}
			}

		case models.QuestionTypeYesNo:
			// Подсчитываем да/нет ответы
			yesCount := 0
			noCount := 0
			for _, answer := range questionAnswers {
				if answer.BoolAnswer != nil {
					if *answer.BoolAnswer {
						yesCount++
					} else {
						noCount++
					}
				}
			}

			questionResult.YesCount = yesCount
			questionResult.NoCount = noCount

			// Создаем результаты как опции для совместимости
			yesPercentage := float64(0)
			noPercentage := float64(0)
			if questionResult.TotalAnswers > 0 {
				yesPercentage = float64(yesCount) / float64(questionResult.TotalAnswers) * 100
				noPercentage = float64(noCount) / float64(questionResult.TotalAnswers) * 100
			}

			questionResult.OptionResults = []models.OptionResult{
				{
					OptionID:   primitive.NewObjectID(),
					OptionText: "Да",
					Count:      yesCount,
					Percentage: yesPercentage,
				},
				{
					OptionID:   primitive.NewObjectID(),
					OptionText: "Нет",
					Count:      noCount,
					Percentage: noPercentage,
				},
			}
		}

		questionResults = append(questionResults, questionResult)
	}

	// Обновляем результаты в базе данных
	results := models.PollResults{
		QuestionResults: questionResults,
		UpdatedAt:       time.Now(),
	}

	h.pollCollection.UpdateOne(ctx, bson.M{"_id": pollID}, bson.M{
		"$set": bson.M{
			"results":    results,
			"updated_at": time.Now(),
		},
	})
}

// Автоматическое закрытие истекших опросов
func (h *PollHandler) CloseExpiredPolls() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()
	result, err := h.pollCollection.UpdateMany(ctx, bson.M{
		"status":   models.PollStatusActive,
		"end_date": bson.M{"$lte": now},
	}, bson.M{
		"$set": bson.M{
			"status":     models.PollStatusCompleted,
			"updated_at": now,
		},
	})

	if err != nil {
		return
	}

	if result.ModifiedCount > 0 {
		// Логируем количество закрытых опросов
		// log.Printf("Closed %d expired polls", result.ModifiedCount)
	}
}

// Функция для запуска в качестве фоновой задачи
func (h *PollHandler) StartPollCleanupScheduler() {
	ticker := time.NewTicker(1 * time.Hour) // Проверяем каждый час
	go func() {
		for {
			select {
			case <-ticker.C:
				h.CloseExpiredPolls()
			}
		}
	}()
}
