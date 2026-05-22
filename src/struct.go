package main

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Message struct {
	RoomID     int    `json:"roomId"`
	SenderID   int    `json:"senderId"`
	SenderName string `json:"senderName"`
	Text       string `json:"text"`
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
