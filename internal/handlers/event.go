// internal/handlers/event.go
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

type EventHandler struct {
	eventCollection *mongo.Collection
	userCollection  *mongo.Collection
}

type CreateEventRequest struct {
	Title           string          `json:"title" validate:"required,min=5,max=200"`
	Description     string          `json:"description" validate:"required,min=10,max=2000"`
	StartDate       time.Time       `json:"start_date" validate:"required"`
	EndDate         *time.Time      `json:"end_date,omitempty"`
	Location        models.Location `json:"location"`
	Address         string          `json:"address"`
	IsOnline        bool            `json:"is_online"`
	MaxParticipants int             `json:"max_participants"`
	IsPublic        bool            `json:"is_public"`
}

type UpdateEventRequest struct {
	Title           string     `json:"title,omitempty" validate:"omitempty,min=5,max=200"`
	Description     string     `json:"description,omitempty" validate:"omitempty,min=10,max=2000"`
	StartDate       *time.Time `json:"start_date,omitempty"`
	EndDate         *time.Time `json:"end_date,omitempty"`
	Address         string     `json:"address,omitempty"`
	IsOnline        *bool      `json:"is_online,omitempty"`
	MaxParticipants *int       `json:"max_participants,omitempty"`
	IsPublic        *bool      `json:"is_public,omitempty"`
}

type EventFilters struct {
	StartDate time.Time `form:"start_date"`
	EndDate   time.Time `form:"end_date"`
	IsOnline  *bool     `form:"is_online"`
	IsPublic  *bool     `form:"is_public"`
	Location  string    `form:"location"`
	Page      int       `form:"page"`
	Limit     int       `form:"limit"`
	SortBy    string    `form:"sort_by"`    // start_date, created_at, participants_count
	SortOrder string    `form:"sort_order"` // asc, desc
	Organizer string    `form:"organizer"`  // filter by organizer
}

func NewEventHandler(eventCollection, userCollection *mongo.Collection) *EventHandler {
	return &EventHandler{
		eventCollection: eventCollection,
		userCollection:  userCollection,
	}
}

func (h *EventHandler) CreateEvent(c *gin.Context) {
	var req CreateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	// Проверяем, что дата начала не в прошлом
	if req.StartDate.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Event start date cannot be in the past",
		})
		return
	}

	// Проверяем, что дата окончания после даты начала
	if req.EndDate != nil && req.EndDate.Before(req.StartDate) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Event end date must be after start date",
		})
		return
	}

	now := time.Now()
	event := models.Event{
		OrganizerID:     userIDObj,
		Title:           req.Title,
		Description:     req.Description,
		StartDate:       req.StartDate,
		EndDate:         req.EndDate,
		Location:        req.Location,
		Address:         req.Address,
		IsOnline:        req.IsOnline,
		Participants:    []primitive.ObjectID{userIDObj}, // Организатор автоматически участник
		MaxParticipants: req.MaxParticipants,
		IsPublic:        req.IsPublic,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.eventCollection.InsertOne(ctx, event)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating event",
		})
		return
	}

	event.ID = result.InsertedID.(primitive.ObjectID)

	c.JSON(http.StatusCreated, event)
}

func (h *EventHandler) GetEvents(c *gin.Context) {
	var filters EventFilters
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
		filters.SortBy = "start_date"
	}
	if filters.SortOrder == "" {
		filters.SortOrder = "asc"
	}

	// Строим фильтр для запроса
	filter := bson.M{}

	if filters.IsPublic == nil || *filters.IsPublic {
		filter["is_public"] = true
	}

	if filters.IsOnline != nil {
		filter["is_online"] = *filters.IsOnline
	}

	if !filters.StartDate.IsZero() || !filters.EndDate.IsZero() {
		dateFilter := bson.M{}
		if !filters.StartDate.IsZero() {
			dateFilter["$gte"] = filters.StartDate
		}
		if !filters.EndDate.IsZero() {
			dateFilter["$lte"] = filters.EndDate
		}
		filter["start_date"] = dateFilter
	} else {
		// По умолчанию показываем только будущие события
		filter["start_date"] = bson.M{"$gte": time.Now()}
	}

	if filters.Organizer != "" {
		organizerID, err := primitive.ObjectIDFromHex(filters.Organizer)
		if err == nil {
			filter["organizer_id"] = organizerID
		}
	}

	// Настройки сортировки
	sortOrder := 1
	if filters.SortOrder == "desc" {
		sortOrder = -1
	}

	// Для сортировки по количеству участников используем агрегацию
	if filters.SortBy == "participants_count" {
		h.getEventsWithAggregation(c, filter, filters, sortOrder)
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

	cursor, err := h.eventCollection.Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching events",
		})
		return
	}
	defer cursor.Close(ctx)

	var events []models.Event
	if err := cursor.All(ctx, &events); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding events",
		})
		return
	}

	// Получаем общее количество для пагинации
	totalCount, err := h.eventCollection.CountDocuments(ctx, filter)
	if err != nil {
		totalCount = 0
	}

	totalPages := (totalCount + int64(filters.Limit) - 1) / int64(filters.Limit)

	c.JSON(http.StatusOK, gin.H{
		"events": events,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

func (h *EventHandler) getEventsWithAggregation(c *gin.Context, filter bson.M, filters EventFilters, sortOrder int) {
	skip := (filters.Page - 1) * filters.Limit

	pipeline := []bson.M{
		{"$match": filter},
		{"$addFields": bson.M{
			"participants_count": bson.M{"$size": "$participants"},
		}},
		{"$sort": bson.M{"participants_count": sortOrder}},
		{"$skip": skip},
		{"$limit": filters.Limit},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.eventCollection.Aggregate(ctx, pipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching events",
		})
		return
	}
	defer cursor.Close(ctx)

	var events []models.Event
	if err := cursor.All(ctx, &events); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding events",
		})
		return
	}

	// Получаем общее количество
	totalCount, err := h.eventCollection.CountDocuments(ctx, filter)
	if err != nil {
		totalCount = 0
	}

	totalPages := (totalCount + int64(filters.Limit) - 1) / int64(filters.Limit)

	c.JSON(http.StatusOK, gin.H{
		"events": events,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

func (h *EventHandler) GetEvent(c *gin.Context) {
	eventID := c.Param("id")
	eventIDObj, err := primitive.ObjectIDFromHex(eventID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid event ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var event models.Event
	err = h.eventCollection.FindOne(ctx, bson.M{
		"_id":       eventIDObj,
		"is_public": true,
	}).Decode(&event)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Event not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	c.JSON(http.StatusOK, event)
}

func (h *EventHandler) GetUserEvents(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	eventType := c.DefaultQuery("type", "organized") // organized, participating, all
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var filter bson.M
	switch eventType {
	case "organized":
		filter = bson.M{"organizer_id": userIDObj}
	case "participating":
		filter = bson.M{"participants": bson.M{"$in": []primitive.ObjectID{userIDObj}}}
	case "all":
		filter = bson.M{
			"$or": []bson.M{
				{"organizer_id": userIDObj},
				{"participants": bson.M{"$in": []primitive.ObjectID{userIDObj}}},
			},
		}
	default:
		filter = bson.M{"organizer_id": userIDObj}
	}

	skip := (page - 1) * limit
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{"start_date", 1}})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.eventCollection.Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching user events",
		})
		return
	}
	defer cursor.Close(ctx)

	var events []models.Event
	if err := cursor.All(ctx, &events); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding events",
		})
		return
	}

	c.JSON(http.StatusOK, events)
}

func (h *EventHandler) UpdateEvent(c *gin.Context) {
	eventID := c.Param("id")
	eventIDObj, err := primitive.ObjectIDFromHex(eventID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid event ID",
		})
		return
	}

	var req UpdateEventRequest
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

	// Проверяем, что пользователь является организатором события
	var event models.Event
	err = h.eventCollection.FindOne(ctx, bson.M{
		"_id":          eventIDObj,
		"organizer_id": userIDObj,
	}).Decode(&event)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Event not found or you don't have permission to edit it",
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
	}
	if req.Description != "" {
		updateData["description"] = req.Description
	}
	if req.StartDate != nil {
		if req.StartDate.Before(time.Now()) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Event start date cannot be in the past",
			})
			return
		}
		updateData["start_date"] = *req.StartDate
	}
	if req.EndDate != nil {
		updateData["end_date"] = *req.EndDate
	}
	if req.Address != "" {
		updateData["address"] = req.Address
	}
	if req.IsOnline != nil {
		updateData["is_online"] = *req.IsOnline
	}
	if req.MaxParticipants != nil {
		updateData["max_participants"] = *req.MaxParticipants
	}
	if req.IsPublic != nil {
		updateData["is_public"] = *req.IsPublic
	}

	result, err := h.eventCollection.UpdateOne(ctx, bson.M{"_id": eventIDObj}, bson.M{
		"$set": updateData,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating event",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Event not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Event updated successfully",
	})
}

func (h *EventHandler) DeleteEvent(c *gin.Context) {
	eventID := c.Param("id")
	eventIDObj, err := primitive.ObjectIDFromHex(eventID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid event ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Удаляем событие (только организатор может удалить)
	result, err := h.eventCollection.DeleteOne(ctx, bson.M{
		"_id":          eventIDObj,
		"organizer_id": userIDObj,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting event",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Event not found or you don't have permission to delete it",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Event deleted successfully",
	})
}

func (h *EventHandler) JoinEvent(c *gin.Context) {
	eventID := c.Param("id")
	eventIDObj, err := primitive.ObjectIDFromHex(eventID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid event ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем существование события и возможность присоединения
	var event models.Event
	err = h.eventCollection.FindOne(ctx, bson.M{
		"_id":        eventIDObj,
		"is_public":  true,
		"start_date": bson.M{"$gt": time.Now()}, // Только будущие события
	}).Decode(&event)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Event not found or already started",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Проверяем, не является ли пользователь уже участником
	for _, participantID := range event.Participants {
		if participantID == userIDObj {
			c.JSON(http.StatusConflict, gin.H{
				"error": "User is already a participant of this event",
			})
			return
		}
	}

	// Проверяем лимит участников
	if event.MaxParticipants > 0 && len(event.Participants) >= event.MaxParticipants {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Event has reached maximum number of participants",
		})
		return
	}

	// Добавляем пользователя в участники
	result, err := h.eventCollection.UpdateOne(ctx, bson.M{"_id": eventIDObj}, bson.M{
		"$push": bson.M{"participants": userIDObj},
		"$set":  bson.M{"updated_at": time.Now()},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error joining event",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Event not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully joined event",
	})
}

func (h *EventHandler) LeaveEvent(c *gin.Context) {
	eventID := c.Param("id")
	eventIDObj, err := primitive.ObjectIDFromHex(eventID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid event ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем, что пользователь не является организатором (организатор не может покинуть событие)
	var event models.Event
	err = h.eventCollection.FindOne(ctx, bson.M{"_id": eventIDObj}).Decode(&event)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Event not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	if event.OrganizerID == userIDObj {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Organizer cannot leave their own event",
		})
		return
	}

	// Убираем пользователя из участников
	result, err := h.eventCollection.UpdateOne(ctx, bson.M{"_id": eventIDObj}, bson.M{
		"$pull": bson.M{"participants": userIDObj},
		"$set":  bson.M{"updated_at": time.Now()},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error leaving event",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Event not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully left event",
	})
}

func (h *EventHandler) GetEventParticipants(c *gin.Context) {
	eventID := c.Param("id")
	eventIDObj, err := primitive.ObjectIDFromHex(eventID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid event ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем существование события
	var event models.Event
	err = h.eventCollection.FindOne(ctx, bson.M{"_id": eventIDObj}).Decode(&event)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Event not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Получаем информацию об участниках
	cursor, err := h.userCollection.Find(ctx, bson.M{
		"_id": bson.M{"$in": event.Participants},
	}, options.Find().SetProjection(bson.M{
		"password_hash": 0, // Исключаем пароль
	}))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching participants",
		})
		return
	}
	defer cursor.Close(ctx)

	var participants []models.User
	if err := cursor.All(ctx, &participants); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding participants",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"participants": participants,
		"count":        len(participants),
	})
}

// AttendEvent - користувач відмічає що відвідає подію
func (h *EventHandler) AttendEvent(c *gin.Context) {
	eventID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid event ID",
			"details": err.Error(),
		})
		return
	}

	// Отримуємо ID користувача
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Unauthorized",
		})
		return
	}

	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Перевіряємо чи подія існує
	var event models.Event
	err = h.eventCollection.FindOne(ctx, bson.M{"_id": eventID}).Decode(&event)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Event not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching event",
		})
		return
	}

	// Перевіряємо чи користувач вже відмітив участь
	for _, attendeeID := range event.Attendees {
		if attendeeID == userIDObj {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "You are already attending this event",
			})
			return
		}
	}

	// Додаємо користувача до списку учасників
	_, err = h.eventCollection.UpdateOne(
		ctx,
		bson.M{"_id": eventID},
		bson.M{
			"$push": bson.M{"attendees": userIDObj},
			"$inc":  bson.M{"attendee_count": 1},
			"$set":  bson.M{"updated_at": time.Now()},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error attending event",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully marked as attending",
	})
}

// ModerateEvent - модерація події (схвалення/відхилення)
func (h *EventHandler) ModerateEvent(c *gin.Context) {
	eventID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid event ID",
			"details": err.Error(),
		})
		return
	}

	type ModerateRequest struct {
		Action string `json:"action" binding:"required,oneof=approve reject"`
		Reason string `json:"reason,omitempty"`
	}

	var req ModerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Визначаємо новий статус
	var newStatus string
	if req.Action == "approve" {
		newStatus = "approved"
	} else {
		newStatus = "rejected"
	}

	// Оновлюємо подію
	result, err := h.eventCollection.UpdateOne(
		ctx,
		bson.M{"_id": eventID},
		bson.M{
			"$set": bson.M{
				"status":            newStatus,
				"moderation_reason": req.Reason,
				"moderated_at":      time.Now(),
				"updated_at":        time.Now(),
			},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error moderating event",
			"details": err.Error(),
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Event not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Event moderated successfully",
		"status":  newStatus,
	})
}

// GetContentStats - статистика контенту (події, оголошення, тощо)
func (h *EventHandler) GetContentStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Загальна кількість подій
	totalEvents, _ := h.eventCollection.CountDocuments(ctx, bson.M{})

	// Події за статусом
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":   "$status",
			"count": bson.M{"$sum": 1},
		}}},
	}

	cursor, err := h.eventCollection.Aggregate(ctx, pipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching event statistics",
		})
		return
	}
	defer cursor.Close(ctx)

	var eventStats []bson.M
	if err := cursor.All(ctx, &eventStats); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding statistics",
		})
		return
	}

	// Найпопулярніші події (за кількістю учасників)
	popularPipeline := mongo.Pipeline{
		{{Key: "$sort", Value: bson.D{{Key: "attendee_count", Value: -1}}}},
		{{Key: "$limit", Value: 5}},
		{{Key: "$project", Value: bson.M{
			"title":          1,
			"attendee_count": 1,
			"start_date":     1,
		}}},
	}

	popularCursor, err := h.eventCollection.Aggregate(ctx, popularPipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching popular events",
		})
		return
	}
	defer popularCursor.Close(ctx)

	var popularEvents []bson.M
	popularCursor.All(ctx, &popularEvents)

	c.JSON(http.StatusOK, gin.H{
		"total_events":     totalEvents,
		"events_by_status": eventStats,
		"popular_events":   popularEvents,
		"timestamp":        time.Now(),
	})
}
