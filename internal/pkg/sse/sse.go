package sse

import "fmt"

type Client chan []byte

type SSEServer struct {
	event         chan []byte
	clients       map[Client]bool
	connecting    chan Client
	disconnecting chan Client
	bufSize       uint
}

func New() *SSEServer {
	s := &SSEServer{
		event:         make(chan []byte),
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

			case cl := <-s.disconnecting:
				delete(s.clients, cl)

			case event := <-s.event:
				for cl := range s.clients {
					// TODO: non-blocking broadcast
					select {
					case cl <- event: // Try to send event to client
					default:
						fmt.Println("Channel full. Discarding value")
					}
				}
			}
		}
	}()
}

func (s *SSEServer) SetBufferSize(size uint) {
	s.bufSize = size
}

func (s *SSEServer) MakeClient() Client {
	c := make(Client, s.bufSize)
	s.connecting <- c
	return c
}

func (s *SSEServer) DestroyClient(c Client) {
	s.disconnecting <- c
}

func (s *SSEServer) SendBytes(b []byte) {
	endStr := []byte("\n\n")
	newB := []byte("data: ")
	newB = append(newB, b...)
	newB = append(newB, endStr...)
	s.event <- newB
}
