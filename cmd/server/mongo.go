package main

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
)

type MongoClient struct {
	client *mongo.Client
}

func NewMongo() *MongoClient {
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(os.Getenv("MONGODB_URI")).SetServerAPIOptions(serverAPI)
	client, err := mongo.Connect(context.TODO(), opts)
	if err != nil {
		panic(err)
	}
	return &MongoClient{client: client}
}

func (m *MongoClient) Close() {
	if err := m.client.Disconnect(context.TODO()); err != nil {
		panic(err)
	}
}

func (m *MongoClient) CreateUser(user *User) error {
	_, err := m.client.Database("disgord").Collection("users").InsertOne(context.TODO(), bson.M{
		"username":      user.Username,
		"lowerUsername": user.LowerUsername,
		"password":      user.Password,
	})
	return err
}
