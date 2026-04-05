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

// フィールドとグリッドの設定
const (
	fieldWidth  = 800
	fieldHeight = 600
	gridSize    = 20                      // 1マスのサイズ（px）
	tickRate    = 150 * time.Millisecond  // 1秒あたり約7tick（蛇ゲームらしい速度）
	initialLen  = 5                       // 蛇の初期の長さ（マス数）
	foodCount   = 5                       // フィールド上のエサ数
)

// Point はグリッド上の1点
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Snake は1匹の蛇
type Snake struct {
	ID        string  `json:"id"`
	Body      []Point `json:"body"`      // body[0] が頭
	Direction string  `json:"direction"` // "up"/"down"/"left"/"right"
	Color     string  `json:"color"`
	Score     int     `json:"score"` // 食べたエサ数
}

// Food はエサ
type Food struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ClientMessage はクライアントから受け取るメッセージ
type ClientMessage struct {
	Type      string `json:"type"`
	Direction string `json:"direction"`
}

// GameState はゲーム全体の状態（これをクライアントに送る）
type GameState struct {
	Type   string            `json:"type"`
	Snakes map[string]*Snake `json:"snakes"`
	Foods  []Food            `json:"foods"`
	MyID   string            `json:"myId,omitempty"`
}

var (
	snakes  = make(map[string]*Snake)
	clients = make(map[string]*Client)
	foods   []Food
	mu      sync.RWMutex
	counter int
)

var playerColors = []string{
	"#ff6b6b", "#4ecdc4", "#45b7d1", "#96ceb4",
	"#ffeaa7", "#dda0dd", "#98d8c8", "#f7dc6f",
	"#a29bfe", "#fd79a8",
}

func main() {
	// エサを初期配置
	for i := 0; i < foodCount; i++ {
		foods = append(foods, spawnFood())
	}

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Static("/", "../client")
	e.GET("/ws", handleWebSocket)

	go gameLoop()

	fmt.Println("サーバー起動: http://localhost:8080")
	log.Fatal(e.Start(":8080"))
}

// spawnFood はランダムな位置にエサを生成する（グリッドに合わせる）
func spawnFood() Food {
	cols := fieldWidth / gridSize
	rows := fieldHeight / gridSize
	return Food{
		X: float64(rand.Intn(cols) * gridSize),
		Y: float64(rand.Intn(rows) * gridSize),
	}
}

// createSnake は新しい蛇を初期化する
func createSnake(id, color string, startX, startY float64) *Snake {
	body := make([]Point, initialLen)
	// 右向きで配置（頭が右、尻尾が左）
	for i := 0; i < initialLen; i++ {
		body[i] = Point{X: startX - float64(i)*gridSize, Y: startY}
	}
	return &Snake{
		ID:        id,
		Body:      body,
		Direction: "right",
		Color:     color,
		Score:     0,
	}
}

// isOppositeDirection は反対方向への転換かどうかをチェックする
func isOppositeDirection(current, next string) bool {
	opposites := map[string]string{
		"up": "down", "down": "up",
		"left": "right", "right": "left",
	}
	return opposites[current] == next
}

// gameLoop はメインのゲームループ
func gameLoop() {
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	for {
		<-ticker.C
		updateGame()
		broadcastState()
	}
}

// updateGame は毎 tick ゲーム状態を更新する
func updateGame() {
	mu.Lock()
	defer mu.Unlock()

	for _, snake := range snakes {
		moveSnake(snake)
	}
}

// moveSnake は蛇を1マス前進させる
func moveSnake(snake *Snake) {
	if len(snake.Body) == 0 {
		return
	}

	// 現在の頭の位置
	head := snake.Body[0]

	// 方向に従って新しい頭の座標を計算
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

	// ラップアラウンド（画面外に出たら反対側へ）
	if newHead.X < 0 {
		newHead.X = float64(fieldWidth - gridSize)
	}
	if newHead.X >= fieldWidth {
		newHead.X = 0
	}
	if newHead.Y < 0 {
		newHead.Y = float64(fieldHeight - gridSize)
	}
	if newHead.Y >= fieldHeight {
		newHead.Y = 0
	}

	// 新しい頭を先頭に追加
	snake.Body = append([]Point{newHead}, snake.Body...)

	// エサとの衝突チェック
	ate := false
	for i, food := range foods {
		if food.X == newHead.X && food.Y == newHead.Y {
			// エサを食べた！
			snake.Score++
			ate = true
			// 食べたエサを新しいエサに置き換える
			foods[i] = spawnFood()
			break
		}
	}

	if !ate {
		// 食べていなければ尻尾を削除（長さを維持）
		snake.Body = snake.Body[:len(snake.Body)-1]
	}
	// 食べていれば尻尾を削除しない（長さが1増える = 成長）
}

// broadcastState は全クライアントにゲーム状態を送信する
func broadcastState() {
	mu.RLock()
	snakesCopy := make(map[string]*Snake, len(snakes))
	for id, s := range snakes {
		sCopy := *s
		bodyCopy := make([]Point, len(s.Body))
		copy(bodyCopy, s.Body)
		sCopy.Body = bodyCopy
		snakesCopy[id] = &sCopy
	}
	foodsCopy := make([]Food, len(foods))
	copy(foodsCopy, foods)
	clientsCopy := make(map[string]*Client, len(clients))
	for id, c := range clients {
		clientsCopy[id] = c
	}
	mu.RUnlock()

	if len(clientsCopy) == 0 {
		return
	}

	msg := GameState{
		Type:   "state",
		Snakes: snakesCopy,
		Foods:  foodsCopy,
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
	snakeID := fmt.Sprintf("snake-%d", counter)
	colorIdx := (counter - 1) % len(playerColors)

	// 初期位置をずらして配置
	cols := fieldWidth / gridSize
	startCol := ((counter - 1) * 8) % (cols - initialLen)
	startX := float64((startCol + initialLen) * gridSize)
	startY := float64(((counter-1)*5)%(fieldHeight/gridSize-1) * gridSize + gridSize)

	snake := createSnake(snakeID, playerColors[colorIdx], startX, startY)
	snakes[snakeID] = snake
	clients[snakeID] = client
	mu.Unlock()

	log.Printf("蛇が接続: %s", snakeID)

	// 自分のIDを通知
	ack, err := json.Marshal(GameState{
		Type:   "state",
		MyID:   snakeID,
		Snakes: map[string]*Snake{},
		Foods:  []Food{},
	})
	if err != nil {
		log.Printf("ack: json.Marshal エラー: %v", err)
	} else if err := client.writeText(ack); err != nil {
		log.Printf("ack 送信エラー: %v", err)
	}

	defer func() {
		mu.Lock()
		delete(snakes, snakeID)
		delete(clients, snakeID)
		mu.Unlock()
		log.Printf("蛇が切断: %s", snakeID)
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg ClientMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		if msg.Type == "input" {
			if msg.Direction == "up" || msg.Direction == "down" ||
				msg.Direction == "left" || msg.Direction == "right" {
				mu.Lock()
				if s, ok := snakes[snakeID]; ok {
					// 反対方向への転換は禁止
					if !isOppositeDirection(s.Direction, msg.Direction) {
						s.Direction = msg.Direction
					}
				}
				mu.Unlock()
			}
		}
	}

	return nil
}
