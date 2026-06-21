package router

import (
	"chatClient/internal/handler"
	"chatClient/internal/middleware"
	"chatClient/internal/ws"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func Setup(hub *ws.Hub) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.CORSConfig())

	store := cookie.NewStore([]byte("dddg"))
	r.Use(sessions.Sessions("mysession", store))

	r.Static("/uploads", "./uploads")

	r.GET("/ws", handler.ServeWs(hub))

	api := r.Group("/api")
	{
		api.POST("/register", handler.Register)
		api.POST("/login", handler.Login)
		api.GET("/me", handler.Me)
		api.POST("/logout", handler.Logout)
		api.GET("/rooms", handler.Rooms)
		api.GET("/messages", handler.Messages)
		api.POST("/online-status", handler.OnlineStatus)
		api.GET("/avatar", handler.GetAvatar)
		api.POST("/avatar", handler.UploadAvatar)
		api.PUT("/profile", handler.UpdateProfile)
	}

	return r
}
