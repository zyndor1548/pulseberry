package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSManager struct {
	clients map[string][]*websocket.Conn
	mu      sync.RWMutex
}

func NewWSManager() *WSManager {
	return &WSManager{
		clients: make(map[string][]*websocket.Conn),
	}
}

func (m *WSManager) HandleWS(w http.ResponseWriter, r *http.Request) {
	paymentID := r.URL.Query().Get("payment_id")
	if paymentID == "" {
		http.Error(w, "payment_id is required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	rCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if cached, err := rdb.Get(rCtx, "payment_result:"+paymentID).Result(); err == nil && cached != "" {
		var result interface{}
		if err := json.Unmarshal([]byte(cached), &result); err == nil {
			conn.WriteJSON(result)
			log.Printf("Pushed cached result to new WS client for: %s", paymentID)
		}
	}

	m.mu.Lock()
	m.clients[paymentID] = append(m.clients[paymentID], conn)
	m.mu.Unlock()

	log.Printf("New WebSocket client subscribed to payment: %s", paymentID)

	go func() {
		defer func() {
			m.mu.Lock()
			conns := m.clients[paymentID]
			for i, c := range conns {
				if c == conn {
					m.clients[paymentID] = append(conns[:i], conns[i+1:]...)
					break
				}
			}
			if len(m.clients[paymentID]) == 0 {
				delete(m.clients, paymentID)
			}
			m.mu.Unlock()
			conn.Close()
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

func (m *WSManager) Notify(paymentID string, result interface{}) {
	m.mu.RLock()
	conns, exists := m.clients[paymentID]
	m.mu.RUnlock()

	if !exists {
		return
	}

	msg, err := json.Marshal(result)
	if err != nil {
		log.Printf("Failed to marshal notification: %v", err)
		return
	}

	for _, conn := range conns {
		err := conn.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			log.Printf("Failed to send WebSocket message: %v", err)
		}
	}
}

var wsManager = NewWSManager()
