package main

import (
	"encoding/json"
	"fmt"
	"log"
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
	tickRate    = 100 * time.Millisecond
	fieldWidth  = 800
	fieldHeight = 600
	gridSize    = 20
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// FullSnake は全量送信用の蛇データ
type FullSnake struct {
	ID        string  `json:"id"`
	Body      []Point `json:"body"`
	Direction string  `json:"direction"`
	Color     string  `json:"color"`
}

// LightSnake は最適化版の蛇データ（フィールド名を短縮）
// "body" → "b", "direction" → "d", "color" → "c"
type LightSnake struct {
	ID string  `json:"i"`
	B  []Point `json:"b"`
	D  string  `json:"d"`
	C  string  `json:"c"`
}

// FullState は全量送信のゲーム状態
type FullState struct {
	Type   string               `json:"type"`
	Snakes map[string]*FullSnake `json:"snakes"`
	MyID   string               `json:"myId,omitempty"`
}

// LightState は最適化版のゲーム状態
type LightState struct {
	T      string               `json:"t"`      // type
	S      map[string]*LightSnake `json:"s"`    // snakes
	MyID   string               `json:"myId,omitempty"`
}

// 統計情報をクライアントに送る（デバッグ用）
type StatsMessage struct {
	Type        string  `json:"type"`
	FullSize    int     `json:"fullSize"`    // 全量のバイト数
	LightSize   int     `json:"lightSize"`   // 軽量版のバイト数
	PlayerCount int     `json:"playerCount"` // 現在のプレイヤー数
	TickCount   int64   `json:"tickCount"`
}

var (
	snakes    = make(map[string]*FullSnake)
	clients   = make(map[string]*websocket.Conn)
	mu        sync.RWMutex
	counter   int
	tickCount int64
)

var playerColors = []string{
	"#ff6b6b", "#4ecdc4", "#45b7d1", "#96ceb4",
	"#ffeaa7", "#dda0dd", "#98d8c8", "#f7dc6f",
}

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Static("/", "../client")
	e.GET("/ws", handleWebSocket)

	go gameLoop()

	fmt.Println("サーバー起動: http://localhost:8080")
	fmt.Println("全量送信と軽量版のサイズ比較ができます")
	log.Fatal(e.Start(":8080"))
}

func gameLoop() {
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()
	for {
		<-ticker.C
		updateGame()
		broadcastWithStats()
	}
}

func updateGame() {
	mu.Lock()
	defer mu.Unlock()
	tickCount++
	for _, s := range snakes {
		moveSnake(s)
	}
}

func moveSnake(s *FullSnake) {
	if len(s.Body) == 0 {
		return
	}
	h := s.Body[0]
	var nh Point
	switch s.Direction {
	case "up":
		nh = Point{h.X, h.Y - gridSize}
	case "down":
		nh = Point{h.X, h.Y + gridSize}
	case "left":
		nh = Point{h.X - gridSize, h.Y}
	case "right":
		nh = Point{h.X + gridSize, h.Y}
	}
	if nh.X < 0 { nh.X = fieldWidth - gridSize }
	if nh.X >= fieldWidth { nh.X = 0 }
	if nh.Y < 0 { nh.Y = fieldHeight - gridSize }
	if nh.Y >= fieldHeight { nh.Y = 0 }
	s.Body = append([]Point{nh}, s.Body...)
	if len(s.Body) > 10 {
		s.Body = s.Body[:10]
	}
}

// broadcastWithStats は全量と軽量両方を計算してサイズ比較統計も送る
func broadcastWithStats() {
	mu.RLock()
	sc := make(map[string]*FullSnake, len(snakes))
	for id, s := range snakes {
		cp := *s
		bc := make([]Point, len(s.Body))
		copy(bc, s.Body)
		cp.Body = bc
		sc[id] = &cp
	}
	cc := make(map[string]*websocket.Conn, len(clients))
	for id, conn := range clients {
		cc[id] = conn
	}
	count := len(snakes)
	tick := tickCount
	mu.RUnlock()

	if len(cc) == 0 {
		return
	}

	// --- 全量送信版 ---
	fullMsg := FullState{Type: "state", Snakes: sc}
	fullData, _ := json.Marshal(fullMsg)

	// --- 軽量版（フィールド名短縮）---
	lightSnakes := make(map[string]*LightSnake, len(sc))
	for id, s := range sc {
		lightSnakes[id] = &LightSnake{
			ID: s.ID,
			B:  s.Body,
			D:  s.Direction,
			C:  s.Color,
		}
	}
	lightMsg := LightState{T: "state", S: lightSnakes}
	lightData, _ := json.Marshal(lightMsg)

	// --- 統計情報 ---
	stats := StatsMessage{
		Type:        "stats",
		FullSize:    len(fullData),
		LightSize:   len(lightData),
		PlayerCount: count,
		TickCount:   tick,
	}
	statsData, _ := json.Marshal(stats)

	// サーバーでもログに出す（一定間隔で）
	if tick%50 == 0 {
		log.Printf("tick %d: full=%d bytes, light=%d bytes, ratio=%.1f%%, players=%d",
			tick, len(fullData), len(lightData),
			float64(len(lightData))/float64(len(fullData))*100,
			count)
	}

	// 全クライアントに送信（全量版 + 統計）
	for _, conn := range cc {
		conn.WriteMessage(websocket.TextMessage, fullData)
		conn.WriteMessage(websocket.TextMessage, statsData)
	}
}

type ClientMessage struct {
	Type      string `json:"type"`
	Direction string `json:"direction"`
}

func handleWebSocket(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	mu.Lock()
	counter++
	id := fmt.Sprintf("snake-%d", counter)
	color := playerColors[(counter-1)%len(playerColors)]
	body := []Point{}
	for i := 0; i < 10; i++ {
		body = append(body, Point{float64((10-i)*gridSize), float64((counter-1)%10*3*gridSize + gridSize)})
	}
	snakes[id] = &FullSnake{ID: id, Body: body, Direction: "right", Color: color}
	clients[id] = conn
	mu.Unlock()

	ack, _ := json.Marshal(FullState{Type: "state", MyID: id, Snakes: map[string]*FullSnake{}})
	conn.WriteMessage(websocket.TextMessage, ack)

	defer func() {
		mu.Lock()
		delete(snakes, id)
		delete(clients, id)
		mu.Unlock()
	}()

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
			opposites := map[string]string{"up": "down", "down": "up", "left": "right", "right": "left"}
			mu.Lock()
			if s, ok := snakes[id]; ok && opposites[s.Direction] != msg.Direction {
				s.Direction = msg.Direction
			}
			mu.Unlock()
		}
	}
	return nil
}
