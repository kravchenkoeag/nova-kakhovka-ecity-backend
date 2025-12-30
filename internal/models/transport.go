// internal/models/transport.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TransportRoute struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	RouteNumber   string             `bson:"route_number" json:"route_number" validate:"required"`
	RouteName     string             `bson:"route_name" json:"route_name" validate:"required,min=5,max=200"`
	TransportType string             `bson:"transport_type" json:"transport_type" validate:"required,oneof=bus trolley minibus taxi"`
	RoutePoints   []Location         `bson:"route_points" json:"route_points"`

	Color       string `bson:"color,omitempty" json:"color,omitempty"`
	Description string `bson:"description,omitempty" json:"description,omitempty"`

	// Маршрут и остановки
	Stops         []TransportStop `bson:"stops" json:"stops" validate:"required,min=2"`
	PathCoords    []Location      `bson:"path_coords" json:"path_coords"`       // Координаты пути между остановками
	TotalDistance float64         `bson:"total_distance" json:"total_distance"` // Общая длина маршрута в км

	// Расписание
	Schedule       []TransportSchedule `bson:"schedule" json:"schedule"`
	FirstDeparture time.Time           `bson:"first_departure" json:"first_departure"`
	LastDeparture  time.Time           `bson:"last_departure" json:"last_departure"`

	// Стоимость и характеристики
	Fare         float64 `bson:"fare" json:"fare"`                   // Стоимость проезда
	IsAccessible bool    `bson:"is_accessible" json:"is_accessible"` // Доступен для людей с ограниченными возможностями
	HasWiFi      bool    `bson:"has_wifi" json:"has_wifi"`
	HasAC        bool    `bson:"has_ac" json:"has_ac"` // Кондиционер

	// Статус и метаданные
	IsActive  bool               `bson:"is_active" json:"is_active"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
	CreatedBy primitive.ObjectID `bson:"created_by" json:"created_by"`
}

type TransportStop struct {
	ID           primitive.ObjectID `bson:"id" json:"id"`
	Name         string             `bson:"name" json:"name" validate:"required,min=2,max=100"`
	Location     Location           `bson:"location" json:"location" validate:"required"`
	StopOrder    int                `bson:"stop_order" json:"stop_order"` // Порядковый номер остановки на маршруте
	HasShelter   bool               `bson:"has_shelter" json:"has_shelter"`
	HasBench     bool               `bson:"has_bench" json:"has_bench"`
	IsAccessible bool               `bson:"is_accessible" json:"is_accessible"`

	// Время в пути до этой остановки от начала маршрута (в минутах)
	TravelTimeFromStart int `bson:"travel_time_from_start" json:"travel_time_from_start"`
}

type TransportSchedule struct {
	DayType       string             `bson:"day_type" json:"day_type"` // ← weekday, saturday, sunday
	StopName      string             `bson:"stop_name" json:"stop_name"`
	StopID        primitive.ObjectID `bson:"stop_id" json:"stop_id"`
	ArrivalTime   string             `bson:"arrival_time" json:"arrival_time"`     // "HH:MM"
	DepartureTime string             `bson:"departure_time" json:"departure_time"` // "HH:MM"
}

type ScheduleInterval struct {
	StartTime string `bson:"start_time" json:"start_time"` // "06:00"
	EndTime   string `bson:"end_time" json:"end_time"`     // "23:00"
	Interval  int    `bson:"interval" json:"interval"`     // Интервал в минутах между рейсами
}

type TransportVehicle struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	VehicleNumber string             `bson:"vehicle_number" json:"vehicle_number" validate:"required"`
	RouteID       primitive.ObjectID `bson:"route_id" json:"route_id" validate:"required"`

	// Характеристики транспорта
	TransportType     string `bson:"transport_type" json:"transport_type" validate:"required,oneof=bus trolley minibus taxi"`
	Model             string `bson:"model" json:"model"`
	Capacity          int    `bson:"capacity" json:"capacity"` // Вместимость пассажиров
	IsAccessible      bool   `bson:"is_accessible" json:"is_accessible"`
	HasWiFi           bool   `bson:"has_wifi" json:"has_wifi"`
	HasAC             bool   `bson:"has_ac" json:"has_ac"`
	HasAirConditioner bool   `bson:"has_air_conditioner" json:"has_air_conditioner"`

	// Текущее состояние (только для активных транспортов с GPS)
	CurrentLocation Location            `bson:"current_location,omitempty" json:"current_location,omitempty"`
	CurrentStopID   *primitive.ObjectID `bson:"current_stop_id,omitempty" json:"current_stop_id,omitempty"`
	Direction       string              `bson:"direction,omitempty" json:"direction,omitempty"` // forward, backward
	Speed           float64             `bson:"speed,omitempty" json:"speed,omitempty"`         // км/ч
	IsActive        bool                `bson:"is_active" json:"is_active"`
	LastUpdate      *time.Time          `bson:"last_update,omitempty" json:"last_update,omitempty"`

	Heading float64 `bson:"heading,omitempty" json:"heading,omitempty"`

	// Статус
	Status    string              `bson:"status" json:"status"`         // active, maintenance, out_of_service
	IsTracked bool                `bson:"is_tracked" json:"is_tracked"` // Есть ли GPS трекинг
	DriverID  *primitive.ObjectID `bson:"driver_id,omitempty" json:"driver_id,omitempty"`

	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`

	// ← ДОДАНО для зручності у response (не зберігається в DB):
	RouteNumber string `bson:"-" json:"route_number,omitempty"`
}

type TransportArrival struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	StopID    primitive.ObjectID `bson:"stop_id" json:"stop_id"`
	VehicleID primitive.ObjectID `bson:"vehicle_id" json:"vehicle_id"`
	RouteID   primitive.ObjectID `bson:"route_id" json:"route_id"`

	ScheduledTime time.Time  `bson:"scheduled_time" json:"scheduled_time"`
	EstimatedTime *time.Time `bson:"estimated_time,omitempty" json:"estimated_time,omitempty"`
	ActualTime    *time.Time `bson:"actual_time,omitempty" json:"actual_time,omitempty"`

	Delay     int    `bson:"delay" json:"delay"`   // Задержка в минутах
	Status    string `bson:"status" json:"status"` // on_time, delayed, cancelled
	Direction string `bson:"direction" json:"direction"`
}

// Типы транспорта
const (
	TransportTypeBus     = "bus"
	TransportTypeTrolley = "trolley"
	TransportTypeMinibus = "minibus"
	TransportTypeTaxi    = "taxi"
)

// Статусы транспорта
const (
	VehicleStatusActive       = "active"
	VehicleStatusMaintenance  = "maintenance"
	VehicleStatusOutOfService = "out_of_service"
)

// Статусы прибытия
const (
	ArrivalStatusOnTime    = "on_time"
	ArrivalStatusDelayed   = "delayed"
	ArrivalStatusCancelled = "cancelled"
)

// Направления движения
const (
	DirectionForward  = "forward"
	DirectionBackward = "backward"
)

// Методы для работы с транспортом

func (r *TransportRoute) GetStopByID(stopID primitive.ObjectID) *TransportStop {
	for i, stop := range r.Stops {
		if stop.ID == stopID {
			return &r.Stops[i]
		}
	}
	return nil
}

func (r *TransportRoute) GetStopByOrder(order int) *TransportStop {
	for i, stop := range r.Stops {
		if stop.StopOrder == order {
			return &r.Stops[i]
		}
	}
	return nil
}

func (r *TransportRoute) GetFirstStop() *TransportStop {
	if len(r.Stops) == 0 {
		return nil
	}
	return r.GetStopByOrder(1)
}

func (r *TransportRoute) GetLastStop() *TransportStop {
	if len(r.Stops) == 0 {
		return nil
	}
	maxOrder := 0
	for _, stop := range r.Stops {
		if stop.StopOrder > maxOrder {
			maxOrder = stop.StopOrder
		}
	}
	return r.GetStopByOrder(maxOrder)
}

func (r *TransportRoute) GetNextStop(currentStopID primitive.ObjectID) *TransportStop {
	currentStop := r.GetStopByID(currentStopID)
	if currentStop == nil {
		return nil
	}
	return r.GetStopByOrder(currentStop.StopOrder + 1)
}

func (r *TransportRoute) GetPreviousStop(currentStopID primitive.ObjectID) *TransportStop {
	currentStop := r.GetStopByID(currentStopID)
	if currentStop == nil {
		return nil
	}
	if currentStop.StopOrder <= 1 {
		return nil
	}
	return r.GetStopByOrder(currentStop.StopOrder - 1)
}

func (r *TransportRoute) GetTotalStops() int {
	return len(r.Stops)
}

func (r *TransportRoute) GetEstimatedTravelTime() int {
	if len(r.Stops) == 0 {
		return 0
	}
	lastStop := r.GetLastStop()
	if lastStop == nil {
		return 0
	}
	return lastStop.TravelTimeFromStart
}

func (v *TransportVehicle) IsActive() bool {
	return v.Status == VehicleStatusActive
}

func (v *TransportVehicle) IsOnline() bool {
	if !v.IsTracked || v.LastUpdate == nil {
		return false
	}
	// Считаем онлайн, если последнее обновление было менее 5 минут назад
	return time.Since(*v.LastUpdate) < 5*time.Minute
}

func (v *TransportVehicle) GetTimeSinceLastUpdate() time.Duration {
	if v.LastUpdate == nil {
		return 0
	}
	return time.Since(*v.LastUpdate)
}

func (v *TransportVehicle) IsAtStop(stopID primitive.ObjectID) bool {
	return v.CurrentStopID != nil && *v.CurrentStopID == stopID
}

func (a *TransportArrival) GetDelayMinutes() int {
	if a.EstimatedTime == nil {
		return 0
	}
	return int(a.EstimatedTime.Sub(a.ScheduledTime).Minutes())
}

func (a *TransportArrival) IsDelayed() bool {
	return a.Status == ArrivalStatusDelayed || a.Delay > 0
}

func (a *TransportArrival) IsCancelled() bool {
	return a.Status == ArrivalStatusCancelled
}

func (a *TransportArrival) GetActualOrEstimatedTime() time.Time {
	if a.ActualTime != nil {
		return *a.ActualTime
	}
	if a.EstimatedTime != nil {
		return *a.EstimatedTime
	}
	return a.ScheduledTime
}

func (a *TransportArrival) GetTimeUntilArrival() time.Duration {
	arrivalTime := a.GetActualOrEstimatedTime()
	duration := arrivalTime.Sub(time.Now())
	if duration < 0 {
		return 0
	}
	return duration
}

func (a *TransportArrival) HasPassed() bool {
	return a.ActualTime != nil || time.Now().After(a.GetActualOrEstimatedTime())
}

func (s *ScheduleInterval) IsTimeInInterval(t time.Time) bool {
	timeStr := t.Format("15:04")
	return timeStr >= s.StartTime && timeStr <= s.EndTime
}

func (s *TransportSchedule) GetScheduleForWeekday(weekday time.Weekday) []ScheduleInterval {
	switch weekday {
	case time.Saturday:
		return s.Saturday
	case time.Sunday:
		return s.Sunday
	default:
		return s.Weekdays
	}
}

func (s *TransportSchedule) IsOperatingNow() bool {
	now := time.Now()
	intervals := s.GetScheduleForWeekday(now.Weekday())

	for _, interval := range intervals {
		if interval.IsTimeInInterval(now) {
			return true
		}
	}
	return false
}

func (s *TransportSchedule) GetNextOperatingTime() *time.Time {
	now := time.Now()
	intervals := s.GetScheduleForWeekday(now.Weekday())

	// Ищем следующий интервал сегодня
	for _, interval := range intervals {
		if now.Format("15:04") < interval.StartTime {
			nextTime := parseTimeToday(interval.StartTime)
			return &nextTime
		}
	}

	// Если сегодня больше нет интервалов, ищем завтра
	tomorrow := now.Add(24 * time.Hour)
	tomorrowIntervals := s.GetScheduleForWeekday(tomorrow.Weekday())
	if len(tomorrowIntervals) > 0 {
		nextTime := parseTime(tomorrow, tomorrowIntervals[0].StartTime)
		return &nextTime
	}

	return nil
}

// Вспомогательные функции
func parseTimeToday(timeStr string) time.Time {
	now := time.Now()
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return now
	}
	return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
}

func parseTime(date time.Time, timeStr string) time.Time {
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return date
	}
	return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), 0, 0, date.Location())
}
