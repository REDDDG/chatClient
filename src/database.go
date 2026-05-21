package main

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var db *sql.DB

func InitDB() {
	dsn := "root:mysql12138@tcp(localhost:3306)/goland"

	DB, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}

	if err := DB.Ping(); err != nil {
		log.Fatal(err)
	}

	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(10)
	DB.SetConnMaxIdleTime(5 * time.Minute)
	DB.SetConnMaxLifetime(10 * time.Minute)

	db = DB
}
