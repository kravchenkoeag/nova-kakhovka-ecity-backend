// internal/handlers/announcement.go
package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"nova-kakhovka-ecity/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AnnouncementHandler struct {
	announcementCollection *mongo.Collection
	userCollection         *mongo.Collection
}

type CreateAnnouncementRequest struct {
	Title       string               `json:"title" validate:"required,min=5,max=200"`
	Description string               `json:"description" validate:"required,min=10,max=2000"`
	Category    string               `json:"category" validate:"required,oneof=work help services housing transport"`
	Location    models.Location      `json:"location"`
	Address     string               `json:"address"`
	Employment  string               `json:"employment" validate:"oneof=once permanent partial"`
	ContactInfo []models.ContactInfo `json:"contact_info" validate:"required,min=1"`
	MediaFiles  []string             `json:"media_files"`
	ExpiresAt   time.Time            `json:"expires_at"`
}

type UpdateAnnouncementRequest struct {
	Title       string               `json:"title,omitempty" validate:"omitempty,min=5,max=200"`
	Description string               `json:"description,omitempty" validate:"omitempty,min=10,max=2000"`
	Address     string               `json:"address,omitempty"`
	Employment  string               `json:"employment,omitempty" validate:"omitempty,oneof=once permanent partial"`
	ContactInfo []models.ContactInfo `json:"contact_info,omitempty"`
	MediaFiles  []string             `json:"media_files,omitempty"`
	IsActive    *bool                `json:"is_active,omitempty"`
}

type AnnouncementFilters struct {
	Category    string    `form:"category"`
	Employment  string    `form:"employment"`
	Location    string    `form:"location"`
	CreatedFrom time.Time `form:"created_from"`
	CreatedTo   time.Time `form:"created_to"`
	Page        int       `form:"page"`
	Limit       int       `form:"limit"`
	SortBy      string    `form:"sort_by"`    // created_at, views, title
	SortOrder   string    `form:"sort_order"` // asc, desc
}

func NewAnnouncementHandler(announcementCollection, userCollection *mongo.Collection) *AnnouncementHandler {
	return &AnnouncementHandler{
		announcementCollection: announcementCollection,
		userCollection:         userCollection,
	}
}

func (h *AnnouncementHandler) CreateAnnouncement(c *gin.Context) {
	var req CreateAnnouncementRequest
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

	// Проверяем лимит на количество активных объявлений от одного пользователя
	activeCount, err := h.announcementCollection.CountDocuments(ctx, bson.M{
		"author_id":  userIDObj,
		"is_active":  true,
		"expires_at": bson.M{"$gt": time.Now()},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	if activeCount >= 5 { // Лимит 5 активных объявлений
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "Too many active announcements. Please wait for some to expire or delete them.",
		})
		return
	}

	// Устанавливаем дату истечения по умолчанию (30 дней)
	if req.ExpiresAt.IsZero() {
		req.ExpiresAt = time.Now().AddDate(0, 0, 30)
	}

	now := time.Now()
	announcement := models.Announcement{
		AuthorID:      userIDObj,
		Title:         req.Title,
		Description:   req.Description,
		Category:      req.Category,
		Location:      req.Location,
		Address:       req.Address,
		Employment:    req.Employment,
		ContactInfo:   req.ContactInfo,
		MediaFiles:    req.MediaFiles,
		IsActive:      true,
		IsVerified:    false, // Требует модерации
		Status:        "pending",
		ViewCount:     0,
		ResponseCount: 0,
		CreatedAt:     now,
		UpdatedAt:     now,
		ExpiresAt:     req.ExpiresAt,
	}

	result, err := h.announcementCollection.InsertOne(ctx, announcement)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating announcement",
		})
		return
	}

	announcement.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, announcement)
}

// GetAnnouncements возвращает список объявлений с фильтрацией
func (h *AnnouncementHandler) GetAnnouncements(c *gin.Context) {
	var filters AnnouncementFilters
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
	query := bson.M{
		"is_active":  true,
		"expires_at": bson.M{"$gt": time.Now()},
	}

	// Показываем только верифицированные объявления обычным пользователям
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		query["is_verified"] = true
		query["status"] = "approved"
	}

	if filters.Category != "" {
		query["category"] = filters.Category
	}
	if filters.Employment != "" {
		query["employment"] = filters.Employment
	}
	if !filters.CreatedFrom.IsZero() || !filters.CreatedTo.IsZero() {
		dateQuery := bson.M{}
		if !filters.CreatedFrom.IsZero() {
			dateQuery["$gte"] = filters.CreatedFrom
		}
		if !filters.CreatedTo.IsZero() {
			dateQuery["$lte"] = filters.CreatedTo
		}
		query["created_at"] = dateQuery
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
	cursor, err := h.announcementCollection.Find(ctx, query, sortOptions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching announcements",
		})
		return
	}
	defer cursor.Close(ctx)

	var announcements []models.Announcement
	if err := cursor.All(ctx, &announcements); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding announcements",
		})
		return
	}

	// Подсчет общего количества
	total, _ := h.announcementCollection.CountDocuments(ctx, query)

	c.JSON(http.StatusOK, gin.H{
		"announcements": announcements,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       total,
			"total_pages": (total + int64(filters.Limit) - 1) / int64(filters.Limit),
		},
	})
}

// GetAnnouncement возвращает детальную информацию об объявлении
func (h *AnnouncementHandler) GetAnnouncement(c *gin.Context) {
	announcementID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var announcement models.Announcement
	err = h.announcementCollection.FindOne(ctx, bson.M{"_id": announcementID}).Decode(&announcement)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Announcement not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching announcement",
		})
		return
	}

	// Увеличиваем счетчик просмотров
	h.announcementCollection.UpdateOne(
		ctx,
		bson.M{"_id": announcementID},
		bson.M{"$inc": bson.M{"view_count": 1}},
	)

	c.JSON(http.StatusOK, announcement)
}

// UpdateAnnouncement обновляет объявление
func (h *AnnouncementHandler) UpdateAnnouncement(c *gin.Context) {
	announcementID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
		})
		return
	}

	var req UpdateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем существование и права доступа
	var announcement models.Announcement
	err = h.announcementCollection.FindOne(ctx, bson.M{"_id": announcementID}).Decode(&announcement)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Announcement not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching announcement",
		})
		return
	}

	// Проверяем права (только автор или модератор)
	userID, _ := c.Get("user_id")
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}
	isModerator, _ := c.Get("is_moderator")

	if announcement.AuthorID != userIDObj && !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You don't have permission to update this announcement",
		})
		return
	}

	// Подготавливаем обновления
	updateFields := bson.M{"updated_at": time.Now()}

	if req.Title != "" {
		updateFields["title"] = req.Title
	}
	if req.Description != "" {
		updateFields["description"] = req.Description
	}
	if req.Address != "" {
		updateFields["address"] = req.Address
	}
	if req.Employment != "" {
		updateFields["employment"] = req.Employment
	}
	if len(req.ContactInfo) > 0 {
		updateFields["contact_info"] = req.ContactInfo
	}
	if len(req.MediaFiles) > 0 {
		updateFields["media_files"] = req.MediaFiles
	}
	if req.IsActive != nil {
		updateFields["is_active"] = *req.IsActive
	}

	// Если обновляет не модератор, сбрасываем верификацию
	if !isModerator.(bool) && (req.Title != "" || req.Description != "") {
		updateFields["is_verified"] = false
		updateFields["status"] = "pending"
	}

	result, err := h.announcementCollection.UpdateOne(
		ctx,
		bson.M{"_id": announcementID},
		bson.M{"$set": updateFields},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating announcement",
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
		"message": "Announcement updated successfully",
	})
}

// DeleteAnnouncement удаляет объявление
func (h *AnnouncementHandler) DeleteAnnouncement(c *gin.Context) {
	announcementID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем существование и права
	var announcement models.Announcement
	err = h.announcementCollection.FindOne(ctx, bson.M{"_id": announcementID}).Decode(&announcement)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Announcement not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching announcement",
		})
		return
	}

	// Проверяем права
	userID, _ := c.Get("user_id")
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}
	isModerator, _ := c.Get("is_moderator")

	if announcement.AuthorID != userIDObj && !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You don't have permission to delete this announcement",
		})
		return
	}

	result, err := h.announcementCollection.DeleteOne(ctx, bson.M{"_id": announcementID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting announcement",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Announcement not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Announcement deleted successfully",
	})
}

// ApproveAnnouncement одобряет объявление (для модераторов)
func (h *AnnouncementHandler) ApproveAnnouncement(c *gin.Context) {
	announcementID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
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

	userID, _ := c.Get("user_id")
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	result, err := h.announcementCollection.UpdateOne(
		ctx,
		bson.M{"_id": announcementID},
		bson.M{
			"$set": bson.M{
				"status":      "approved",
				"is_verified": true,
				"verified_by": userIDObj,
				"verified_at": time.Now(),
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

// RejectAnnouncement отклоняет объявление (для модераторов)
func (h *AnnouncementHandler) RejectAnnouncement(c *gin.Context) {
	announcementID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
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

	var rejectionReq struct {
		Reason string `json:"reason" validate:"required,min=10,max=500"`
	}
	if err := c.ShouldBindJSON(&rejectionReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userID, _ := c.Get("user_id")
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	result, err := h.announcementCollection.UpdateOne(
		ctx,
		bson.M{"_id": announcementID},
		bson.M{
			"$set": bson.M{
				"status":           "rejected",
				"is_verified":      false,
				"is_active":        false,
				"rejected_by":      userIDObj,
				"rejected_at":      time.Now(),
				"rejection_reason": rejectionReq.Reason,
				"updated_at":       time.Now(),
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

// GetMyAnnouncements возвращает объявления пользователя
func (h *AnnouncementHandler) GetMyAnnouncements(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Получаем параметры пагинации
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	skip := (page - 1) * limit
	findOptions := options.Find().
		SetSort(bson.D{{"created_at", -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip))

	cursor, err := h.announcementCollection.Find(
		ctx,
		bson.M{"author_id": userIDObj},
		findOptions,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching announcements",
		})
		return
	}
	defer cursor.Close(ctx)

	var announcements []models.Announcement
	if err := cursor.All(ctx, &announcements); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding announcements",
		})
		return
	}

	// Подсчет общего количества
	total, _ := h.announcementCollection.CountDocuments(ctx, bson.M{"author_id": userIDObj})

	c.JSON(http.StatusOK, gin.H{
		"announcements": announcements,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetPendingAnnouncements возвращает объявления на модерации (для модераторов)
func (h *AnnouncementHandler) GetPendingAnnouncements(c *gin.Context) {
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

	cursor, err := h.announcementCollection.Find(
		ctx,
		bson.M{"status": "pending"},
		options.Find().SetSort(bson.D{{"created_at", 1}}), // Старые первыми
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching pending announcements",
		})
		return
	}
	defer cursor.Close(ctx)

	var announcements []models.Announcement
	if err := cursor.All(ctx, &announcements); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding announcements",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"announcements": announcements,
		"count":         len(announcements),
	})
}

// IncrementResponseCount увеличивает счетчик откликов на объявление
func (h *AnnouncementHandler) IncrementResponseCount(c *gin.Context) {
	announcementID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.announcementCollection.UpdateOne(
		ctx,
		bson.M{"_id": announcementID},
		bson.M{"$inc": bson.M{"response_count": 1}},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating response count",
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
		"message": "Response count updated",
	})
}
