# Step 6: 蛇ゲームにする

## このステップの目標

- プレイヤーが「点」から「体を持つ蛇」になる
- エサを食べると伸びる
- 他プレイヤーの蛇も表示される

---

## 蛇の表現：body配列

蛇は「頭 + 胴体のリスト」で表現します。

```
蛇の初期状態（右向き、3マス）:
  [x:50,y:100] [x:40,y:100] [x:30,y:100]
      ↑頭（先頭）              ↑尾（末尾）

1 tick 後（右に進む）:
  [x:60,y:100] [x:50,y:100] [x:40,y:100]
  ↑新しい頭     ↑元の頭が胴体に  ↑末尾は削除
```

### 移動のアルゴリズム

```
1. 頭の新しい座標を計算する（方向に従って1歩進む）
2. 新しい頭を配列の先頭に追加する（unshift）
3. 配列の末尾を削除する（pop）
   → 長さが変わらない = 通常の移動

4. エサを食べたとき:
   3の「末尾を削除」をスキップする
   → 長さが1増える = 成長
```

---

## データモデル

### サーバー側

```go
type Point struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
}

type Snake struct {
    ID        string  `json:"id"`
    Body      []Point `json:"body"`      // body[0] が頭
    Direction string  `json:"direction"`
    Color     string  `json:"color"`
    Alive     bool    `json:"alive"`
}

type Food struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
}

type GameState struct {
    Snakes map[string]*Snake `json:"snakes"`
    Foods  []Food            `json:"foods"`
}
```

---

## グリッドベース設計

蛇ゲームはグリッド（マス目）で動かすと実装が楽です。

```
グリッドサイズ: 20px × 20px
フィールド: 800px × 600px = 40 × 30 マス

蛇の1マスは 20×20 px の正方形
エサも同じグリッド上に配置
```

グリッドに合わせることで：
- 衝突判定が「同じグリッド座標か？」だけで済む
- 蛇の体が綺麗に並ぶ

---

## エサのシステム

```go
// エサをランダムな位置に配置する
func spawnFood() Food {
    x := float64(rand.Intn(fieldWidth/gridSize)) * gridSize
    y := float64(rand.Intn(fieldHeight/gridSize)) * gridSize
    return Food{X: x, Y: y}
}
```

---

## ファイル構成

```
step-06-snake/
├── README.md
├── server/
│   ├── main.go
│   └── go.mod
└── client/
    └── index.html
```

---

## このステップではやらないこと

- 壁への衝突判定（蛇が壁にぶつかって死ぬ）
- 自分の体への衝突判定
- 他プレイヤーへの衝突判定

これらは後のステップで追加します。
まずは「蛇が動いてエサを食べて伸びる」を動かします。

---

## よくある不具合と対処

### 1. 蛇がテレポートする
**原因:** 移動量がグリッドサイズと合っていない
**対処:** `moveSpeed = gridSize` にする（1tickで1マス進む）

### 2. 体が重なる
**原因:** 方向転換の処理が間違っている
**対処:** 180度の方向転換（上→下、左→右）を禁止する処理を追加

```go
// 反対方向への転換を禁止
func isOppositeDirection(current, next string) bool {
    opposites := map[string]string{
        "up": "down", "down": "up",
        "left": "right", "right": "left",
    }
    return opposites[current] == next
}
```

### 3. エサが蛇の体の上に生成される
**対処:** エサ生成時に蛇の体の座標リストを確認して、被らない位置を探す

---

## Canvas 描画

```javascript
// 蛇を描画
function drawSnake(snake) {
  for (let i = 0; i < snake.body.length; i++) {
    const segment = snake.body[i];
    const isHead = i === 0;

    ctx.fillStyle = isHead ? snake.color : adjustColor(snake.color, -30);
    ctx.fillRect(
      segment.x + 1,          // 1pxのギャップで各セグメントを区別
      segment.y + 1,
      gridSize - 2,
      gridSize - 2
    );
  }
}
```

---

## 練習問題

1. **基本:** 蛇が動いてエサを食べて伸びることを確認する
2. **応用:** 180度転換を禁止する（逆方向への転換を無効にする）
3. **応用:** エサを複数（3つなど）配置する
4. **発展:** スコア（食べたエサの数）を表示する

---

## 理解確認クイズ

1. 蛇の移動を「body配列の先頭追加 + 末尾削除」で表現する理由は何ですか？
2. 成長するときと通常移動の違いは何ですか？（コードレベルで）
3. なぜグリッドベースにすると衝突判定が楽になるのですか？
4. このステップでネットワークに流れるデータ量は Step 5 より多いですか？なぜ？
5. 「蛇の頭」は `body` 配列のどのインデックスですか？

---

## 次のステップへ

Step 6で蛇ゲームの形になりました。
ここで「10人プレイするとデータ量が増える」ことも体感できたはずです。

Step 7では、わざと遅延を入れてリアルタイム通信の問題を体験します。
「WebSocketを使えば遅延はない」という誤解を潰します。
