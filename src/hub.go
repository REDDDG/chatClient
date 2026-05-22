package main

import (
	"encoding/json"
	"log"
)

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	clientRoom map[int]map[*Client]bool
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte),
		clientRoom: make(map[int]map[*Client]bool),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			delete(h.clients, client)
			close(client.send)
		case message := <-h.broadcast:
			var msg Message
			err := json.Unmarshal(message, &msg)
			if err != nil {
				log.Println("error:", err)
				continue
			}
			for client := range h.clientRoom[msg.RoomID] {
				select {
				case client.send <- message:
				default:
					for _, roomId := range client.roomList {
						delete(client.hub.clientRoom[roomId], client)
						if (len(client.hub.clientRoom[roomId])) == 0 {
							delete(client.hub.clientRoom, roomId)
						}
					}
					client.roomList = nil
					delete(h.clients, client)
					close(client.send)
				}
			}
		}

	}
}
