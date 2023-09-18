package main

import (
	"fmt"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"html/template"
	"time"
)

func getFuncMap() template.FuncMap {
	return template.FuncMap{
		"getFirstLetter":   getFirstLetter,
		"formatTime":       formatTime,
		"formatObjectId":   formatObjectId,
		"createStringDict": createStringDict,
		"getChannel":       getChannel,
	}
}

func getFirstLetter(s string) string {
	return string(s[0])
}

func formatTime(t time.Time) string {
	y, m, d := time.Now().UTC().Date()
	if t.Year() == y && t.Month() == m && t.Day() == d {
		return fmt.Sprintf("Today at %s", t.Format("3:04 PM"))
	} else if t.Year() == y && t.Month() == m && t.Day() == d-1 {
		return fmt.Sprintf("Yesterday at %s", t.Format("3:04 PM"))
	}
	return t.Format("01/02/2006 3:04 PM")
}

func formatObjectId(objId primitive.ObjectID) string {
	return objId.Hex()
}

func getChannel(channelId string, channels []Channel) *Channel {
	for _, channel := range channels {
		if channel.ID.Hex() == channelId {
			return &channel
		}
	}
	return nil
}

// TODO this could probably be done better
func createStringDict(v ...interface{}) map[string]string {
	dict := make(map[string]string)
	if len(v)%2 != 0 {
		return dict
	}
	for i := 0; i < len(v); i += 2 {
		t, ok := v[i+1].(string)
		if !ok {
			t = ""
		}
		dict[v[i].(string)] = t
	}
	return dict
}
