# Step 9: 10人部屋を作る

## このステップの目標

- 部屋（Room）単位でゲームを管理できる
- 新規接続を適切な部屋に振り分けられる
- 部屋ごとにゲームループを持てる

---

## なぜグローバル1マップではダメなのか

Step 6 までは全員が同じ1つのフィールドにいました。

問題点：
```
1. スケールしない
   → 100人が1フィールドにいると、毎tick100人分のデータを全員に送る
   → 1人が受け取るデータ量が100人分になる

2. ゲームバランスが崩れる
   → 人数が増えるほどフィールドが混雑する
   → ゲームデザインが成立しない

3. プライバシー（遊びの隔離）
   → 知らない人と同じフィールドに強制されたくない場合も
```

**10人部屋に分ければ：**
```
→ 各部屋は最大10人のゲーム空間
→ 部屋内の通信は部屋内だけで閉じる
→ サーバーが複数部屋を管理できる
```

---

## Room の設計

```go
type Room struct {
    ID      string
    Snakes  map[string]*Snake
    Clients map[string]*websocket.Conn
    Foods   []Food
    mu      sync.RWMutex
    stopCh  chan struct{} // ゲームループを止めるためのチャンネル
}

const MaxPlayersPerRoom = 10
```

---

## 部屋の振り分けロジック

```
新しいプレイヤーが接続した
    ↓
空き部屋（10人未満の部屋）はあるか？
    ↓ ある             ↓ ない
    その部屋に入る      新しい部屋を作る
```

```go
func findOrCreateRoom() *Room {
    for _, room := range rooms {
        if len(room.Snakes) < MaxPlayersPerRoom {
            return room
        }
    }
    // 満室なら新しい部屋を作る
    return createRoom()
}
```

---

## 部屋ごとのゲームループ

各部屋が自分のゲームループを持ちます。

```go
func (r *Room) Start() {
    ticker := time.NewTicker(tickRate)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            r.update()
            r.broadcast()
        case <-r.stopCh:
            return // 部屋が空になったらループを止める
        }
    }
}
```

---

## データの流れ

```
接続 → findOrCreateRoom() → Room に追加 → Room のゲームループが動く
  ↓
入力受付ハンドラ → Room の Snake の方向を更新
  ↓
Room のゲームループが移動 → Room 内の全員にブロードキャスト
```

---

## ファイル構成

```
step-09-rooms/
├── README.md
├── server/
│   ├── main.go
│   └── go.mod
└── client/
    └── index.html
```

---

## よくある設計ミス

### 1. 部屋のゲームループを複数起動してしまう
```go
// ❌ 接続のたびに goroutine が増える
func handleWebSocket(c echo.Context) error {
    room := findOrCreateRoom()
    go room.Start() // ← 2回目の接続で2つ目のループが起動する
```
対策：`room.running` フラグで管理し、起動済みなら起動しない

### 2. 部屋のロックを取りすぎる
```go
// ❌ 外側のロックと内側のロックが絡み合うとデッドロックになる
globalMu.Lock()
room.mu.Lock() // ← ロックの順番が違うと危険
```

### 3. 空になった部屋を削除しない
全員が退出した部屋は削除しないと、メモリが増え続ける

---

## 練習問題

1. **基本:** 11人目が接続したとき、自動で2つ目の部屋が作られることを確認する
2. **応用:** 部屋一覧をクライアントに表示する（部屋ID、現在人数）
3. **応用:** 空になった部屋を自動で削除する
4. **発展:** プレイヤーが部屋番号を指定して入れるようにする

---

## 理解確認クイズ

1. なぜグローバル1マップではスケールしないのですか？
2. 部屋が「満室（10人）」になったらどうなりますか？
3. 各部屋が自分のゲームループを持つ理由は何ですか？
4. `stopCh chan struct{}` は何に使いますか？
5. 部屋Aにいるプレイヤーが部屋Bのプレイヤーの状態を受け取らないようにするにはどうしますか？

---

## 次のステップへ

Step 9で複数の部屋を管理できるようになりました。
Step 10では、現実のネットワーク問題——切断・再接続——への対応を学びます。

「切断処理を書いていない実装は未完成」
次はこれを解決します。
