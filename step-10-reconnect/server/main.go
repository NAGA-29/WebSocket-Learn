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

// ⚠️  開発用設定: CheckOrigin で全オリジンを許可している。
// これは Cross-Site WebSocket Hijacking（CSWSH）に対して無防備になるため
// 本番環境では絶対に使用しないこと。
//
// 本番向けの最低限の対策例:
//   CheckOrigin: func(r *http.Request) bool {
//       origin := r.Header.Get("Origin")
//       return origin == "https://yourdomain.example.com"
//   }
// さらに認証が必要な場合は JWT や Cookie セッションをアップグレード前に検証する。
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // 開発専用: 全オリジン許可
}

// heartbeat の設定
const (
	pingInterval = 10 * time.Second // この間隔で ping を送る
	pongWait     = 15 * time.Second // この時間内に pong が来なければ切断扱い

	tickRate    = 150 * time.Millisecond
	fieldWidth  = 800
	fieldHeight = 600
	gridSize    = 20
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
	Type   string            `json:"type"`
	Snakes map[string]*Snake `json:"snakes"`
	MyID   string            `json:"myId,omitempty"`
}

type ClientMessage struct {
	Type      string `json:"type"`
	Direction string `json:"direction"`
}

// Client は接続と書き込みロックをまとめた構造体。
//
// gorilla/websocket の仕様:
//   「同一コネクションへの WriteMessage 系呼び出しは同時に1つしか許可されない」
//
// broadcastState（gameLoop goroutine）と ping 送信 goroutine が
// 同じ conn に並行して書き込む可能性があるため、
// WriteMessage はすべて writeMu で直列化する。
// ping には WriteControl を使う（gorilla ドキュメントに
// "Close and WriteControl can be called concurrently with all other methods"
// と明記されているため writeMu 不要）。
type Client struct {
	conn    *websocket.Conn
	writeMu sync.Mutex // WriteMessage 呼び出しを直列化するロック
}

// writeText は writeMu を取得してから TextMessage を送る。
// エラーを返すので呼び出し側でハンドリングする。
func (c *Client) writeText(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

var (
	snakes  = make(map[string]*Snake)
	clients = make(map[string]*Client)
	mu      sync.RWMutex
	counter int
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
	fmt.Printf("ping間隔: %v / pong待ち: %v\n", pingInterval, pongWait)
	log.Fatal(e.Start(":8080"))
}

func gameLoop() {
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()
	for {
		<-ticker.C
		updateGame()
		broadcastState()
	}
}

func updateGame() {
	mu.Lock()
	defer mu.Unlock()
	for _, s := range snakes {
		moveSnake(s)
	}
}

func moveSnake(s *Snake) {
	if len(s.Body) == 0 {
		return
	}
	head := s.Body[0]
	var nh Point
	switch s.Direction {
	case "up":
		nh = Point{head.X, head.Y - gridSize}
	case "down":
		nh = Point{head.X, head.Y + gridSize}
	case "left":
		nh = Point{head.X - gridSize, head.Y}
	case "right":
		nh = Point{head.X + gridSize, head.Y}
	}
	if nh.X < 0 {
		nh.X = fieldWidth - gridSize
	}
	if nh.X >= fieldWidth {
		nh.X = 0
	}
	if nh.Y < 0 {
		nh.Y = fieldHeight - gridSize
	}
	if nh.Y >= fieldHeight {
		nh.Y = 0
	}
	s.Body = append([]Point{nh}, s.Body...)
	if len(s.Body) > 5 {
		s.Body = s.Body[:5]
	}
}

func broadcastState() {
	mu.RLock()
	sc := make(map[string]*Snake, len(snakes))
	for id, s := range snakes {
		cp := *s
		bc := make([]Point, len(s.Body))
		copy(bc, s.Body)
		cp.Body = bc
		sc[id] = &cp
	}
	cc := make(map[string]*Client, len(clients))
	for id, c := range clients {
		cc[id] = c
	}
	mu.RUnlock()

	if len(cc) == 0 {
		return
	}

	data, err := json.Marshal(GameState{Type: "state", Snakes: sc})
	if err != nil {
		log.Printf("broadcastState: json.Marshal エラー: %v", err)
		return
	}

	// 送信エラーが起きた接続を後でまとめて削除するためのリスト
	var failed []string
	for id, client := range cc {
		if err := client.writeText(data); err != nil {
			log.Printf("broadcastState: 送信エラー（切断扱い）: %s: %v", id, err)
			failed = append(failed, id)
		}
	}

	// 送信失敗した接続を削除する
	if len(failed) > 0 {
		mu.Lock()
		for _, id := range failed {
			if c, ok := clients[id]; ok {
				c.conn.Close()
			}
			delete(clients, id)
			delete(snakes, id)
		}
		mu.Unlock()
	}
}

func handleWebSocket(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// --- Heartbeat の設定 ---

	// 最初の read deadline を設定。
	// この時間内に pong（またはメッセージ）が来なければ ReadMessage がエラーを返す。
	conn.SetReadDeadline(time.Now().Add(pongWait))

	// pong を受け取ったら deadline を延長する
	conn.SetPongHandler(func(appData string) error {
		log.Printf("pong 受信")
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// プレイヤー登録
	mu.Lock()
	counter++
	id := fmt.Sprintf("snake-%d", counter)
	color := playerColors[(counter-1)%len(playerColors)]
	body := []Point{}
	for i := 0; i < 5; i++ {
		body = append(body, Point{
			float64((10 - i) * gridSize),
			float64((counter-1)%10*3*gridSize + gridSize),
		})
	}
	client := &Client{conn: conn}
	snakes[id] = &Snake{ID: id, Body: body, Direction: "right", Color: color}
	clients[id] = client
	mu.Unlock()

	log.Printf("プレイヤー接続: %s", id)

	// 初回 ack を送信（writeMu で直列化）
	ack, err := json.Marshal(GameState{Type: "state", MyID: id, Snakes: map[string]*Snake{}})
	if err != nil {
		log.Printf("ack: json.Marshal エラー: %v", err)
	} else if err := client.writeText(ack); err != nil {
		log.Printf("ack 送信エラー: %v", err)
	}

	// --- ping を定期送信する goroutine ---
	// WriteControl は WriteMessage と同時に呼んでも安全（gorilla の仕様より）。
	// writeMu を取得する必要はなく、WriteControl のみを使う。
	pingStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// WriteControl は deadline を引数に取る
				deadline := time.Now().Add(5 * time.Second)
				if err := conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
					log.Printf("ping 送信エラー: %v", err)
					return
				}
				log.Printf("ping 送信 → %s", id)
			case <-pingStop:
				return
			}
		}
	}()

	defer func() {
		close(pingStop) // ping goroutine を止める

		mu.Lock()
		delete(snakes, id)
		delete(clients, id)
		mu.Unlock()
		log.Printf("プレイヤー切断: %s", id)
	}()

	// メッセージ受信ループ
	// read deadline を超えると ReadMessage がエラーを返す → 切断扱いになる
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("異常切断: %s: %v", id, err)
			} else {
				log.Printf("切断: %s: %v", id, err)
			}
			break
		}

		// メッセージを受け取ったら read deadline をリセット
		conn.SetReadDeadline(time.Now().Add(pongWait))

		var msg ClientMessage
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}

		if msg.Type == "input" {
			opposites := map[string]string{
				"up": "down", "down": "up", "left": "right", "right": "left",
			}
			mu.Lock()
			if s, ok := snakes[id]; ok && opposites[s.Direction] != msg.Direction {
				s.Direction = msg.Direction
			}
			mu.Unlock()
		}
	}

	return nil
}
