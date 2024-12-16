package client

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	Hub "poker-server/internal/transport/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
	ready   = []byte("ready to play")
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client это посредник между хабом и вебсокет соединением
type Client struct {
	hub    *Hub.Hub
	conn   *websocket.Conn
	Send   chan []byte
	Action chan []byte
	Id     string
	Seat   uint8
}
type Request struct {
	Action  string
	Bet     int
	Message string
}

// readPump передает сообщения от вебсокет соединения на хаб
//
// readPump запускается для каждого соединения в отдельной горутине. Приложение гарантирует, что есть не более 1 reader для этой горутины
func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		r := Request{}
		err = json.Unmarshal(message, &r)
		if err != nil {
			log.Printf("error: %v", err)
			return
		}
		log.Println(r, " show me R ============================")
		//if err != nil {
		//	log.Fatal("json unmarshal error:", err)
		//}
		var chatMessage []byte
		var actionMessage []byte
		if r.Message != "" {
			chatMessage = bytes.TrimSpace(bytes.Replace([]byte(r.Message), newline, space, -1))
			c.hub.Broadcast <- chatMessage
		}
		if r.Action != "" {
			actionMessage = message
			log.Println(message, " client request message")
			c.Action <- actionMessage // TODO сделать канал буферизованным 1, если не пустой, читать и записывать новое
			log.Println(c.Action, " client action BEFORE ----")
		}
	}
}

// writePump передает сообщения из хаба на вебсокет соединение
// Для каждого соединения запускается goroutine, запускающая writePump.
// Приложение гарантирует, что в соединении есть не более одного writer, выполняя все записи из этой goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Хаб закрыл канал
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Добавляет сообщения из очереди к текущему сообщению вебсокета
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) test(test []byte) {
	c.Action <- test
}

// ServeWs обрабатывает запросы websocket от одного узла.
func ServeWs(hub *Hub.Hub, w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	cookie, err := r.Cookie("sid")
	if err != nil {
		log.Println("cookie not found")
		return
	}
	// TODO select из БД данных по клиенту(chips, id, etc..)
	client := &Client{
		hub:    hub,
		conn:   conn,
		Send:   make(chan []byte, 256),
		Id:     cookie.Value,
		Seat:   uint8(len(hub.Register)),
		Action: make(chan []byte, 256)}
	client.hub.Register <- client

	go client.writePump()
	go client.readPump()
}
