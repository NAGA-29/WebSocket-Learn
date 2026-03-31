package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const (
	MaxPlayersPerRoom = 10
	tickRate          = 150 * time.Millisecond
	fieldWidth        = 800
	fieldHeight       = 600
	gridSize          = 20
	initialSnakeLen   = 5
	foodCount         = 5
)

// ----- データ構造 -----

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Snake struct {
	ID        string  `json:"id"`
	Body      []Point `json:"body"`
	Direction string  `json:"direction"`
	Color     string  `json:"color"`
	Score     int     `json:"score"`
}

type Food struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type ClientMessage struct {
	Type      string `json:"type"`
	Direction string `json:"direction"`
}

// RoomInfo はクライアントに送る部屋情報
type RoomInfo struct {
	RoomID string `json:"roomId"`
	Count  int    `json:"count"`
}

type GameState struct {
	Type    string             `json:"type"`
	Snakes  map[string]*Snake  `json:"snakes"`
	Foods   []Food             `json:"foods"`
	MyID    string             `json:"myId,omitempty"`
	RoomID  string             `json:"roomId"`
	Count   int                `json:"count"`
}

// ----- Room -----

type Room struct {
	ID      string
	Snakes  map[string]*Snake
	Clients map[string]*websocket.Conn
	Foods   []Food
	mu      sync.RWMutex
	stopCh  chan struct{}
	running bool
}

func newRoom(id string) *Room {
	r := &Room{
		ID:      id,
		Snakes:  make(map[string]*Snake),
		Clients: make(map[string]*websocket.Conn),
		stopCh:  make(chan struct{}),
	}
	// エサを初期配置
	for i := 0; i < foodCount; i++ {
		r.Foods = append(r.Foods, spawnFood())
	}
	return r
}

func spawnFood() Food {
	return Food{
		X: float64(rand.Intn(fieldWidth/gridSize) * gridSize),
		Y: float64(rand.Intn(fieldHeight/gridSize) * gridSize),
	}
}

// Start はこの部屋のゲームループを起動する（1部屋1回だけ呼ぶ）
func (r *Room) Start() {
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	log.Printf("部屋 %s: ゲームループ開始", r.ID)

	for {
		select {
		case <-ticker.C:
			r.update()
			r.broadcastState()
		case <-r.stopCh:
			log.Printf("部屋 %s: ゲームループ停止", r.ID)
			return
		}
	}
}

func (r *Room) update() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, snake := range r.Snakes {
		r.moveSnake(snake)
	}
}

func (r *Room) moveSnake(snake *Snake) {
	if len(snake.Body) == 0 {
		return
	}
	head := snake.Body[0]
	var newHead Point
	switch snake.Direction {
	case "up":
		newHead = Point{X: head.X, Y: head.Y - gridSize}
	case "down":
		newHead = Point{X: head.X, Y: head.Y + gridSize}
	case "left":
		newHead = Point{X: head.X - gridSize, Y: head.Y}
	case "right":
		newHead = Point{X: head.X + gridSize, Y: head.Y}
	}
	if newHead.X < 0 { newHead.X = fieldWidth - gridSize }
	if newHead.X >= fieldWidth { newHead.X = 0 }
	if newHead.Y < 0 { newHead.Y = fieldHeight - gridSize }
	if newHead.Y >= fieldHeight { newHead.Y = 0 }

	snake.Body = append([]Point{newHead}, snake.Body...)

	ate := false
	for i, food := range r.Foods {
		if food.X == newHead.X && food.Y == newHead.Y {
			snake.Score++
			ate = true
			r.Foods[i] = spawnFood()
			break
		}
	}
	if !ate && len(snake.Body) > initialSnakeLen {
		snake.Body = snake.Body[:len(snake.Body)-1]
	} else if !ate {
		snake.Body = snake.Body[:len(snake.Body)-1]
	}
}

func (r *Room) broadcastState() {
	r.mu.RLock()
	snakesCopy := make(map[string]*Snake, len(r.Snakes))
	for id, s := range r.Snakes {
		sc := *s
		bc := make([]Point, len(s.Body))
		copy(bc, s.Body)
		sc.Body = bc
		snakesCopy[id] = &sc
	}
	foodsCopy := make([]Food, len(r.Foods))
	copy(foodsCopy, r.Foods)
	clientsCopy := make(map[string]*websocket.Conn, len(r.Clients))
	for id, conn := range r.Clients {
		clientsCopy[id] = conn
	}
	count := len(r.Snakes)
	roomID := r.ID
	r.mu.RUnlock()

	if len(clientsCopy) == 0 {
		return
	}

	msg := GameState{
		Type:   "state",
		Snakes: snakesCopy,
		Foods:  foodsCopy,
		RoomID: roomID,
		Count:  count,
	}
	data, _ := json.Marshal(msg)
	for _, conn := range clientsCopy {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

func (r *Room) addPlayer(id string, conn *websocket.Conn, snake *Snake) {
	r.mu.Lock()
	r.Snakes[id] = snake
	r.Clients[id] = conn
	r.mu.Unlock()
}

func (r *Room) removePlayer(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.Snakes, id)
	delete(r.Clients, id)
	return len(r.Snakes) == 0
}

func (r *Room) PlayerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Snakes)
}

// ----- グローバル管理 -----

var (
	rooms      = make(map[string]*Room)
	globalMu   sync.Mutex
	roomCounter int
	snakeCounter int
)

var playerColors = []string{
	"#ff6b6b", "#4ecdc4", "#45b7d1", "#96ceb4",
	"#ffeaa7", "#dda0dd", "#98d8c8", "#f7dc6f",
	"#a29bfe", "#fd79a8",
}

// findOrCreateRoom は空き部屋を探す、なければ作る
func findOrCreateRoom() *Room {
	globalMu.Lock()
	defer globalMu.Unlock()

	for _, room := range rooms {
		if room.PlayerCount() < MaxPlayersPerRoom {
			return room
		}
	}

	// 新しい部屋を作る
	roomCounter++
	roomID := fmt.Sprintf("room-%d", roomCounter)
	room := newRoom(roomID)
	rooms[roomID] = room

	// 部屋のゲームループを起動（1回だけ）
	room.running = true
	go room.Start()

	log.Printf("新しい部屋を作成: %s", roomID)
	return room
}

// deleteRoomIfEmpty は部屋が空なら削除する
func deleteRoomIfEmpty(roomID string) {
	globalMu.Lock()
	defer globalMu.Unlock()

	if room, ok := rooms[roomID]; ok {
		if room.PlayerCount() == 0 {
			close(room.stopCh) // ゲームループを止める
			delete(rooms, roomID)
			log.Printf("空になった部屋を削除: %s", roomID)
		}
	}
}

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Static("/", "../client")
	e.GET("/ws", handleWebSocket)

	fmt.Println("サーバー起動: http://localhost:8080")
	fmt.Printf("1部屋最大: %d人\n", MaxPlayersPerRoom)
	log.Fatal(e.Start(":8080"))
}

func handleWebSocket(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 空き部屋を探すか新規作成
	room := findOrCreateRoom()

	// プレイヤーを作成
	globalMu.Lock()
	snakeCounter++
	snakeID := fmt.Sprintf("snake-%d", snakeCounter)
	colorIdx := (snakeCounter - 1) % len(playerColors)
	globalMu.Unlock()

	body := []Point{}
	for i := 0; i < initialSnakeLen; i++ {
		body = append(body, Point{
			X: float64((10-i) * gridSize),
			Y: float64(rand.Intn(fieldHeight/gridSize-1)*gridSize + gridSize),
		})
	}
	snake := &Snake{
		ID:        snakeID,
		Body:      body,
		Direction: "right",
		Color:     playerColors[colorIdx],
	}

	room.addPlayer(snakeID, conn, snake)
	log.Printf("プレイヤー %s が部屋 %s に入室（現在%d人）", snakeID, room.ID, room.PlayerCount())

	// 自分のID と 部屋IDを通知
	ack, _ := json.Marshal(GameState{
		Type:   "state",
		MyID:   snakeID,
		RoomID: room.ID,
		Snakes: map[string]*Snake{},
		Foods:  []Food{},
	})
	conn.WriteMessage(websocket.TextMessage, ack)

	// 切断時の処理
	defer func() {
		empty := room.removePlayer(snakeID)
		log.Printf("プレイヤー %s が部屋 %s を退室", snakeID, room.ID)
		if empty {
			deleteRoomIfEmpty(room.ID)
		}
	}()

	// 入力受付ループ
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg ClientMessage
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		if msg.Type == "input" {
			opposites := map[string]string{
				"up": "down", "down": "up", "left": "right", "right": "left",
			}
			room.mu.Lock()
			if s, ok := room.Snakes[snakeID]; ok && opposites[s.Direction] != msg.Direction {
				s.Direction = msg.Direction
			}
			room.mu.Unlock()
		}
	}
	return nil
}
