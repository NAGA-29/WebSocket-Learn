# Step 10: 切断・再接続に対応する

## このステップの目標

- heartbeat（ping/pong）で生きている接続を確認する
- 切断を確実に検知してゴーストプレイヤーを防ぐ
- 再接続の設計パターンを理解する

---

## 「切断処理を書いていない実装は未完成」

実際のネットワーク環境では、接続が突然切れることは日常的です。

```
切断のパターン:
1. ユーザーがブラウザを閉じる（close イベントが届く）
2. PCがスリープした（数秒後に切断を検知）
3. ネットワーク障害（タイムアウトまで検知できない場合がある）
4. 中間ルーターがアイドル接続を切断する（無音接続の切断）
```

特に **3と4** が問題です。TCP の接続が「論理的には切れているのに、サーバーが気づかない」状態になります。

---

## ゴーストプレイヤー問題

```
何が起きるか:
1. プレイヤーがネットワーク障害で切断する
2. サーバーは気づかない（close イベントが来ない）
3. そのプレイヤーは「接続中」のままサーバーに残り続ける
4. 他のプレイヤーには「ゴースト（幽霊）」として見え続ける
5. ゲームが正常に動作しなくなる

解決策: Heartbeat（ping/pong）
→ 定期的に「生きてますか？」と確認する
→ 返事がなければ「切断済み」として扱う
```

---

## Heartbeat の仕組み

```
サーバー → クライアント: ping（「生きてますか？」）
クライアント → サーバー: pong（「生きてます」）

設定例:
  ping送信間隔: 30秒
  pong待ちタイムアウト: 10秒（この間に返事が来なければ切断扱い）
```

WebSocket プロトコルには ping/pong フレームが組み込まれています。
gorilla/websocket でも `SetPingHandler` / `SetPongHandler` で扱えます。

---

## gorilla/websocket の ping/pong

```go
// サーバー側
conn.SetPongHandler(func(string) error {
    // pong を受け取ったら deadline を延長する
    conn.SetReadDeadline(time.Now().Add(pongWait))
    return nil
})

// 定期的に ping を送る
go func() {
    ticker := time.NewTicker(pingInterval)
    for {
        <-ticker.C
        conn.WriteMessage(websocket.PingMessage, nil)
    }
}()

// read deadline を設定（これを超えると ReadMessage がエラーを返す）
conn.SetReadDeadline(time.Now().Add(pongWait))
```

---

## close / error / timeout の違い

| 種類 | 発生原因 | gorilla での挙動 |
|---|---|---|
| close | 正常な切断（`ws.close()`） | `ReadMessage` が `*websocket.CloseError` を返す |
| error | 接続エラー | `ReadMessage` がエラーを返す |
| read deadline | timeout（応答なし） | `ReadMessage` がタイムアウトエラーを返す |

全て `ReadMessage` がエラーを返すことで検知できます。
つまり、「`ReadMessage` がエラーを返したらプレイヤーを削除」という処理でOKです。

---

## 再接続の設計パターン

### パターン1: 切断したら別のプレイヤーとして再入場
最もシンプルです。
- 切断 → サーバーからプレイヤーを削除
- 再接続 → 新しいプレイヤーとして扱う
- 進行途中のスコアや位置は失われる

### パターン2: セッションIDで再接続を識別する
少し複雑ですが、状態を引き継げます。
- 最初の接続時にセッションIDを発行
- クライアントが localStorage にセッションIDを保存
- 再接続時にセッションIDを送ると、前の状態を復元

### クライアント側の再接続

```javascript
function connect() {
    ws = new WebSocket('ws://localhost:8080/ws');

    ws.onclose = () => {
        // 一定時間後に再接続を試みる
        setTimeout(connect, 3000);
    };
}
```

---

## ファイル構成

```
step-10-reconnect/
├── README.md
├── server/
│   ├── main.go
│   └── go.mod
└── client/
    └── index.html
```

---

## 練習問題

1. **観察:** タブを閉じてサーバーのログを見る。切断検知のタイミングを確認する
2. **実験:** デバイスのWiFiをオフにしてからオンにする。タイムアウトまでの時間を計る
3. **応用:** クライアントに自動再接続を実装する（3秒後に再接続）
4. **発展:** セッションIDを使った再接続で、スコアを引き継ぐ処理を実装する

---

## 理解確認クイズ

1. ゴーストプレイヤーとは何ですか？
2. heartbeat がないと何が問題になりますか？
3. ping/pong の役割をそれぞれ説明してください
4. `SetReadDeadline` を使う理由は何ですか？
5. 再接続後に状態を引き継ぐにはどうすればいいですか？

---

## 次のステップへ

Step 10で現実的なネットワーク問題に対応できるようになりました。
Step 11では、「動く」から「効率よく動く」へ。通信量の最適化を学びます。
