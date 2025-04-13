package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

//go:embed static/*
var content embed.FS

const InactiveTime int = 30

type Client struct {
	ID      string
	MsgChan chan ChatMsg
}

type ChatRoom struct {
	clients        map[string]*Client
	register       chan *Client
	unregister     chan *Client
	broadcast      chan ChatMsg
	history        []ChatMsg
	lastSeen       map[string]time.Time
	disconnectTime map[string]time.Time
	mutex          sync.RWMutex
}

type ChatMsg struct {
	Client   string    `json:"client"`
	Message  string    `json:"msg"`
	IsSystem bool      `json:"isSys"`
	IsActive bool      `json:"isAct"`
	Time     time.Time `json:"time"`
}

func NewChatRoom() *ChatRoom {
	r := &ChatRoom{
		clients:        make(map[string]*Client),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		broadcast:      make(chan ChatMsg),
		history:        []ChatMsg{},
		disconnectTime: make(map[string]time.Time),
		lastSeen:       make(map[string]time.Time),
	}
	go r.run()
	go r.monitorInactivity()
	return r
}

func (r *ChatRoom) run() {
	for {
		select {
		case client := <-r.register:
			r.mutex.Lock()
			r.clients[client.ID] = client
			r.lastSeen[client.ID] = time.Now()
			r.mutex.Unlock()
			log.Println("Client joined:", client.ID)

		case client := <-r.unregister:
			r.mutex.Lock()
			if _, ok := r.clients[client.ID]; ok {
				close(r.clients[client.ID].MsgChan)
				r.lastSeen[client.ID] = time.Now()
				r.disconnectTime[client.ID] = time.Now()
				delete(r.clients, client.ID)
				log.Println("Client left:", client.ID)
			}
			r.mutex.Unlock()

		case msg := <-r.broadcast:
			if msg.Time.IsZero() {
				msg.Time = time.Now()
			}
			r.mutex.Lock()
			r.history = append(r.history, msg)
			if len(r.history) > 100 {
				r.history = r.history[len(r.history)-100:]
			}
			if !msg.IsSystem {
				r.lastSeen[msg.Client] = time.Now()
			}
			r.mutex.Unlock()

			r.mutex.RLock()
			for _, client := range r.clients {
				select {
				case client.MsgChan <- msg:
				default:
					log.Printf("Dropping message for %s (channel full)", client.ID)
				}
			}
			r.mutex.RUnlock()
		}
	}
}

func (r *ChatRoom) monitorInactivity() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		var inactiveClients []*Client
		var messages []ChatMsg

		r.mutex.RLock()
		for id, client := range r.clients {
			last, ok := r.lastSeen[id]
			if !ok {
				continue
			}
			if now.Sub(last) > (time.Duration(InactiveTime) * time.Second) {
				msg := ChatMsg{
					Client:   id,
					Message:  " left the chat due to inactivity",
					IsSystem: true,
					IsActive: false,
					Time:     now,
				}
				messages = append(messages, msg)
				inactiveClients = append(inactiveClients, client)
			}
		}
		r.mutex.RUnlock()

		for _, msg := range messages {
			r.broadcast <- msg
		}
		for _, client := range inactiveClients {
			log.Printf("Removing client [%s] after %d seconds inactivity", client.ID, InactiveTime)
			time.Sleep(1 * time.Second)
			r.unregister <- client
		}
	}
}

var chatRoom = NewChatRoom()

func joinHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing client id", http.StatusBadRequest)
		return
	}

	client := &Client{
		ID:      id,
		MsgChan: make(chan ChatMsg, 100),
	}
	chatRoom.register <- client

	chatRoom.broadcast <- ChatMsg{
		Client:   id,
		Message:  " joined the chat",
		IsSystem: true,
		IsActive: true,
		Time:     time.Now(),
	}

	w.Write([]byte("Joined chat room"))
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	message := r.URL.Query().Get("message")
	if id == "" || message == "" {
		http.Error(w, "Missing id or message", http.StatusBadRequest)
		return
	}

	chatRoom.mutex.Lock()
	chatRoom.lastSeen[id] = time.Now()
	chatRoom.mutex.Unlock()

	chatRoom.broadcast <- ChatMsg{
		Client:   id,
		Message:  message,
		IsSystem: false,
		IsActive: true,
		Time:     time.Now(),
	}
	w.Write([]byte("Message sent"))
}

func leaveHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing client id", http.StatusBadRequest)
		return
	}

	chatRoom.mutex.RLock()
	client, exists := chatRoom.clients[id]
	chatRoom.mutex.RUnlock()
	if exists {
		chatRoom.mutex.Lock()
		chatRoom.lastSeen[id] = time.Now()
		chatRoom.disconnectTime[id] = time.Now()
		chatRoom.mutex.Unlock()

		chatRoom.broadcast <- ChatMsg{
			Client:   id,
			Message:  " left the chat",
			IsSystem: true,
			IsActive: false,
			Time:     time.Now(),
		}
		time.Sleep(2 * time.Second)
		chatRoom.unregister <- client
		w.Write([]byte("Left chat room"))
	} else {
		http.Error(w, "Client not found", http.StatusNotFound)
	}
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing client id", http.StatusBadRequest)
		return
	}

	chatRoom.mutex.RLock()
	client, exists := chatRoom.clients[id]
	chatRoom.mutex.RUnlock()
	if !exists {
		http.Error(w, "Client not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	notify := r.Context().Done()

	chatRoom.mutex.RLock()
	history := []ChatMsg{}
	// lastSeen := chatRoom.lastSeen[id]
	lastDisconnect, hasLeft := chatRoom.disconnectTime[id]
	if !hasLeft {
		lastDisconnect = time.Time{}
	}
	for _, msg := range chatRoom.history {
		if msg.Time.After(lastDisconnect) {
			history = append(history, msg)
		}
	}
	chatRoom.mutex.RUnlock()

	for _, msg := range history {
		data, _ := json.Marshal(msg)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-notify:
				return
			case <-ticker.C:
				fmt.Fprint(w, ": ping\n\n")
				flusher.Flush()
			}
		}
	}()

	for {
		select {
		case msg := <-client.MsgChan:
			data, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-notify:
			log.Printf("%s connection closed", id)
			return
		}
	}
}

func main() {
	http.HandleFunc("/join", joinHandler)
	http.HandleFunc("/send", sendHandler)
	http.HandleFunc("/leave", leaveHandler)
	http.HandleFunc("/messages", streamHandler)

	// http.Handle("/", http.FileServer(http.Dir("./")))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := content.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "Index file not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	fmt.Println("Chat server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
