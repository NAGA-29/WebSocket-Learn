# Step 10 処理フロー図

このファイルは `step-10-reconnect` の処理フローをまとめたものです。

## サーバー側フロー

```mermaid
flowchart TD
    A[WebSocket 接続] --> B[handleWebSocket]
    B --> C[SetReadDeadline で pongWait を設定]
    C --> D[SetPongHandler を登録]
    D --> E[Snake / Client を登録]
    E --> F[ack 送信]
    F --> G[ping goroutine 起動]
    G --> H[一定間隔で PingMessage 送信]
    H --> I{pong が返るか}
    I -- はい --> J[pong handler で read deadline 延長]
    J --> H
    I -- いいえ --> K[ReadMessage が timeout/error]
    K --> L[受信ループを抜ける]
    L --> M[ping goroutine 停止]
    M --> N[snakes / clients から削除]

    O[gameLoop] --> P[tick ごとに updateGame]
    P --> Q[broadcastState]
    Q --> R{送信成功か}
    R -- はい --> O
    R -- いいえ --> S[送信失敗した Client を削除]
```

## クライアント側フロー

```mermaid
flowchart TD
    A[接続ボタン] --> B[connect]
    B --> C[WebSocket 作成]
    C --> D{onopen}
    D --> E[接続中 UI に変更]
    C --> F{onmessage}
    F --> G[myId / state を更新]
    G --> H[canvas に描画]
    C --> I{onclose}
    I --> J[切断中 UI に変更]
    J --> K[state を空にして再描画]
    K --> L{自動再接続 ON か}
    L -- はい --> M[3秒後に connect を再実行]
    L -- いいえ --> N[停止]
    C --> O{onerror}
    O --> P[ログ表示]
```

## 重要ポイント

- ping/pong と read deadline で無応答接続を切断扱いにする。
- `broadcastState()` の送信失敗も切断として扱い、該当プレイヤーを削除する。
- クライアントの再接続は「新しいプレイヤーとして再入場」する単純な方式。
- `Client.writeText()` で通常メッセージ送信を直列化し、ping は `WriteControl` で送る。
