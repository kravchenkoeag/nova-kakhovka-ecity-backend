package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"nova-kakhovka-ecity/internal/config"

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

// Создание индексов для коллекций
func (m *MongoDB) CreateIndexes(ctx context.Context) error {
	// Создание индексов для пользователей
	userCollection := m.Database.Collection("users")
	userIndexes := []mongo.IndexModel{
		{
			Keys:    map[string]interface{}{"email": 1},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    map[string]interface{}{"phone": 1},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys: map[string]interface{}{"location": "2dsphere"},
		},
	}

	if _, err := userCollection.Indexes().CreateMany(ctx, userIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для пользователей: %w", err)
	}

	// Создание индексов для объявлений
	announcementCollection := m.Database.Collection("announcements")
	announcementIndexes := []mongo.IndexModel{
		{
			Keys: map[string]interface{}{"category": 1, "location": 1},
		},
		{
			Keys: map[string]interface{}{"created_at": -1},
		},
		{
			Keys: map[string]interface{}{"location": "2dsphere"},
		},
	}

	if _, err := announcementCollection.Indexes().CreateMany(ctx, announcementIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для объявлений: %w", err)
	}

	// Создание индексов для событий
	eventCollection := m.Database.Collection("events")
	eventIndexes := []mongo.IndexModel{
		{
			Keys: map[string]interface{}{"date": 1, "location": 1},
		},
		{
			Keys: map[string]interface{}{"created_at": -1},
		},
	}

	if _, err := eventCollection.Indexes().CreateMany(ctx, eventIndexes); err != nil {
		return fmt.Errorf("ошибка создания индексов для событий: %w", err)
	}

	log.Println("Индексы успешно созданы")
	return nil
}
