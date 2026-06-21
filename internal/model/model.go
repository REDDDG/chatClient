package model

import "time"

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Avatar   string `json:"avatar"`
}

type Message struct {
	ID         int64     `json:"id"`
	RoomID     int       `json:"roomId"`
	SenderID   int       `json:"senderId"`
	SenderName string    `json:"senderName"`
	Text       string    `json:"text"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Friends struct {
	FriendId   int    `json:"friendId"`
	RoomId     int    `json:"roomId"`
	FriendName string `json:"friendName"`
}

type Rooms struct {
	RoomId   int    `json:"roomId"`
	RoomName string `json:"roomName"`
}
