package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Message struct {
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

type broadcastMsg struct {
	client  *Client
	message []byte
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan broadcastMsg
	register   chan *Client
	unregister chan *Client
	clientRoom map[int]map[*Client]bool
	mu         sync.RWMutex
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	id       int
	roomList []int
	userName string
	closed   int32
}

// tryClose 原子地关闭 send channel，确保只关闭一次
func (c *Client) tryClose() bool {
	return atomic.CompareAndSwapInt32(&c.closed, 0, 1)
}
