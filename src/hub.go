package main

import (
	"context"
	"encoding/json"
	"log"
	"time"
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
			if client.tryClose() {
				close(client.send)
			}
		case bm := <-h.broadcast:
			var msg Message
			err := json.Unmarshal(bm.message, &msg)
			if err != nil {
				log.Println("error:", err)
				continue
			}
			// 服务端覆盖身份，防止客户端伪造
			msg.SenderID = bm.client.id
			msg.SenderName = bm.client.userName
			msg.CreatedAt = time.Now()
			if len(msg.Text) > 500 {
				log.Printf("client %d sent message exceeding 500 chars", bm.client.id)
				continue
			}
			sanitized, err := json.Marshal(msg)
			if err != nil {
				log.Println("error marshaling message:", err)
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
			// 消息持久化到 MySQL
			_, err = db.ExecContext(context.Background(),
				"INSERT INTO messages(room_id, sender_id, sender_name, text, created_at) VALUES(?,?,?,?,?)",
				msg.RoomID, msg.SenderID, msg.SenderName, msg.Text, msg.CreatedAt)
			if err != nil {
				log.Printf("failed to save message to db: %v", err)
			}
			h.mu.RLock()
			roomClients := h.clientRoom[msg.RoomID]
			clients := make([]*Client, 0, len(roomClients))
			for client := range roomClients {
				clients = append(clients, client)
			}
			h.mu.RUnlock()
			for _, client := range clients {
				if client == bm.client {
					continue // 跳过发送者，避免回显
				}
				select {
				case client.send <- sanitized:
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
					if client.tryClose() {
						close(client.send)
					}
				}
			}
		}

	}
}
