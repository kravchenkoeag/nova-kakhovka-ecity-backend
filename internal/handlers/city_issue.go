// internal/handlers/city_issue.go
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

type CityIssueHandler struct {
	issueCollection     *mongo.Collection
	userCollection      *mongo.Collection
	notificationService *services.NotificationService
}

type CreateIssueRequest struct {
	Title       string          `json:"title" validate:"required,min=5,max=200"`
	Description string          `json:"description" validate:"required,min=10,max=1000"`
	Category    string          `json:"category" validate:"required,oneof=road lighting water electricity waste transport building safety other"`
	Priority    string          `json:"priority" validate:"oneof=low medium high critical"`
	Location    models.Location `json:"location" validate:"required"`
	Address     string          `json:"address" validate:"required"`
	Photos      []string        `json:"photos"`
	Videos      []string        `json:"videos"`
}

type UpdateIssueStatusRequest struct {
	Status         string `json:"status" validate:"required,oneof=reported in_progress resolved rejected duplicate"`
	AssignedDept   string `json:"assigned_dept,omitempty"`
	Resolution     string `json:"resolution,omitempty"`
	ResolutionNote string `json:"resolution_note,omitempty"`
	DuplicateOf    string `json:"duplicate_of,omitempty"`
}

type AddCommentRequest struct {
	Content string `json:"content" validate:"required,min=1,max=500"`
}

type IssueFilters struct {
	Category   string    `form:"category"`
	Status     string    `form:"status"`
	Priority   string    `form:"priority"`
	ReporterID string    `form:"reporter_id"`
	AssignedTo string    `form:"assigned_to"`
	DateFrom   time.Time `form:"date_from"`
	DateTo     time.Time `form:"date_to"`
	IsVerified *bool     `form:"is_verified"`
	Bounds     string    `form:"bounds"` // "lat1,lng1,lat2,lng2" для карты
	Page       int       `form:"page"`
	Limit      int       `form:"limit"`
	SortBy     string    `form:"sort_by"`    // created_at, upvotes, priority
	SortOrder  string    `form:"sort_order"` // asc, desc
}

func NewCityIssueHandler(issueCollection, userCollection *mongo.Collection, notificationService *services.NotificationService) *CityIssueHandler {
	return &CityIssueHandler{
		issueCollection:     issueCollection,
		userCollection:      userCollection,
		notificationService: notificationService,
	}
}

func (h *CityIssueHandler) CreateIssue(c *gin.Context) {
	var req CreateIssueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	// Устанавливаем приоритет по умолчанию
	if req.Priority == "" {
		req.Priority = models.IssuePriorityMedium
	}

	// Валидация медиафайлов
	if len(req.Photos)+len(req.Videos) > 10 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Maximum 10 media files allowed",
		})
		return
	}

	now := time.Now()
	issue := models.CityIssue{
		ReporterID:  userIDObj,
		Title:       req.Title,
		Description: req.Description,
		Category:    req.Category,
		Priority:    req.Priority,
		Location:    req.Location,
		Address:     req.Address,
		Photos:      req.Photos,
		Videos:      req.Videos,
		Status:      models.IssueStatusReported,
		Upvotes:     []primitive.ObjectID{},
		Comments:    []models.IssueComment{},
		Subscribers: []primitive.ObjectID{userIDObj}, // Репортер автоматически подписан
		IsVerified:  false,
		IsPublic:    true,
		ViewCount:   0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.issueCollection.InsertOne(ctx, issue)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating issue",
		})
		return
	}

	issue.ID = result.InsertedID.(primitive.ObjectID)

	// Отправляем уведомление модераторам о новой проблеме
	go h.notifyModeratorsAboutNewIssue(issue)

	c.JSON(http.StatusCreated, issue)
}

func (h *CityIssueHandler) GetIssues(c *gin.Context) {
	var filters IssueFilters
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
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	if filters.SortBy == "" {
		filters.SortBy = "created_at"
	}
	if filters.SortOrder == "" {
		filters.SortOrder = "desc"
	}

	// Строим фильтр для запроса
	filter := bson.M{"is_public": true}

	if filters.Category != "" {
		filter["category"] = filters.Category
	}
	if filters.Status != "" {
		filter["status"] = filters.Status
	}
	if filters.Priority != "" {
		filter["priority"] = filters.Priority
	}
	if filters.ReporterID != "" {
		reporterID, err := primitive.ObjectIDFromHex(filters.ReporterID)
		if err == nil {
			filter["reporter_id"] = reporterID
		}
	}
	if filters.AssignedTo != "" {
		assignedTo, err := primitive.ObjectIDFromHex(filters.AssignedTo)
		if err == nil {
			filter["assigned_to"] = assignedTo
		}
	}
	if filters.IsVerified != nil {
		filter["is_verified"] = *filters.IsVerified
	}

	// Фильтр по дате
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

	// Настройки сортировки
	sortOrder := 1
	if filters.SortOrder == "desc" {
		sortOrder = -1
	}

	// Для сортировки по количеству голосов используем агрегацию
	if filters.SortBy == "upvotes" {
		h.getIssuesWithAggregation(c, filter, filters, sortOrder)
		return
	}

	// Параметры пагинации
	skip := (filters.Page - 1) * filters.Limit
	opts := options.Find().
		SetLimit(int64(filters.Limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{Key: filters.SortBy, Value: sortOrder}})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.issueCollection.Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching issues",
		})
		return
	}
	defer cursor.Close(ctx)

	var issues []models.CityIssue
	if err := cursor.All(ctx, &issues); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding issues",
		})
		return
	}

	// Получаем общее количество для пагинации
	totalCount, err := h.issueCollection.CountDocuments(ctx, filter)
	if err != nil {
		totalCount = 0
	}

	totalPages := (totalCount + int64(filters.Limit) - 1) / int64(filters.Limit)

	c.JSON(http.StatusOK, gin.H{
		"issues": issues,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

func (h *CityIssueHandler) getIssuesWithAggregation(c *gin.Context, filter bson.M, filters IssueFilters, sortOrder int) {
	skip := (filters.Page - 1) * filters.Limit

	pipeline := []bson.M{
		{"$match": filter},
		{"$addFields": bson.M{
			"upvotes_count": bson.M{"$size": "$upvotes"},
		}},
		{"$sort": bson.M{"upvotes_count": sortOrder}},
		{"$skip": skip},
		{"$limit": filters.Limit},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.issueCollection.Aggregate(ctx, pipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching issues",
		})
		return
	}
	defer cursor.Close(ctx)

	var issues []models.CityIssue
	if err := cursor.All(ctx, &issues); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding issues",
		})
		return
	}

	totalCount, err := h.issueCollection.CountDocuments(ctx, filter)
	if err != nil {
		totalCount = 0
	}

	totalPages := (totalCount + int64(filters.Limit) - 1) / int64(filters.Limit)

	c.JSON(http.StatusOK, gin.H{
		"issues": issues,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

func (h *CityIssueHandler) GetIssue(c *gin.Context) {
	issueID := c.Param("id")
	issueIDObj, err := primitive.ObjectIDFromHex(issueID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid issue ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var issue models.CityIssue
	err = h.issueCollection.FindOne(ctx, bson.M{
		"_id":       issueIDObj,
		"is_public": true,
	}).Decode(&issue)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Issue not found",
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
		h.issueCollection.UpdateOne(ctx, bson.M{"_id": issueIDObj}, bson.M{
			"$inc": bson.M{"view_count": 1},
		})
	}()

	c.JSON(http.StatusOK, issue)
}

func (h *CityIssueHandler) UpvoteIssue(c *gin.Context) {
	issueID := c.Param("id")
	issueIDObj, err := primitive.ObjectIDFromHex(issueID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid issue ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем, не голосовал ли пользователь уже
	var issue models.CityIssue
	err = h.issueCollection.FindOne(ctx, bson.M{"_id": issueIDObj}).Decode(&issue)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Issue not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Проверяем, голосовал ли уже пользователь
	alreadyUpvoted := false
	for _, upvoterID := range issue.Upvotes {
		if upvoterID == userIDObj {
			alreadyUpvoted = true
			break
		}
	}

	var update bson.M
	var message string

	if alreadyUpvoted {
		// Убираем голос
		update = bson.M{
			"$pull": bson.M{"upvotes": userIDObj},
			"$set":  bson.M{"updated_at": time.Now()},
		}
		message = "Upvote removed"
	} else {
		// Добавляем голос
		update = bson.M{
			"$push": bson.M{"upvotes": userIDObj},
			"$set":  bson.M{"updated_at": time.Now()},
		}
		message = "Upvoted successfully"
	}

	result, err := h.issueCollection.UpdateOne(ctx, bson.M{"_id": issueIDObj}, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating upvote",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Issue not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": message,
	})
}

func (h *CityIssueHandler) AddComment(c *gin.Context) {
	issueID := c.Param("id")
	issueIDObj, err := primitive.ObjectIDFromHex(issueID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid issue ID",
		})
		return
	}

	var req AddCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)
	isModerator, _ := c.Get("is_moderator")

	now := time.Now()
	comment := models.IssueComment{
		ID:         primitive.NewObjectID(),
		AuthorID:   userIDObj,
		Content:    req.Content,
		IsOfficial: isModerator.(bool),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.issueCollection.UpdateOne(ctx, bson.M{
		"_id":       issueIDObj,
		"is_public": true,
	}, bson.M{
		"$push": bson.M{"comments": comment},
		"$set":  bson.M{"updated_at": now},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error adding comment",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Issue not found",
		})
		return
	}

	// Уведомляем подписчиков о новом комментарии
	go h.notifySubscribersAboutComment(issueIDObj, userIDObj, req.Content, comment.IsOfficial)

	c.JSON(http.StatusCreated, comment)
}

func (h *CityIssueHandler) SubscribeToIssue(c *gin.Context) {
	issueID := c.Param("id")
	issueIDObj, err := primitive.ObjectIDFromHex(issueID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid issue ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем, не подписан ли уже пользователь
	count, err := h.issueCollection.CountDocuments(ctx, bson.M{
		"_id":         issueIDObj,
		"subscribers": bson.M{"$in": []primitive.ObjectID{userIDObj}},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Already subscribed to this issue",
		})
		return
	}

	// Добавляем подписку
	result, err := h.issueCollection.UpdateOne(ctx, bson.M{"_id": issueIDObj}, bson.M{
		"$push": bson.M{"subscribers": userIDObj},
		"$set":  bson.M{"updated_at": time.Now()},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error subscribing to issue",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Issue not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully subscribed to issue",
	})
}

func (h *CityIssueHandler) UnsubscribeFromIssue(c *gin.Context) {
	issueID := c.Param("id")
	issueIDObj, err := primitive.ObjectIDFromHex(issueID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid issue ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.issueCollection.UpdateOne(ctx, bson.M{"_id": issueIDObj}, bson.M{
		"$pull": bson.M{"subscribers": userIDObj},
		"$set":  bson.M{"updated_at": time.Now()},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error unsubscribing from issue",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Issue not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully unsubscribed from issue",
	})
}

// Админские функции для модераторов
func (h *CityIssueHandler) UpdateIssueStatus(c *gin.Context) {
	issueID := c.Param("id")
	issueIDObj, err := primitive.ObjectIDFromHex(issueID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid issue ID",
		})
		return
	}

	var req UpdateIssueStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
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

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updateData := bson.M{
		"status":      req.Status,
		"updated_at":  time.Now(),
		"assigned_to": userIDObj,
	}

	if req.AssignedDept != "" {
		updateData["assigned_dept"] = req.AssignedDept
	}
	if req.Resolution != "" {
		updateData["resolution"] = req.Resolution
	}
	if req.ResolutionNote != "" {
		updateData["resolution_note"] = req.ResolutionNote
	}
	if req.Status == models.IssueStatusResolved {
		updateData["resolved_at"] = time.Now()
	}
	if req.Status == models.IssueStatusDuplicate && req.DuplicateOf != "" {
		duplicateID, err := primitive.ObjectIDFromHex(req.DuplicateOf)
		if err == nil {
			updateData["duplicate_of"] = duplicateID
		}
	}

	result, err := h.issueCollection.UpdateOne(ctx, bson.M{"_id": issueIDObj}, bson.M{
		"$set": updateData,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating issue status",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Issue not found",
		})
		return
	}

	// Уведомляем подписчиков об изменении статуса
	go h.notifySubscribersAboutStatusChange(issueIDObj, req.Status, req.ResolutionNote)

	c.JSON(http.StatusOK, gin.H{
		"message": "Issue status updated successfully",
	})
}

func (h *CityIssueHandler) GetIssueStats(c *gin.Context) {
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
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$status",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	cursor, err := h.issueCollection.Aggregate(ctx, pipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error getting issue stats",
		})
		return
	}
	defer cursor.Close(ctx)

	statusStats := make(map[string]int)
	for cursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int    `bson:"count"`
		}
		if err := cursor.Decode(&result); err != nil {
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

	categoryCursor, err := h.issueCollection.Aggregate(ctx, categoryPipeline)
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

	c.JSON(http.StatusOK, gin.H{
		"status_stats":   statusStats,
		"category_stats": categoryStats,
	})
}

// Вспомогательные функции для уведомлений
func (h *CityIssueHandler) notifyModeratorsAboutNewIssue(issue models.CityIssue) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Получаем всех модераторов
	cursor, err := h.userCollection.Find(ctx, bson.M{"is_moderator": true})
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	var moderatorIDs []primitive.ObjectID
	for cursor.Next(ctx) {
		var user models.User
		if err := cursor.Decode(&user); err != nil {
			continue
		}
		moderatorIDs = append(moderatorIDs, user.ID)
	}

	if len(moderatorIDs) > 0 {
		data := map[string]interface{}{
			"issue_id": issue.ID.Hex(),
			"category": issue.Category,
			"priority": issue.Priority,
		}

		h.notificationService.SendNotificationToUsers(
			ctx,
			moderatorIDs,
			"Новая проблема в городе",
			fmt.Sprintf("Категория: %s - %s", issue.Category, issue.Title),
			services.NotificationTypeSystem,
			data,
			&issue.ID,
		)
	}
}

func (h *CityIssueHandler) notifySubscribersAboutComment(issueID, authorID primitive.ObjectID, commentText string, isOfficial bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Получаем проблему с подписчиками
	var issue models.CityIssue
	err := h.issueCollection.FindOne(ctx, bson.M{"_id": issueID}).Decode(&issue)
	if err != nil {
		return
	}

	// Исключаем автора комментария из уведомлений
	var subscribersToNotify []primitive.ObjectID
	for _, subscriberID := range issue.Subscribers {
		if subscriberID != authorID {
			subscribersToNotify = append(subscribersToNotify, subscriberID)
		}
	}

	if len(subscribersToNotify) > 0 {
		var title string
		if isOfficial {
			title = "Официальный ответ по проблеме"
		} else {
			title = "Новый комментарий к проблеме"
		}

		data := map[string]interface{}{
			"issue_id":    issueID.Hex(),
			"is_official": isOfficial,
		}

		preview := commentText
		if len(preview) > 50 {
			preview = preview[:50] + "..."
		}

		h.notificationService.SendNotificationToUsers(
			ctx,
			subscribersToNotify,
			title,
			fmt.Sprintf("%s: %s", issue.Title, preview),
			services.NotificationTypeSystem,
			data,
			&issueID,
		)
	}
}

func (h *CityIssueHandler) notifySubscribersAboutStatusChange(issueID primitive.ObjectID, newStatus, note string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Получаем проблему с подписчиками
	var issue models.CityIssue
	err := h.issueCollection.FindOne(ctx, bson.M{"_id": issueID}).Decode(&issue)
	if err != nil {
		return
	}

	if len(issue.Subscribers) > 0 {
		statusTranslations := map[string]string{
			models.IssueStatusReported:   "зарегистрирована",
			models.IssueStatusInProgress: "принята в работу",
			models.IssueStatusResolved:   "решена",
			models.IssueStatusRejected:   "отклонена",
			models.IssueStatusDuplicate:  "является дубликатом",
		}

		statusText := statusTranslations[newStatus]
		if statusText == "" {
			statusText = newStatus
		}

		body := fmt.Sprintf("Проблема '%s' %s", issue.Title, statusText)
		if note != "" {
			body += ". " + note
		}

		data := map[string]interface{}{
			"issue_id":   issueID.Hex(),
			"new_status": newStatus,
		}

		h.notificationService.SendNotificationToUsers(
			ctx,
			issue.Subscribers,
			"Изменение статуса проблемы",
			body,
			services.NotificationTypeSystem,
			data,
			&issueID,
		)
	}
}
