package main

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
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

func (m *MongoClient) CreateUser(user *User) (primitive.ObjectID, error) {
	o, err := m.client.Database("disgord").Collection("users").InsertOne(context.TODO(), bson.M{
		"username":      user.Username,
		"lowerUsername": user.LowerUsername,
		"password":      user.Password,
	})
	return o.InsertedID.(primitive.ObjectID), err
}

func (m *MongoClient) CreateChannel(channel *Channel) (primitive.ObjectID, error) {
	o, err := m.client.Database("disgord").Collection("channels").InsertOne(context.TODO(), bson.M{
		"name":     channel.Name,
		"serverId": channel.ServerId,
		"type":     channel.Type,
	})
	return o.InsertedID.(primitive.ObjectID), err
}

func (m *MongoClient) CreateServer(server *Server) (primitive.ObjectID, error) {
	o, err := m.client.Database("disgord").Collection("servers").InsertOne(context.TODO(), bson.M{
		"name": server.Name,
	})
	return o.InsertedID.(primitive.ObjectID), err
}

func (m *MongoClient) GetServers() ([]Server, error) {
	pipeline := []bson.M{
		{
			"$lookup": bson.M{
				"from":         "channels",
				"localField":   "_id",
				"foreignField": "serverId",
				"as":           "channels",
			},
		},
	}

	cursor, err := m.client.Database("disgord").Collection("servers").Aggregate(context.Background(), pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())
	var results []Server
	for cursor.Next(context.Background()) {
		var server Server
		if err := cursor.Decode(&server); err != nil {
			log.Fatalf("Failed to decode document: %v", err)
		}
		results = append(results, server)
	}
	if err := cursor.Err(); err != nil {
		log.Fatalf("Cursor error: %v", err)
	}
	return results, nil
}

func (m *MongoClient) GetServer(serverId string) (*Server, error) {
	serverO, err := primitive.ObjectIDFromHex(serverId)
	if err != nil {
		return nil, err
	}
	pipeline := mongo.Pipeline{
		bson.D{
			{"$lookup", bson.D{
				{"from", "channels"},
				{"localField", "_id"},
				{"foreignField", "serverId"},
				{"as", "channels"},
			}},
		},
		bson.D{
			{"$match", bson.D{
				{"_id", serverO},
			}},
		},
		bson.D{
			{"$limit", 1},
		},
	}
	cursor, err := m.client.Database("disgord").Collection("servers").Aggregate(context.Background(), pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())
	if cursor.Next(context.Background()) {
		var server Server
		if err := cursor.Decode(&server); err != nil {
			return nil, err
		}
		return &server, nil
	}
	return nil, nil
}

func (m *MongoClient) CreateMessage(msg *Message) (primitive.ObjectID, error) {
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
	return o.InsertedID.(primitive.ObjectID), err
}

func (m *MongoClient) GetMessages(serverId string, channelId string) ([]Message, error) {
	serverO, err := primitive.ObjectIDFromHex(serverId)
	if err != nil {
		return nil, err
	}
	channelO, err := primitive.ObjectIDFromHex(channelId)
	if err != nil {
		return nil, err
	}

	var messages []Message
	filter := bson.M{
		"server":  serverO,
		"channel": channelO,
	}
	cursor, err := m.client.Database("disgord").Collection("messages").Find(context.TODO(), filter,
		options.Find().SetSort(bson.M{"timestamp": 1}).SetLimit(50))
	if err != nil {
		return nil, err
	}
	if err := cursor.All(context.Background(), &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (m *MongoClient) GetUser(id string) (*User, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	var user User
	filter := bson.M{
		"_id": objectID,
	}
	err = m.client.Database("disgord").Collection("users").FindOne(context.TODO(), filter).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (m *MongoClient) UpdateUser(userId string, update bson.M) error {
	objectID, err := primitive.ObjectIDFromHex(userId)
	if err != nil {
		return err
	}
	filter := bson.M{"_id": objectID}
	coll := m.client.Database("disgord").Collection("users")
	_, err = coll.UpdateOne(context.Background(), filter, update)
	return err
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
