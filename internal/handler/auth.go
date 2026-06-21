package handler

import (
	"database/sql"
	"net/http"

	"chatClient/internal/database"
	"chatClient/internal/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func Register(c *gin.Context) {
	var req model.User
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
	err := database.DB.QueryRowContext(c.Request.Context(), "select id from user where username = ?", req.Username).Scan(&tmp)
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
	result, err := database.DB.ExecContext(c.Request.Context(), "INSERT INTO user(username,password) VALUES(?,?)", req.Username, string(hashedPassword))
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
	})
	session.Save()
	c.JSON(http.StatusOK, gin.H{"message": "register success"})
}

func Login(c *gin.Context) {
	var req model.User
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
	err := database.DB.QueryRowContext(c.Request.Context(), "select id, password from user where username = ?", req.Username).Scan(&id, &hashedPassword)
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
	})
	session.Save()
	c.JSON(http.StatusOK, gin.H{})
}

func Me(c *gin.Context) {
	var username, avatar string
	id, _ := c.Get("id")
	if err := database.DB.QueryRowContext(c.Request.Context(), "select username, avatar from user where id = ?", id).Scan(&username, &avatar); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"username": username, "id": c.Get("id"), "avatar": avatar})
}

func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}
