package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Player はプレイヤーの状態を表す構造体
// サーバーがこれを保持し、クライアントはこれを受け取って表示するだけ
type Player struct {
	ID        string  `json:"id"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Direction string  `json:"direction"`
	Color     string  `json:"color"`
}

// ClientMessage はクライアントから受け取るメッセージ
type ClientMessage struct {
	Type      string `json:"type"`
	Direction string `json:"direction"`
}

// ServerMessage はクライアントへ送るメッセージ
type ServerMessage struct {
	Type    string              `json:"type"`
	Players map[string]*Player  `json:"players"`
	MyID    string              `json:"myId,omitempty"` // 自分のIDをack時に通知
}

// ゲームのグローバル状態
// サーバーが「真実」として保持する
var (
	players = make(map[string]*Player)
	clients = make(map[string]*websocket.Conn)
	mu      sync.RWMutex
	counter int
)

// プレイヤーに割り当てる色のリスト
var playerColors = []string{
	"#ff6b6b", "#4ecdc4", "#45b7d1", "#96ceb4",
	"#ffeaa7", "#dda0dd", "#98d8c8", "#f7dc6f",
	"#a29bfe", "#fd79a8",
}

// 移動速度（1tick あたりのピクセル数）
const moveSpeed = 5.0

// フィールドサイズ
const fieldWidth = 800.0
const fieldHeight = 600.0

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Static("/", "../client")
	e.GET("/ws", handleWebSocket)

	fmt.Println("サーバー起動: http://localhost:8080")
	log.Fatal(e.Start(":8080"))
}

// broadcast は現在のゲーム状態を全クライアントに送信する
func broadcast() {
	mu.RLock()
	// 状態のスナップショットを作成
	playersCopy := make(map[string]*Player, len(players))
	for id, p := range players {
		copy := *p
		playersCopy[id] = &copy
	}
	clientsCopy := make(map[string]*websocket.Conn, len(clients))
	for id, conn := range clients {
		clientsCopy[id] = conn
	}
	mu.RUnlock()

	msg := ServerMessage{
		Type:    "state",
		Players: playersCopy,
	}
	data, _ := json.Marshal(msg)

	for _, conn := range clientsCopy {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

// movePlayer は入力方向に応じてプレイヤーを移動させる
// これがサーバー側の「真実の更新」
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

	// 画面外に出たら反対側から出てくる（ラップアラウンド）
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

func handleWebSocket(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// プレイヤーIDと色を割り当てる
	mu.Lock()
	counter++
	playerID := fmt.Sprintf("player-%d", counter)
	colorIdx := (counter - 1) % len(playerColors)

	// 新しいプレイヤーを登録（サーバーが初期状態を決める）
	player := &Player{
		ID:        playerID,
		X:         float64(100 + (counter-1)*50), // 少しずらして配置
		Y:         300,
		Direction: "right",
		Color:     playerColors[colorIdx],
	}
	players[playerID] = player
	clients[playerID] = conn
	mu.Unlock()

	log.Printf("プレイヤー接続: %s", playerID)

	// 自分のIDを通知する
	ack, _ := json.Marshal(ServerMessage{
		Type: "state",
		MyID: playerID,
		Players: map[string]*Player{},
	})
	conn.WriteMessage(websocket.TextMessage, ack)

	// 現在の状態を全員にブロードキャスト
	broadcast()

	// 切断時の処理
	defer func() {
		mu.Lock()
		delete(players, playerID)
		delete(clients, playerID)
		mu.Unlock()
		log.Printf("プレイヤー切断: %s", playerID)
		broadcast()
	}()

	// メッセージ受信ループ
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
			// 入力を受け取ったら、サーバーが状態を更新する
			if msg.Direction == "up" || msg.Direction == "down" ||
				msg.Direction == "left" || msg.Direction == "right" {

				mu.Lock()
				if p, ok := players[playerID]; ok {
					// 方向を更新して即座に1歩移動
					p.Direction = msg.Direction
					movePlayer(p)
				}
				mu.Unlock()

				// 全員に最新状態を配信
				broadcast()
			}
		}
	}

	return nil
}
