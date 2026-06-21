package ws

import (
	"context"
	"log"
	"strconv"
	"time"

	"chatClient/internal/database"

	"github.com/gorilla/websocket"
)

const (
	WriteTime      = time.Second * 10
	PongWait       = time.Second * 60
	PingPeriod     = PongWait * 9 / 10
	MaxMessageSize = 512
)

var (
	Newline = []byte{'\n'}
	Space   = []byte{' '}
)

func NewClient(hub *Hub, conn *websocket.Conn, id int, userName string) *Client {
	return &Client{
		Hub:      hub,
		Conn:     conn,
		Send:     make(chan []byte),
		ID:       id,
		UserName: userName,
		RoomList: make([]int, 0),
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Mu.Lock()
		for _, roomId := range c.RoomList {
			delete(c.Hub.ClientRoom[roomId], c)
			if len(c.Hub.ClientRoom[roomId]) == 0 {
				delete(c.Hub.ClientRoom, roomId)
			}
		}
		c.RoomList = nil
		c.Hub.Mu.Unlock()
		c.Hub.Unregister <- c
		c.Conn.Close()
		database.RDB.Del(context.Background(), strconv.Itoa(c.ID))
	}()
	c.Conn.SetReadDeadline(time.Now().Add(PongWait))
	c.Conn.SetReadLimit(MaxMessageSize)
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(PongWait))
		return nil
	})
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		c.Hub.Broadcast <- broadcastMsg{client: c, message: message}
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(PingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(PongWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(PongWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
