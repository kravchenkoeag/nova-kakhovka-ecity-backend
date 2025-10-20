package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Підключення до MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(ctx)

	collection := client.Database("nova_kakhovka_ecity").Collection("users")

	// Міграція: встановити Role для користувачів де його немає
	result, err := collection.UpdateMany(
		ctx,
		bson.M{
			"$or": []bson.M{
				{"role": bson.M{"$exists": false}},
				{"role": ""},
			},
		},
		[]bson.M{
			{
				"$set": bson.M{
					"role": bson.M{
						"$cond": bson.A{
							"$is_moderator",
							"MODERATOR",
							"USER",
						},
					},
				},
			},
		},
	)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Мігровано %d користувачів\n", result.ModifiedCount)
}
