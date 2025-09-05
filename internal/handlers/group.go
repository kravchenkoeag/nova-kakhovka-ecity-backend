package handlers

import (
	"context"
	"net/http"
	"time"

	"nova-kakhovka-ecity/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type GroupHandler struct {
	groupCollection   *mongo.Collection
	userCollection    *mongo.Collection
	messageCollection *mongo.Collection
}

type CreateGroupRequest struct {
	Name           string   `json:"name" validate:"required,min=3,max=100"`
	Description    string   `json:"description" validate:"max=500"`
	Type           string   `json:"type" validate:"required,oneof=country region city interest"`
	LocationFilter string   `json:"location_filter,omitempty"`
	InterestFilter []string `json:"interest_filter,omitempty"`
	IsPublic       bool     `json:"is_public"`
	AutoJoin       bool     `json:"auto_join"`
	MaxMembers     int      `json:"max_members"`
}

type SendMessageRequest struct {
	Content   string              `json:"content" validate:"required,max=1000"`
	Type      string              `json:"type" validate:"required,oneof=text image video file link"`
	MediaURL  string              `json:"media_url,omitempty"`
	ReplyToID *primitive.ObjectID `json:"reply_to_id,omitempty"`
}

func NewGroupHandler(groupCollection, userCollection, messageCollection *mongo.Collection) *GroupHandler {
	return &GroupHandler{
		groupCollection:   groupCollection,
		userCollection:    userCollection,
		messageCollection: messageCollection,
	}
}

func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	now := time.Now()
	group := models.Group{
		Name:           req.Name,
		Description:    req.Description,
		Type:           req.Type,
		LocationFilter: req.LocationFilter,
		InterestFilter: req.InterestFilter,
		Members:        []primitive.ObjectID{userIDObj},
		Admins:         []primitive.ObjectID{userIDObj},
		Moderators:     []primitive.ObjectID{},
		IsPublic:       req.IsPublic,
		AutoJoin:       req.AutoJoin,
		MaxMembers:     req.MaxMembers,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      userIDObj,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.groupCollection.InsertOne(ctx, group)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating group",
		})
		return
	}

	group.ID = result.InsertedID.(primitive.ObjectID)

	// Добавляем группу в список групп пользователя
	_, err = h.userCollection.UpdateOne(ctx, bson.M{"_id": userIDObj}, bson.M{
		"$push": bson.M{"groups": group.ID},
		"$set":  bson.M{"updated_at": now},
	})
	if err != nil {
		// Логируем ошибку, но не отменяем создание группы
		// log.Printf("Error adding group to user: %v", err)
	}

	c.JSON(http.StatusCreated, group)
}

func (h *GroupHandler) GetUserGroups(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Находим группы, где пользователь является участником
	cursor, err := h.groupCollection.Find(ctx, bson.M{
		"members": bson.M{"$in": []primitive.ObjectID{userIDObj}},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching groups",
		})
		return
	}
	defer cursor.Close(ctx)

	var groups []models.Group
	if err := cursor.All(ctx, &groups); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding groups",
		})
		return
	}

	c.JSON(http.StatusOK, groups)
}

func (h *GroupHandler) GetPublicGroups(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Параметры пагинации
	page := 1
	limit := 20
	skip := (page - 1) * limit

	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{"created_at", -1}})

	cursor, err := h.groupCollection.Find(ctx, bson.M{"is_public": true}, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching groups",
		})
		return
	}
	defer cursor.Close(ctx)

	var groups []models.Group
	if err := cursor.All(ctx, &groups); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding groups",
		})
		return
	}

	c.JSON(http.StatusOK, groups)
}

func (h *GroupHandler) JoinGroup(c *gin.Context) {
	groupID := c.Param("id")
	groupIDObj, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid group ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем существование группы и её настройки
	var group models.Group
	err = h.groupCollection.FindOne(ctx, bson.M{"_id": groupIDObj}).Decode(&group)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Group not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	// Проверяем, не является ли пользователь уже участником
	for _, memberID := range group.Members {
		if memberID == userIDObj {
			c.JSON(http.StatusConflict, gin.H{
				"error": "User is already a member of this group",
			})
			return
		}
	}

	// Проверяем лимит участников
	if group.MaxMembers > 0 && len(group.Members) >= group.MaxMembers {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Group has reached maximum number of members",
		})
		return
	}

	now := time.Now()

	// Добавляем пользователя в группу
	_, err = h.groupCollection.UpdateOne(ctx, bson.M{"_id": groupIDObj}, bson.M{
		"$push": bson.M{"members": userIDObj},
		"$set":  bson.M{"updated_at": now},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error joining group",
		})
		return
	}

	// Добавляем группу в список групп пользователя
	_, err = h.userCollection.UpdateOne(ctx, bson.M{"_id": userIDObj}, bson.M{
		"$push": bson.M{"groups": groupIDObj},
		"$set":  bson.M{"updated_at": now},
	})
	if err != nil {
		// Логируем ошибку, но не отменяем присоединение к группе
		// log.Printf("Error adding group to user: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully joined group",
	})
}

func (h *GroupHandler) SendMessage(c *gin.Context) {
	groupID := c.Param("id")
	groupIDObj, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid group ID",
		})
		return
	}

	var req SendMessageRequest
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

	// Проверяем, является ли пользователь участником группы
	var group models.Group
	err = h.groupCollection.FindOne(ctx, bson.M{
		"_id":     groupIDObj,
		"members": bson.M{"$in": []primitive.ObjectID{userIDObj}},
	}).Decode(&group)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "User is not a member of this group",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	now := time.Now()
	message := models.Message{
		GroupID:   groupIDObj,
		UserID:    userIDObj,
		Content:   req.Content,
		Type:      req.Type,
		MediaURL:  req.MediaURL,
		ReplyToID: req.ReplyToID,
		IsEdited:  false,
		IsDeleted: false,
		CreatedAt: now,
		UpdatedAt: now,
	}

	result, err := h.messageCollection.InsertOne(ctx, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error sending message",
		})
		return
	}

	message.ID = result.InsertedID.(primitive.ObjectID)

	c.JSON(http.StatusCreated, message)
}

func (h *GroupHandler) GetMessages(c *gin.Context) {
	groupID := c.Param("id")
	groupIDObj, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid group ID",
		})
		return
	}

	userID, _ := c.Get("user_id")
	userIDObj := userID.(primitive.ObjectID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем, является ли пользователь участником группы
	count, err := h.groupCollection.CountDocuments(ctx, bson.M{
		"_id":     groupIDObj,
		"members": bson.M{"$in": []primitive.ObjectID{userIDObj}},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}
	if count == 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "User is not a member of this group",
		})
		return
	}

	// Параметры пагинации
	page := 1
	limit := 50
	skip := (page - 1) * limit

	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{"created_at", -1}})

	cursor, err := h.messageCollection.Find(ctx, bson.M{
		"group_id":   groupIDObj,
		"is_deleted": false,
	}, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching messages",
		})
		return
	}
	defer cursor.Close(ctx)

	var messages []models.Message
	if err := cursor.All(ctx, &messages); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding messages",
		})
		return
	}

	// Реверсируем массив, чтобы показать сообщения в хронологическом порядке
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	c.JSON(http.StatusOK, messages)
}
