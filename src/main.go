package main

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
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
	dsn := "root:mysql12138@tcp(localhost:3306)/goland"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(10 * time.Minute)
	{
		api := router.Group("/api")
		api.POST("/register", func(c *gin.Context) {
			type User struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			var req User
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}
			var tmp int
			err := db.QueryRowContext(c.Request.Context(), "select id from user where username = ?", req.Username).Scan(&tmp)
			if err == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "user exist"})
				return
			}
			result, err := db.ExecContext(c.Request.Context(), "INSERT INTO user(username,password) VALUES(?,?)", req.Username, req.Password)
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

			session.Set("id", id)

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
			var id int
			session := sessions.Default(c)
			err := db.QueryRowContext(c.Request.Context(), "select id from user where username = ? and password =?", req.Username, req.Password).Scan(&id)
			if err == sql.ErrNoRows {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
				return
			}
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
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
			}
			var username string
			db.QueryRowContext(c.Request.Context(), "select username from user where id = ?", id).Scan(&username)
			c.JSON(http.StatusOK, gin.H{"username": username, "id": id})
		})
		api.POST("/logout", func(c *gin.Context) {
			session := sessions.Default(c)
			session.Clear()
			session.Save()
			session.Options(sessions.Options{
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteNoneMode,
			})
			c.JSON(http.StatusOK, gin.H{"message": "logged out"})
		})
		api.GET("/friends", func(c *gin.Context) {
			session := sessions.Default(c)
			id := session.Get("id")
			if id == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "login failed"})
			}
			rows, err := db.QueryContext(c.Request.Context(), "select friendid from chatfriends where userid = ?", id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			}
			var friends []Friends
			defer rows.Close()
			for rows.Next() {
				var friendId int
				rows.Scan(&friendId)
				friends = append(friends, Friends{FriendId: friendId})
			}
			c.JSON(http.StatusOK, gin.H{"friends": friends})
		})
	}

	router.Run(":9090")
}
