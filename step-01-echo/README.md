# Step 1: まずは1本つなぐ（エコーサーバー）

## このステップの目標

- ブラウザからサーバーにWebSocket接続できる
- メッセージを送信し、サーバーがそのまま返せる（エコー）
- open / message / close / error イベントを理解する

**ゲームはまだ作らない。まず「接続」を体で理解する。**

---

## 重要な概念

### WebSocket の4つのイベント

| イベント | タイミング | やること |
|---|---|---|
| `open` | 接続が確立したとき | 「接続完了」と表示 |
| `message` | データを受信したとき | 受け取ったデータを処理 |
| `close` | 接続が閉じたとき | 「切断」と表示 |
| `error` | エラーが発生したとき | エラー内容を表示 |

### このステップで理解すべき本質

HTTPは「リクエストするたびに新しい接続」ですが、
WebSocketは「**一度接続したらずっと維持される**」接続です。

ボタンを押すたびに接続するのではなく、
**最初に一回接続して、その接続を使い回す**のがポイントです。

---

## ファイル構成

```
step-01-echo/
├── README.md
├── server/
│   ├── main.go
│   └── go.mod
└── client/
    └── index.html
```

---

## サーバーコード

`server/go.mod`:
```
module step01-echo

go 1.21

require (
    github.com/gorilla/websocket v1.5.0
    github.com/labstack/echo/v4 v4.11.0
)
```

`server/main.go` の内容は [server/main.go](./server/main.go) を参照。

---

## コード解説

### サーバー側（Go）

```
接続の流れ：
1. クライアントが ws://localhost:8080/ws に接続を要求
2. echo がリクエストを受け取る
3. upgrader.Upgrade() で HTTP → WebSocket に切り替え
4. 無限ループでメッセージを待ち受け
5. 受け取ったメッセージをそのまま返す
```

重要なポイント：
- `upgrader.Upgrade()` が HTTP → WebSocket の「アップグレード」
- `conn.ReadMessage()` はメッセージが来るまでブロックする（待ち続ける）
- `conn.WriteMessage()` でクライアントへ送信
- `defer conn.Close()` で関数終了時に必ず接続を閉じる

### クライアント側（JavaScript）

```javascript
// WebSocket接続の作成
const ws = new WebSocket('ws://localhost:8080/ws');

// 接続確立時
ws.onopen = () => { ... };

// メッセージ受信時
ws.onmessage = (event) => { ... };

// 接続切断時
ws.onclose = () => { ... };

// エラー時
ws.onerror = (error) => { ... };

// 送信
ws.send('テキスト');
```

---

## ローカル起動手順

```bash
# 1. サーバーの依存パッケージを取得
cd step-01-echo/server
go mod tidy

# 2. サーバーを起動
go run main.go

# 3. ブラウザで開く
# client/index.html をブラウザで直接開く（ダブルクリックでOK）
# または http://localhost:8080/ にアクセス（静的ファイル配信している場合）
```

> **Note:** Goが入っていない場合は `https://go.dev/dl/` からインストール

---

## よくあるエラーと対処

### 1. `dial tcp [::1]:8080: connect: connection refused`
**原因:** サーバーが起動していない
**対処:** `go run main.go` でサーバーを起動してから接続する

### 2. `upgrade: websocket: request origin not allowed`
**原因:** CORSポリシーでブロックされている
**対処:** サーバーの `upgrader` に `CheckOrigin: func(r *http.Request) bool { return true }` を追加する（開発用）

### 3. `WebSocket is closed before the connection is established`
**原因:** 接続完了前に `ws.send()` を呼んでいる
**対処:** `ws.onopen` の中で送信する

### 4. ブラウザコンソールに何も表示されない
**対処:** F12 で開発者ツールを開き、Console タブを確認する

---

## 練習問題

1. **基本:** エコーサーバーを動かして、いくつかメッセージを送ってみる
2. **応用:** サーバー側でメッセージを大文字にして返すように変更する
3. **応用:** 送信回数をカウントして、メッセージと一緒に返すように変更する
   - 例: `[1] hello` → `[1] hello` のように番号付きで返す

---

## 理解確認クイズ

1. `ws.onopen` はいつ呼ばれますか？
2. `ws.onmessage` の `event.data` には何が入っていますか？
3. サーバーが `conn.ReadMessage()` を呼んでいる間、何が起きていますか？
4. `defer conn.Close()` を書く理由は何ですか？
5. HTTPとWebSocketで、接続確立の仕方はどう違いますか？

---

## 次のステップへ

Step 1では「1対1」の通信を学びました。
Step 2では「1対多」、つまり複数のクライアントに同じメッセージを送る方法を学びます。

「複数のタブを開いたとき、サーバーはどう管理するのか？」
これが次の疑問です。
