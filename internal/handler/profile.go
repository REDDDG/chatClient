package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"chatClient/internal/database"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func GetAvatar(c *gin.Context) {
	userID, err := strconv.Atoi(c.Query("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"avatar": ""})
		return
	}
	var avatar string
	database.DB.QueryRowContext(c.Request.Context(), "SELECT avatar FROM user WHERE id=?", userID).Scan(&avatar)
	c.JSON(http.StatusOK, gin.H{"avatar": avatar})
}

func UploadAvatar(c *gin.Context) {
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
	os.MkdirAll("./uploads/avatars", 0755)
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
	_, err = database.DB.ExecContext(c.Request.Context(), "UPDATE user SET avatar=? WHERE id=?", avatarPath, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update avatar"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"avatar": avatarPath})
}

func UpdateProfile(c *gin.Context) {
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
		err := database.DB.QueryRowContext(c.Request.Context(), "select id from user where username=? and id!=?", req.Username, uid).Scan(&tmp)
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
	query += " WHERE id=?"
	if _, err := database.DB.ExecContext(c.Request.Context(), query, args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "profile updated"})
}
