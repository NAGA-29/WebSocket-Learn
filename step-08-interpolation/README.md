# Step 8: 補間と予測を入れる

## このステップの目標

- クライアント側で補間（interpolation）を実装する
- 補間なし/ありの違いを体感する
- 「サーバーの真実」と「画面上の見え方」を分けて考えられる

---

## 補間とは

**補間（interpolation）**: 2点間をなめらかに繋ぐこと

```
サーバーからの更新が 100ms ごとに来る場合：

補間なし:
  t=0ms:   x=100（描画）
  t=1〜99ms: x=100のまま（止まって見える）
  t=100ms: x=120（瞬間移動）
  → カクカクしてワープして見える

補間あり:
  t=0ms:   x=100（描画）
  t=50ms:  x=110（前の位置と次の位置の中間を補間）
  t=100ms: x=120（次の位置に到達）
  → 滑らかに動いて見える
```

---

## 線形補間（Linear Interpolation / Lerp）

```
lerp(a, b, t) = a + (b - a) * t
  a: 開始値
  b: 終了値
  t: 0.0〜1.0（0=a、1=b）

例:
  lerp(100, 120, 0.0) = 100  （位置A）
  lerp(100, 120, 0.5) = 110  （中間点）
  lerp(100, 120, 1.0) = 120  （位置B）
```

---

## 実装のアイデア

### クライアント側の状態

```javascript
// サーバーから受け取った「前の位置」と「次の位置」を持つ
const interpolation = {
  prevSnapshot: null,    // 1つ前のサーバー状態
  nextSnapshot: null,    // 最新のサーバー状態
  prevTime: 0,           // 前のスナップショットを受け取った時刻
  nextTime: 0,           // 次のスナップショットを受け取った時刻
};
```

### ゲームループ（クライアント側）

```javascript
// requestAnimationFrame でなめらかに描画
function gameLoop(timestamp) {
  const now = Date.now();
  const elapsed = now - interpolation.prevTime;
  const duration = interpolation.nextTime - interpolation.prevTime;

  // 0.0〜1.0 の補間係数を計算
  const t = Math.min(elapsed / duration, 1.0);

  // 各プレイヤーの位置を補間して描画
  drawInterpolated(t);

  requestAnimationFrame(gameLoop);
}
```

### 補間して描画

```javascript
function getInterpolatedPosition(prevPos, nextPos, t) {
  return {
    x: lerp(prevPos.x, nextPos.x, t),
    y: lerp(prevPos.y, nextPos.y, t),
  };
}

function lerp(a, b, t) {
  return a + (b - a) * t;
}
```

---

## 「正しいこと」と「気持ちよく見えること」は別

**重要なポイント：**

- サーバーの状態が「正しい真実」
- クライアントの補間位置は「見せかけの位置」
- これらは意図的にズレていてよい

```
サーバーの真実:  x=100 → x=120 （100ms後）
クライアントの表示: x=100 → x=110 → x=120 （50msごとに補間）

この「嘘」によって画面が滑らかに見える
```

これを「見た目の誤魔化し（visual smoothing）」と言います。
ゲームでは標準的な手法です。

---

## 補間の限界

| 問題 | 内容 |
|---|---|
| 補間の遅れ | 常に「1フレーム前の状態」を補間するので、リアルタイムより遅れる |
| 急な方向転換 | 補間により、実際よりカーブしたように見える |
| 長い遅延 | 遅延が大きすぎると補間しきれなくなる |
| ワープ | 大きすぎる位置変化は補間してもワープに見える |

---

## ファイル構成

```
step-08-interpolation/
├── README.md
├── server/
│   ├── main.go    （Step 7 と同じ、遅延設定あり）
│   └── go.mod
└── client/
    └── index.html （補間ありの描画）
```

---

## Step 7との違い（クライアント側）

Step 7（補間なし）:
```javascript
ws.onmessage = (event) => {
  state = JSON.parse(event.data);
  draw(); // 受け取ったらすぐ描画
};
```

Step 8（補間あり）:
```javascript
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  prevSnapshot = nextSnapshot;      // 1つ前の状態を保存
  nextSnapshot = msg;               // 新しい状態を保存
  snapshotTimestamp = Date.now();   // 受け取り時刻を記録
  // 描画は requestAnimationFrame に任せる
};

function gameLoop() {
  const t = calcInterpolationT(); // 補間係数を計算
  drawInterpolated(t);            // 補間して描画
  requestAnimationFrame(gameLoop);
}
```

---

## 練習問題

1. **観察:** 補間なし（Step 7）と補間あり（Step 8）の動きを比べる
2. **実験:** `tickRate = 500ms` に設定して、補間の効果を確認する
3. **応用:** 補間係数 `t` を `0.0〜1.0` 以外にしたらどうなるか試す
4. **考察:** 「補間できない問題」を1つ考えて説明する

---

## 理解確認クイズ

1. `lerp(10, 20, 0.3)` の結果は何ですか？
2. 補間は「正確さ」と「滑らかさ」のどちらを優先していますか？
3. なぜ `requestAnimationFrame` を使うのですか？
4. 補間があると、表示上は常にサーバー状態より少し「遅れて」いますか？
5. 補間だけではワープを完全に防げない理由は何ですか？

---

## 次のステップへ

Step 8で「見た目を滑らかにする」ための基礎を学びました。
Step 9では、1つの部屋から10人部屋への拡張を学びます。

「1つのグローバルなゲーム空間」から「複数の部屋」に分ける設計です。
