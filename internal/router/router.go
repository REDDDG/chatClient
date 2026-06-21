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
		api.POST("/logout", handler.Logout)

		auth := api.Group("")
		auth.Use(middleware.AuthMiddleware())
		{
			auth.GET("/me", handler.Me)
			auth.GET("/rooms", handler.Rooms)
			auth.GET("/messages", handler.Messages)
			auth.POST("/online-status", handler.OnlineStatus)
			auth.GET("/avatar", handler.GetAvatar)
			auth.POST("/avatar", handler.UploadAvatar)
			auth.PUT("/profile", handler.UpdateProfile)
		}
	}

	return r
}
