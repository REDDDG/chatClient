package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Server  ServerConfig  `json:"server"`
	MySQL   MySQLConfig   `json:"mysql"`
	Redis   RedisConfig   `json:"redis"`
	Session SessionConfig `json:"session"`
	CORS    CORSConfig    `json:"cors"`
}

type ServerConfig struct {
	Port string `json:"port"`
}

type MySQLConfig struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
}

func (m MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		m.User, m.Password, m.Host, m.Port, m.Database)
}

type RedisConfig struct {
	Addr string `json:"addr"`
	DB   int    `json:"db"`
}

type SessionConfig struct {
	Secret string `json:"secret"`
}

type CORSConfig struct {
	Origin string `json:"origin"`
}

var Cfg *Config

func Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}
	Cfg = &cfg
	return nil
}
