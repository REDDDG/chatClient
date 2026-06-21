package database

import (
	"chatClient/internal/config"
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client

func InitRedis() {
	RDB = redis.NewClient(&redis.Options{
		Addr: config.Cfg.Redis.Addr,
		DB:   config.Cfg.Redis.DB,
	})
	if err := RDB.Ping(context.Background()).Err(); err != nil {
		log.Fatal(err)
	}
}
