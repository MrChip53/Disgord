package sse

import (
	"fmt"
	"log"
	"time"
)

type Client struct {
	UserId    string
	Username  string
	ServerId  string
	ChannelId string
	Channel   chan Event
}

type ClientEvent struct {
	UserId    string
	ServerId  string
	ChannelId string
}

type Event struct {
	ServerId  string
	ChannelId string
	Id        string
	Event     string
	Data      []byte
}

type Server struct {
	event         chan Event
	clients       map[Client]bool
	connecting    chan Client
	disconnecting chan Client
	clientUpdate  chan ClientEvent
	bufSize       uint
}

func New() *Server {
	s := &Server{
		event:         make(chan Event),
		clients:       make(map[Client]bool),
		connecting:    make(chan Client),
		disconnecting: make(chan Client),
		clientUpdate:  make(chan ClientEvent),
		bufSize:       2,
	}

	s.run()
	return s
}

func (s *Server) run() {
	go func() {
		for {
			select {
			case cl := <-s.connecting:
				s.clients[cl] = true
				log.Printf("new sse client connected. connected clients: %d\n", len(s.clients))
			case cl := <-s.disconnecting:
				delete(s.clients, cl)
				log.Printf("sse client disconnected. connected clients: %d\n", len(s.clients))
			case event := <-s.event:
				log.Printf("new sse event. event: %s\n", event.String())
				for v := range s.clients {
					if (event.ServerId != "" && event.ServerId != v.ServerId) || (event.ChannelId != "" && event.ChannelId != v.ChannelId) {
						continue
					}
					// TODO: non-blocking broadcast
					select {
					case v.Channel <- event: // Try to send event to client
					default:
						fmt.Println("Channel full. Discarding value")
					}
				}
			}
		}
	}()
	go func() {
		for {
			event := Event{
				Id:    "0",
				Event: "ping",
				Data:  []byte("ping"),
			}
			for v := range s.clients {
				select {
				case v.Channel <- event:
				default:
					fmt.Println("Channel full. Discarding value")
				}
			}
			time.Sleep(time.Second * 60)
		}
	}()
}

func (s *Server) SetBufferSize(size uint) {
	s.bufSize = size
}

func (s *Server) MakeClient(username string, serverId string, channelId string) *Client {
	c := &Client{
		Username:  username,
		Channel:   make(chan Event, s.bufSize),
		ServerId:  serverId,
		ChannelId: channelId,
	}
	s.connecting <- *c
	return c
}

func (s *Server) DestroyClient(c *Client) {
	s.disconnecting <- *c
}

func (s *Server) SendBytes(id string, event string, b []byte) {
	e := Event{
		Id:    id,
		Event: event,
		Data:  b,
	}
	s.event <- e
}

func (s *Server) SendMessage(id string, event string, b []byte, serverId string, channelId string) {
	e := Event{
		ServerId:  serverId,
		ChannelId: channelId,
		Id:        id,
		Event:     event,
		Data:      b,
	}
	s.event <- e
}

func (e *Event) String() string {
	return fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", e.Id, e.Event, e.Data)
}

func FormatEvent(id string, event string, data string) string {
	return fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", id, event, data)
}
