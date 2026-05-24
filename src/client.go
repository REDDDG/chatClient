package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	writeTime      = time.Second * 10
	pongWait       = time.Second * 60
	pingPeriod     = pongWait * 9 / 10
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (c *Client) readPump() {
	defer func() {
		c.hub.mu.Lock()
		for _, roomId := range c.roomList {
			delete(c.hub.clientRoom[roomId], c)
			if len(c.hub.clientRoom[roomId]) == 0 {
				delete(c.hub.clientRoom, roomId)
			}
		}
		c.roomList = nil
		c.hub.mu.Unlock()
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		c.hub.broadcast <- broadcastMsg{client: c, message: message}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(pongWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(pongWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func serveWs(hub *Hub, c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println(err)
		return
	}
	session := sessions.Default(c)
	userId, ok := session.Get("id").(int)
	if !ok {
		log.Println("user not authenticated")
		conn.Close()
		return
	}
	var userName string
	if err := db.QueryRowContext(c.Request.Context(), "select username from user where id = ?", userId).Scan(&userName); err != nil {
		log.Println("failed to get username:", err)
		conn.Close()
		return
	}
	rows, err := db.QueryContext(c.Request.Context(), "select roomId from chatfriends where userId =?", userId)
	if err != nil {
		log.Println(err)
		conn.Close()
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte), id: userId, userName: userName, roomList: make([]int, 0)}
	client.hub.register <- client
	for rows.Next() {
		var roomId int
		rows.Scan(&roomId)
		client.roomList = append(client.roomList, roomId)
	}
	rows.Close()
	rows, err = db.QueryContext(c.Request.Context(), "select roomId from userhave where userId =?", userId)
	if err != nil {
		log.Println(err)
		return
	}
	for rows.Next() {
		var roomId int
		rows.Scan(&roomId)
		client.roomList = append(client.roomList, roomId)
	}
	rows.Close()
	hub.mu.Lock()
	for _, roomId := range client.roomList {
		if hub.clientRoom[roomId] == nil {
			hub.clientRoom[roomId] = make(map[*Client]bool)
		}
		hub.clientRoom[roomId][client] = true
	}
	hub.mu.Unlock()
	go client.readPump()
	go client.writePump()
}
