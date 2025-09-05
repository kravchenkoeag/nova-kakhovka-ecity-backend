// internal/handlers/transport.go
package handlers

import (
	"context"
	"math"
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

type TransportHandler struct {
	routeCollection   *mongo.Collection
	vehicleCollection *mongo.Collection
	userCollection    *mongo.Collection
}

type CreateRouteRequest struct {
	RouteNumber   string                    `json:"route_number" validate:"required"`
	RouteName     string                    `json:"route_name" validate:"required,min=5,max=200"`
	TransportType string                    `json:"transport_type" validate:"required,oneof=bus trolley minibus taxi"`
	Stops         []CreateTransportStop     `json:"stops" validate:"required,min=2"`
	PathCoords    []models.Location         `json:"path_coords"`
	Schedule      models.TransportSchedule  `json:"schedule"`
	FirstDeparture time.Time               `json:"first_departure"`
	LastDeparture  time.Time               `json:"last_departure"`
	Fare          float64                   `json:"fare" validate:"min=0"`
	IsAccessible  bool                      `json:"is_accessible"`
	HasWiFi       bool                      `json:"has_wifi"`
	HasAC         bool                      `json:"has_ac"`
}

type CreateTransportStop struct {
	Name        string          `json:"name" validate:"required,min=2,max=100"`
	Location    models.Location `json:"location" validate:"required"`
	StopOrder   int             `json:"stop_order"`
	HasShelter  bool            `json:"has_shelter"`
	HasBench    bool            `json:"has_bench"`
	IsAccessible bool           `json:"is_accessible"`
	TravelTimeFromStart int     `json:"travel_time_from_start"`
}

type CreateVehicleRequest struct {
	VehicleNumber string `json:"vehicle_number" validate:"required"`
	RouteID       string `json:"route_id" validate:"required"`
	TransportType string `json:"transport_type" validate:"required,oneof=bus trolley minibus taxi"`
	Model         string `json:"model"`
	Capacity      int    `json:"capacity" validate:"min=1"`
	IsAccessible  bool   `json:"is_accessible"`
	HasWiFi       bool   `json:"has_wifi"`
	HasAC         bool   `json:"has_ac"`
	IsTracked     bool   `json:"is_tracked"`
}

type UpdateVehicleLocationRequest struct {
	Location      models.Location `json:"location" validate:"required"`
	Speed         float64         `json:"speed"`
	Direction     string          `json:"direction" validate:"oneof=forward backward"`
	CurrentStopID string          `json:"current_stop_id,omitempty"`
}

type TransportFilters struct {
	TransportType string    `form:"transport_type"`
	IsAccessible  *bool     `form:"is_accessible"`
	HasWiFi       *bool     `form:"has_wifi"`
	HasAC         *bool     `form:"has_ac"`
	IsActive      *bool     `form:"is_active"`
	NearLocation  string    `form:"near_location"` // "lat,lng,radius_km"
	Page          int       `form:"page"`
	Limit         int       `form:"limit"`
	Search        string    `form:"search"`
}

func NewTransportHandler(routeCollection, vehicleCollection, userCollection *mongo.Collection) *TransportHandler {
	return &TransportHandler{
		routeCollection:   routeCollection,
		vehicleCollection: vehicleCollection,
		userCollection:    userCollection,
	}
}

func (h *TransportHandler) CreateRoute(c *gin.Context) {
	var req CreateRouteRequest
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

	// Преобразуем остановки
	var stops []models.TransportStop
	totalDistance := 0.0

	for i, reqStop := range req.Stops {
		stop := models.TransportStop{
			ID:                  primitive.NewObjectID(),
			Name:                reqStop.Name,
			Location:            reqStop.Location,
			StopOrder:           reqStop.StopOrder,
			HasShelter:          reqStop.HasShelter,
			HasBench:            reqStop.HasBench,
			IsAccessible:        reqStop.IsAccessible,
			TravelTimeFromStart: reqStop.TravelTimeFromStart,
		}

		// Если порядок не указан, присваиваем автоматически
		if stop.StopOrder == 0 {
			stop.StopOrder = i + 1
		}

		stops = append(stops, stop)

		// Вычисляем расстояние между остановками (упрощенно)
		if i > 0 {
			distance := calculateDistance(stops[i-1].Location, stop.Location)
			totalDistance += distance
		}
	}

	now := time.Now()
	route := models.TransportRoute{
		RouteNumber:    req.RouteNumber,
		RouteName:      req.RouteName,
		TransportType:  req.TransportType,
		Stops:          stops,
		PathCoords:     req.PathCoords,
		TotalDistance:  totalDistance,
		Schedule:       req.Schedule,
		FirstDeparture: req.FirstDeparture,
		LastDeparture:  req.LastDeparture,
		Fare:           req.Fare,
		IsAccessible:   req.IsAccessible,
		HasWiFi:        req.HasWiFi,
		HasAC:          req.HasAC,
		IsActive:       true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      userIDObj,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем уникальность номера маршрута
	count, err := h.routeCollection.CountDocuments(ctx, bson.M{
		"route_number":   req.RouteNumber,
		"transport_type": req.TransportType,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Route number already exists for this transport type",
		})
		return
	}

	result, err := h.routeCollection.InsertOne(ctx, route)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating route",
		})
		return
	}

	route.ID = result.InsertedID.(primitive.ObjectID)

	c.JSON(http.StatusCreated, route)
}

func (h *TransportHandler) GetRoutes(c *gin.Context) {
	var filters TransportFilters
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

	// Строим фильтр для запроса
	filter := bson.M{}

	if filters.TransportType != "" {
		filter["transport_type"] = filters.TransportType
	}
	if filters.IsAccessible != nil {
		filter["is_accessible"] = *filters.IsAccessible
	}
	if filters.HasWiFi != nil {
		filter["has_wifi"] = *filters.HasWiFi
	}
	if filters.HasAC != nil {
		filter["has_ac"] = *filters.HasAC
	}
	if filters.IsActive != nil {
		filter["is_active"] = *filters.IsActive
	}

	// Поиск по тексту
	if filters.Search != "" {
		filter["$or"] = []bson.M{
			{"route_number": bson.M{"$regex": filters.Search, "$options": "i"}},
			{"route_name": bson.M{"$regex": filters.Search, "$options": "i"}},
		}
	}

	// Параметры пагинации
	skip := (filters.Page - 1) * filters.Limit
	opts := options.Find().
		SetLimit(int64(filters.Limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{"route_number", 1}})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.routeCollection.Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching routes",
		})
		return
	}
	defer cursor.Close(ctx)

	var routes []models.TransportRoute
