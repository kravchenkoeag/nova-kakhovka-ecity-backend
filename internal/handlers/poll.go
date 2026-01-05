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

// PollHandler обробляє запити, пов'язані з опитуваннями
type PollHandler struct {
	pollCollection      *mongo.Collection
	userCollection      *mongo.Collection
	notificationService *services.NotificationService
}

// NewPollHandler створює новий екземпляр PollHandler
func NewPollHandler(db *mongo.Database, notificationService *services.NotificationService) *PollHandler {
	return &PollHandler{
		pollCollection:      db.Collection("polls"),
		userCollection:      db.Collection("users"),
		notificationService: notificationService,
	}
}

// ========================================
// REQUEST/RESPONSE STRUCTURES
// ========================================

// CreatePollRequest структура запиту для створення опроса
type CreatePollRequest struct {
	Title            string                 `json:"title" validate:"required,min=5,max=300"`
	Description      string                 `json:"description" validate:"required,min=10,max=2000"`
	Category         string                 `json:"category" validate:"required,oneof=city_planning transport infrastructure social environment governance budget education healthcare"`
	Questions        []CreatePollQuestion   `json:"questions" validate:"required,min=1,max=20"`
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

// CreatePollQuestion структура питання для створення опроса
type CreatePollQuestion struct {
	Text       string             `json:"text" validate:"required,min=5,max=500"`
	Type       string             `json:"type" validate:"required,oneof=single_choice multiple_choice rating text scale yes_no"`
	IsRequired bool               `json:"is_required"`
	Options    []CreatePollOption `json:"options"`
	MinRating  int                `json:"min_rating,omitempty"`
	MaxRating  int                `json:"max_rating,omitempty"`
	MaxLength  int                `json:"max_length,omitempty"`
}

// CreatePollOption структура опції відповіді для питання
type CreatePollOption struct {
	Text string `json:"text" validate:"required,min=1,max=200"`
}

// SubmitPollResponseRequest структура відповіді користувача на опитування
type SubmitPollResponseRequest struct {
	Answers []PollAnswerRequest `json:"answers" validate:"required,min=1"`
}

// PollAnswerRequest структура одної відповіді на питання
type PollAnswerRequest struct {
	QuestionID   string   `json:"question_id" validate:"required"`
	OptionIDs    []string `json:"option_ids,omitempty"`
	TextAnswer   *string  `json:"text_answer,omitempty"`
	NumberAnswer *int     `json:"number_answer,omitempty"`
	BoolAnswer   *bool    `json:"bool_answer,omitempty"`
}

// PollFilters структура для фільтрації опросів
type PollFilters struct {
	Status    string `form:"status"`
	Category  string `form:"category"`
	CreatorID string `form:"creator_id"`
	Tag       string `form:"tag"`
	IsPublic  *bool  `form:"is_public"`
	SortBy    string `form:"sort_by"`
	SortOrder string `form:"sort_order"`
	Page      int    `form:"page" binding:"min=1"`
	Limit     int    `form:"limit" binding:"min=1,max=100"`
}

// ========================================
// HELPER FUNCTIONS
// ========================================

// checkModerator безпечно перевіряє, чи є користувач модератором
func checkModerator(c *gin.Context) bool {
	if isMod, exists := c.Get("is_moderator"); exists {
		if modBool, ok := isMod.(bool); ok {
			return modBool
		}
	}
	return false
}

// getUserID отримує ID користувача з контексту Gin
func getUserID(c *gin.Context) (primitive.ObjectID, error) {
	userID, exists := c.Get("user_id")
	if !exists {
		return primitive.NilObjectID, fmt.Errorf("user_id not found in context")
	}

	userIDStr, ok := userID.(string)
	if !ok {
		return primitive.NilObjectID, fmt.Errorf("invalid user_id type")
	}

	userIDObj, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("invalid user_id format: %v", err)
	}

	return userIDObj, nil
}

// ========================================
// CRUD OPERATIONS
// ========================================

// CreatePoll створює новий опрос
// @Summary Створити новий опрос
// @Tags polls
// @Accept json
// @Produce json
// @Param poll body CreatePollRequest true "Дані опроса"
// @Success 201 {object} models.Poll
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 429 {object} gin.H
// @Router /api/v1/polls [post]
func (h *PollHandler) CreatePoll(c *gin.Context) {
	// Парсинг запиту
	var req CreatePollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Отримання ID користувача
	userIDObj, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "User not authenticated",
			"details": err.Error(),
		})
		return
	}

	// ✅ ВАЛІДАЦІЯ 1: Мінімальна тривалість опросу (1 година)
	if req.StartDate.IsZero() {
		req.StartDate = time.Now()
	}
	duration := req.EndDate.Sub(req.StartDate)
	if duration < 1*time.Hour {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid poll duration",
			"details": "Poll duration must be at least 1 hour",
		})
		return
	}

	// ✅ ВАЛІДАЦІЯ 2: Перевірка логіки дат (EndDate > StartDate)
	if req.EndDate.Before(req.StartDate) || req.EndDate.Equal(req.StartDate) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid date range",
			"details": "End date must be after start date",
		})
		return
	}

	// ✅ ВАЛІДАЦІЯ 3: Ліміт активних опросів (5 на користувача)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	activeCount, err := h.pollCollection.CountDocuments(ctx, bson.M{
		"creator_id": userIDObj,
		"status":     models.PollStatusActive,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error checking active polls",
			"details": err.Error(),
		})
		return
	}

	if activeCount >= 5 {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":   "Active poll limit reached",
			"details": "You can have maximum 5 active polls at a time",
		})
		return
	}

	// ✅ ВАЛІДАЦІЯ 4: Захист від спаму (rate limiting - 5 хвилин між створенням)
	var lastPoll models.Poll
	findOptions := options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}})
	err = h.pollCollection.FindOne(ctx, bson.M{"creator_id": userIDObj}, findOptions).Decode(&lastPoll)

	if err == nil {
		timeSinceLastPoll := time.Since(lastPoll.CreatedAt)
		if timeSinceLastPoll < 5*time.Minute {
			remainingTime := 5*time.Minute - timeSinceLastPoll
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "Rate limit exceeded",
				"details": fmt.Sprintf("Please wait %v before creating another poll", remainingTime.Round(time.Second)),
			})
			return
		}
	}

	// Перетворення груп
	var targetGroupIDs []primitive.ObjectID
	for _, groupIDStr := range req.TargetGroups {
		groupID, err := primitive.ObjectIDFromHex(groupIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid target group ID",
				"details": fmt.Sprintf("Group ID '%s' is not valid", groupIDStr),
			})
			return
		}
		targetGroupIDs = append(targetGroupIDs, groupID)
	}

	// Створення питань з опціями
	var questions []models.PollQuestion
	for _, q := range req.Questions {
		question := models.PollQuestion{
			ID:         primitive.NewObjectID(),
			Text:       q.Text,
			Type:       q.Type,
			IsRequired: q.IsRequired,
			Options:    []models.PollOption{},
			MinRating:  q.MinRating,
			MaxRating:  q.MaxRating,
			MaxLength:  q.MaxLength,
		}

		// Додавання опцій для питань з вибором
		if q.Type == models.QuestionTypeSingleChoice || q.Type == models.QuestionTypeMultipleChoice {
			if len(q.Options) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid question options",
					"details": fmt.Sprintf("Question '%s' requires at least one option", q.Text),
				})
				return
			}

			for _, opt := range q.Options {
				option := models.PollOption{
					ID:   primitive.NewObjectID(),
					Text: opt.Text,
				}
				question.Options = append(question.Options, option)
			}
		}

		// Валідація rating питань
		if q.Type == models.QuestionTypeRating {
			if q.MinRating >= q.MaxRating {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid rating range",
					"details": fmt.Sprintf("Min rating must be less than max rating for question '%s'", q.Text),
				})
				return
			}
		}

		questions = append(questions, question)
	}

	// Створення об'єкту опросу
	poll := models.Poll{
		ID:               primitive.NewObjectID(),
		Title:            req.Title,
		Description:      req.Description,
		Category:         req.Category,
		CreatorID:        userIDObj,
		Questions:        questions,
		Responses:        []models.PollResponse{},
		Status:           models.PollStatusDraft, // За замовчуванням Draft
		AllowMultiple:    req.AllowMultiple,
		IsAnonymous:      req.IsAnonymous,
		IsPublic:         req.IsPublic,
		TargetGroups:     targetGroupIDs,
		AgeRestriction:   req.AgeRestriction,
		LocationRequired: req.LocationRequired,
		StartDate:        req.StartDate,
		EndDate:          req.EndDate,
		Tags:             req.Tags,
		ViewCount:        0,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Якщо StartDate настав, змінюємо статус на Active
	if !poll.StartDate.After(time.Now()) {
		poll.Status = models.PollStatusActive
	}

	// Збереження в базі даних
	_, err = h.pollCollection.InsertOne(ctx, poll)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error creating poll",
			"details": err.Error(),
		})
		return
	}

	// Надсилання повідомлень цільовим групам
	if len(poll.TargetGroups) > 0 {
		go h.notificationService.NotifyNewPoll(poll.ID, poll.TargetGroups)
	}

	c.JSON(http.StatusCreated, poll)
}

// GetAllPolls повертає список всіх опросів з фільтрацією та пагінацією
// @Summary Отримати список опросів
// @Tags polls
// @Accept json
// @Produce json
// @Param status query string false "Статус опроса"
// @Param category query string false "Категорія опроса"
// @Param page query int false "Номер сторінки" default(1)
// @Param limit query int false "Кількість елементів на сторінці" default(10)
// @Success 200 {object} gin.H
// @Router /api/v1/polls [get]
func (h *PollHandler) GetAllPolls(c *gin.Context) {
	// Парсинг фільтрів
	var filters PollFilters
	if err := c.ShouldBindQuery(&filters); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid query parameters",
			"details": err.Error(),
		})
		return
	}

	// Встановлення значень за замовчуванням
	if filters.Page == 0 {
		filters.Page = 1
	}
	if filters.Limit == 0 {
		filters.Limit = 10
	}

	// Побудова запиту
	query := bson.M{}

	// Фільтр за статусом
	if filters.Status != "" {
		query["status"] = filters.Status
	}

	// Фільтр за категорією
	if filters.Category != "" {
		query["category"] = filters.Category
	}

	// Фільтр за автором
	if filters.CreatorID != "" {
		creatorID, err := primitive.ObjectIDFromHex(filters.CreatorID)
		if err == nil {
			query["creator_id"] = creatorID
		}
	}

	// Фільтр за тегом
	if filters.Tag != "" {
		query["tags"] = filters.Tag
	}

	// Фільтр за публічністю
	if filters.IsPublic != nil {
		query["is_public"] = *filters.IsPublic
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Налаштування сортування
	sortOptions := options.Find()
	if filters.SortBy != "" {
		sortOrder := 1
		if filters.SortOrder == "desc" {
			sortOrder = -1
		}
		sortOptions.SetSort(bson.D{{Key: filters.SortBy, Value: sortOrder}})
	} else {
		sortOptions.SetSort(bson.D{{Key: "created_at", Value: -1}})
	}

	// Пагінація
	skip := (filters.Page - 1) * filters.Limit
	sortOptions.SetLimit(int64(filters.Limit))
	sortOptions.SetSkip(int64(skip))

	// Виконання запиту
	cursor, err := h.pollCollection.Find(ctx, query, sortOptions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error fetching polls",
			"details": err.Error(),
		})
		return
	}
	defer cursor.Close(ctx)

	var polls []models.Poll
	if err := cursor.All(ctx, &polls); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error decoding polls",
			"details": err.Error(),
		})
		return
	}

	// Підрахунок загальної кількості
	total, err := h.pollCollection.CountDocuments(ctx, query)
	if err != nil {
		total = 0
	}

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

// GetPoll повертає детальну інформацію про конкретний опрос
// @Summary Отримати опрос за ID
// @Tags polls
// @Accept json
// @Produce json
// @Param id path string true "ID опроса"
// @Success 200 {object} models.Poll
// @Failure 400 {object} gin.H
// @Failure 404 {object} gin.H
// @Router /api/v1/polls/{id} [get]
func (h *PollHandler) GetPoll(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid poll ID",
			"details": err.Error(),
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
			"error":   "Error fetching poll",
			"details": err.Error(),
		})
		return
	}

	// Збільшення лічильника переглядів
	go func() {
		updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer updateCancel()
		h.pollCollection.UpdateOne(
			updateCtx,
			bson.M{"_id": pollID},
			bson.M{"$inc": bson.M{"view_count": 1}},
		)
	}()

	c.JSON(http.StatusOK, poll)
}

// UpdatePoll оновлює інформацію про опрос
// @Summary Оновити опрос
// @Tags polls
// @Accept json
// @Produce json
// @Param id path string true "ID опроса"
// @Param poll body map[string]interface{} true "Оновлені дані"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 403 {object} gin.H
// @Failure 404 {object} gin.H
// @Router /api/v1/polls/{id} [put]
func (h *PollHandler) UpdatePoll(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid poll ID",
			"details": err.Error(),
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

	// Перевірка існування опроса та прав доступу
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
			"error":   "Error fetching poll",
			"details": err.Error(),
		})
		return
	}

	// ✅ Перевірка прав (тільки створювач або модератор)
	userIDObj, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "User not authenticated",
			"details": err.Error(),
		})
		return
	}

	if poll.CreatorID != userIDObj && !checkModerator(c) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Access denied",
			"details": "You don't have permission to update this poll",
		})
		return
	}

	// Видалення полів, які не повинні оновлюватися
	delete(updateReq, "_id")
	delete(updateReq, "creator_id")
	delete(updateReq, "responses")
	delete(updateReq, "created_at")
	delete(updateReq, "view_count")

	updateReq["updated_at"] = time.Now()

	result, err := h.pollCollection.UpdateOne(
		ctx,
		bson.M{"_id": pollID},
		bson.M{"$set": updateReq},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error updating poll",
			"details": err.Error(),
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

// DeletePoll видаляє опрос
// @Summary Видалити опрос
// @Tags polls
// @Accept json
// @Produce json
// @Param id path string true "ID опроса"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 403 {object} gin.H
// @Failure 404 {object} gin.H
// @Router /api/v1/polls/{id} [delete]
func (h *PollHandler) DeletePoll(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid poll ID",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Перевірка існування та прав
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
			"error":   "Error fetching poll",
			"details": err.Error(),
		})
		return
	}

	// ✅ Перевірка прав (тільки створювач або модератор)
	userIDObj, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "User not authenticated",
			"details": err.Error(),
		})
		return
	}

	if poll.CreatorID != userIDObj && !checkModerator(c) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Access denied",
			"details": "You don't have permission to delete this poll",
		})
		return
	}

	result, err := h.pollCollection.DeleteOne(ctx, bson.M{"_id": pollID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error deleting poll",
			"details": err.Error(),
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

// ========================================
// VOTING OPERATIONS
// ========================================

// VotePoll дозволяє користувачу проголосувати в опросі
// @Summary Проголосувати в опросі
// @Tags polls
// @Accept json
// @Produce json
// @Param id path string true "ID опроса"
// @Param response body SubmitPollResponseRequest true "Відповіді користувача"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 409 {object} gin.H
// @Router /api/v1/polls/{id}/respond [post]
func (h *PollHandler) VotePoll(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid poll ID",
			"details": err.Error(),
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

	userIDObj, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "User not authenticated",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Отримання опроса
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
			"error":   "Error fetching poll",
			"details": err.Error(),
		})
		return
	}

	// Перевірка статусу опроса
	if poll.Status != models.PollStatusActive {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Poll not available",
			"details": "Poll is not active",
		})
		return
	}

	// Перевірка дат
	now := time.Now()
	if now.Before(poll.StartDate) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Poll not started",
			"details": fmt.Sprintf("Poll will start at %s", poll.StartDate.Format(time.RFC3339)),
		})
		return
	}
	if now.After(poll.EndDate) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Poll ended",
			"details": fmt.Sprintf("Poll ended at %s", poll.EndDate.Format(time.RFC3339)),
		})
		return
	}

	// Перевірка, чи користувач вже голосував
	if !poll.AllowMultiple {
		for _, response := range poll.Responses {
			if response.UserID == userIDObj { // ✅ UserID НЕ вказівник
				c.JSON(http.StatusConflict, gin.H{
					"error":   "Already voted",
					"details": "You have already voted in this poll",
				})
				return
			}
		}
	}

	// Створення відповіді
	response := models.PollResponse{
		ID:          primitive.NewObjectID(),
		PollID:      pollID,
		Answers:     []models.PollAnswer{},
		CreatedAt:   now,
		UpdatedAt:   now,
		SubmittedAt: now,
	}

	// Якщо опрос не анонімний, зберігаємо ID користувача
	if !poll.IsAnonymous {
		response.UserID = userIDObj
	} else {
		response.UserID = primitive.NilObjectID // ✅ Для анонімних
	}

	// Обробка кожної відповіді
	for _, answer := range req.Answers {
		questionID, err := primitive.ObjectIDFromHex(answer.QuestionID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid question ID",
				"details": fmt.Sprintf("Question ID '%s' is not valid", answer.QuestionID),
			})
			return
		}

		// Пошук питання
		var question *models.PollQuestion
		for i := range poll.Questions {
			if poll.Questions[i].ID == questionID {
				question = &poll.Questions[i]
				break
			}
		}

		if question == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Question not found",
				"details": fmt.Sprintf("Question with ID '%s' not found in this poll", answer.QuestionID),
			})
			return
		}

		pollAnswer := models.PollAnswer{
			QuestionID: questionID,
		}

		// Валідація відповіді залежно від типу питання
		switch question.Type {
		case models.QuestionTypeSingleChoice:
			if len(answer.OptionIDs) == 0 && question.IsRequired {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Missing required answer",
					"details": fmt.Sprintf("Question '%s' is required", question.Text),
				})
				return
			}
			if len(answer.OptionIDs) > 1 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Too many options",
					"details": fmt.Sprintf("Question '%s' allows only one option", question.Text),
				})
				return
			}

			// Перевірка існування опції
			if len(answer.OptionIDs) > 0 {
				optionID, err := primitive.ObjectIDFromHex(answer.OptionIDs[0])
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "Invalid option ID",
						"details": err.Error(),
					})
					return
				}

				optionExists := false
				for _, opt := range question.Options {
					if opt.ID == optionID {
						optionExists = true
						break
					}
				}

				if !optionExists {
					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "Invalid option",
						"details": "Selected option not found in question",
					})
					return
				}

				pollAnswer.OptionIDs = []primitive.ObjectID{optionID}
			}

		case models.QuestionTypeMultipleChoice:
			if len(answer.OptionIDs) == 0 && question.IsRequired {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Missing required answer",
					"details": fmt.Sprintf("Question '%s' is required", question.Text),
				})
				return
			}

			// Перевірка всіх вибраних опцій
			var optionIDs []primitive.ObjectID
			for _, optIDStr := range answer.OptionIDs {
				optionID, err := primitive.ObjectIDFromHex(optIDStr)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "Invalid option ID",
						"details": err.Error(),
					})
					return
				}

				optionExists := false
				for _, opt := range question.Options {
					if opt.ID == optionID {
						optionExists = true
						break
					}
				}

				if !optionExists {
					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "Invalid option",
						"details": "Selected option not found in question",
					})
					return
				}

				optionIDs = append(optionIDs, optionID)
			}

			pollAnswer.OptionIDs = optionIDs

		case models.QuestionTypeText:
			if answer.TextAnswer == nil || *answer.TextAnswer == "" {
				if question.IsRequired {
					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "Missing required answer",
						"details": fmt.Sprintf("Question '%s' is required", question.Text),
					})
					return
				}
				pollAnswer.TextAnswer = ""
			} else {
				textValue := *answer.TextAnswer // ✅ Розіменувати

				if question.MaxLength > 0 && len(textValue) > question.MaxLength {
					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "Text too long",
						"details": fmt.Sprintf("Answer exceeds maximum length of %d", question.MaxLength),
					})
					return
				}

				pollAnswer.TextAnswer = textValue // ✅ Присвоїти string
			}

		case models.QuestionTypeRating:
			if answer.NumberAnswer == nil && question.IsRequired {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Missing required answer",
					"details": fmt.Sprintf("Question '%s' is required", question.Text),
				})
				return
			}
			if answer.NumberAnswer != nil {
				if *answer.NumberAnswer < question.MinRating || *answer.NumberAnswer > question.MaxRating {
					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "Invalid rating",
						"details": fmt.Sprintf("Rating must be between %d and %d", question.MinRating, question.MaxRating),
					})
					return
				}
			}
			pollAnswer.NumberAnswer = answer.NumberAnswer

		case models.QuestionTypeYesNo:
			if answer.BoolAnswer == nil && question.IsRequired {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Missing required answer",
					"details": fmt.Sprintf("Question '%s' is required", question.Text),
				})
				return
			}
			pollAnswer.BoolAnswer = answer.BoolAnswer
		}

		response.Answers = append(response.Answers, pollAnswer)
	}

	// Перевірка, що всі обов'язкові питання мають відповіді
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
					"error":   "Missing required answers",
					"details": fmt.Sprintf("Question '%s' is required but not answered", question.Text),
				})
				return
			}
		}
	}

	// Оновлення лічильників голосів для вибраних опцій
	//for _, answer := range response.Answers {
	//	for _, optionID := range answer.OptionIDs {
	//		// Пошук питання та опції
	//		for i, question := range poll.Questions {
	//			if question.ID == answer.QuestionID {
	//				for j, option := range question.Options {
	//					if option.ID == optionID {
	//						poll.Questions[i].Options[j].Votes++
	//						break
	//					}
	//				}
	//				break
	//			}
	//		}
	//	}
	//}

	// Додавання відповіді до опроса
	poll.Responses = append(poll.Responses, response)

	// Збереження оновленого опроса
	_, err = h.pollCollection.ReplaceOne(
		ctx,
		bson.M{"_id": pollID},
		poll,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error saving vote",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Vote submitted successfully",
	})
}

// GetPollResults повертає результати опросу з використанням MongoDB aggregation
// @Summary Отримати результати опросу
// @Tags polls
// @Accept json
// @Produce json
// @Param id path string true "ID опроса"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 404 {object} gin.H
// @Router /api/v1/polls/{id}/results [get]
func (h *PollHandler) GetPollResults(c *gin.Context) {
	pollID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid poll ID",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
			"error":   "Error fetching poll",
			"details": err.Error(),
		})
		return
	}

	// ✅ ВИКОРИСТАННЯ MongoDB AGGREGATION для ефективного підрахунку
	results := gin.H{
		"poll_id":         poll.ID,
		"title":           poll.Title,
		"total_responses": len(poll.Responses),
		"questions":       []gin.H{},
	}

	// Обробка кожного питання
	for _, question := range poll.Questions {
		questionResult := gin.H{
			"question_id":   question.ID,
			"text":          question.Text,
			"type":          question.Type,
			"total_answers": 0,
		}

		switch question.Type {
		case models.QuestionTypeSingleChoice, models.QuestionTypeMultipleChoice:
			// Підрахунок голосів для кожної опції
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
					"percentage": 0.0, // Буде обчислено пізніше
				})
			}

			// Обчислення відсотків
			for i := range options {
				if totalVotes > 0 {
					votes := options[i]["votes"].(int)
					percentage := (float64(votes) / float64(totalVotes)) * 100
					options[i]["percentage"] = fmt.Sprintf("%.2f", percentage)
				}
			}

			questionResult["options"] = options
			questionResult["total_answers"] = totalVotes

		case models.QuestionTypeText:
			// Збір текстових відповідей
			textAnswers := []gin.H{}
			for _, response := range poll.Responses {
				for _, answer := range response.Answers {
					if answer.QuestionID == question.ID && answer.TextAnswer != "" {
						textAnswers = append(textAnswers, gin.H{
							"text":       answer.TextAnswer,
							"created_at": response.CreatedAt,
						})
					}
				}
			}
			questionResult["text_answers"] = textAnswers
			questionResult["total_answers"] = len(textAnswers)

		case models.QuestionTypeRating:
			// Підрахунок середнього рейтингу
			var sum int
			var count int
			ratings := make(map[int]int)

			for _, response := range poll.Responses {
				for _, answer := range response.Answers {
					if answer.QuestionID == question.ID && answer.NumberAnswer != nil {
						rating := *answer.NumberAnswer
						sum += rating
						count++
						ratings[rating]++
					}
				}
			}

			var average float64
			if count > 0 {
				average = float64(sum) / float64(count)
			}

			questionResult["average_rating"] = fmt.Sprintf("%.2f", average)
			questionResult["total_answers"] = count
			questionResult["rating_distribution"] = ratings

		case models.QuestionTypeYesNo:
			// Підрахунок Так/Ні
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
			var yesPercentage, noPercentage float64
			if total > 0 {
				yesPercentage = (float64(yesCount) / float64(total)) * 100
				noPercentage = (float64(noCount) / float64(total)) * 100
			}

			questionResult["yes_count"] = yesCount
			questionResult["no_count"] = noCount
			questionResult["yes_percentage"] = fmt.Sprintf("%.2f", yesPercentage)
			questionResult["no_percentage"] = fmt.Sprintf("%.2f", noPercentage)
			questionResult["total_answers"] = total
		}

		results["questions"] = append(results["questions"].([]gin.H), questionResult)
	}

	c.JSON(http.StatusOK, results)
}

// ========================================
// BACKGROUND TASKS
// ========================================

// StartPollCleanupTask запускає фонову задачу для видалення старих опросів
func StartPollCleanupTask(pollCollection *mongo.Collection) {
	ticker := time.NewTicker(24 * time.Hour)

	// Перший запуск відразу
	go func() {
		cleanupOldPolls(pollCollection)
	}()

	// Регулярне виконання
	go func() {
		for range ticker.C {
			cleanupOldPolls(pollCollection)
		}
	}()
}

// cleanupOldPolls видаляє опроси старші 90 днів
func cleanupOldPolls(pollCollection *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Видалення опросів старших 90 днів
	cutoffDate := time.Now().AddDate(0, 0, -90)

	result, err := pollCollection.DeleteMany(ctx, bson.M{
		"end_date": bson.M{"$lt": cutoffDate},
	})

	if err != nil {
		fmt.Printf("Error cleaning up old polls: %v\n", err)
		return
	}

	if result.DeletedCount > 0 {
		fmt.Printf("Deleted %d old polls (older than 90 days)\n", result.DeletedCount)
	}
}

// GetPollStats повертає статистику опитувань для адміністратора
func (h *PollHandler) GetPollStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pollCollection := h.pollCollection

	// Загальна кількість опитувань
	totalPolls, err := pollCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error counting polls",
			"details": err.Error(),
		})
		return
	}

	// Опитування за статусом
	statusPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":   "$status",
			"count": bson.M{"$sum": 1},
		}}},
	}

	statusCursor, err := pollCollection.Aggregate(ctx, statusPipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error fetching status statistics",
			"details": err.Error(),
		})
		return
	}
	defer statusCursor.Close(ctx)

	var pollsByStatus []bson.M
	if err := statusCursor.All(ctx, &pollsByStatus); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error decoding status statistics",
			"details": err.Error(),
		})
		return
	}

	// Опитування за категоріями
	categoryPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":   "$category",
			"count": bson.M{"$sum": 1},
		}}},
	}

	categoryCursor, err := pollCollection.Aggregate(ctx, categoryPipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching category statistics",
		})
		return
	}
	defer categoryCursor.Close(ctx)

	var pollsByCategory []bson.M
	if err := categoryCursor.All(ctx, &pollsByCategory); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding category statistics",
		})
		return
	}

	// Найактивніші опитування (за кількістю відповідей)
	popularPipeline := mongo.Pipeline{
		{{Key: "$sort", Value: bson.D{{Key: "response_count", Value: -1}}}},
		{{Key: "$limit", Value: 5}},
		{{Key: "$project", Value: bson.M{
			"title":          1,
			"category":       1,
			"response_count": 1,
			"status":         1,
			"end_date":       1,
		}}},
	}

	popularCursor, err := pollCollection.Aggregate(ctx, popularPipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching popular polls",
		})
		return
	}
	defer popularCursor.Close(ctx)

	var popularPolls []bson.M
	if err := popularCursor.All(ctx, &popularPolls); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding popular polls",
		})
		return
	}

	// Загальна кількість відповідей
	responsePipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":             nil,
			"total_responses": bson.M{"$sum": "$response_count"},
		}}},
	}

	responseCursor, err := pollCollection.Aggregate(ctx, responsePipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching response statistics",
		})
		return
	}
	defer responseCursor.Close(ctx)

	var responseStats []bson.M
	responseCursor.All(ctx, &responseStats)

	totalResponses := int64(0)
	if len(responseStats) > 0 {
		if count, ok := responseStats[0]["total_responses"].(int32); ok {
			totalResponses = int64(count)
		} else if count, ok := responseStats[0]["total_responses"].(int64); ok {
			totalResponses = count
		}
	}

	// Активні опитування
	activePolls, _ := pollCollection.CountDocuments(ctx, bson.M{
		"status":   "active",
		"end_date": bson.M{"$gte": time.Now()},
	})

	// Завершені опитування
	completedPolls, _ := pollCollection.CountDocuments(ctx, bson.M{
		"status": bson.M{"$in": []string{"completed", "closed"}},
	})

	// Опитування створені за останній місяць
	oneMonthAgo := time.Now().AddDate(0, -1, 0)
	recentPolls, _ := pollCollection.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": oneMonthAgo},
	})

	// Середня кількість відповідей на опитування
	averageResponses := float64(0)
	if totalPolls > 0 {
		averageResponses = float64(totalResponses) / float64(totalPolls)
	}

	c.JSON(http.StatusOK, gin.H{
		"total_polls":       totalPolls,
		"total_responses":   totalResponses,
		"average_responses": averageResponses,
		"active_polls":      activePolls,
		"completed_polls":   completedPolls,
		"recent_polls":      recentPolls,
		"polls_by_status":   pollsByStatus,
		"polls_by_category": pollsByCategory,
		"popular_polls":     popularPolls,
		"timestamp":         time.Now(),
	})
}
