package main

// Step 8 のサーバーは Step 7 と同じ構成で、tickRate と maxRandomDelayMs を調整して使う
// クライアント側の補間の実験に使うため、遅延設定は残してある

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

// ⚠️  開発用設定: 全オリジンを許可。本番では許可オリジンを限定すること。
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client は接続と書き込みロックをまとめた構造体。
//
// gorilla/websocket は「同一コネクションへの WriteMessage 系呼び出しは
// 同時に1つだけ」という制約がある。
// gameLoop の broadcastState と handleWebSocket の ack 送信が
// 同一 conn に並行して書き込む可能性があるため、writeMu で直列化する。
type Client struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// writeText は writeMu を取得してからメッセージを送る。
func (c *Client) writeText(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// 補間実験用の設定
// 意図的に遅い tickRate でも補間で滑らかに見えることを確認する
const (
	tickRate         = 100 * time.Millisecond // 変えて実験してみてください
	maxRandomDelayMs = 0                      // ランダム遅延（0=なし）
	fieldWidth       = 800
	fieldHeight      = 600
	gridSize         = 20
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Snake struct {
	ID        string  `json:"id"`
	Body      []Point `json:"body"`
	Direction string  `json:"direction"`
	Color     string  `json:"color"`
}

type GameState struct {
	Type       string            `json:"type"`
	Snakes     map[string]*Snake `json:"snakes"`
	TickCount  int64             `json:"tickCount"`
	ServerTime int64             `json:"serverTime"` // クライアントの補間計算に使う
	MyID       string            `json:"myId,omitempty"`
}

type ClientMessage struct {
	Type      string `json:"type"`
	Direction string `json:"direction"`
}

var (
	snakes  = make(map[string]*Snake)
	clients = make(map[string]*Client)
	mu      sync.RWMutex
	counter int
	tick    int64
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

	fmt.Printf("サーバー起動: http://localhost:8080\n")
	fmt.Printf("tick間隔: %v（補間実験用）\n", tickRate)
	log.Fatal(e.Start(":8080"))
}

func gameLoop() {
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	for {
		<-ticker.C
		updateGame()

		if maxRandomDelayMs > 0 {
			time.Sleep(time.Duration(rand.Intn(maxRandomDelayMs)) * time.Millisecond)
		}

		broadcastState()
	}
}

func updateGame() {
	mu.Lock()
	defer mu.Unlock()

	tick++
	for _, snake := range snakes {
		moveSnake(snake)
	}
}

func moveSnake(snake *Snake) {
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
	if newHead.X < 0 {
		newHead.X = fieldWidth - gridSize
	}
	if newHead.X >= fieldWidth {
		newHead.X = 0
	}
	if newHead.Y < 0 {
		newHead.Y = fieldHeight - gridSize
	}
	if newHead.Y >= fieldHeight {
		newHead.Y = 0
	}

	snake.Body = append([]Point{newHead}, snake.Body...)
	if len(snake.Body) > 5 {
		snake.Body = snake.Body[:5]
	}
}

func broadcastState() {
	mu.RLock()
	snakesCopy := make(map[string]*Snake, len(snakes))
	for id, s := range snakes {
		sc := *s
		bc := make([]Point, len(s.Body))
		copy(bc, s.Body)
		sc.Body = bc
		snakesCopy[id] = &sc
	}
	clientsCopy := make(map[string]*Client, len(clients))
	for id, c := range clients {
		clientsCopy[id] = c
	}
	t := tick
	mu.RUnlock()

	if len(clientsCopy) == 0 {
		return
	}

	msg := GameState{
		Type:       "state",
		Snakes:     snakesCopy,
		TickCount:  t,
		ServerTime: time.Now().UnixMilli(),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("broadcastState: json.Marshal エラー: %v", err)
		return
	}
	for _, client := range clientsCopy {
		if err := client.writeText(data); err != nil {
			log.Printf("broadcastState: 送信エラー: %v", err)
		}
	}
}

func handleWebSocket(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Client を作成
	client := &Client{conn: conn}

	mu.Lock()
	counter++
	id := fmt.Sprintf("snake-%d", counter)
	color := playerColors[(counter-1)%len(playerColors)]
	body := []Point{}
	for i := 0; i < 5; i++ {
		body = append(body, Point{
			X: float64((10 - i) * gridSize),
			Y: float64(((counter-1)%10)*3*gridSize + gridSize),
		})
	}
	snakes[id] = &Snake{ID: id, Body: body, Direction: "right", Color: color}
	clients[id] = client
	mu.Unlock()

	ack, err := json.Marshal(GameState{Type: "state", MyID: id, Snakes: map[string]*Snake{}})
	if err != nil {
		log.Printf("ack: json.Marshal エラー: %v", err)
	} else if err := client.writeText(ack); err != nil {
		log.Printf("ack 送信エラー: %v", err)
	}

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
