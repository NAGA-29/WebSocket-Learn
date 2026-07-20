# Step 11 処理フロー図

このファイルは `step-11-optimization` の処理フローをまとめたものです。

## 最適化計測フロー

```mermaid
flowchart TD
    A[WebSocket 接続] --> B[handleWebSocket]
    B --> C[Snake / Conn を登録]
    C --> D[myId を含む ack を送信]
    D --> E[入力受付ループ]
    E --> F[矢印キー input を受信]
    F --> G[逆走でなければ Direction 更新]

    H[gameLoop] --> I[100ms ごとに updateGame]
    I --> J[tickCount を増やす]
    J --> K[全 Snake を移動]
    K --> L[broadcastWithStats]
    L --> M[FullState を作成]
    M --> N[JSON marshal して fullData サイズ計測]
    N --> O[LightState を作成]
    O --> P[短いフィールド名で lightData サイズ計測]
    P --> Q[StatsMessage を作成]
    Q --> R[各 client に FullState を送信]
    R --> S[各 client に stats を送信]
    S --> T{送信失敗あり}
    T -- あり --> U[該当 client / snake を削除]
    T -- なし --> H

    V[クライアント onmessage] --> W{state か}
    W -- はい --> X[実受信サイズを計測して描画]
    V --> Y{stats か}
    Y -- はい --> Z[全量サイズ / 軽量サイズ / 削減率 / 帯域を表示]
```

## 重要ポイント

- 実際に送っているゲーム状態は `FullState`。
- 同じ tick 内で `LightState` も作り、短い JSON フィールド名にした場合のサイズを比較する。
- `StatsMessage` を別メッセージとして送り、クライアントでサイズ・削減率・推定帯域を表示する。
- この step の主目的は、差分送信や軽量フォーマットへ進む前に「全量送信のコストを観測する」こと。
