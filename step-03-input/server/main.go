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

// ⚠️  開発用設定: 全オリジンを許可。本番では許可オリジンを限定すること。
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client は接続と書き込みロックをまとめた構造体
//
// gorilla/websocket は「同一コネクションへの WriteMessage 系呼び出しは
// 同時に1つだけ」という制約がある
// broadcast() は複数の接続ハンドラ goroutine から同時に呼ばれ得るため、
// すべての WriteMessage を writeMu で直列化する
type Client struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// writeText は writeMu を取得してからメッセージを送る
func (c *Client) writeText(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// ClientMessage はクライアントから受け取るメッセージの形式
type ClientMessage struct {
	Type      string `json:"type"`      // "input"
	Direction string `json:"direction"` // "up" / "down" / "left" / "right"
}

// ServerMessage はサーバーからクライアントへ送るメッセージの形式
//
// Type には2種類ある:
//   - "ack"   : 接続直後に1回だけ送る確認応答。自分のプレイヤーIDを通知する。
//   - "state" : 誰かが入力するたびに全クライアントへ送る状態配信。
//     全プレイヤーの最新入力方向を inputs マップとして渡す。
type ServerMessage struct {
	Type     string            `json:"type"`     // "ack" または "state"
	PlayerID string            `json:"playerId"` // 自分のID（ack時）
	Inputs   map[string]string `json:"inputs"`   // 全プレイヤーの最新入力（state時）
}

// clients は接続中のクライアント（プレイヤーID → Client）
var clients = make(map[string]*Client)

// playerInputs は各プレイヤーの最新入力方向を保持するmap
// キー: プレイヤーID（接続ごとに割り当てる）
// 値: 最新の方向 ("up" / "down" / "left" / "right")
var playerInputs = make(map[string]string)

var mu sync.Mutex

// idCounter はシンプルな連番プレイヤーID生成用（mu で保護する）
var idCounter int

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
	// 現在の inputs と clients をコピー（ロック中の処理を最小限にするため）
	inputsCopy := make(map[string]string, len(playerInputs))
	for id, dir := range playerInputs {
		inputsCopy[id] = dir
	}
	clientsCopy := make(map[string]*Client, len(clients))
	for id, c := range clients {
		clientsCopy[id] = c
	}
	mu.Unlock()

	msg := ServerMessage{
		Type:   "state",
		Inputs: inputsCopy,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("broadcast: json.Marshal エラー: %v", err)
		return
	}

	// 各 Client の writeMu が並行 write を直列化する
	for _, client := range clientsCopy {
		if err := client.writeText(data); err != nil {
			log.Printf("broadcast: 送信エラー: %v", err)
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

	// プレイヤーIDの割り当てと登録を同じロック内で行い、競合を防ぐ
	mu.Lock()
	idCounter++
	playerID := fmt.Sprintf("player-%d", idCounter)
	clients[playerID] = client
	playerInputs[playerID] = "right" // 初期方向
	mu.Unlock()

	log.Printf("プレイヤー接続: %s", playerID)

	// 自分のIDをクライアントに通知
	ack, err := json.Marshal(ServerMessage{
		Type:     "ack",
		PlayerID: playerID,
	})
	if err != nil {
		log.Printf("ack: json.Marshal エラー: %v", err)
	} else if err := client.writeText(ack); err != nil {
		log.Printf("ack 送信エラー: %v", err)
	}

	// 切断時の後処理
	defer func() {
		mu.Lock()
		delete(clients, playerID)      // クライアント一覧から削除
		delete(playerInputs, playerID) // プレイヤーの入力状態も削除
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
