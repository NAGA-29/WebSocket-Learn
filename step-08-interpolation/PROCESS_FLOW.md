# Step 8 処理フロー図

このファイルは `step-08-interpolation` の処理フローをまとめたものです。

## WebSocket と補間描画の全体フロー

```mermaid
flowchart TD
    A[ブラウザで接続ボタンを押す] --> B[WebSocket /ws に接続]
    B --> C[server: handleWebSocket]
    C --> D[Client を作成]
    D --> E[Snake を作成して snakes / clients に登録]
    E --> F[myId を含む ack を送信]
    F --> G[client: myId を保存]

    H[server: gameLoop] --> I[tickRate ごとに updateGame]
    I --> J[全 Snake を moveSnake]
    J --> K{maxRandomDelayMs > 0}
    K -- はい --> L[ランダム遅延を入れる]
    K -- いいえ --> M[broadcastState]
    L --> M
    M --> N[GameState に TickCount / ServerTime を入れる]
    N --> O[全 client に state を送信]

    O --> P[client: onmessage]
    P --> Q[補間なし rawState に最新 state を保存]
    P --> R[補間あり prevSnapshot / nextSnapshot を更新]
    R --> S[受信間隔から interpolationDuration を決定]
    S --> T[prevTime / nextTime を更新]

    U[client: requestAnimationFrame] --> V[calcInterpolationT]
    V --> W[t = elapsed / duration を 0.0 から 1.0 に制限]
    W --> X[drawRaw: 最新 state をそのまま描画]
    W --> Y[drawInterpolated: prev と next の間を lerp して描画]
    X --> U
    Y --> U

    Z[矢印キー入力] --> AA[input メッセージ送信]
    AA --> AB[server: ReadMessage]
    AB --> AC[逆走でなければ Direction 更新]
```

## 補間処理だけのフロー

```mermaid
flowchart TD
    A[サーバー状態 A を受信] --> B[画面上の開始位置にする]
    B --> C[サーバー状態 B を受信]
    C --> D[prevSnapshot = A]
    C --> E[nextSnapshot = B]
    D --> F[prevTime = 受信時刻]
    E --> G[nextTime = prevTime + 補間時間]
    F --> H[毎フレーム t を計算]
    G --> H
    H --> I{t < 1.0}
    I -- はい --> J[lerp(A, B, t) を描画]
    I -- いいえ --> K[B の位置で止まって待つ]
    J --> H
    K --> L[次のサーバー状態 C を待つ]
    L --> M[prevSnapshot = B / nextSnapshot = C に更新]
    M --> H
```

## 重要ポイント

- サーバーの `gameLoop()` は `tickRate` ごとに正しいゲーム状態を更新する。
- クライアントは WebSocket の受信タイミングではなく、`requestAnimationFrame` で毎フレーム描画する。
- 補間なしは、受信した最新座標をそのまま描画するためカクつきやすい。
- 補間ありは、前回座標と今回座標の間を `lerp(a, b, t)` で埋める。
- この step は補間のみで、次の状態を予測して進む外挿は実装していない。

## 視覚的な解説ページ

補間の動きだけをサーバーなしで確認したい場合は、次の HTML をブラウザで開いてください。

```text
step-08-interpolation/client/interpolation-explained.html
```
