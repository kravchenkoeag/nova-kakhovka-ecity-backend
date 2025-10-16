// internal/database/mongodb.go
package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"nova-kakhovka-ecity/internal/config"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type MongoDB struct {
	Client   *mongo.Client
	Database *mongo.Database
}

func NewMongoDB(cfg *config.Config) (*MongoDB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.MongoTimeout)*time.Second)
	defer cancel()

	// Настройки клиента
	clientOptions := options.Client().
		ApplyURI(cfg.MongoURI).
		SetMaxPoolSize(100).
		SetMinPoolSize(5).
		SetMaxConnIdleTime(30 * time.Second)

	// Создание клиента
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к MongoDB: %w", err)
	}

	// Проверка подключения
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("ошибка пинга MongoDB: %w", err)
	}

	database := client.Database(cfg.DatabaseName)

	log.Printf("Успешно подключен к MongoDB: %s", cfg.DatabaseName)

	return &MongoDB{
		Client:   client,
		Database: database,
	}, nil
}

func (m *MongoDB) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.Client.Disconnect(ctx); err != nil {
		return fmt.Errorf("ошибка отключения от MongoDB: %w", err)
	}

	log.Println("Отключен от MongoDB")
	return nil
}

// CreateIndexes создает индексы для всех коллекций
// ВАЖНО: Используем bson.D вместо map для сохранения порядка ключей
func (m *MongoDB) CreateIndexes(ctx context.Context) error {
	// Создание индексов для пользователей
	userCollection := m.Database.Collection("users")
	userIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "phone", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys: bson.D{{Key: "location", Value: "2dsphere"}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	if _, err := userCollection.Indexes().CreateMany(ctx, userIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для пользователей: %w", err)
	}

	// Создание индексов для объявлений
	announcementCollection := m.Database.Collection("announcements")
	announcementIndexes := []mongo.IndexModel{
		{
			// Составной индекс для фильтрации по категории
			Keys: bson.D{
				{Key: "category", Value: 1},
				{Key: "is_active", Value: 1},
			},
		},
		{
			// Индекс для сортировки по дате
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			// Геопространственный индекс для поиска по локации
			Keys: bson.D{{Key: "location", Value: "2dsphere"}},
		},
		{
			// Индекс для поиска объявлений пользователя
			Keys: bson.D{{Key: "author_id", Value: 1}},
		},
		{
			// Индекс для срока действия
			Keys: bson.D{{Key: "expires_at", Value: 1}},
		},
	}

	if _, err := announcementCollection.Indexes().CreateMany(ctx, announcementIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для объявлений: %w", err)
	}

	// Создание индексов для событий
	eventCollection := m.Database.Collection("events")
	eventIndexes := []mongo.IndexModel{
		{
			// Составной индекс для фильтрации событий по дате
			Keys: bson.D{
				{Key: "start_date", Value: 1},
				{Key: "status", Value: 1},
			},
		},
		{
			// Индекс для сортировки по дате создания
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			// Геопространственный индекс
			Keys: bson.D{{Key: "location", Value: "2dsphere"}},
		},
		{
			// Индекс для организатора
			Keys: bson.D{{Key: "organizer_id", Value: 1}},
		},
	}

	if _, err := eventCollection.Indexes().CreateMany(ctx, eventIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для событий: %w", err)
	}

	// Создание индексов для групп
	groupCollection := m.Database.Collection("groups")
	groupIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "name", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			// Индекс для поиска групп пользователя
			Keys: bson.D{{Key: "members.user_id", Value: 1}},
		},
	}

	if _, err := groupCollection.Indexes().CreateMany(ctx, groupIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для групп: %w", err)
	}

	// Создание индексов для сообщений
	messageCollection := m.Database.Collection("messages")
	messageIndexes := []mongo.IndexModel{
		{
			// Составной индекс для получения сообщений группы
			Keys: bson.D{
				{Key: "group_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			// Индекс для автора
			Keys: bson.D{{Key: "author_id", Value: 1}},
		},
	}

	if _, err := messageCollection.Indexes().CreateMany(ctx, messageIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для сообщений: %w", err)
	}

	// Создание индексов для петиций
	petitionCollection := m.Database.Collection("petitions")
	petitionIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "author_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "category", Value: 1}},
		},
		{
			// Индекс для подписей
			Keys: bson.D{{Key: "signatures.user_id", Value: 1}},
		},
	}

	if _, err := petitionCollection.Indexes().CreateMany(ctx, petitionIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для петиций: %w", err)
	}

	// Создание индексов для опросов
	pollCollection := m.Database.Collection("polls")
	pollIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "creator_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "category", Value: 1}},
		},
	}

	if _, err := pollCollection.Indexes().CreateMany(ctx, pollIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для опросов: %w", err)
	}

	// Создание индексов для городских проблем
	cityIssueCollection := m.Database.Collection("city_issues")
	cityIssueIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "category", Value: 1},
			},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "location", Value: "2dsphere"}},
		},
		{
			Keys: bson.D{{Key: "reporter_id", Value: 1}},
		},
	}

	if _, err := cityIssueCollection.Indexes().CreateMany(ctx, cityIssueIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для городских проблем: %w", err)
	}

	// Создание индексов для транспортных маршрутов
	transportRouteCollection := m.Database.Collection("transport_routes")
	transportRouteIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "route_number", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "transport_type", Value: 1}},
		},
		{
			// Геопространственный индекс для остановок
			Keys: bson.D{{Key: "stops.location", Value: "2dsphere"}},
		},
	}

	if _, err := transportRouteCollection.Indexes().CreateMany(ctx, transportRouteIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для транспортных маршрутов: %w", err)
	}

	// Создание индексов для транспортных средств
	transportVehicleCollection := m.Database.Collection("transport_vehicles")
	transportVehicleIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "vehicle_number", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "route_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "current_location", Value: "2dsphere"}},
		},
	}

	if _, err := transportVehicleCollection.Indexes().CreateMany(ctx, transportVehicleIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для транспортных средств: %w", err)
	}

	// Создание индексов для уведомлений
	notificationCollection := m.Database.Collection("notifications")
	notificationIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "is_read", Value: 1},
			},
		},
	}

	if _, err := notificationCollection.Indexes().CreateMany(ctx, notificationIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для уведомлений: %w", err)
	}

	// Создание индексов для токенов устройств
	deviceTokenCollection := m.Database.Collection("device_tokens")
	deviceTokenIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "user_id", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "fcm_token", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}

	if _, err := deviceTokenCollection.Indexes().CreateMany(ctx, deviceTokenIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для токенов устройств: %w", err)
	}

	log.Println("✅ Индексы успешно созданы для всех коллекций")
	return nil
}
