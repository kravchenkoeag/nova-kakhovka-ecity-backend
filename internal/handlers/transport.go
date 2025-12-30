// internal/handlers/transport.go
package handlers

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
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
	Number      string                     `json:"number" validate:"required"`
	Type        string                     `json:"type" validate:"required,oneof=bus trolleybus tram"`
	Name        string                     `json:"name" validate:"required"`
	Description string                     `json:"description"`
	Color       string                     `json:"color"`
	Stops       []models.TransportStop     `json:"stops" validate:"required,min=2"`
	RoutePoints []models.Location          `json:"route_points" validate:"required,min=2"`
	Schedule    []models.TransportSchedule `json:"schedule"`
	IsActive    bool                       `json:"is_active"`
	Fare        float64                    `json:"fare"`
}

type CreateVehicleRequest struct {
	RouteID           string          `json:"route_id" validate:"required"`
	VehicleNumber     string          `json:"vehicle_number" validate:"required"`
	Type              string          `json:"type" validate:"required,oneof=bus trolleybus tram"`
	Model             string          `json:"model"`
	Capacity          int             `json:"capacity"`
	CurrentLocation   models.Location `json:"current_location"`
	IsActive          bool            `json:"is_active"`
	HasAirConditioner bool            `json:"has_air_conditioner"`
	HasWiFi           bool            `json:"has_wifi"`
	IsAccessible      bool            `json:"is_accessible"`
}

type UpdateVehicleLocationRequest struct {
	Location models.Location `json:"location" validate:"required"`
	Speed    float64         `json:"speed"`
	Heading  float64         `json:"heading"`
}

type RouteFilters struct {
	Type         string `form:"type"`
	IsActive     *bool  `form:"is_active"`
	NearLocation string `form:"near_location"` // "lat,lng,radius_km"
	Page         int    `form:"page"`
	Limit        int    `form:"limit"`
	Search       string `form:"search"`
}

func NewTransportHandler(routeCollection, vehicleCollection, userCollection *mongo.Collection) *TransportHandler {
	return &TransportHandler{
		routeCollection:   routeCollection,
		vehicleCollection: vehicleCollection,
		userCollection:    userCollection,
	}
}

// Вспомогательная функция для вычисления расстояния
func calculateDistance(loc1, loc2 models.Location) float64 {
	const earthRadiusKm = 6371

	lat1Rad := toRadians(loc1.Coordinates[1])
	lon1Rad := toRadians(loc1.Coordinates[0])
	lat2Rad := toRadians(loc2.Coordinates[1])
	lon2Rad := toRadians(loc2.Coordinates[0])

	deltaLat := lat2Rad - lat1Rad
	deltaLon := lon2Rad - lon1Rad

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

func toRadians(degrees float64) float64 {
	return degrees * (math.Pi / 180)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем уникальность номера маршрута
	count, err := h.routeCollection.CountDocuments(ctx, bson.M{
		"number": req.Number,
		"type":   req.Type,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Route with this number already exists",
		})
		return
	}

	now := time.Now()
	route := models.TransportRoute{
		Number:      req.Number,
		Type:        req.Type,
		Name:        req.Name,
		Description: req.Description,
		Color:       req.Color,
		Stops:       req.Stops,
		RoutePoints: req.RoutePoints,
		Schedule:    req.Schedule,
		IsActive:    req.IsActive,
		Fare:        req.Fare,
		CreatedBy:   userIDObj,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Вычисляем общую длину маршрута
	totalDistance := 0.0
	for i := 1; i < len(route.RoutePoints); i++ {
		totalDistance += calculateDistance(route.RoutePoints[i-1], route.RoutePoints[i])
	}
	route.TotalDistance = totalDistance

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

// GetRoutes возвращает список маршрутов с фильтрацией
func (h *TransportHandler) GetRoutes(c *gin.Context) {
	var filters RouteFilters
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
	query := bson.M{}

	if filters.Type != "" {
		query["type"] = filters.Type
	}
	if filters.IsActive != nil {
		query["is_active"] = *filters.IsActive
	}
	if filters.Search != "" {
		query["$or"] = []bson.M{
			{"number": bson.M{"$regex": filters.Search, "$options": "i"}},
			{"name": bson.M{"$regex": filters.Search, "$options": "i"}},
			{"description": bson.M{"$regex": filters.Search, "$options": "i"}},
		}
	}

	// Фильтрация по близости к локации
	if filters.NearLocation != "" {
		parts := strings.Split(filters.NearLocation, ",")
		if len(parts) == 3 {
			lat, _ := strconv.ParseFloat(parts[0], 64)
			lng, _ := strconv.ParseFloat(parts[1], 64)
			radiusKm, _ := strconv.ParseFloat(parts[2], 64)

			// Находим маршруты, проходящие через указанную область
			query["stops.location"] = bson.M{
				"$near": bson.M{
					"$geometry": bson.M{
						"type":        "Point",
						"coordinates": []float64{lng, lat},
					},
					"$maxDistance": radiusKm * 1000, // Конвертируем в метры
				},
			}
		}
	}

	// Настройка сортировки
	sortOptions := options.Find()
	sortOptions.SetSort(bson.D{{"number", 1}})

	// Пагинация
	skip := (filters.Page - 1) * filters.Limit
	sortOptions.SetLimit(int64(filters.Limit))
	sortOptions.SetSkip(int64(skip))

	// Выполнение запроса
	cursor, err := h.routeCollection.Find(ctx, query, sortOptions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching routes",
		})
		return
	}
	defer cursor.Close(ctx)

	var routes []models.TransportRoute
	if err := cursor.All(ctx, &routes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding routes",
		})
		return
	}

	// Подсчет общего количества
	total, _ := h.routeCollection.CountDocuments(ctx, query)

	c.JSON(http.StatusOK, gin.H{
		"routes": routes,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       total,
			"total_pages": (total + int64(filters.Limit) - 1) / int64(filters.Limit),
		},
	})
}

// GetRoute возвращает детальную информацию о маршруте
func (h *TransportHandler) GetRoute(c *gin.Context) {
	routeID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid route ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var route models.TransportRoute
	err = h.routeCollection.FindOne(ctx, bson.M{"_id": routeID}).Decode(&route)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Route not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching route",
		})
		return
	}

	// Получаем активные транспортные средства на маршруте
	cursor, err := h.vehicleCollection.Find(ctx, bson.M{
		"route_id":  routeID,
		"is_active": true,
	})
	if err == nil {
		var vehicles []models.TransportVehicle
		cursor.All(ctx, &vehicles)
		cursor.Close(ctx)

		// Добавляем информацию о транспортных средствах к ответу
		c.JSON(http.StatusOK, gin.H{
			"route":    route,
			"vehicles": vehicles,
		})
		return
	}

	c.JSON(http.StatusOK, route)
}

// UpdateRoute обновляет информацию о маршруте
func (h *TransportHandler) UpdateRoute(c *gin.Context) {
	routeID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid route ID",
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

	// Удаляем поля, которые не должны обновляться
	delete(updateReq, "_id")
	delete(updateReq, "created_by")
	delete(updateReq, "created_at")

	updateReq["updated_at"] = time.Now()

	// Если обновляются точки маршрута, пересчитываем расстояние
	if routePoints, ok := updateReq["route_points"].([]interface{}); ok && len(routePoints) > 1 {
		totalDistance := 0.0
		for i := 1; i < len(routePoints); i++ {
			// Конвертация и расчет расстояния
			// В реальном приложении нужна более сложная логика преобразования типов
		}
		updateReq["total_distance"] = totalDistance
	}

	result, err := h.routeCollection.UpdateOne(
		ctx,
		bson.M{"_id": routeID},
		bson.M{"$set": updateReq},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating route",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Route not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Route updated successfully",
	})
}

// DeleteRoute удаляет маршрут
func (h *TransportHandler) DeleteRoute(c *gin.Context) {
	routeID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid route ID",
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

	// Проверяем, есть ли активные транспортные средства на маршруте
	vehicleCount, err := h.vehicleCollection.CountDocuments(ctx, bson.M{
		"route_id":  routeID,
		"is_active": true,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	if vehicleCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Cannot delete route with active vehicles",
		})
		return
	}

	result, err := h.routeCollection.DeleteOne(ctx, bson.M{"_id": routeID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting route",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Route not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Route deleted successfully",
	})
}

// CreateVehicle добавляет новое транспортное средство
func (h *TransportHandler) CreateVehicle(c *gin.Context) {
	var req CreateVehicleRequest
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

	routeID, err := primitive.ObjectIDFromHex(req.RouteID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid route ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Проверяем существование маршрута
	count, err := h.routeCollection.CountDocuments(ctx, bson.M{"_id": routeID})
	if err != nil || count == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Route not found",
		})
		return
	}

	// Проверяем уникальность номера транспортного средства
	count, err = h.vehicleCollection.CountDocuments(ctx, bson.M{
		"vehicle_number": req.VehicleNumber,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}

	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Vehicle with this number already exists",
		})
		return
	}

	now := time.Now()
	vehicle := models.TransportVehicle{
		RouteID:           routeID,
		VehicleNumber:     req.VehicleNumber,
		Type:              req.Type,
		Model:             req.Model,
		Capacity:          req.Capacity,
		CurrentLocation:   req.CurrentLocation,
		IsActive:          req.IsActive,
		IsOnline:          false,
		LastUpdateTime:    now,
		CreatedAt:         now,
		UpdatedAt:         now,
		HasAirConditioner: req.HasAirConditioner,
		HasWiFi:           req.HasWiFi,
		IsAccessible:      req.IsAccessible,
	}

	result, err := h.vehicleCollection.InsertOne(ctx, vehicle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error creating vehicle",
		})
		return
	}

	vehicle.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, vehicle)
}

// GetVehicles возвращает список транспортных средств
func (h *TransportHandler) GetVehicles(c *gin.Context) {
	routeIDStr := c.Query("route_id")
	isActiveStr := c.Query("is_active")
	isOnlineStr := c.Query("is_online")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := bson.M{}

	if routeIDStr != "" {
		if routeID, err := primitive.ObjectIDFromHex(routeIDStr); err == nil {
			query["route_id"] = routeID
		}
	}

	if isActiveStr != "" {
		isActive, _ := strconv.ParseBool(isActiveStr)
		query["is_active"] = isActive
	}

	if isOnlineStr != "" {
		isOnline, _ := strconv.ParseBool(isOnlineStr)
		query["is_online"] = isOnline
	}

	cursor, err := h.vehicleCollection.Find(ctx, query, options.Find().SetSort(bson.D{{"vehicle_number", 1}}))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching vehicles",
		})
		return
	}
	defer cursor.Close(ctx)

	var vehicles []models.TransportVehicle
	if err := cursor.All(ctx, &vehicles); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding vehicles",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"vehicles": vehicles,
		"count":    len(vehicles),
	})
}

// UpdateVehicle обновляет информацию о транспортном средстве
func (h *TransportHandler) UpdateVehicle(c *gin.Context) {
	vehicleID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid vehicle ID",
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

	// Удаляем поля, которые не должны обновляться
	delete(updateReq, "_id")
	delete(updateReq, "created_at")

	updateReq["updated_at"] = time.Now()

	result, err := h.vehicleCollection.UpdateOne(
		ctx,
		bson.M{"_id": vehicleID},
		bson.M{"$set": updateReq},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating vehicle",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Vehicle not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Vehicle updated successfully",
	})
}

// DeleteVehicle удаляет транспортное средство
func (h *TransportHandler) DeleteVehicle(c *gin.Context) {
	vehicleID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid vehicle ID",
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

	result, err := h.vehicleCollection.DeleteOne(ctx, bson.M{"_id": vehicleID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error deleting vehicle",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Vehicle not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Vehicle deleted successfully",
	})
}

// UpdateVehicleLocation обновляет местоположение транспортного средства (для водителей)
func (h *TransportHandler) UpdateVehicleLocation(c *gin.Context) {
	vehicleID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid vehicle ID",
		})
		return
	}

	var req UpdateVehicleLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Обновляем местоположение и статус онлайн
	update := bson.M{
		"$set": bson.M{
			"current_location": req.Location,
			"speed":            req.Speed,
			"heading":          req.Heading,
			"is_online":        true,
			"last_update_time": time.Now(),
		},
	}

	result, err := h.vehicleCollection.UpdateOne(
		ctx,
		bson.M{"_id": vehicleID},
		update,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error updating vehicle location",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Vehicle not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Location updated successfully",
	})
}

// GetLiveVehicles возвращает транспортные средства в реальном времени
func (h *TransportHandler) GetLiveVehicles(c *gin.Context) {
	routeIDStr := c.Query("route_id")
	bounds := c.Query("bounds") // "lat1,lng1,lat2,lng2"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := bson.M{
		"is_active": true,
		"is_online": true,
		// Только транспорт, который обновлялся в последние 5 минут
		"last_update_time": bson.M{
			"$gte": time.Now().Add(-5 * time.Minute),
		},
	}

	if routeIDStr != "" {
		if routeID, err := primitive.ObjectIDFromHex(routeIDStr); err == nil {
			query["route_id"] = routeID
		}
	}

	// Фильтрация по границам карты
	if bounds != "" {
		var lat1, lng1, lat2, lng2 float64
		if _, err := fmt.Sscanf(bounds, "%f,%f,%f,%f", &lat1, &lng1, &lat2, &lng2); err == nil {
			query["current_location"] = bson.M{
				"$geoWithin": bson.M{
					"$box": [][]float64{
						{lng1, lat1}, // Нижний левый угол
						{lng2, lat2}, // Верхний правый угол
					},
				},
			}
		}
	}

	cursor, err := h.vehicleCollection.Find(ctx, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching live vehicles",
		})
		return
	}
	defer cursor.Close(ctx)

	var vehicles []models.TransportVehicle
	if err := cursor.All(ctx, &vehicles); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding vehicles",
		})
		return
	}

	// Дополняем информацией о маршрутах
	for i := range vehicles {
		var route models.TransportRoute
		if err := h.routeCollection.FindOne(ctx, bson.M{"_id": vehicles[i].RouteID}).Decode(&route); err == nil {
			// Добавляем информацию о маршруте для отображения на карте
			vehicles[i].RouteNumber = route.Number
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"vehicles":  vehicles,
		"count":     len(vehicles),
		"timestamp": time.Now(),
	})
}

// GetRouteSchedule возвращает расписание маршрута
func (h *TransportHandler) GetRouteSchedule(c *gin.Context) {
	routeID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid route ID",
		})
		return
	}

	stopName := c.Query("stop")                      // Опционально: расписание для конкретной остановки
	dayType := c.DefaultQuery("day_type", "weekday") // weekday, saturday, sunday

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var route models.TransportRoute
	err = h.routeCollection.FindOne(ctx, bson.M{"_id": routeID}).Decode(&route)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Route not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching route",
		})
		return
	}

	// Фильтруем расписание по типу дня и остановке
	var filteredSchedule []models.TransportSchedule
	for _, schedule := range route.Schedule {
		if schedule.DayType == dayType {
			if stopName == "" || schedule.StopName == stopName {
				filteredSchedule = append(filteredSchedule, schedule)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"route_number": route.Number,
		"route_name":   route.Name,
		"day_type":     dayType,
		"stop":         stopName,
		"schedule":     filteredSchedule,
	})
}

// GetNearestStops возвращает ближайшие остановки
func (h *TransportHandler) GetNearestStops(c *gin.Context) {
	lat := c.Query("lat")
	lng := c.Query("lng")
	radiusStr := c.DefaultQuery("radius", "500") // радиус в метрах

	if lat == "" || lng == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Latitude and longitude are required",
		})
		return
	}

	latitude, _ := strconv.ParseFloat(lat, 64)
	longitude, _ := strconv.ParseFloat(lng, 64)
	radius, _ := strconv.ParseFloat(radiusStr, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Агрегация для поиска ближайших остановок
	pipeline := []bson.M{
		{"$unwind": "$stops"},
		{
			"$geoNear": bson.M{
				"near": bson.M{
					"type":        "Point",
					"coordinates": []float64{longitude, latitude},
				},
				"distanceField": "distance",
				"maxDistance":   radius,
				"spherical":     true,
			},
		},
		{
			"$group": bson.M{
				"_id":      "$stops.name",
				"location": bson.M{"$first": "$stops.location"},
				"distance": bson.M{"$first": "$distance"},
				"routes": bson.M{
					"$push": bson.M{
						"route_id":     "$_id",
						"route_number": "$number",
						"route_type":   "$type",
						"route_name":   "$name",
					},
				},
			},
		},
		{"$sort": bson.M{"distance": 1}},
		{"$limit": 10},
	}

	cursor, err := h.routeCollection.Aggregate(ctx, pipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error finding nearest stops",
		})
		return
	}
	defer cursor.Close(ctx)

	var stops []bson.M
	if err := cursor.All(ctx, &stops); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding stops",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"stops": stops,
		"count": len(stops),
	})
}

// StartScheduleGenerator запускает фоновую задачу генерации расписания
func (h *TransportHandler) StartScheduleGenerator() {
	// В реальном приложении здесь была бы более сложная логика
	// генерации и обновления расписания транспорта
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				h.updateSchedules()
			}
		}
	}()
}

// updateSchedules обновляет расписания маршрутов
func (h *TransportHandler) updateSchedules() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Здесь может быть логика автоматического обновления расписаний
	// на основе статистики, праздников, событий и т.д.

	// Пример: обновление статуса онлайн для неактивных транспортных средств
	h.vehicleCollection.UpdateMany(
		ctx,
		bson.M{
			"is_online": true,
			"last_update_time": bson.M{
				"$lt": time.Now().Add(-10 * time.Minute),
			},
		},
		bson.M{
			"$set": bson.M{
				"is_online": false,
			},
		},
	)
}
