package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	InitDB()
	InitRedis()
	defer db.Close()
	defer rdb.Close()
	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:8080"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))
	hub := newHub()
	go hub.run()
	store := cookie.NewStore([]byte("dddg"))
	router.Use(sessions.Sessions("mysession", store))

	router.GET("/ws", func(c *gin.Context) {
		serveWs(hub, c)
	})
	router.Static("/uploads", "./uploads")
	{
		api := router.Group("/api")
		api.POST("/register", func(c *gin.Context) {
			var req User
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}
			if len(req.Username) < 1 || len(req.Username) > 12 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "username must be 1-12 characters"})
				return
			}
			if len(req.Password) < 4 || len(req.Password) > 20 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "password must be 4-20 characters"})
				return
			}
			var tmp int
			err := db.QueryRowContext(c.Request.Context(), "select id from user where username = ?", req.Username).Scan(&tmp)
			if err == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "user exist"})
				return
			}
			if err != sql.ErrNoRows {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
				return
			}
			result, err := db.ExecContext(c.Request.Context(), "INSERT INTO user(username,password) VALUES(?,?)", req.Username, string(hashedPassword))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "insert failed"})
				return
			}
			id, err := result.LastInsertId()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "get user id failed"})
				return
			}
			session := sessions.Default(c)

			session.Set("id", int(id))
			session.Options(sessions.Options{
				Path:     "/",
				MaxAge:   3600 * 24,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteNoneMode,
			})
			session.Save()
			c.JSON(http.StatusOK, gin.H{
				"message": "register success",
			})
		})

		api.POST("/login", func(c *gin.Context) {
			var req User
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}
			if len(req.Username) < 1 || len(req.Password) < 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
				return
			}
			var id int
			var hashedPassword string
			session := sessions.Default(c)
			err := db.QueryRowContext(c.Request.Context(), "select id, password from user where username = ?", req.Username).Scan(&id, &hashedPassword)
			if err == sql.ErrNoRows {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
				return
			}
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
			if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password)); err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
				return
			}
			session.Set("id", id)
			session.Options(sessions.Options{
				Path:     "/",
				MaxAge:   3600 * 24,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteNoneMode,
			})
			session.Save()
			c.JSON(http.StatusOK, gin.H{})
		})

		api.GET("/me", func(c *gin.Context) {
			session := sessions.Default(c)
			id := session.Get("id")
			if id == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "login failed"})
				return
			}
			var username, avatar string
			if err := db.QueryRowContext(c.Request.Context(), "select username, avatar from user where id = ?", id).Scan(&username, &avatar); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"username": username, "id": id, "avatar": avatar})
		})
		api.POST("/logout", func(c *gin.Context) {
			session := sessions.Default(c)
			session.Clear()
			session.Save()
			c.JSON(http.StatusOK, gin.H{"message": "logged out"})
		})
		api.GET("/rooms", func(c *gin.Context) {
			session := sessions.Default(c)
			id := session.Get("id")
			log.Println(id)
			if id == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "login failed"})
				return
			}
			rows, err := db.QueryContext(c.Request.Context(), "select roomId,friendId,friendName from chatfriends where userid = ?", id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
			var friends []Friends
			defer rows.Close()
			for rows.Next() {
				var friendId int
				var roomId int
				var friendName string
				if err := rows.Scan(&roomId, &friendId, &friendName); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
					return
				}
				friends = append(friends, Friends{
					FriendId: friendId, RoomId: roomId, FriendName: friendName,
				})
			}
			if err := rows.Err(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
			var rooms []Rooms
			rows, err = db.QueryContext(c.Request.Context(), "select roomId,roomName from userhave where userId = ?", id)
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
				rooms = append(rooms, Rooms{
					RoomId: roomId, RoomName: roomName,
				})
			}
			if err := rows.Err(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"friends": friends, "rooms": rooms})
		})
		api.GET("/messages", func(c *gin.Context) {
			session := sessions.Default(c)
			uid := session.Get("id")
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
			// 验证用户是房间成员
			var ok int
			err = db.QueryRowContext(c.Request.Context(),
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
				// 翻历史消息：跳过未读逻辑
				beforeTime, parseErr := time.Parse(time.RFC3339Nano, before)
				if parseErr != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before format"})
					return
				}
				rows, err = db.QueryContext(c.Request.Context(),
					"SELECT id, room_id, sender_id, sender_name, text, created_at FROM messages WHERE room_id=? AND created_at < ? ORDER BY created_at DESC LIMIT ?",
					roomID, beforeTime, limit)
			} else if msgType == "unread" {
				// 增量加载未读消息
				var firstUnreadMsgID int64
				err = db.QueryRowContext(c.Request.Context(),
					"SELECT first_unread_msg_id, unread_count FROM user_unread WHERE user_id=? AND room_id=?",
					userID, roomID).Scan(&firstUnreadMsgID, &unreadCount)
				if err != nil || unreadCount == 0 {
					c.JSON(http.StatusOK, gin.H{"messages": []Message{}, "unreadCount": 0})
					return
				}
				afterIDStr := c.Query("afterId")
				if afterIDStr == "" {
					rows, err = db.QueryContext(c.Request.Context(),
						"SELECT id, room_id, sender_id, sender_name, text, created_at FROM messages WHERE room_id=? AND id >= ? ORDER BY id ASC LIMIT ?",
						roomID, firstUnreadMsgID, limit)
				} else {
					afterID, parseErr := strconv.ParseInt(afterIDStr, 10, 64)
					if parseErr != nil {
						c.JSON(http.StatusBadRequest, gin.H{"error": "invalid afterId"})
						return
					}
					rows, err = db.QueryContext(c.Request.Context(),
						"SELECT id, room_id, sender_id, sender_name, text, created_at FROM messages WHERE room_id=? AND id > ? ORDER BY id ASC LIMIT ?",
						roomID, afterID, limit)
				}
			} else {
				// 默认：最近 limit 条
				rows, err = db.QueryContext(c.Request.Context(),
					"SELECT id, room_id, sender_id, sender_name, text, created_at FROM messages WHERE room_id=? ORDER BY created_at DESC LIMIT ?",
					roomID, limit)
			}

			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
			defer rows.Close()
			var messages []Message
			var lastID int64
			for rows.Next() {
				var m Message
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
				messages = []Message{}
			}

			// 更新已读状态
			if msgType == "unread" && len(messages) > 0 {
				remaining := unreadCount - len(messages)
				if remaining < 0 {
					remaining = 0
				}
				newFirstUnread := lastID + 1
				if remaining == 0 {
					newFirstUnread = 0
				}
				_, _ = db.ExecContext(c.Request.Context(),
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
		})
		api.POST("/online-status", func(c *gin.Context) {
			session := sessions.Default(c)
			if session.Get("id") == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
				return
			}
			var req struct {
				Ids []int `json:"ids"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}
			online := make(map[int]bool)
			for _, uid := range req.Ids {
				n, err := rdb.Exists(c.Request.Context(), strconv.Itoa(uid)).Result()
				if err != nil {
					online[uid] = false
					continue
				}
				online[uid] = n > 0
			}
			c.JSON(http.StatusOK, gin.H{"online": online})
		})
		// 无需登录：按用户 ID 获取头像
		api.GET("/avatar", func(c *gin.Context) {
			userID, err := strconv.Atoi(c.Query("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"avatar": ""})
				return
			}
			var avatar string
			db.QueryRowContext(c.Request.Context(), "SELECT avatar FROM user WHERE id=?", userID).Scan(&avatar)
			c.JSON(http.StatusOK, gin.H{"avatar": avatar})
		})
		api.POST("/avatar", func(c *gin.Context) {
			session := sessions.Default(c)
			uid := session.Get("id")
			if uid == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
				return
			}
			file, header, err := c.Request.FormFile("avatar")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read file"})
				return
			}
			defer file.Close()
			if header.Size > 200*1024 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "file too large, max 200KB"})
				return
			}
			contentType := header.Header.Get("Content-Type")
			if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/gif" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "only jpg, png, gif allowed"})
				return
			}
			ext := filepath.Ext(header.Filename)
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".gif" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file extension"})
				return
			}
			os.MkdirAll("./uploads/avatars", 0755) // ignore error: directory may already exist
			filename := fmt.Sprintf("%d_%d%s", uid, time.Now().UnixMilli(), ext)
			dst, err := os.Create(filepath.Join("uploads", "avatars", filename))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
				return
			}
			defer dst.Close()
			if _, err := io.Copy(dst, file); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
				return
			}
			avatarPath := "/uploads/avatars/" + filename
			_, err = db.ExecContext(c.Request.Context(), "UPDATE user SET avatar=? WHERE id=?", avatarPath, uid)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update avatar"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"avatar": avatarPath})
		})
		api.PUT("/profile", func(c *gin.Context) {
			session := sessions.Default(c)
			uid := session.Get("id")
			if uid == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
				return
			}
			var req struct {
				Username string `json:"username"`
				Password string `json:"password"`
				Avatar   string `json:"avatar"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}
			if req.Username == "" && req.Password == "" && req.Avatar == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "at least one field required"})
				return
			}
			var sets []string
			var args []interface{}
			if req.Username != "" {
				if len(req.Username) < 1 || len(req.Username) > 12 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "username must be 1-12 characters"})
					return
				}
				var tmp int
				err := db.QueryRowContext(c.Request.Context(), "select id from user where username=? and id!=?", req.Username, uid).Scan(&tmp)
				if err == nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "username already taken"})
					return
				}
				sets = append(sets, "username=?")
				args = append(args, req.Username)
			}
			if req.Password != "" {
				if len(req.Password) < 4 || len(req.Password) > 20 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "password must be 4-20 characters"})
					return
				}
				hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
					return
				}
				sets = append(sets, "password=?")
				args = append(args, string(hashed))
			}
			if req.Avatar != "" {
				sets = append(sets, "avatar=?")
				args = append(args, req.Avatar)
			}
			args = append(args, uid)
			query := "UPDATE user SET " + sets[0]
			for i := 1; i < len(sets); i++ {
				query += ", " + sets[i]
			}
			var ade = "WHERE id=?"
			query += ade
			if _, err := db.ExecContext(c.Request.Context(), query, args...); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "profile updated"})
		})
	}

	router.Run(":9090")
}
