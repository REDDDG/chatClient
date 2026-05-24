package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
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
		AllowOrigins:     []string{"http://localhost:8080", "http://10.166.91.254:8080"},
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
			var username string
			if err := db.QueryRowContext(c.Request.Context(), "select username from user where id = ?", id).Scan(&username); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"username": username, "id": id})
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
			var rows *sql.Rows
			before := c.Query("before")
			if before == "" {
				rows, err = db.QueryContext(c.Request.Context(),
					"SELECT room_id, sender_id, sender_name, text, created_at FROM messages WHERE room_id=? ORDER BY created_at DESC LIMIT ?",
					roomID, limit)
			} else {
				beforeTime, parseErr := time.Parse(time.RFC3339Nano, before)
				if parseErr != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before format"})
					return
				}
				rows, err = db.QueryContext(c.Request.Context(),
					"SELECT room_id, sender_id, sender_name, text, created_at FROM messages WHERE room_id=? AND created_at < ? ORDER BY created_at DESC LIMIT ?",
					roomID, beforeTime, limit)
			}
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
			defer rows.Close()
			var messages []Message
			for rows.Next() {
				var m Message
				if scanErr := rows.Scan(&m.RoomID, &m.SenderID, &m.SenderName, &m.Text, &m.CreatedAt); scanErr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
					return
				}
				messages = append(messages, m)
			}
			if rows.Err() != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
			if messages == nil {
				messages = []Message{}
			}
			c.JSON(http.StatusOK, gin.H{"messages": messages})
		})
		api.GET("/online-status", func(c *gin.Context) {
			session := sessions.Default(c)
			id := session.Get("id")
			if id == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
				return
			}
			// 收集所有关联用户 ID
			userIds := make(map[int]bool)
			// 好友
			rows, err := db.QueryContext(c.Request.Context(), "SELECT friendId FROM chatfriends WHERE userId = ?", id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
			for rows.Next() {
				var fid int
				rows.Scan(&fid)
				userIds[fid] = true
			}
			rows.Close()
			// 同群成员
			rows, err = db.QueryContext(c.Request.Context(),
				"SELECT userId FROM userhave WHERE roomId IN (SELECT roomId FROM userhave WHERE userId = ?)", id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
			for rows.Next() {
				var uid int
				rows.Scan(&uid)
				userIds[uid] = true
			}
			rows.Close()
			// 逐条检查 Redis 在线状态
			online := make(map[int]bool)
			for uid := range userIds {
				n, err := rdb.Exists(c.Request.Context(), fmt.Sprintf("online:%d", uid)).Result()
				if err != nil {
					online[uid] = false
					continue
				}
				online[uid] = n > 0
			}
			c.JSON(http.StatusOK, gin.H{"online": online})
		})
	}

	router.Run(":9090")
}
