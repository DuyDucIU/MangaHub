package websocket

import gorillaws "github.com/gorilla/websocket"

type Client struct {
	hub      *ChatHub
	conn     *gorillaws.Conn
	send     chan []byte
	userID   string
	username string
	roomID   string
}
