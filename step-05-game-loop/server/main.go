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

// Player はプレイヤーの状態
type Player struct {
	ID        string  `json:"id"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Direction string  `json:"direction"`
	Color     string  `json:"color"`
}

// ClientMessage はクライアントからの入力
type ClientMessage struct {
	Type      string `json:"type"`
	Direction string `json:"direction"`
}

// ServerMessage はクライアントへのゲーム状態
type ServerMessage struct {
	Type      string             `json:"type"`
	Players   map[string]*Player `json:"players"`
	TickCount int64              `json:"tickCount"` // 何tick目か（デバッグ用）
	MyID      string             `json:"myId,omitempty"`
}

var (
	players   = make(map[string]*Player)
	clients   = make(map[string]*Client)
	mu        sync.RWMutex
	counter   int
	tickCount int64
)

var playerColors = []string{
	"#ff6b6b", "#4ecdc4", "#45b7d1", "#96ceb4",
	"#ffeaa7", "#dda0dd", "#98d8c8", "#f7dc6f",
}

const (
	moveSpeed   = 4.0
	fieldWidth  = 800.0
	fieldHeight = 600.0
	tickRate    = 100 * time.Millisecond // 100ms = 10 tick/秒
)

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Static("/", "../client")
	e.GET("/ws", handleWebSocket)

	// ゲームループをバックグラウンドで起動（main で1回だけ）
	go gameLoop()

	fmt.Println("サーバー起動: http://localhost:8080")
	fmt.Printf("tick レート: %v\n", tickRate)
	log.Fatal(e.Start(":8080"))
}

// gameLoop はゲームの心臓部：一定周期で状態を更新して配信する
func gameLoop() {
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	for {
		<-ticker.C // ticker がカウントするたびにここを通過する

		updateGame()     // ゲームの状態を更新
		broadcastState() // 全クライアントに配信
	}
}

// updateGame は毎 tick 呼ばれる状態更新処理
// 「物理エンジン」に相当する部分
func updateGame() {
	mu.Lock()
	defer mu.Unlock()

	tickCount++

	for _, player := range players {
		movePlayer(player)
	}
}

// movePlayer はプレイヤーを1 tick 分前進させる
func movePlayer(p *Player) {
	switch p.Direction {
	case "up":
		p.Y -= moveSpeed
	case "down":
		p.Y += moveSpeed
	case "left":
		p.X -= moveSpeed
	case "right":
		p.X += moveSpeed
	}

	// ラップアラウンド（画面の端を超えたら反対側へ）
	if p.X < 0 {
		p.X = fieldWidth
	}
	if p.X > fieldWidth {
		p.X = 0
	}
	if p.Y < 0 {
		p.Y = fieldHeight
	}
	if p.Y > fieldHeight {
		p.Y = 0
	}
}

// broadcastState は全クライアントにゲーム状態を送信する
func broadcastState() {
	mu.RLock()
	playersCopy := make(map[string]*Player, len(players))
	for id, p := range players {
		cp := *p
		playersCopy[id] = &cp
	}
	clientsCopy := make(map[string]*Client, len(clients))
	for id, c := range clients {
		clientsCopy[id] = c
	}
	tick := tickCount
	mu.RUnlock()

	if len(clientsCopy) == 0 {
		return // 接続中のクライアントがいなければ何もしない
	}

	msg := ServerMessage{
		Type:      "state",
		Players:   playersCopy,
		TickCount: tick,
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

	// プレイヤー登録
	mu.Lock()
	counter++
	playerID := fmt.Sprintf("player-%d", counter)
	colorIdx := (counter - 1) % len(playerColors)
	player := &Player{
		ID:        playerID,
		X:         float64(100 + ((counter-1)%8)*80),
		Y:         300,
		Direction: "right",
		Color:     playerColors[colorIdx],
	}
	players[playerID] = player
	clients[playerID] = client
	mu.Unlock()

	log.Printf("プレイヤー接続: %s", playerID)

	// 自分のIDを通知
	ack, err := json.Marshal(ServerMessage{
		Type:    "state",
		MyID:    playerID,
		Players: map[string]*Player{},
	})
	if err != nil {
		log.Printf("ack: json.Marshal エラー: %v", err)
	} else if err := client.writeText(ack); err != nil {
		log.Printf("ack 送信エラー: %v", err)
	}

	// 切断処理
	defer func() {
		mu.Lock()
		delete(players, playerID)
		delete(clients, playerID)
		mu.Unlock()
		log.Printf("プレイヤー切断: %s", playerID)
	}()

	// 入力受付ループ（ゲームループとは独立して動く）
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg ClientMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		// 入力を受け取ったら「方向」だけ更新する
		// 実際の移動はゲームループが次のtickでやる
		if msg.Type == "input" {
			if msg.Direction == "up" || msg.Direction == "down" ||
				msg.Direction == "left" || msg.Direction == "right" {
				mu.Lock()
				if p, ok := players[playerID]; ok {
					p.Direction = msg.Direction // 方向だけ更新
				}
				mu.Unlock()
				// broadcastはしない！ゲームループが定期的にやる
			}
		}
	}

	return nil
}
