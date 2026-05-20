package main

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Message struct {
	Id      int    `json:"id"`
	Content string `json:"text"`
}
