package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// upgrader は HTTP 接続を WebSocket 接続にアップグレードするための設定
// CheckOrigin: 開発用に全てのオリジンを許可（本番では制限すること）
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	// Echo インスタンスを作成
	e := echo.New()

	// ログとリカバリのミドルウェアを追加
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// 静的ファイル（client/index.html）を配信
	// ブラウザで http://localhost:8080/ にアクセスすると index.html が表示される
	e.Static("/", "../client")

	// WebSocket のエンドポイント
	// ブラウザから ws://localhost:8080/ws に接続する
	e.GET("/ws", handleWebSocket)

	// サーバーをポート 8080 で起動
	fmt.Println("サーバー起動: http://localhost:8080")
	log.Fatal(e.Start(":8080"))
}

// handleWebSocket は WebSocket 接続を処理するハンドラ
func handleWebSocket(c echo.Context) error {
	// HTTP → WebSocket にアップグレード
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Printf("アップグレード失敗: %v", err)
		return err
	}
	// 関数が終わったら必ず接続を閉じる
	defer conn.Close()

	log.Println("新しいクライアントが接続しました")

	// メッセージを受け取り続けるループ
	for {
		// メッセージを受信（メッセージが来るまでここで待機する）
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			// 接続が切れたらループを抜ける
			log.Printf("受信エラー（接続切断）: %v", err)
			break
		}

		// 受け取ったメッセージをログに表示
		log.Printf("受信: %s", string(message))

		// 受け取ったメッセージをそのままクライアントに返す（エコー）
		err = conn.WriteMessage(messageType, message)
		if err != nil {
			log.Printf("送信エラー: %v", err)
			break
		}
	}

	log.Println("クライアントが切断しました")
	return nil
}
