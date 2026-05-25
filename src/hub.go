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
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			result, err := db.ExecContext(ctx,
				"INSERT INTO messages(room_id, sender_id, sender_name, text, created_at) VALUES(?,?,?,?,?)",
				msg.RoomID, msg.SenderID, msg.SenderName, msg.Text, msg.CreatedAt)
			cancel()
			if err != nil {
				log.Printf("failed to save message to db: %v", err)
			} else {
				msg.ID, _ = result.LastInsertId()
				// 更新未读计数：查询房间所有成员
				ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
				rows, err := db.QueryContext(ctx2,
					"SELECT userId FROM chatfriends WHERE roomId=? UNION SELECT userId FROM userhave WHERE roomId=?",
					msg.RoomID, msg.RoomID)
				if err != nil {
					log.Printf("failed to query room members: %v", err)
				} else {
					for rows.Next() {
						var memberID int
						if scanErr := rows.Scan(&memberID); scanErr != nil {
							continue
						}
						if memberID == msg.SenderID {
							// 发送者：确保记录存在，first_unread_msg_id 不为 0
							_, _ = db.ExecContext(ctx2,
								`INSERT INTO user_unread (user_id, room_id, first_unread_msg_id, unread_count)
								 VALUES (?, ?, ?, 0)
								 ON DUPLICATE KEY UPDATE
								     first_unread_msg_id = IF(first_unread_msg_id = 0, VALUES(first_unread_msg_id), first_unread_msg_id)`,
								memberID, msg.RoomID, msg.ID)
						} else {
							// 其他成员：未读计数 +1
							_, _ = db.ExecContext(ctx2,
								`INSERT INTO user_unread (user_id, room_id, first_unread_msg_id, unread_count)
								 VALUES (?, ?, ?, 1)
								 ON DUPLICATE KEY UPDATE
								     unread_count = unread_count + 1,
								     first_unread_msg_id = IF(first_unread_msg_id = 0, VALUES(first_unread_msg_id), first_unread_msg_id)`,
								memberID, msg.RoomID, msg.ID)
						}
					}
					rows.Close()
				}
				cancel2()
				// 重新序列化（msg.ID 已填充）
				if s, err2 := json.Marshal(msg); err2 == nil {
					sanitized = s
				}
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
