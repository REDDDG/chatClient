package main

import (
	"chatClient/internal/database"
	"chatClient/internal/router"
	"chatClient/internal/ws"
)

func main() {
	database.InitMySQL()
	database.InitRedis()
	defer database.DB.Close()
	defer database.RDB.Close()

	hub := ws.NewHub()
	go hub.Run()

	r := router.Setup(hub)
	r.Run(":9090")
}
