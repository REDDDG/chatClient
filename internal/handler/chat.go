package handler

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"

	"chatClient/internal/database"
	"chatClient/internal/model"

	"github.com/gin-gonic/gin"
)

func Rooms(c *gin.Context) {
	id, _ := c.Get("id")
	log.Println(id)
	if id == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login failed"})
		return
	}
	rows, err := database.DB.QueryContext(c.Request.Context(), "select roomId,friendId,friendName from chatfriends where userid = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	var friends []model.Friends
	defer rows.Close()
	for rows.Next() {
		var friendId int
		var roomId int
		var friendName string
		if err := rows.Scan(&roomId, &friendId, &friendName); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		friends = append(friends, model.Friends{
			FriendId: friendId, RoomId: roomId, FriendName: friendName,
		})
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	var rooms []model.Rooms
	rows, err = database.DB.QueryContext(c.Request.Context(), "select roomId,roomName from userhave where userId = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()
	for rows.Next() {
		var roomId int
		var roomName string
		if err := rows.Scan(&roomId, &roomName); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		rooms = append(rooms, model.Rooms{
			RoomId: roomId, RoomName: roomName,
		})
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"friends": friends, "rooms": rooms})
}

func Messages(c *gin.Context) {
	uid, _ := c.Get("id")
	if uid == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}
	userID := uid.(int)
	roomID, err := strconv.Atoi(c.Query("roomId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid roomId"})
		return
	}
	limit := 50
	if l, e := strconv.Atoi(c.DefaultQuery("limit", "50")); e == nil && l > 0 && l <= 100 {
		limit = l
	}
	var ok int
	err = database.DB.QueryRowContext(c.Request.Context(),
		"SELECT 1 FROM (SELECT roomId FROM chatfriends WHERE userId=? AND roomId=? UNION SELECT roomId FROM userhave WHERE userId=? AND roomId=?) AS t LIMIT 1",
		uid, roomID, uid, roomID).Scan(&ok)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	before := c.Query("before")
	msgType := c.Query("type")

	var rows *sql.Rows
	var unreadCount int

	if before != "" {
		beforeTime, parseErr := time.Parse(time.RFC3339Nano, before)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before format"})
			return
		}
		rows, err = database.DB.QueryContext(c.Request.Context(),
			"SELECT id, room_id, sender_id, sender_name, text, created_at FROM messages WHERE room_id=? AND created_at < ? ORDER BY created_at DESC LIMIT ?",
			roomID, beforeTime, limit)
	} else if msgType == "unread" {
		var firstUnreadMsgID int64
		err = database.DB.QueryRowContext(c.Request.Context(),
			"SELECT first_unread_msg_id, unread_count FROM user_unread WHERE user_id=? AND room_id=?",
			userID, roomID).Scan(&firstUnreadMsgID, &unreadCount)
		if err != nil || unreadCount == 0 {
			c.JSON(http.StatusOK, gin.H{"messages": []model.Message{}, "unreadCount": 0})
			return
		}
		afterIDStr := c.Query("afterId")
		if afterIDStr == "" {
			rows, err = database.DB.QueryContext(c.Request.Context(),
				"SELECT id, room_id, sender_id, sender_name, text, created_at FROM messages WHERE room_id=? AND id >= ? ORDER BY id ASC LIMIT ?",
				roomID, firstUnreadMsgID, limit)
		} else {
			afterID, parseErr := strconv.ParseInt(afterIDStr, 10, 64)
			if parseErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid afterId"})
				return
			}
			rows, err = database.DB.QueryContext(c.Request.Context(),
				"SELECT id, room_id, sender_id, sender_name, text, created_at FROM messages WHERE room_id=? AND id > ? ORDER BY id ASC LIMIT ?",
				roomID, afterID, limit)
		}
	} else {
		rows, err = database.DB.QueryContext(c.Request.Context(),
			"SELECT id, room_id, sender_id, sender_name, text, created_at FROM messages WHERE room_id=? ORDER BY created_at DESC LIMIT ?",
			roomID, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()
	var messages []model.Message
	var lastID int64
	for rows.Next() {
		var m model.Message
		if scanErr := rows.Scan(&m.ID, &m.RoomID, &m.SenderID, &m.SenderName, &m.Text, &m.CreatedAt); scanErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		messages = append(messages, m)
		if m.ID > lastID {
			lastID = m.ID
		}
	}
	if rows.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if messages == nil {
		messages = []model.Message{}
	}

	if msgType == "unread" && len(messages) > 0 {
		remaining := unreadCount - len(messages)
		if remaining < 0 {
			remaining = 0
		}
		newFirstUnread := lastID + 1
		if remaining == 0 {
			newFirstUnread = 0
		}
		_, _ = database.DB.ExecContext(c.Request.Context(),
			`INSERT INTO user_unread (user_id, room_id, first_unread_msg_id, unread_count)
			 VALUES (?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE
			     first_unread_msg_id = VALUES(first_unread_msg_id),
			     unread_count = VALUES(unread_count)`,
			userID, roomID, newFirstUnread, remaining)
		c.JSON(http.StatusOK, gin.H{"messages": messages, "unreadCount": remaining})
	} else {
		c.JSON(http.StatusOK, gin.H{"messages": messages})
	}
}

func OnlineStatus(c *gin.Context) {
	var req struct {
		Ids []int `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	online := make(map[int]bool)
	for _, uid := range req.Ids {
		n, err := database.RDB.Exists(c.Request.Context(), strconv.Itoa(uid)).Result()
		if err != nil {
			online[uid] = false
			continue
		}
		online[uid] = n > 0
	}
	c.JSON(http.StatusOK, gin.H{"online": online})
}
