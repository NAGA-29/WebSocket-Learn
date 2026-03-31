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

// WebSocket アップグレーダー
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// clients は接続中の全クライアントを保持するmap
// *websocket.Conn をキー、bool を値として使う（セットとして利用）
var clients = make(map[*websocket.Conn]bool)

// mu はクライアントマップへの同時アクセスを防ぐためのミューテックス
// 複数の goroutine が同時に clients を読み書きするので、排他制御が必要
var mu sync.Mutex

// Message はクライアントとサーバー間でやり取りするJSON形式
type Message struct {
	Type    string `json:"type"`    // メッセージの種類 ("chat", "system")
	Content string `json:"content"` // メッセージ内容
	Count   int    `json:"count"`   // 現在の接続数
}

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// 静的ファイルを配信
	e.Static("/", "../client")

	// WebSocket エンドポイント
	e.GET("/ws", handleWebSocket)

	fmt.Println("サーバー起動: http://localhost:8080")
	log.Fatal(e.Start(":8080"))
}

// broadcast は接続中の全クライアントにメッセージを送信する
func broadcast(msg Message) {
	// JSON にシリアライズ
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("JSON変換エラー: %v", err)
		return
	}

	// mu.Lock() で clients マップへの排他アクセスを確保
	mu.Lock()
	defer mu.Unlock()

	// 全クライアントにメッセージを送信
	for conn := range clients {
		err := conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			// 送信失敗した接続は削除（切断済みの可能性が高い）
			log.Printf("送信エラー、接続を削除: %v", err)
			conn.Close()
			delete(clients, conn)
		}
	}
}

// getClientCount は現在の接続数を返す
func getClientCount() int {
	mu.Lock()
	defer mu.Unlock()
	return len(clients)
}

// handleWebSocket は各 WebSocket 接続を処理する
func handleWebSocket(c echo.Context) error {
	// HTTP → WebSocket にアップグレード
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Printf("アップグレード失敗: %v", err)
		return err
	}
	defer conn.Close()

	// 新しいクライアントを登録
	mu.Lock()
	clients[conn] = true
	count := len(clients)
	mu.Unlock()

	log.Printf("新しいクライアントが接続 (現在: %d人)", count)

	// 入室を全員に通知
	broadcast(Message{
		Type:    "system",
		Content: "新しいユーザーが入室しました",
		Count:   count,
	})

	// クライアントが切断したときの後処理
	defer func() {
		mu.Lock()
		delete(clients, conn)
		count := len(clients)
		mu.Unlock()

		log.Printf("クライアントが切断 (残り: %d人)", count)

		// 退室を全員に通知
		broadcast(Message{
			Type:    "system",
			Content: "ユーザーが退室しました",
			Count:   count,
		})
	}()

	// メッセージを受け取り続けるループ
	for {
		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			// 接続が切れたらループを抜ける
			log.Printf("受信エラー（切断）: %v", err)
			break
		}

		// 受け取ったメッセージを全員にブロードキャスト
		log.Printf("受信してブロードキャスト: %s", string(rawMsg))
		broadcast(Message{
			Type:    "chat",
			Content: string(rawMsg),
			Count:   getClientCount(),
		})
	}

	return nil
}
