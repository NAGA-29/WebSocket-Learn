# Step 4: サーバーで状態を持つ

## このステップの目標

- サーバーが各プレイヤーの `x, y, direction` を保持する
- クライアントは入力だけ送る
- サーバーが状態を更新して全員に配信する
- クライアントはCanvasに描画するだけ

**このステップが全カリキュラムの核心です。**

---

## 超重要：「サーバーが真実を持つ」とは

### クライアントとサーバーの役割分担

```
クライアントの役割：
  - キー入力をサーバーに送る
  - サーバーから受け取った状態を画面に表示する
  - それだけ

サーバーの役割：
  - 入力を受け取る
  - ゲームの状態（全プレイヤーの位置など）を更新する
  - 全クライアントに最新状態を配信する
  - それが真実
```

### なぜこの設計が重要か

もしクライアントが自分で座標を計算して送っていたら：

```
プレイヤーAのブラウザ: 「私は x=100 y=200 にいます」
プレイヤーBのブラウザ: 「私は x=102 y=198 にいます」
（微妙にズレている）

→ 衝突判定はどちらのブラウザがやる？
→ チートしたプレイヤーが好き勝手な座標を送ったら？
→ ネットワーク遅延でどちらの位置情報が「正しい」のか分からない
```

サーバーが全状態を持てば：

```
全員が「サーバーの状態」を表示するだけ
→ 全員が同じ世界を見ている
→ 衝突判定もサーバーだけがやればいい
→ チートしても無駄（サーバーが無視する）
```

---

## データ構造

### プレイヤー状態

```go
type Player struct {
    ID        string  `json:"id"`
    X         float64 `json:"x"`
    Y         float64 `json:"y"`
    Direction string  `json:"direction"` // "up" / "down" / "left" / "right"
    Color     string  `json:"color"`     // 識別用の色
}
```

### サーバーが送る状態メッセージ

```go
type ServerMessage struct {
    Type    string             `json:"type"`
    Players map[string]*Player `json:"players"`
    MyID    string             `json:"myId,omitempty"`
}
```

### 通信メッセージ設計

クライアント → サーバー：
```json
{ "type": "input", "direction": "up" }
```

サーバー → クライアント（状態更新）：
```json
{
  "type": "state",
  "players": {
    "player-1": { "id": "player-1", "x": 100, "y": 200, "direction": "up", "color": "#ff6b6b" },
    "player-2": { "id": "player-2", "x": 300, "y": 150, "direction": "right", "color": "#4ecdc4" }
  }
}
```

接続直後も同じ `"state"` メッセージ形式で、自分のIDだけを `myId` として受け取ります。

```json
{ "type": "state", "myId": "player-1", "players": {} }
```

---

## Canvas 描画

```javascript
// Canvas を使ってプレイヤーを丸で描く
function draw(state) {
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  for (const player of Object.values(state.players)) {
    ctx.beginPath();
    ctx.arc(player.x, player.y, 15, 0, Math.PI * 2);
    ctx.fillStyle = player.color;
    ctx.fill();

    // 名前を表示
    ctx.fillStyle = 'black';
    ctx.fillText(player.id, player.x - 20, player.y - 20);
  }
}
```

---

## ファイル構成

```
step-04-server-state/
├── README.md
├── server/
│   ├── main.go
│   └── go.mod
└── client/
    └── index.html
```

---

## 動作確認の手順

1. サーバーを起動
2. タブ1でブラウザを開いて接続
3. タブ2でもブラウザを開いて接続
4. どちらのタブでも矢印キーを押す
5. **両方のタブで同じ位置情報が表示されること**を確認する

実装では入力を受け取ったタイミングで `moveSpeed = 5.0` だけ移動し、その直後に全員へ状態を配信します。

---

## よくある設計ミス

### ミス1: クライアントがサーバーに座標を送る
```json
// ❌ これをやらない
{ "type": "move", "x": 150, "y": 200 }
```
→ チート可能になる。サーバーが何でも受け入れてしまう。

### ミス2: 状態更新をクライアントがやる
```javascript
// ❌ これもダメ
player.x += 1; // クライアントで移動計算する
ws.send({ x: player.x, y: player.y }); // 結果を送る
```
→ 各クライアントが独自に計算するので、ズレが生じる。

### ミス3: ミューテックスを使わない
```go
// ❌ 競合が起きる
players[id] = newPlayer // 複数goroutineから同時に書き込む
```
→ `sync.Mutex` や `sync.RWMutex` で保護する。

---

## 練習問題

1. **基本:** 複数タブで接続して、全員の位置がCanvasに表示されることを確認する
2. **応用:** プレイヤーの移動速度を変える定数を追加する
3. **応用:** 画面外に出たらもう一方から出てくる（ラップアラウンド）処理を追加する
4. **発展:** プレイヤーが入退室したとき、残りのプレイヤーに通知を送る

---

## 理解確認クイズ

1. なぜクライアントは座標を計算してはいけないのですか？
2. `sync.Mutex` は何を防ぐためのものですか？
3. サーバーが状態を保持するメリットを3つ挙げてください
4. このステップでクライアントが「やること」は何ですか？（2つ）
5. 「サーバー権威型設計」を知らない人に、一言で説明してください

---

## 次のステップへ

Step 4では「サーバーが状態を持つ」を実装しました。
でも今の実装は、入力があったときだけ位置を更新しています。

蛇ゲームでは「キーを押していなくても蛇は動き続ける」必要があります。
Step 5では、一定周期でゲームを進める**ゲームループ**を導入します。
