// internal/handlers/transport.go
package handlers

import (
	"context"
	"fmt"
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
	RouteNumber    string                   `json:"route_number" validate:"required"`
	RouteName      string                   `json:"route_name" validate:"required,min=5,max=200"`
	TransportType  string                   `json:"transport_type" validate:"required,oneof=bus trolley minibus taxi"`
	Stops          []CreateTransportStop    `json:"stops" validate:"required,min=2"`
	PathCoords     []models.Location        `json:"path_coords"`
	Schedule       models.TransportSchedule `json:"schedule"`
	FirstDeparture time.Time                `json:"first_departure"`
	LastDeparture  time.Time                `json:"last_departure"`
	Fare           float64                  `json:"fare" validate:"min=0"`
	IsAccessible   bool                     `json:"is_accessible"`
	HasWiFi        bool                     `json:"has_wifi"`
	HasAC          bool                     `json:"has_ac"`
}

type CreateTransportStop struct {
	Name                string          `json:"name" validate:"required,min=2,max=100"`
	Location            models.Location `json:"location" validate:"required"`
	StopOrder           int             `json:"stop_order"`
	HasShelter          bool            `json:"has_shelter"`
	HasBench            bool            `json:"has_bench"`
	IsAccessible        bool            `json:"is_accessible"`
	TravelTimeFromStart int             `json:"travel_time_from_start"`
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
	TransportType string `form:"transport_type"`
	IsAccessible  *bool  `form:"is_accessible"`
	HasWiFi       *bool  `form:"has_wifi"`
	HasAC         *bool  `form:"has_ac"`
	IsActive      *bool  `form:"is_active"`
	NearLocation  string `form:"near_location"` // "lat,lng,radius_km"
	Page          int    `form:"page"`
	Limit         int    `form:"limit"`
	Search        string `form:"search"`
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
	if err := cursor.All(ctx, &routes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding routes",
		})
		return
	}

	// Получаем общее количество для пагинации
	totalCount, err := h.routeCollection.CountDocuments(ctx, filter)
	if err != nil {
		totalCount = 0
	}

	totalPages := (totalCount + int64(filters.Limit) - 1) / int64(filters.Limit)

	c.JSON(http.StatusOK, gin.H{
		"routes": routes,
		"pagination": gin.H{
			"page":        filters.Page,
			"limit":       filters.Limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

func (h *TransportHandler) GetRoute(c *gin.Context) {
	routeID := c.Param("id")
	routeIDObj, err := primitive.ObjectIDFromHex(routeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid route ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var route models.TransportRoute
	err = h.routeCollection.FindOne(ctx, bson.M{"_id": routeIDObj}).Decode(&route)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Route not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	c.JSON(http.StatusOK, route)
}

func (h *TransportHandler) UpdateRoute(c *gin.Context) {
	routeID := c.Param("id")
	routeIDObj, err := primitive.ObjectIDFromHex(routeID)
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

	var updateData bson.M
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Удаляем поля, которые нельзя обновлять
	delete(updateData, "_id")
	delete(updateData, "created_at")
	delete(updateData, "created_by")

	// Добавляем временную метку обновления
	updateData["updated_at"] = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.routeCollection.UpdateOne(ctx, bson.M{"_id": routeIDObj}, bson.M{
		"$set": updateData,
	})

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
	routeCount, err := h.routeCollection.CountDocuments(ctx, bson.M{"_id": routeID})
	if err != nil || routeCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Route not found",
		})
		return
	}

	// Проверяем уникальность номера транспортного средства
	vehicleCount, err := h.vehicleCollection.CountDocuments(ctx, bson.M{
		"vehicle_number": req.VehicleNumber,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}
	if vehicleCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Vehicle number already exists",
		})
		return
	}

	now := time.Now()
	vehicle := models.TransportVehicle{
		VehicleNumber: req.VehicleNumber,
		RouteID:       routeID,
		TransportType: req.TransportType,
		Model:         req.Model,
		Capacity:      req.Capacity,
		IsAccessible:  req.IsAccessible,
		HasWiFi:       req.HasWiFi,
		HasAC:         req.HasAC,
		IsTracked:     req.IsTracked,
		Status:        models.VehicleStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
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

func (h *TransportHandler) GetVehicles(c *gin.Context) {
	routeID := c.Query("route_id")
	status := c.Query("status")
	isTracked := c.Query("is_tracked")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	filter := bson.M{}

	if routeID != "" {
		routeIDObj, err := primitive.ObjectIDFromHex(routeID)
		if err == nil {
			filter["route_id"] = routeIDObj
		}
	}

	if status != "" {
		filter["status"] = status
	}

	if isTracked != "" {
		if tracked, err := strconv.ParseBool(isTracked); err == nil {
			filter["is_tracked"] = tracked
		}
	}

	skip := (page - 1) * limit
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{"vehicle_number", 1}})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := h.vehicleCollection.Find(ctx, filter, opts)
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

	c.JSON(http.StatusOK, vehicles)
}

func (h *TransportHandler) UpdateVehicleLocation(c *gin.Context) {
	vehicleID := c.Param("id")
	vehicleIDObj, err := primitive.ObjectIDFromHex(vehicleID)
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

	// Проверяем права модератора
	isModerator, _ := c.Get("is_moderator")
	if !isModerator.(bool) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Moderator access required",
		})
		return
	}

	updateData := bson.M{
		"current_location": req.Location,
		"speed":            req.Speed,
		"direction":        req.Direction,
		"last_update":      time.Now(),
		"updated_at":       time.Now(),
	}

	if req.CurrentStopID != "" {
		currentStopID, err := primitive.ObjectIDFromHex(req.CurrentStopID)
		if err == nil {
			updateData["current_stop_id"] = currentStopID
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.vehicleCollection.UpdateOne(ctx, bson.M{"_id": vehicleIDObj}, bson.M{
		"$set": updateData,
	})

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
		"message": "Vehicle location updated successfully",
	})
}

func (h *TransportHandler) GetNearbyStops(c *gin.Context) {
	latStr := c.Query("lat")
	lngStr := c.Query("lng")
	radiusStr := c.DefaultQuery("radius", "1") // радиус в км

	if latStr == "" || lngStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Latitude and longitude are required",
		})
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid latitude",
		})
		return
	}

	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid longitude",
		})
		return
	}

	radius, err := strconv.ParseFloat(radiusStr, 64)
	if err != nil {
		radius = 1.0
	}

	// Конвертируем радиус из км в метры для MongoDB
	radiusMeters := radius * 1000

	userLocation := models.Location{
		Type:        "Point",
		Coordinates: []float64{lng, lat},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Агрегация для поиска ближайших остановок
	pipeline := []bson.M{
		{
			"$unwind": "$stops",
		},
		{
			"$addFields": bson.M{
				"distance": bson.M{
					"$geoNear": bson.M{
						"near":          userLocation,
						"distanceField": "distance",
						"maxDistance":   radiusMeters,
						"spherical":     true,
					},
				},
			},
		},
		{
			"$sort": bson.M{"distance": 1},
		},
		{
			"$limit": 20,
		},
		{
			"$project": bson.M{
				"route_id":       "$_id",
				"route_number":   "$route_number",
				"route_name":     "$route_name",
				"transport_type": "$transport_type",
				"stop":           "$stops",
				"distance":       1,
			},
		},
	}

	cursor, err := h.routeCollection.Aggregate(ctx, pipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error finding nearby stops",
		})
		return
	}
	defer cursor.Close(ctx)

	var nearbyStops []bson.M
	if err := cursor.All(ctx, &nearbyStops); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error decoding nearby stops",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_location": userLocation,
		"radius_km":     radius,
		"nearby_stops":  nearbyStops,
	})
}

func (h *TransportHandler) GetArrivals(c *gin.Context) {
	stopID := c.Query("stop_id")
	routeID := c.Query("route_id")

	if stopID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Stop ID is required",
		})
		return
	}

	stopIDObj, err := primitive.ObjectIDFromHex(stopID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid stop ID",
		})
		return
	}

	filter := bson.M{
		"stop_id": stopIDObj,
		"scheduled_time": bson.M{
			"$gte": time.Now().Add(-30 * time.Minute), // Показываем рейсы за последние 30 минут
			"$lte": time.Now().Add(2 * time.Hour),     // и на следующие 2 часа
		},
	}

	if routeID != "" {
		routeIDObj, err := primitive.ObjectIDFromHex(routeID)
		if err == nil {
			filter["route_id"] = routeIDObj
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Поскольку у нас нет коллекции arrivals, создаем мок-данные на основе расписаний
	var route models.TransportRoute
	if routeID != "" {
		routeIDObj, _ := primitive.ObjectIDFromHex(routeID)
		h.routeCollection.FindOne(ctx, bson.M{"_id": routeIDObj}).Decode(&route)
	}

	// Генерируем примерные времена прибытия на основе расписания
	arrivals := h.generateMockArrivals(stopIDObj, routeID)

	c.JSON(http.StatusOK, gin.H{
		"stop_id":  stopID,
		"route_id": routeID,
		"arrivals": arrivals,
	})
}

func (h *TransportHandler) generateMockArrivals(stopID primitive.ObjectID, routeID string) []gin.H {
	now := time.Now()
	var arrivals []gin.H

	// Генерируем несколько примерных рейсов
	for i := 0; i < 5; i++ {
		arrivalTime := now.Add(time.Duration(10+i*15) * time.Minute)
		estimatedTime := arrivalTime.Add(time.Duration(-2+i) * time.Minute) // Небольшие задержки

		status := "on_time"
		delay := 0
		if estimatedTime.After(arrivalTime) {
			status = "delayed"
			delay = int(estimatedTime.Sub(arrivalTime).Minutes())
		}

		arrival := gin.H{
			"id":             primitive.NewObjectID(),
			"stop_id":        stopID,
			"route_number":   fmt.Sprintf("Route %d", i+1),
			"scheduled_time": arrivalTime,
			"estimated_time": estimatedTime,
			"status":         status,
			"delay":          delay,
			"direction":      "forward",
		}

		if routeID != "" {
			routeIDObj, _ := primitive.ObjectIDFromHex(routeID)
			arrival["route_id"] = routeIDObj
		}

		arrivals = append(arrivals, arrival)
	}

	return arrivals
}

func (h *TransportHandler) GetLiveTracking(c *gin.Context) {
	routeID := c.Query("route_id")

	if routeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Route ID is required",
		})
		return
	}

	routeIDObj, err := primitive.ObjectIDFromHex(routeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid route ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Получаем активные транспортные средства на маршруте
	filter := bson.M{
		"route_id":   routeIDObj,
		"status":     models.VehicleStatusActive,
		"is_tracked": true,
	}

	cursor, err := h.vehicleCollection.Find(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching live tracking data",
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

	// Фильтруем только онлайн транспорт (обновления менее 5 минут назад)
	var liveVehicles []gin.H
	cutoff := time.Now().Add(-5 * time.Minute)

	for _, vehicle := range vehicles {
		if vehicle.LastUpdate != nil && vehicle.LastUpdate.After(cutoff) {
			liveVehicle := gin.H{
				"vehicle_id":     vehicle.ID,
				"vehicle_number": vehicle.VehicleNumber,
				"location":       vehicle.CurrentLocation,
				"speed":          vehicle.Speed,
				"direction":      vehicle.Direction,
				"last_update":    vehicle.LastUpdate,
				"is_online":      true,
			}

			if vehicle.CurrentStopID != nil {
				liveVehicle["current_stop_id"] = vehicle.CurrentStopID
			}

			liveVehicles = append(liveVehicles, liveVehicle)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"route_id":      routeID,
		"live_vehicles": liveVehicles,
		"last_updated":  time.Now(),
	})
}

func (h *TransportHandler) GetTransportStats(c *gin.Context) {
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

	// Статистика маршрутов по типам транспорта
	routePipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$transport_type",
				"count": bson.M{"$sum": 1},
				"active_count": bson.M{
					"$sum": bson.M{
						"$cond": bson.A{
							"$is_active",
							1,
							0,
						},
					},
				},
			},
		},
	}

	routeCursor, err := h.routeCollection.Aggregate(ctx, routePipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error getting route stats",
		})
		return
	}
	defer routeCursor.Close(ctx)

	routeStats := make(map[string]interface{})
	for routeCursor.Next(ctx) {
		var result struct {
			ID          string `bson:"_id"`
			Count       int    `bson:"count"`
			ActiveCount int    `bson:"active_count"`
		}
		if err := routeCursor.Decode(&result); err != nil {
			continue
		}
		routeStats[result.ID] = gin.H{
			"total":  result.Count,
			"active": result.ActiveCount,
		}
	}

	// Статистика транспортных средств по статусам
	vehiclePipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$status",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	vehicleCursor, err := h.vehicleCollection.Aggregate(ctx, vehiclePipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error getting vehicle stats",
		})
		return
	}
	defer vehicleCursor.Close(ctx)

	vehicleStats := make(map[string]int)
	for vehicleCursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int    `bson:"count"`
		}
		if err := vehicleCursor.Decode(&result); err != nil {
			continue
		}
		vehicleStats[result.ID] = result.Count
	}

	c.JSON(http.StatusOK, gin.H{
		"route_stats":   routeStats,
		"vehicle_stats": vehicleStats,
		"updated_at":    time.Now(),
	})
}

// Функция для генерации расписаний (можно запускать как фоновую задачу)
func (h *TransportHandler) StartScheduleGenerator() {
	// Запускаем в горутине генерацию расписаний каждые 30 минут
	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				h.generateSchedules()
			}
		}
	}()
}

func (h *TransportHandler) generateSchedules() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Получаем все активные маршруты
	cursor, err := h.routeCollection.Find(ctx, bson.M{"is_active": true})
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	var routes []models.TransportRoute
	if err := cursor.All(ctx, &routes); err != nil {
		return
	}

	// Генерируем расписания для каждого маршрута
	for _, route := range routes {
		h.generateRouteSchedule(route)
	}
}

func (h *TransportHandler) generateRouteSchedule(route models.TransportRoute) {
	// Здесь можно реализовать логику генерации расписания
	// на основе интервалов движения, времени работы маршрута и т.д.

	// Это упрощенная заглушка для демонстрации
	now := time.Now()
	_ = now
	_ = route

	// В реальной реализации здесь будет:
	// 1. Парсинг расписания маршрута
	// 2. Генерация времен прибытия для каждой остановки
	// 3. Сохранение в коллекцию arrivals
}
