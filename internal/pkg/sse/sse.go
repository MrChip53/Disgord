package sse

import (
	"fmt"
	"log"
	"time"
)

type Client struct {
	Username string
	Channel  chan Event
}

type Event struct {
	Id    string
	Event string
	Data  []byte
}

type SSEServer struct {
	event         chan Event
	clients       map[Client]bool
	connecting    chan Client
	disconnecting chan Client
	bufSize       uint
}

func New() *SSEServer {
	s := &SSEServer{
		event:         make(chan Event),
		clients:       make(map[Client]bool),
		connecting:    make(chan Client),
		disconnecting: make(chan Client),
		bufSize:       2,
	}

	s.run()
	return s
}

func (s *SSEServer) run() {
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
				for cl := range s.clients {
					// TODO: non-blocking broadcast
					select {
					case cl.Channel <- event: // Try to send event to client
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
			for cl := range s.clients {
				select {
				case cl.Channel <- event:
				default:
					fmt.Println("Channel full. Discarding value")
				}
			}
			time.Sleep(time.Second * 60)
		}
	}()
}

func (s *SSEServer) SetBufferSize(size uint) {
	s.bufSize = size
}

func (s *SSEServer) MakeClient(username string) *Client {
	c := &Client{
		Username: username,
		Channel:  make(chan Event, s.bufSize),
	}
	s.connecting <- *c
	return c
}

func (s *SSEServer) DestroyClient(c *Client) {
	s.disconnecting <- *c
}

func (s *SSEServer) SendBytes(id string, event string, b []byte) {
	e := Event{
		Id:    id,
		Event: event,
		Data:  b,
	}
	s.event <- e
}

func (e *Event) String() string {
	return fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", e.Id, e.Event, e.Data)
}

func FormatEvent(id string, event string, data string) string {
	return fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", id, event, data)
}
