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

// ClientMessage はクライアントから受け取るメッセージの形式
type ClientMessage struct {
	Type      string `json:"type"`      // "input"
	Direction string `json:"direction"` // "up" / "down" / "left" / "right"
}

// ServerMessage はサーバーからクライアントへ送るメッセージの形式
type ServerMessage struct {
	Type     string            `json:"type"`     // "ack" または "state"
	PlayerID string            `json:"playerId"` // 自分のID（ack時）
	Inputs   map[string]string `json:"inputs"`   // 全プレイヤーの最新入力（state時）
}

// playerInputs は各プレイヤーの最新入力方向を保持するmap
// キー: プレイヤーID（接続ごとに割り当てる）
// 値: 最新の方向 ("up" / "down" / "left" / "right")
var playerInputs = make(map[string]string)

// clients は接続中のクライアント（プレイヤーID → WebSocket接続）
var clients = make(map[string]*websocket.Conn)

var mu sync.Mutex

// idCounter はシンプルな連番プレイヤーID生成用
var idCounter int

func generatePlayerID() string {
	idCounter++
	return fmt.Sprintf("player-%d", idCounter)
}

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Static("/", "../client")
	e.GET("/ws", handleWebSocket)

	fmt.Println("サーバー起動: http://localhost:8080")
	log.Fatal(e.Start(":8080"))
}

// broadcast は全クライアントに現在の全入力状態を送る
func broadcast() {
	mu.Lock()
	// 現在の inputs をコピー（ロック中の処理を最小限にするため）
	inputsCopy := make(map[string]string, len(playerInputs))
	for id, dir := range playerInputs {
		inputsCopy[id] = dir
	}
	clientsCopy := make(map[string]*websocket.Conn, len(clients))
	for id, conn := range clients {
		clientsCopy[id] = conn
	}
	mu.Unlock()

	msg := ServerMessage{
		Type:   "state",
		Inputs: inputsCopy,
	}
	data, _ := json.Marshal(msg)

	for _, conn := range clientsCopy {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

func handleWebSocket(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 新しいプレイヤーにIDを割り当てる
	playerID := generatePlayerID()

	mu.Lock()
	clients[playerID] = conn
	playerInputs[playerID] = "right" // 初期方向
	mu.Unlock()

	log.Printf("プレイヤー接続: %s", playerID)

	// 自分のIDをクライアントに通知
	ack, _ := json.Marshal(ServerMessage{
		Type:     "ack",
		PlayerID: playerID,
	})
	conn.WriteMessage(websocket.TextMessage, ack)

	// 切断時の後処理
	defer func() {
		mu.Lock()
		delete(clients, playerID)
		delete(playerInputs, playerID)
		mu.Unlock()
		log.Printf("プレイヤー切断: %s", playerID)
	}()

	// メッセージ受信ループ
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg ClientMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("JSON解析エラー: %v", err)
			continue
		}

		// 入力を受け取ったら処理
		if msg.Type == "input" {
			// 有効な方向かチェック
			if msg.Direction == "up" || msg.Direction == "down" ||
				msg.Direction == "left" || msg.Direction == "right" {

				mu.Lock()
				playerInputs[playerID] = msg.Direction
				mu.Unlock()

				log.Printf("%s の入力: %s", playerID, msg.Direction)

				// 全員に現在の入力状態を配信
				broadcast()
			}
		}
	}

	return nil
}
