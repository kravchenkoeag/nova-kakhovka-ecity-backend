package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"nova-kakhovka-ecity/internal/models"
	"nova-kakhovka-ecity/pkg/auth"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// В продакшене здесь должна быть проверка origin
		return true
	},
}

type Hub struct {
	// Зарегистрированные клиенты по группам
	clients map[primitive.ObjectID]map[*Client]bool

	// Канал для регистрации клиентов
	register chan *Client

	// Канал для отмены регистрации клиентов
	unregister chan *Client

	// Входящие сообщения от клиентов
	broadcast chan *BroadcastMessage

	mutex sync.RWMutex
}

type Client struct {
	hub     *Hub
	conn    *websocket.Conn
	send    chan []byte
	userID  primitive.ObjectID
	groupID primitive.ObjectID
}

type BroadcastMessage struct {
	GroupID primitive.ObjectID `json:"group_id"`
	Message *models.Message    `json:"message"`
}

type WSMessage struct {
	Type    string      `json:"type"`
	GroupID string      `json:"group_id,omitempty"`
	Data    interface{} `json:"data"`
}

type WebSocketHandler struct {
	hub               *Hub
	jwtManager        *auth.JWTManager
	groupCollection   *mongo.Collection
	messageCollection *mongo.Collection
}

func NewWebSocketHandler(jwtManager *auth.JWTManager, groupCollection, messageCollection *mongo.Collection) *WebSocketHandler {
	hub := &Hub{
		clients:    make(map[primitive.ObjectID]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *BroadcastMessage),
	}

	return &WebSocketHandler{
		hub:               hub,
		jwtManager:        jwtManager,
		groupCollection:   groupCollection,
		messageCollection: messageCollection,
	}
}

func (h *WebSocketHandler) StartHub() {
	go h.hub.run()
}

func (hub *Hub) run() {
	for {
		select {
		case client := <-hub.register:
			hub.mutex.Lock()
			if hub.clients[client.groupID] == nil {
				hub.clients[client.groupID] = make(map[*Client]bool)
			}
			hub.clients[client.groupID][client] = true
			hub.mutex.Unlock()
			log.Printf("Client registered for group %s", client.groupID.Hex())

		case client := <-hub.unregister:
			hub.mutex.Lock()
			if clients, ok := hub.clients[client.groupID]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.send)
					if len(clients) == 0 {
						delete(hub.clients, client.groupID)
					}
				}
			}
			hub.mutex.Unlock()
			log.Printf("Client unregistered from group %s", client.groupID.Hex())

		case message := <-hub.broadcast:
			hub.mutex.RLock()
			clients := hub.clients[message.GroupID]
			hub.mutex.RUnlock()

			messageBytes, err := json.Marshal(WSMessage{
				Type: "new_message",
				Data: message.Message,
			})
			if err != nil {
				log.Printf("Error marshaling message: %v", err)
				continue
			}

			for client := range clients {
				select {
				case client.send <- messageBytes:
				default:
					hub.mutex.Lock()
					close(client.send)
					delete(clients, client)
					if len(clients) == 0 {
						delete(hub.clients, message.GroupID)
					}
					hub.mutex.Unlock()
				}
			}
		}
	}
}

func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	// Получаем JWT токен из query параметра
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Token is required",
		})
		return
	}

	// Валидируем токен
	claims, err := h.jwtManager.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid token",
		})
		return
	}

	// Получаем ID группы
	groupID := c.Query("group_id")
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Group ID is required",
		})
		return
	}

	groupIDObj, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid group ID",
		})
		return
	}

	// Проверяем, является ли пользователь участником группы
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	count, err := h.groupCollection.CountDocuments(ctx, bson.M{
		"_id":     groupIDObj,
		"members": bson.M{"$in": []primitive.ObjectID{claims.UserID}},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database error",
		})
		return
	}
	if count == 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "User is not a member of this group",
		})
		return
	}

	// Устанавливаем WebSocket соединение
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:     h.hub,
		conn:    conn,
		send:    make(chan []byte, 256),
		userID:  claims.UserID,
		groupID: groupIDObj,
	}

	client.hub.register <- client

	// Запускаем горутины для чтения и записи
	go client.writePump()
	go client.readPump(h)
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

func (c *Client) readPump(h *WebSocketHandler) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		var wsMsg WSMessage
		err := c.conn.ReadJSON(&wsMsg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Обрабатываем разные типы сообщений
		switch wsMsg.Type {
		case "send_message":
			h.handleSendMessage(c, wsMsg.Data)
		case "typing":
			h.handleTyping(c, wsMsg.GroupID)
		case "ping":
			c.send <- []byte(`{"type": "pong"}`)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Добавляем все ожидающие сообщения в текущее сообщение
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
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

func (h *WebSocketHandler) handleSendMessage(client *Client, data interface{}) {
	// Преобразуем data в map для удобства работы
	messageData, ok := data.(map[string]interface{})
	if !ok {
		log.Printf("Invalid message data format")
		return
	}

	content, ok := messageData["content"].(string)
	if !ok || content == "" {
		log.Printf("Invalid or empty message content")
		return
	}

	messageType, ok := messageData["type"].(string)
	if !ok {
		messageType = "text"
	}

	mediaURL, _ := messageData["media_url"].(string)

	// Создаем новое сообщение
	now := time.Now()
	message := models.Message{
		GroupID:   client.groupID,
		UserID:    client.userID,
		Content:   content,
		Type:      messageType,
		MediaURL:  mediaURL,
		IsEdited:  false,
		IsDeleted: false,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Сохраняем сообщение в базу данных
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.messageCollection.InsertOne(ctx, message)
	if err != nil {
		log.Printf("Error saving message: %v", err)
		return
	}

	message.ID = result.InsertedID.(primitive.ObjectID)

	// Отправляем сообщение всем участникам группы
	broadcastMsg := &BroadcastMessage{
		GroupID: client.groupID,
		Message: &message,
	}

	h.hub.broadcast <- broadcastMsg
}

func (h *WebSocketHandler) handleTyping(client *Client, groupID string) {
	if groupID == "" {
		groupID = client.groupID.Hex()
	}

	groupIDObj, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		return
	}

	// Отправляем уведомление о печати всем участникам группы, кроме отправителя
	h.hub.mutex.RLock()
	clients := h.hub.clients[groupIDObj]
	h.hub.mutex.RUnlock()

	typingMsg, _ := json.Marshal(WSMessage{
		Type: "user_typing",
		Data: map[string]interface{}{
			"user_id":  client.userID.Hex(),
			"group_id": groupID,
		},
	})

	for c := range clients {
		if c.userID != client.userID {
			select {
			case c.send <- typingMsg:
			default:
				close(c.send)
				delete(clients, c)
			}
		}
	}
}

// Метод для отправки системных уведомлений
func (h *WebSocketHandler) SendSystemMessage(groupID primitive.ObjectID, messageType string, data interface{}) {
	h.hub.mutex.RLock()
	clients := h.hub.clients[groupID]
	h.hub.mutex.RUnlock()

	systemMsg, err := json.Marshal(WSMessage{
		Type: messageType,
		Data: data,
	})
	if err != nil {
		log.Printf("Error marshaling system message: %v", err)
		return
	}

	for client := range clients {
		select {
		case client.send <- systemMsg:
		default:
			close(client.send)
			delete(clients, client)
		}
	}
}
