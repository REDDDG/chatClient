package handler

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"chatClient/internal/database"
	"chatClient/internal/ws"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func ServeWs(hub *ws.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
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
		if err := database.DB.QueryRowContext(c.Request.Context(), "select username from user where id = ?", userId).Scan(&userName); err != nil {
			log.Println("failed to get username:", err)
			conn.Close()
			return
		}
		rows, err := database.DB.QueryContext(c.Request.Context(), "select roomId from chatfriends where userId =?", userId)
		if err != nil {
			log.Println(err)
			conn.Close()
			return
		}
		client := ws.NewClient(hub, conn, userId, userName)
		client.Hub.Register <- client
		for rows.Next() {
			var roomId int
			rows.Scan(&roomId)
			client.RoomList = append(client.RoomList, roomId)
		}
		rows.Close()
		rows, err = database.DB.QueryContext(c.Request.Context(), "select roomId from userhave where userId =?", userId)
		if err != nil {
			log.Println(err)
			client.Hub.Unregister <- client
			conn.Close()
			return
		}
		for rows.Next() {
			var roomId int
			rows.Scan(&roomId)
			client.RoomList = append(client.RoomList, roomId)
		}
		rows.Close()
		hub.Mu.Lock()
		for _, roomId := range client.RoomList {
			if hub.ClientRoom[roomId] == nil {
				hub.ClientRoom[roomId] = make(map[*ws.Client]bool)
			}
			hub.ClientRoom[roomId][client] = true
		}
		hub.Mu.Unlock()
		go client.ReadPump()
		go client.WritePump()
		database.RDB.Set(context.Background(), strconv.Itoa(client.ID), "1", 60*time.Second)
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					database.RDB.Expire(context.Background(), strconv.Itoa(client.ID), 60*time.Second)
				case _, ok := <-client.Send:
					if !ok {
						return
					}
				}
			}
		}()
	}
}
