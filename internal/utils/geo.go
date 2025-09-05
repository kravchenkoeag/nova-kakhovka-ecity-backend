package utils

import (
	"math"
	"nova-kakhovka-ecity/internal/models"
)

// Вычисляет расстояние между двумя точками в километрах используя формулу Haversine
func CalculateDistance(loc1, loc2 models.Location) float64 {
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
