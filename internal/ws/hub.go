package ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"chatClient/internal/database"
	"chatClient/internal/model"

	"github.com/gorilla/websocket"
)

type broadcastMsg struct {
	client  *Client
	message []byte
}

type Hub struct {
	clients    map[*Client]bool
	Broadcast  chan broadcastMsg
	Register   chan *Client
	Unregister chan *Client
	ClientRoom map[int]map[*Client]bool
	Mu         sync.RWMutex
}

type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte
	ID       int
	RoomList []int
	UserName string
	closed   int32
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan broadcastMsg),
		ClientRoom: make(map[int]map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.clients[client] = true
		case client := <-h.Unregister:
			delete(h.clients, client)
			if client.tryClose() {
				close(client.Send)
			}
		case bm := <-h.Broadcast:
			var msg model.Message
			err := json.Unmarshal(bm.message, &msg)
			if err != nil {
				log.Println("error:", err)
				continue
			}
			msg.SenderID = bm.client.ID
			msg.SenderName = bm.client.UserName
			msg.CreatedAt = time.Now()
			if len(msg.Text) > 500 {
				log.Printf("client %d sent message exceeding 500 chars", bm.client.ID)
				continue
			}
			sanitized, err := json.Marshal(msg)
			if err != nil {
				log.Println("error marshaling message:", err)
				continue
			}
			allowed := false
			for _, roomId := range bm.client.RoomList {
				if roomId == msg.RoomID {
					allowed = true
					break
				}
			}
			if !allowed {
				log.Printf("client %d attempted to send to unauthorized room %d", bm.client.ID, msg.RoomID)
				continue
			}
			// 消息持久化
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			result, err := database.DB.ExecContext(ctx,
				"INSERT INTO messages(room_id, sender_id, sender_name, text, created_at) VALUES(?,?,?,?,?)",
				msg.RoomID, msg.SenderID, msg.SenderName, msg.Text, msg.CreatedAt)
			cancel()
			if err != nil {
				log.Printf("failed to save message to db: %v", err)
			} else {
				msg.ID, _ = result.LastInsertId()
				ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
				rows, err := database.DB.QueryContext(ctx2,
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
							_, _ = database.DB.ExecContext(ctx2,
								`INSERT INTO user_unread (user_id, room_id, first_unread_msg_id, unread_count)
								 VALUES (?, ?, ?, 0)
								 ON DUPLICATE KEY UPDATE
								     first_unread_msg_id = IF(first_unread_msg_id = 0, VALUES(first_unread_msg_id), first_unread_msg_id)`,
								memberID, msg.RoomID, msg.ID)
						} else {
							_, _ = database.DB.ExecContext(ctx2,
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
				if s, err2 := json.Marshal(msg); err2 == nil {
					sanitized = s
				}
			}
			h.Mu.RLock()
			roomClients := h.ClientRoom[msg.RoomID]
			clients := make([]*Client, 0, len(roomClients))
			for client := range roomClients {
				clients = append(clients, client)
			}
			h.Mu.RUnlock()
			for _, client := range clients {
				if client == bm.client {
					continue
				}
				select {
				case client.Send <- sanitized:
				default:
					h.Mu.Lock()
					for _, roomId := range client.RoomList {
						delete(client.Hub.ClientRoom[roomId], client)
						if len(client.Hub.ClientRoom[roomId]) == 0 {
							delete(client.Hub.ClientRoom, roomId)
						}
					}
					client.RoomList = nil
					h.Mu.Unlock()
					delete(h.clients, client)
					if client.tryClose() {
						close(client.Send)
					}
				}
			}
		}
	}
}

func (c *Client) tryClose() bool {
	return atomic.CompareAndSwapInt32(&c.closed, 0, 1)
}
