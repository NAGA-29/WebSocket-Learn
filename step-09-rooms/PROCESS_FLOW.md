# Step 9 処理フロー図

このファイルは `step-09-rooms` の処理フローをまとめたものです。

## 10人部屋の全体フロー

```mermaid
flowchart TD
    A[ブラウザで接続ボタンを押す] --> B[WebSocket /ws に接続]
    B --> C[handleWebSocket]
    C --> D[findOrCreateRoom]
    D --> E{10人未満の部屋があるか}
    E -- ある --> F[既存 Room を選択]
    E -- ない --> G[newRoom で部屋作成]
    G --> H[部屋専用ゲームループ Start を goroutine 起動]
    F --> I[プレイヤー Snake を作成]
    H --> I
    I --> J[room.addPlayer で Snakes / Clients に追加]
    J --> K[自分の ID と RoomID を ack 送信]
    K --> L[入力受付ループへ]

    L --> M[クライアントが矢印キー入力を送信]
    M --> N[方向が逆走でなければ Snake.Direction 更新]

    H --> O[tick ごとに room.update]
    O --> P[各 Snake を移動]
    P --> Q[エサ判定とスコア更新]
    Q --> R[room.broadcastState]
    R --> S[同じ Room の Clients だけへ state 送信]
    S --> T[クライアントが描画]

    L --> U{ReadMessage エラー}
    U --> V[room.removePlayer]
    V --> W{部屋が空か}
    W -- はい --> X[stopCh を close して部屋削除]
    W -- いいえ --> Y[退室のみ]
```

## 重要ポイント

- 最大人数は `MaxPlayersPerRoom = 10`。
- `findOrCreateRoom()` が `room.PlayerCount() < MaxPlayersPerRoom` を満たす部屋を探す。
- 部屋が満室なら新しい `Room` を作成し、その部屋専用のゲームループを起動する。
- `broadcastState()` は同じ `Room` の `Clients` にだけ送信する。
- 部屋が空になると `stopCh` でゲームループを止め、`rooms` から削除する。
