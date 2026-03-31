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

var (
	snakes  = make(map[string]*Snake)
	clients = make(map[string]*websocket.Conn)
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
	if nh.X < 0 { nh.X = fieldWidth - gridSize }
	if nh.X >= fieldWidth { nh.X = 0 }
	if nh.Y < 0 { nh.Y = fieldHeight - gridSize }
	if nh.Y >= fieldHeight { nh.Y = 0 }
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
	cc := make(map[string]*websocket.Conn, len(clients))
	for id, conn := range clients {
		cc[id] = conn
	}
	mu.RUnlock()

	if len(cc) == 0 {
		return
	}

	data, _ := json.Marshal(GameState{Type: "state", Snakes: sc})
	for _, conn := range cc {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

func handleWebSocket(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// --- Heartbeat の設定 ---

	// 最初のread deadlineを設定
	// この時間内に pong（またはメッセージ）が来なければ ReadMessage がエラーを返す
	conn.SetReadDeadline(time.Now().Add(pongWait))

	// pong を受け取ったら deadline を更新する
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
		body = append(body, Point{float64((10 - i) * gridSize), float64((counter-1)%10*3*gridSize + gridSize)})
	}
	snakes[id] = &Snake{ID: id, Body: body, Direction: "right", Color: color}
	clients[id] = conn
	mu.Unlock()

	log.Printf("プレイヤー接続: %s", id)

	ack, _ := json.Marshal(GameState{Type: "state", MyID: id, Snakes: map[string]*Snake{}})
	conn.WriteMessage(websocket.TextMessage, ack)

	// --- ping を定期送信する goroutine ---
	pingStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// PingMessage を送信
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
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
