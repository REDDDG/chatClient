package main

import (
	"encoding/json"
	"log"
)

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan broadcastMsg),
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
		case bm := <-h.broadcast:
			var msg Message
			err := json.Unmarshal(bm.message, &msg)
			if err != nil {
				log.Println("error:", err)
				continue
			}
			allowed := false
			for _, roomId := range bm.client.roomList {
				if roomId == msg.RoomID {
					allowed = true
					break
				}
			}
			if !allowed {
				log.Printf("client %d attempted to send to unauthorized room %d", bm.client.id, msg.RoomID)
				continue
			}
			h.mu.RLock()
			roomClients := h.clientRoom[msg.RoomID]
			clients := make([]*Client, 0, len(roomClients))
			for client := range roomClients {
				clients = append(clients, client)
			}
			h.mu.RUnlock()
			for _, client := range clients {
				select {
				case client.send <- bm.message:
				default:
					h.mu.Lock()
					for _, roomId := range client.roomList {
						delete(client.hub.clientRoom[roomId], client)
						if len(client.hub.clientRoom[roomId]) == 0 {
							delete(client.hub.clientRoom, roomId)
						}
					}
					client.roomList = nil
					h.mu.Unlock()
					delete(h.clients, client)
					close(client.send)
				}
			}
		}

	}
}
