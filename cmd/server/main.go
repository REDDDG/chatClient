package main

import (
	"chatClient/internal/config"
	"chatClient/internal/database"
	"chatClient/internal/router"
	"chatClient/internal/ws"
	"log"
)

func main() {
	if err := config.Load("config.json"); err != nil {
		log.Fatal("failed to load config:", err)
	}

	database.InitMySQL()
	database.InitRedis()
	defer database.DB.Close()
	defer database.RDB.Close()

	hub := ws.NewHub()
	go hub.Run()

	r := router.Setup(hub)
	r.Run(config.Cfg.Server.Port)
}
