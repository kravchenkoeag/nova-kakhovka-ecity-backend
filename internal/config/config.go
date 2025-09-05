package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server настройки
	Port string
	Host string
	Env  string

	// MongoDB настройки
	MongoURI     string
	DatabaseName string
	MongoTimeout int

	// JWT настройки
	JWTSecret     string
	JWTExpiration int

	// Firebase настройки
	FirebaseKey string

	// Google Maps API
	GoogleMapsKey string

	// SMS сервис настройки
	SMSProvider string
	SMSKey      string

	// Email настройки
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
}

func Load() *Config {
	// Загружаем переменные из .env файла
	if err := godotenv.Load(); err != nil {
		log.Printf("Не удалось загрузить .env файл: %v", err)
	}

	config := &Config{
		Port:          getEnv("PORT", "8080"),
		Host:          getEnv("HOST", "0.0.0.0"),
		Env:           getEnv("ENV", "development"),
		MongoURI:      getEnv("MONGO_URI", "mongodb://localhost:27017"),
		DatabaseName:  getEnv("DATABASE_NAME", "nova_kakhovka_ecity"),
		MongoTimeout:  getEnvAsInt("MONGO_TIMEOUT", 10),
		JWTSecret:     getEnv("JWT_SECRET", "your-secret-key"),
		JWTExpiration: getEnvAsInt("JWT_EXPIRATION", 24), // часы
		FirebaseKey:   getEnv("FIREBASE_KEY", ""),
		GoogleMapsKey: getEnv("GOOGLE_MAPS_KEY", ""),
		SMSProvider:   getEnv("SMS_PROVIDER", ""),
		SMSKey:        getEnv("SMS_KEY", ""),
		SMTPHost:      getEnv("SMTP_HOST", ""),
		SMTPPort:      getEnvAsInt("SMTP_PORT", 587),
		SMTPUsername:  getEnv("SMTP_USERNAME", ""),
		SMTPPassword:  getEnv("SMTP_PASSWORD", ""),
	}

	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
