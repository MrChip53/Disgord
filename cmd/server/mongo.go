package main

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

func (m *MongoClient) CreateMessage(msg *Message) (interface{}, error) {
	o, err := m.client.Database("disgord").Collection("messages").InsertOne(context.TODO(), bson.M{
		"server":         msg.Server,
		"channel":        msg.Channel,
		"authorId":       msg.AuthorId,
		"username":       msg.Username,
		"avatarObjectId": msg.AvatarObjectId,
		"timestamp":      msg.Timestamp,
		"type":           msg.Type,
		"message":        msg.Message,
		"command":        msg.Command,
	})
	return o.InsertedID, err
}

func (m *MongoClient) GetMessages(serverId string, channelId string) ([]Message, error) {
	var messages []Message
	//filter := bson.M{
	//	"server":  serverId,
	//	"channel": channelId,
	//}
	cursor, err := m.client.Database("disgord").Collection("messages").Find(context.TODO(), bson.D{},
		options.Find().SetSort(bson.M{"timestamp": 1}).SetLimit(50))
	if err != nil {
		return nil, err
	}
	if err := cursor.All(context.Background(), &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (m *MongoClient) GetMessage(id string) (*Message, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	var message Message
	filter := bson.M{
		"_id": objectID,
	}
	err = m.client.Database("disgord").Collection("messages").FindOne(context.TODO(), filter).Decode(&message)
	if err != nil {
		return nil, err
	}
	return &message, nil
}

func (m *MongoClient) DeleteMessage(id string) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	filter := bson.M{
		"_id": objectID,
	}
	_, err = m.client.Database("disgord").Collection("messages").DeleteOne(context.TODO(), filter)
	return err
}
