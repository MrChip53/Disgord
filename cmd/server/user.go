package main

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"log"
)

type User struct {
	ID                primitive.ObjectID  `bson:"_id"`
	LastActiveServer  primitive.ObjectID  `bson:"lastActiveServer"`
	LastActiveChannel primitive.ObjectID  `bson:"lastActiveChannel"`
	Username          string              `bson:"username"`
	LowerUsername     string              `bson:"lowerUsername"`
	Password          string              `bson:"password"`
	AvatarObjectId    string              `bson:"avatarObjectId"`
	Servers           []ServerMemberships `bson:"servers"`
}

func (u *User) GetServers(mongo *MongoClient) []*Server {
	servers := make([]*Server, 0)
	for _, serverMem := range u.Servers {
		server, err := mongo.GetServer(serverMem.ServerId.Hex())
		if err != nil {
			log.Println(err)
			continue
		}
		servers = append(servers, server)
	}
	return servers
}

func (u *User) UpdateLastServerAndChannel(mongo *MongoClient, serverId string, channelId string) {
	serverOid, err := primitive.ObjectIDFromHex(serverId)
	if err != nil {
		log.Println(err)
		return
	}
	channelOid, err := primitive.ObjectIDFromHex(channelId)
	if err != nil {
		log.Println(err)
		return
	}
	u.LastActiveChannel = channelOid
	u.LastActiveServer = serverOid
	err = mongo.UpdateUser(u.ID.Hex(), bson.M{
		"$set": bson.M{"lastActiveServer": u.LastActiveServer, "lastActiveChannel": u.LastActiveChannel},
	})
	if err != nil {
		log.Println(err)
		return
	}
}

func (u *User) HasServerAccess(serverId string) bool {
	for _, server := range u.Servers {
		if server.ServerId.Hex() == serverId {
			return true
		}
	}
	return false
}

func (u *User) AddServer(serverId string, mongo *MongoClient) error {
	serverOid, err := primitive.ObjectIDFromHex(serverId)
	if err != nil {
		log.Println(err)
		return err
	}
	u.Servers = append(u.Servers, ServerMemberships{
		ServerId: serverOid,
		IsOwner:  false,
	})
	return mongo.UpdateUser(u.ID.Hex(), bson.M{
		"$push": bson.M{"servers": bson.M{"serverId": serverOid, "isOwner": false}},
	})
}
