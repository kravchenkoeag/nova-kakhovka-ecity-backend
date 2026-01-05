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
	Bounds     string    `form:"bounds"`
	Page       int       `form:"page"`
	Limit      int       `form:"limit"`
	SortBy     string    `form:"sort_by"`
	SortOrder  string    `form:"sort_order"`
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
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	if req.Priority == "" {
		req.Priority = models.PriorityMedium
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	activeCount, err := h.issueCollection.CountDocuments(ctx, bson.M{
		"reporter_id": userIDObj,
		"status":      bson.M{"$in": []string{models.IssueStatusReported, models.IssueStatusInProgress}},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	if activeCount >= 10 {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "Too many active issues. Please wait for some to be resolved.",
		})
		return
	}

	now := time.Now()
	issue := models.CityIssue{
		ReporterID:  userIDObj,
		Title:       req.Title,
		Description: req.Description,
		Category:    req.Category,
		Status:      models.IssueStatusReported,
		Priority:    req.Priority,
		Location:    req.Location,
		Address:     req.Address,
		Photos:      req.Photos,
		Videos:      req.Videos,
		Comments:    []models.IssueComment{},
		StatusHistory: []models.IssueStatusChange{
			{
				Status:    models.IssueStatusReported,
				ChangedBy: userIDObj,
				ChangedAt: now,
				Note:      "Issue reported",
			},
		},
		UpVotes:     []primitive.ObjectID{},
		UpVoteCount: 0,
		IsVerified:  false,
		CreatedAt:   now,
		UpdatedAt:   now,
		ViewCount:   0,
		Subscribers: []primitive.ObjectID{userIDObj},
	}

	result, err := h.issueCollection.InsertOne(ctx, issue)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating issue",
		})
		return
	}

	issue.ID = result.InsertedID.(primitive.ObjectID)

	if req.Priority == models.PriorityCritical {
		h.notifyModeratorsAboutNewIssue(issue)
	}

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

	if filters.Page < 1 {
		filters.Page = 1
	}
	if filters.Limit < 1 || filters.Limit > 100 {
		filters.Limit = 20
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := bson.M{}

	if filters.Category != "" {
		query["category"] = filters.Category
	}
	if filters.Status != "" {
		query["status"] = filters.Status
	}
	if filters.Priority != "" {
		query["priority"] = filters.Priority
	}
	if filters.ReporterID != "" {
		if reporterID, err := primitive.ObjectIDFromHex(filters.ReporterID); err == nil {
			query["reporter_id"] = reporterID
		}
	}
	if filters.AssignedTo != "" {
		query["assigned_dept"] = filters.AssignedTo
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
	if filters.IsVerified != nil {
		query["is_verified"] = *filters.IsVerified
	}

	if filters.Bounds != "" {
		var lat1, lng1, lat2, lng2 float64
		if _, err := fmt.Sscanf(filters.Bounds, "%f,%f,%f,%f", &lat1, &lng1, &lat2, &lng2); err == nil {
			query["location"] = bson.M{
				"$geoWithin": bson.M{
					"$box": [][]float64{
						{lng1, lat1},
						{lng2, lat2},
					},
				},
			}
		}
	}

	sortOptions := options.Find()
	if filters.SortBy != "" {
		sortOrder := 1
		if filters.SortOrder == "desc" {
			sortOrder = -1
		}

		if filters.SortBy == "priority" {
			sortOptions.SetSort(bson.D{
				{"priority_weight", sortOrder},
				{"created_at", -1},
			})
		} else {
			sortOptions.SetSort(bson.D{{filters.SortBy, sortOrder}})
		}
	} else {
		sortOptions.SetSort(bson.D{{"created_at", -1}})
	}

	skip := (filters.Page - 1) * filters.Limit
	sortOptions.SetLimit(int64(filters.Limit))
	sortOptions.SetSkip(int64(skip))

	cursor, err := h.issueCollection.Find(ctx, query, sortOptions)
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

	total, _ := h.issueCollection.CountDocuments(ctx, query)

	c.JSON(http.StatusOK, gin.H{
		"issues": issues,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       total,
			"total_pages": (total + int64(filters.Limit) - 1) / int64(filters.Limit),
		},
	})
}

func (h *CityIssueHandler) GetIssue(c *gin.Context) {
	issueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid issue ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var issue models.CityIssue
	err = h.issueCollection.FindOne(ctx, bson.M{"_id": issueID}).Decode(&issue)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Issue not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching issue",
		})
		return
	}

	h.issueCollection.UpdateOne(
		ctx,
		bson.M{"_id": issueID},
		bson.M{"$inc": bson.M{"view_count": 1}},
	)

	c.JSON(http.StatusOK, issue)
}

// UpvoteIssue - голосування за проблему
func (h *CityIssueHandler) UpvoteIssue(c *gin.Context) {
	issueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid issue ID",
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

	var issue models.CityIssue
	err = h.issueCollection.FindOne(ctx, bson.M{"_id": issueID}).Decode(&issue)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Issue not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching issue",
		})
		return
	}

	// Перевіряємо чи вже є голос
	for _, voterID := range issue.UpVotes {
		if voterID == userIDObj {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "You have already upvoted this issue",
			})
			return
		}
	}

	// Додаємо голос
	_, err = h.issueCollection.UpdateOne(
		ctx,
		bson.M{"_id": issueID},
		bson.M{
			"$push": bson.M{"upvotes": userIDObj},
			"$inc":  bson.M{"upvote_count": 1},
			"$set":  bson.M{"updated_at": time.Now()},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error upvoting issue",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Issue upvoted successfully",
	})
}

func (h *CityIssueHandler) AddComment(c *gin.Context) {
	issueID, err := primitive.ObjectIDFromHex(c.Param("id"))
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
	userIDObj, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}
	isModerator, _ := c.Get("is_moderator")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comment := models.IssueComment{
		ID:         primitive.NewObjectID(),
		AuthorID:   userIDObj,
		Content:    req.Content,
		CreatedAt:  time.Now(),
		IsOfficial: isModerator.(bool),
	}

	result, err := h.issueCollection.UpdateOne(
		ctx,
		bson.M{"_id": issueID},
		bson.M{
			"$push": bson.M{"comments": comment},
			"$set":  bson.M{"updated_at": time.Now()},
		},
	)

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

	h.notifySubscribersAboutComment(issueID, userIDObj, req.Content, isModerator.(bool))

	c.JSON(http.StatusCreated, comment)
}

func (h *CityIssueHandler) SubscribeToIssue(c *gin.Context) {
	issueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid issue ID",
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

	count, err := h.issueCollection.CountDocuments(ctx, bson.M{
		"_id":         issueID,
		"subscribers": userIDObj,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	if count > 0 {
		result, err := h.issueCollection.UpdateOne(
			ctx,
			bson.M{"_id": issueID},
			bson.M{"$pull": bson.M{"subscribers": userIDObj}},
		)
		if err != nil || result.MatchedCount == 0 {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Error unsubscribing",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":    "Unsubscribed successfully",
			"subscribed": false,
		})
	} else {
		result, err := h.issueCollection.UpdateOne(
			ctx,
			bson.M{"_id": issueID},
			bson.M{"$addToSet": bson.M{"subscribers": userIDObj}},
		)
		if err != nil || result.MatchedCount == 0 {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Error subscribing",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":    "Subscribed successfully",
			"subscribed": true,
		})
	}
}

func (h *CityIssueHandler) GetNearbyIssues(c *gin.Context) {
	lat := c.DefaultQuery("lat", "")
	lng := c.DefaultQuery("lng", "")
	radiusStr := c.DefaultQuery("radius", "1000")

	if lat == "" || lng == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Latitude and longitude are required",
		})
		return
	}

	var latitude, longitude float64
	var radius int
	if _, err := fmt.Sscanf(lat, "%f", &latitude); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid latitude",
		})
		return
	}
	if _, err := fmt.Sscanf(lng, "%f", &longitude); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid longitude",
		})
		return
	}
	if _, err := fmt.Sscanf(radiusStr, "%d", &radius); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid radius",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.issueCollection.Find(ctx, bson.M{
		"location": bson.M{
			"$near": bson.M{
				"$geometry": bson.M{
					"type":        "Point",
					"coordinates": []float64{longitude, latitude},
				},
				"$maxDistance": radius,
			},
		},
		"status": bson.M{"$nin": []string{models.IssueStatusResolved, models.IssueStatusRejected}},
	}, options.Find().SetLimit(50))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching nearby issues",
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

	c.JSON(http.StatusOK, gin.H{
		"issues": issues,
		"count":  len(issues),
	})
}

func (h *CityIssueHandler) GetIssueStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	statusPipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$status",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	statusCursor, err := h.issueCollection.Aggregate(ctx, statusPipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error calculating status stats",
		})
		return
	}
	defer statusCursor.Close(ctx)

	var statusStats []bson.M
	statusCursor.All(ctx, &statusStats)

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
			"error": "Error calculating category stats",
		})
		return
	}
	defer categoryCursor.Close(ctx)

	var categoryStats []bson.M
	categoryCursor.All(ctx, &categoryStats)

	c.JSON(http.StatusOK, gin.H{
		"status_stats":   statusStats,
		"category_stats": categoryStats,
	})
}

// UpdateIssue - оновлення проблеми (автором)
func (h *CityIssueHandler) UpdateIssue(c *gin.Context) {
	issueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid issue ID",
			"details": err.Error(),
		})
		return
	}

	type UpdateIssueRequest struct {
		Title       string `json:"title,omitempty"`
		Description string `json:"description,omitempty"`
	}

	var req UpdateIssueRequest
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

	var issue models.CityIssue
	err = h.issueCollection.FindOne(ctx, bson.M{"_id": issueID}).Decode(&issue)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Issue not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching issue",
		})
		return
	}

	if issue.ReporterID != userIDObj {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Only the author can update this issue",
		})
		return
	}

	update := bson.M{
		"updated_at": time.Now(),
	}

	if req.Title != "" {
		update["title"] = req.Title
	}
	if req.Description != "" {
		update["description"] = req.Description
	}

	_, err = h.issueCollection.UpdateOne(
		ctx,
		bson.M{"_id": issueID},
		bson.M{"$set": update},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating issue",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Issue updated successfully",
	})
}

// UpdateIssueStatus - оновлення статусу (модератор)
func (h *CityIssueHandler) UpdateIssueStatus(c *gin.Context) {
	issueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid issue ID",
			"details": err.Error(),
		})
		return
	}

	type StatusUpdateRequest struct {
		Status string `json:"status" binding:"required,oneof=pending reported in_progress resolved rejected"`
		Note   string `json:"note,omitempty"`
	}

	var req StatusUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid status",
			"details": "Status must be pending, reported, in_progress, resolved, or rejected",
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

	update := bson.M{
		"status":      req.Status,
		"status_note": req.Note,
		"updated_at":  time.Now(),
	}

	// Додаємо запис в історію статусів
	statusChange := models.IssueStatusChange{
		Status:    req.Status,
		ChangedBy: userIDObj,
		ChangedAt: time.Now(),
		Note:      req.Note,
	}

	if req.Status == "resolved" {
		update["resolved_at"] = time.Now()
	}

	result, err := h.issueCollection.UpdateOne(
		ctx,
		bson.M{"_id": issueID},
		bson.M{
			"$set":  update,
			"$push": bson.M{"status_history": statusChange},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating status",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Issue not found",
		})
		return
	}

	// Сповіщаємо підписників
	h.notifySubscribersAboutStatusChange(issueID, req.Status, req.Note)

	c.JSON(http.StatusOK, gin.H{
		"message": "Issue status updated successfully",
		"status":  req.Status,
	})
}

// AssignIssue - призначення відповідального
func (h *CityIssueHandler) AssignIssue(c *gin.Context) {
	issueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid issue ID",
			"details": err.Error(),
		})
		return
	}

	type AssignRequest struct {
		AssignedToID string `json:"assigned_to_id" binding:"required"`
		Note         string `json:"note,omitempty"`
	}

	var req AssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	assignedToID, err := primitive.ObjectIDFromHex(req.AssignedToID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user models.User
	err = h.userCollection.FindOne(ctx, bson.M{"_id": assignedToID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching user",
		})
		return
	}

	update := bson.M{
		"assigned_to_id":  assignedToID,
		"assignment_note": req.Note,
		"assigned_at":     time.Now(),
		"updated_at":      time.Now(),
	}

	result, err := h.issueCollection.UpdateOne(
		ctx,
		bson.M{"_id": issueID},
		bson.M{"$set": update},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error assigning issue",
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
		"message":     "Issue assigned successfully",
		"assigned_to": user.GetFullName(),
	})
}

// Допоміжні функції для сповіщень
func (h *CityIssueHandler) notifyModeratorsAboutNewIssue(issue models.CityIssue) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	var issue models.CityIssue
	err := h.issueCollection.FindOne(ctx, bson.M{"_id": issueID}).Decode(&issue)
	if err != nil {
		return
	}

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
