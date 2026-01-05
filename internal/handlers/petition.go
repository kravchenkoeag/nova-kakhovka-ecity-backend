// internal/handlers/petition.go
package handlers

import (
	"context"
	"fmt"
	"net/http"
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

type PetitionHandler struct {
	petitionCollection  *mongo.Collection
	userCollection      *mongo.Collection
	notificationService *services.NotificationService
}

type CreatePetitionRequest struct {
	Title              string    `json:"title" validate:"required,min=10,max=300"`
	Description        string    `json:"description" validate:"required,min=50,max=5000"`
	Category           string    `json:"category" validate:"required,oneof=infrastructure social environment economy governance safety transport education healthcare"`
	RequiredSignatures int       `json:"required_signatures" validate:"min=100"`
	Demands            string    `json:"demands" validate:"required,min=20,max=2000"`
	EndDate            time.Time `json:"end_date" validate:"required"`
	Tags               []string  `json:"tags"`
	AttachmentURLs     []string  `json:"attachment_urls"`
}

type SignPetitionRequest struct {
	Comment   string  `json:"comment,omitempty" validate:"max=500"`
	DiiaKeyID *string `json:"diia_key_id,omitempty"`
}

type OfficialResponseRequest struct {
	Response   string   `json:"response" validate:"required,min=50,max=5000"`
	Decision   string   `json:"decision" validate:"required,oneof=accepted rejected partially_accepted"`
	ActionPlan string   `json:"action_plan,omitempty" validate:"max=3000"`
	Documents  []string `json:"documents,omitempty"`
}

type PetitionFilters struct {
	Category      string    `form:"category"`
	Status        string    `form:"status"`
	AuthorID      string    `form:"author_id"`
	DateFrom      time.Time `form:"date_from"`
	DateTo        time.Time `form:"date_to"`
	MinSignatures int       `form:"min_signatures"`
	MaxSignatures int       `form:"max_signatures"`
	Tags          []string  `form:"tags"`
	Page          int       `form:"page"`
	Limit         int       `form:"limit"`
	SortBy        string    `form:"sort_by"`    // created_at, signature_count, end_date
	SortOrder     string    `form:"sort_order"` // asc, desc
	Search        string    `form:"search"`     // Поиск по заголовку и описанию
	GoalReached   *bool     `form:"goal_reached"`
}

func NewPetitionHandler(petitionCollection, userCollection *mongo.Collection, notificationService *services.NotificationService) *PetitionHandler {
	return &PetitionHandler{
		petitionCollection:  petitionCollection,
		userCollection:      userCollection,
		notificationService: notificationService,
	}
}

func (h *PetitionHandler) CreatePetition(c *gin.Context) {
	var req CreatePetitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Проверяем, что дата окончания в будущем
	if req.EndDate.Before(time.Now().Add(24 * time.Hour)) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "End date must be at least 24 hours from now",
		})
		return
	}

	// Устанавливаем минимальное количество подписей по умолчанию
	if req.RequiredSignatures < 100 {
		req.RequiredSignatures = 100
	}

	// Проверяем лимит на количество активных петиций от одного пользователя
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	activeCount, err := h.petitionCollection.CountDocuments(ctx, bson.M{
		"author_id": userIDObj,
		"status":    bson.M{"$in": []string{models.PetitionStatusDraft, models.PetitionStatusActive}},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	if activeCount >= 3 { // Лимит 3 активные петиции
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "Too many active petitions. Please complete or delete existing petitions first.",
		})
		return
	}

	now := time.Now()
	petition := models.Petition{
		AuthorID:           userIDObj,
		Title:              req.Title,
		Description:        req.Description,
		Category:           req.Category,
		RequiredSignatures: req.RequiredSignatures,
		Demands:            req.Demands,
		Signatures:         []models.PetitionSignature{},
		SignatureCount:     0,
		Status:             models.PetitionStatusDraft, // Создается как черновик
		IsVerified:         false,
		StartDate:          now,
		EndDate:            req.EndDate,
		CreatedAt:          now,
		UpdatedAt:          now,
		Tags:               req.Tags,
		ViewCount:          0,
		ShareCount:         0,
		AttachmentURLs:     req.AttachmentURLs,
	}

	result, err := h.petitionCollection.InsertOne(ctx, petition)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating petition",
		})
		return
	}

	petition.ID = result.InsertedID.(primitive.ObjectID)

	c.JSON(http.StatusCreated, petition)
}

func (h *PetitionHandler) PublishPetition(c *gin.Context) {
	petitionID := c.Param("id")
	petitionIDObj, err := primitive.ObjectIDFromHex(petitionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid petition ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем, что пользователь является автором петиции
	var petition models.Petition
	err = h.petitionCollection.FindOne(ctx, bson.M{
		"_id":       petitionIDObj,
		"author_id": userIDObj,
		"status":    models.PetitionStatusDraft,
	}).Decode(&petition)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Petition not found or already published",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Обновляем статус на активный
	now := time.Now()
	result, err := h.petitionCollection.UpdateOne(ctx, bson.M{"_id": petitionIDObj}, bson.M{
		"$set": bson.M{
			"status":     models.PetitionStatusActive,
			"start_date": now,
			"updated_at": now,
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error publishing petition",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Petition not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Petition published successfully",
	})
}

func (h *PetitionHandler) GetPetitions(c *gin.Context) {
	var filters PetitionFilters
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
	filter := bson.M{
		"status": bson.M{"$ne": models.PetitionStatusDraft}, // Исключаем черновики
	}

	if filters.Category != "" {
		filter["category"] = filters.Category
	}
	if filters.Status != "" {
		filter["status"] = filters.Status
	}
	if filters.AuthorID != "" {
		authorID, err := primitive.ObjectIDFromHex(filters.AuthorID)
		if err == nil {
			filter["author_id"] = authorID
		}
	}
	if filters.MinSignatures > 0 {
		filter["signature_count"] = bson.M{"$gte": filters.MinSignatures}
	}
	if filters.MaxSignatures > 0 {
		if filter["signature_count"] == nil {
			filter["signature_count"] = bson.M{}
		}
		filter["signature_count"].(bson.M)["$lte"] = filters.MaxSignatures
	}
	if len(filters.Tags) > 0 {
		filter["tags"] = bson.M{"$in": filters.Tags}
	}
	if filters.GoalReached != nil {
		// Используем агрегацию для сравнения signature_count с required_signatures
		if *filters.GoalReached {
			filter["$expr"] = bson.M{"$gte": []string{"$signature_count", "$required_signatures"}}
		} else {
			filter["$expr"] = bson.M{"$lt": []string{"$signature_count", "$required_signatures"}}
		}
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

	// Поиск по тексту
	if filters.Search != "" {
		filter["$or"] = []bson.M{
			{"title": bson.M{"$regex": filters.Search, "$options": "i"}},
			{"description": bson.M{"$regex": filters.Search, "$options": "i"}},
			{"demands": bson.M{"$regex": filters.Search, "$options": "i"}},
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

	cursor, err := h.petitionCollection.Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching petitions",
		})
		return
	}
	defer cursor.Close(ctx)

	var petitions []models.Petition
	if err := cursor.All(ctx, &petitions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding petitions",
		})
		return
	}

	// Получаем общее количество для пагинации
	totalCount, err := h.petitionCollection.CountDocuments(ctx, filter)
	if err != nil {
		totalCount = 0
	}

	totalPages := (totalCount + int64(filters.Limit) - 1) / int64(filters.Limit)

	c.JSON(http.StatusOK, gin.H{
		"petitions": petitions,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

func (h *PetitionHandler) GetPetition(c *gin.Context) {
	petitionID := c.Param("id")
	petitionIDObj, err := primitive.ObjectIDFromHex(petitionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid petition ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var petition models.Petition
	err = h.petitionCollection.FindOne(ctx, bson.M{
		"_id":    petitionIDObj,
		"status": bson.M{"$ne": models.PetitionStatusDraft},
	}).Decode(&petition)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Petition not found",
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
		h.petitionCollection.UpdateOne(ctx, bson.M{"_id": petitionIDObj}, bson.M{
			"$inc": bson.M{"view_count": 1},
		})
	}()

	c.JSON(http.StatusOK, petition)
}

func (h *PetitionHandler) SignPetition(c *gin.Context) {
	petitionID := c.Param("id")
	petitionIDObj, err := primitive.ObjectIDFromHex(petitionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid petition ID",
		})
		return
	}

	var req SignPetitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Получаем информацию о пользователе
	var user models.User
	err = h.userCollection.FindOne(ctx, bson.M{"_id": userIDObj}).Decode(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error getting user info",
		})
		return
	}

	// Проверяем существование петиции и возможность подписи
	var petition models.Petition
	err = h.petitionCollection.FindOne(ctx, bson.M{
		"_id":      petitionIDObj,
		"status":   models.PetitionStatusActive,
		"end_date": bson.M{"$gt": time.Now()},
	}).Decode(&petition)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Petition not found, not active, or expired",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Проверяем, не подписывал ли уже пользователь
	for _, signature := range petition.Signatures {
		if signature.UserID == userIDObj {
			c.JSON(http.StatusConflict, gin.H{
				"error": "User has already signed this petition",
			})
			return
		}
	}

	// Создаем подпись
	now := time.Now()
	signature := models.PetitionSignature{
		UserID:     userIDObj,
		FullName:   user.FirstName + " " + user.LastName,
		DiiaKeyID:  req.DiiaKeyID,
		IsVerified: req.DiiaKeyID != nil, // Если есть ДІЯ ключ, считаем верифицированным
		SignedAt:   now,
		Comment:    req.Comment,
	}

	// Добавляем подпись
	result, err := h.petitionCollection.UpdateOne(ctx, bson.M{"_id": petitionIDObj}, bson.M{
		"$push": bson.M{"signatures": signature},
		"$inc":  bson.M{"signature_count": 1},
		"$set":  bson.M{"updated_at": now},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error signing petition",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Petition not found",
		})
		return
	}

	// Проверяем, достигнуто ли необходимое количество подписей
	newSignatureCount := petition.SignatureCount + 1
	if newSignatureCount >= petition.RequiredSignatures {
		// Обновляем статус на "completed"
		h.petitionCollection.UpdateOne(ctx, bson.M{"_id": petitionIDObj}, bson.M{
			"$set": bson.M{
				"status":       models.PetitionStatusCompleted,
				"completed_at": now,
			},
		})

		// Уведомляем автора о достижении цели
		go h.notifyAuthorAboutCompletion(petition.AuthorID, petition.Title, petitionIDObj)
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":         "Petition signed successfully",
		"signature_count": newSignatureCount,
		"completed":       newSignatureCount >= petition.RequiredSignatures,
	})
}

func (h *PetitionHandler) GetUserPetitions(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	petitionType := c.DefaultQuery("type", "authored") // authored, signed
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var filter bson.M
	switch petitionType {
	case "authored":
		filter = bson.M{"author_id": userIDObj}
	case "signed":
		filter = bson.M{"signatures.user_id": userIDObj}
	default:
		filter = bson.M{"author_id": userIDObj}
	}

	skip := (page - 1) * limit
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{"created_at", -1}})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.petitionCollection.Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching user petitions",
		})
		return
	}
	defer cursor.Close(ctx)

	var petitions []models.Petition
	if err := cursor.All(ctx, &petitions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding petitions",
		})
		return
	}

	c.JSON(http.StatusOK, petitions)
}

func (h *PetitionHandler) DeletePetition(c *gin.Context) {
	petitionID := c.Param("id")
	petitionIDObj, err := primitive.ObjectIDFromHex(petitionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid petition ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Можно удалить только свои петиции в статусе черновика
	result, err := h.petitionCollection.DeleteOne(ctx, bson.M{
		"_id":       petitionIDObj,
		"author_id": userIDObj,
		"status":    models.PetitionStatusDraft,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting petition",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Petition not found or cannot be deleted",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Petition deleted successfully",
	})
}

// Админские функции для модераторов
func (h *PetitionHandler) AddOfficialResponse(c *gin.Context) {
	petitionID := c.Param("id")
	petitionIDObj, err := primitive.ObjectIDFromHex(petitionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid petition ID",
		})
		return
	}

	var req OfficialResponseRequest
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
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Получаем информацию о модераторе
	var moderator models.User
	err = h.userCollection.FindOne(ctx, bson.M{"_id": userIDObj}).Decode(&moderator)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error getting moderator info",
		})
		return
	}

	now := time.Now()
	officialResponse := models.OfficialResponse{
		ResponderID:   userIDObj,
		ResponderName: moderator.FirstName + " " + moderator.LastName,
		Position:      "City Administration", // Можно сделать настраиваемым
		Response:      req.Response,
		Decision:      req.Decision,
		ActionPlan:    req.ActionPlan,
		RespondedAt:   now,
		Documents:     req.Documents,
	}

	// Определяем новый статус в зависимости от решения
	var newStatus string
	switch req.Decision {
	case models.PetitionDecisionAccepted:
		newStatus = models.PetitionStatusAccepted
	case models.PetitionDecisionRejected:
		newStatus = models.PetitionStatusRejected
	case models.PetitionDecisionPartiallyAccepted:
		newStatus = models.PetitionStatusAccepted
	}

	result, err := h.petitionCollection.UpdateOne(ctx, bson.M{
		"_id":    petitionIDObj,
		"status": bson.M{"$in": []string{models.PetitionStatusCompleted, models.PetitionStatusUnderReview}},
	}, bson.M{
		"$set": bson.M{
			"official_response": officialResponse,
			"status":            newStatus,
			"updated_at":        now,
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error adding official response",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Petition not found or not ready for response",
		})
		return
	}

	// Уведомляем автора петиции об официальном ответе
	go h.notifyAuthorAboutResponse(petitionIDObj, req.Decision)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Official response added successfully",
	})
}

func (h *PetitionHandler) GetPetitionStats(c *gin.Context) {
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

	statusCursor, err := h.petitionCollection.Aggregate(ctx, statusPipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error getting petition stats",
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

	categoryCursor, err := h.petitionCollection.Aggregate(ctx, categoryPipeline)
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
func (h *PetitionHandler) notifyAuthorAboutCompletion(authorID primitive.ObjectID, petitionTitle string, petitionID primitive.ObjectID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data := map[string]interface{}{
		"petition_id": petitionID.Hex(),
		"action":      "view_petition",
	}

	h.notificationService.SendNotificationToUser(
		ctx,
		authorID,
		"Петиция набрала необходимое количество подписей",
		fmt.Sprintf("Ваша петиция '%s' успешно набрала необходимое количество подписей и будет рассмотрена администрацией", petitionTitle),
		services.NotificationTypeSystem,
		data,
		&petitionID,
	)
}

func (h *PetitionHandler) notifyAuthorAboutResponse(petitionID primitive.ObjectID, decision string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Получаем петицию
	var petition models.Petition
	err := h.petitionCollection.FindOne(ctx, bson.M{"_id": petitionID}).Decode(&petition)
	if err != nil {
		return
	}

	decisionTexts := map[string]string{
		models.PetitionDecisionAccepted:          "принята к исполнению",
		models.PetitionDecisionRejected:          "отклонена",
		models.PetitionDecisionPartiallyAccepted: "частично принята",
	}

	decisionText := decisionTexts[decision]
	if decisionText == "" {
		decisionText = decision
	}

	data := map[string]interface{}{
		"petition_id": petitionID.Hex(),
		"decision":    decision,
		"action":      "view_petition",
	}

	h.notificationService.SendNotificationToUser(
		ctx,
		petition.AuthorID,
		"Официальный ответ на петицию",
		fmt.Sprintf("По вашей петиции '%s' получен официальный ответ: %s", petition.Title, decisionText),
		services.NotificationTypeSystem,
		data,
		&petitionID,
	)
}

// UpdatePetition - оновлення петиції (автором або модератором)
func (h *PetitionHandler) UpdatePetition(c *gin.Context) {
	petitionID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid petition ID",
			"details": err.Error(),
		})
		return
	}

	type UpdatePetitionRequest struct {
		Status   string `json:"status,omitempty" binding:"omitempty,oneof=open closed under_review approved rejected"`
		Response string `json:"response,omitempty"` // Офіційна відповідь
	}

	var req UpdatePetitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Формуємо оновлення
	update := bson.M{
		"updated_at": time.Now(),
	}

	if req.Status != "" {
		update["status"] = req.Status
	}

	if req.Response != "" {
		update["official_response"] = req.Response
		update["response_date"] = time.Now()
	}

	// Оновлюємо петицію
	result, err := h.petitionCollection.UpdateOne(
		ctx,
		bson.M{"_id": petitionID},
		bson.M{"$set": update},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating petition",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Petition not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Petition updated successfully",
	})
}
