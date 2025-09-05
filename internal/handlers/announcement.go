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
	userIDObj := userID.(primitive.ObjectID)

	// Если дата истечения не указана, устанавливаем 30 дней
	if req.ExpiresAt.IsZero() {
		req.ExpiresAt = time.Now().Add(30 * 24 * time.Hour)
	}

	now := time.Now()
	announcement := models.Announcement{
		AuthorID:    userIDObj,
		Title:       req.Title,
		Description: req.Description,
		Category:    req.Category,
		Location:    req.Location,
		Address:     req.Address,
		Employment:  req.Employment,
		ContactInfo: req.ContactInfo,
		MediaFiles:  req.MediaFiles,
		IsActive:    true,
		IsModerated: false, // Требует модерации
		IsBlocked:   false,
		Views:       0,
		Contacts:    0,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExpiresAt:   req.ExpiresAt,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

func (h *AnnouncementHandler) GetAnnouncements(c *gin.Context) {
	var filters AnnouncementFilters
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
		"is_active":    true,
		"is_moderated": true,
		"is_blocked":   false,
		"expires_at":   bson.M{"$gt": time.Now()},
	}

	if filters.Category != "" {
		filter["category"] = filters.Category
	}
	if filters.Employment != "" {
		filter["employment"] = filters.Employment
	}
	if !filters.CreatedFrom.IsZero() || !filters.CreatedTo.IsZero() {
		dateFilter := bson.M{}
		if !filters.CreatedFrom.IsZero() {
			dateFilter["$gte"] = filters.CreatedFrom
		}
		if !filters.CreatedTo.IsZero() {
			dateFilter["$lte"] = filters.CreatedTo
		}
		filter["created_at"] = dateFilter
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

	cursor, err := h.announcementCollection.Find(ctx, filter, opts)
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

	// Получаем общее количество для пагинации
	totalCount, err := h.announcementCollection.CountDocuments(ctx, filter)
	if err != nil {
		totalCount = 0
	}

	totalPages := (totalCount + int64(filters.Limit) - 1) / int64(filters.Limit)

	c.JSON(http.StatusOK, gin.H{
		"announcements": announcements,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

func (h *AnnouncementHandler) GetAnnouncement(c *gin.Context) {
	announcementID := c.Param("id")
	announcementIDObj, err := primitive.ObjectIDFromHex(announcementID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var announcement models.Announcement
	err = h.announcementCollection.FindOne(ctx, bson.M{
		"_id":          announcementIDObj,
		"is_active":    true,
		"is_moderated": true,
		"is_blocked":   false,
		"expires_at":   bson.M{"$gt": time.Now()},
	}).Decode(&announcement)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Announcement not found",
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
		h.announcementCollection.UpdateOne(ctx, bson.M{"_id": announcementIDObj}, bson.M{
			"$inc": bson.M{"views": 1},
		})
	}()

	c.JSON(http.StatusOK, announcement)
}

func (h *AnnouncementHandler) GetUserAnnouncements(c *gin.Context) {
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

	cursor, err := h.announcementCollection.Find(ctx, bson.M{
		"author_id": userIDObj,
	}, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching user announcements",
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

	c.JSON(http.StatusOK, announcements)
}

func (h *AnnouncementHandler) UpdateAnnouncement(c *gin.Context) {
	announcementID := c.Param("id")
	announcementIDObj, err := primitive.ObjectIDFromHex(announcementID)
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

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем, что пользователь является автором объявления
	var announcement models.Announcement
	err = h.announcementCollection.FindOne(ctx, bson.M{
		"_id":       announcementIDObj,
		"author_id": userIDObj,
	}).Decode(&announcement)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Announcement not found or you don't have permission to edit it",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Строим обновления
	updateData := bson.M{
		"updated_at": time.Now(),
	}

	if req.Title != "" {
		updateData["title"] = req.Title
		updateData["is_moderated"] = false // Требует повторной модерации
	}
	if req.Description != "" {
		updateData["description"] = req.Description
		updateData["is_moderated"] = false
	}
	if req.Address != "" {
		updateData["address"] = req.Address
	}
	if req.Employment != "" {
		updateData["employment"] = req.Employment
	}
	if req.ContactInfo != nil {
		updateData["contact_info"] = req.ContactInfo
	}
	if req.MediaFiles != nil {
		updateData["media_files"] = req.MediaFiles
	}
	if req.IsActive != nil {
		updateData["is_active"] = *req.IsActive
	}

	result, err := h.announcementCollection.UpdateOne(ctx, bson.M{"_id": announcementIDObj}, bson.M{
		"$set": updateData,
	})
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

func (h *AnnouncementHandler) DeleteAnnouncement(c *gin.Context) {
	announcementID := c.Param("id")
	announcementIDObj, err := primitive.ObjectIDFromHex(announcementID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Мягкое удаление - просто помечаем как неактивное
	result, err := h.announcementCollection.UpdateOne(ctx, bson.M{
		"_id":       announcementIDObj,
		"author_id": userIDObj,
	}, bson.M{
		"$set": bson.M{
			"is_active":  false,
			"updated_at": time.Now(),
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting announcement",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Announcement not found or you don't have permission to delete it",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Announcement deleted successfully",
	})
}

func (h *AnnouncementHandler) ContactOwner(c *gin.Context) {
	announcementID := c.Param("id")
	announcementIDObj, err := primitive.ObjectIDFromHex(announcementID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid announcement ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Увеличиваем счетчик контактов
	result, err := h.announcementCollection.UpdateOne(ctx, bson.M{
		"_id":        announcementIDObj,
		"is_active":  true,
		"is_blocked": false,
		"expires_at": bson.M{"$gt": time.Now()},
	}, bson.M{
		"$inc": bson.M{"contacts": 1},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating contact count",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Announcement not found or expired",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Contact recorded successfully",
	})
}
