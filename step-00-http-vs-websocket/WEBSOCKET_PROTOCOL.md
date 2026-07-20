# WebSocketプロトコル仕様メモ

このファイルは、Step 0 の補足資料です。
WebSocketを「便利な通信API」としてだけでなく、下の層でどんなプロトコルとして動いているのかを理解するために、ハンドシェイク、フレーム、opcode、マスク、close、ping/pong をまとめます。

WebSocketの基本仕様は RFC 6455 で定義されています。

---

## 1. WebSocketは何のプロトコルか

WebSocketは、1本のTCP接続上で双方向にメッセージを送り合うためのプロトコルです。

HTTPとの大きな違いは、HTTPが「リクエストに対してレスポンスを返す」形なのに対して、WebSocketは接続確立後にクライアントとサーバーのどちらからでもデータを送れることです。

```
HTTP:
  client -> request  -> server
  client <- response <- server

WebSocket:
  client <----------> server
  接続を維持したまま、どちらからでも送れる
```

WebSocketはTCP上で動くため、TCPの特徴を引き継ぎます。

- 順序保証がある
- 再送制御がある
- バイトストリームとして届く
- UDPのように「古いパケットを捨てて新しい状態だけ使う」ことはできない
- 前のデータが詰まると後ろのデータも待たされることがある

ゲームで使う場合は、「届くこと」と「低遅延で届くこと」は別物だと考える必要があります。

---

## 2. 接続確立: HTTP Upgradeハンドシェイク

WebSocket接続は、最初だけHTTPリクエストとして始まります。
クライアントが「このHTTP接続をWebSocketに切り替えたい」と要求し、サーバーが `101 Switching Protocols` を返すと、その後はWebSocketフレームで通信します。

### クライアントからのリクエスト例

```http
GET /ws HTTP/1.1
Host: localhost:8080
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
Origin: http://localhost:8080
```

重要なヘッダー:

| ヘッダー | 意味 |
|---|---|
| `Upgrade: websocket` | WebSocketへ切り替えたいことを示す |
| `Connection: Upgrade` | この接続をアップグレード対象にする |
| `Sec-WebSocket-Key` | クライアントが生成するランダムな値 |
| `Sec-WebSocket-Version: 13` | 現在一般的に使われるWebSocket仕様バージョン |
| `Origin` | ブラウザが送る接続元の情報 |

### サーバーからのレスポンス例

```http
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

`Sec-WebSocket-Accept` は、クライアントから受け取った `Sec-WebSocket-Key` に固定文字列を連結し、SHA-1でハッシュ化してBase64エンコードしたものです。

計算の流れ:

```text
Sec-WebSocket-Accept =
  base64(sha1(Sec-WebSocket-Key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
```

この仕組みにより、サーバーがWebSocketハンドシェイクを理解して応答していることを確認できます。

---

## 3. ws:// と wss://

WebSocket URLには2種類あります。

| URL | 下の層 | 用途 |
|---|---|---|
| `ws://` | 平文TCP | ローカル開発など |
| `wss://` | TLS上のWebSocket | 本番環境 |

`wss://` は HTTPS と同じようにTLSで暗号化されます。
本番のブラウザアプリでは基本的に `wss://` を使います。

HTTPSページから `ws://` へ接続しようとすると、ブラウザのMixed Content制限でブロックされることがあります。

---

## 4. WebSocketのデータ単位: メッセージとフレーム

アプリケーションから見ると、WebSocketでは「メッセージ」を送っています。

```javascript
ws.send("hello");
```

しかしプロトコル上では、データは「フレーム」という単位で流れます。

```
アプリのメッセージ
  ↓
WebSocketフレーム
  ↓
TCPのバイト列
```

通常の短いテキストメッセージなら、1メッセージ = 1フレーム と考えて問題ありません。
ただし大きなメッセージは複数フレームに分割できます。

---

## 5. フレームの基本構造

WebSocketフレームは、ヘッダーとペイロードで構成されます。

```text
0                   1                   2                   3
0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-------+-+-------------+-------------------------------+
|F|R|R|R| opcode|M| Payload len | Extended payload length        |
|I|S|S|S|  (4)  |A|     (7)     |  (16/64 bit if needed)         |
|N|V|V|V|       |S|             |                               |
| |1|2|3|       |K|             |                               |
+-+-+-+-+-------+-+-------------+-------------------------------+
| Masking-key, if MASK set                                      |
+---------------------------------------------------------------+
| Payload data                                                  |
+---------------------------------------------------------------+
```

実際に重要なのは次の項目です。

| フィールド | ビット数 | 意味 |
|---|---:|---|
| `FIN` | 1 | このフレームでメッセージが終わるか |
| `RSV1-3` | 各1 | 拡張機能用。通常は0 |
| `opcode` | 4 | フレームの種類 |
| `MASK` | 1 | ペイロードがマスクされているか |
| `Payload len` | 7 | ペイロード長、または拡張長の目印 |
| `Extended payload length` | 16/64 | 長いペイロード用の追加長 |
| `Masking-key` | 32 | クライアントからサーバーへの送信で使う |
| `Payload data` | 可変 | 実際のデータ |

---

## 6. opcode: フレームの種類

`opcode` は、そのフレームが何を表すかを示します。

| opcode | 種類 | 説明 |
|---:|---|---|
| `0x0` | continuation | 分割メッセージの続き |
| `0x1` | text | UTF-8テキスト |
| `0x2` | binary | バイナリデータ |
| `0x8` | close | 接続終了 |
| `0x9` | ping | 生存確認 |
| `0xA` | pong | pingへの応答 |

この教材では主に `text` フレームを使っています。

Goの `gorilla/websocket` では、例えば以下のように送ります。

```go
conn.WriteMessage(websocket.TextMessage, []byte("hello"))
```

`TextMessage` は `opcode = 0x1` に対応します。

---

## 7. ペイロード長の表し方

WebSocketフレームのペイロード長は、サイズによって表現が変わります。

| 最初の `Payload len` | 意味 |
|---:|---|
| `0`〜`125` | その値がそのままペイロード長 |
| `126` | 続く16bitがペイロード長 |
| `127` | 続く64bitがペイロード長 |

例:

```text
ペイロードが 5 bytes:
  Payload len = 5

ペイロードが 300 bytes:
  Payload len = 126
  Extended payload length = 300

ペイロードが 70000 bytes:
  Payload len = 127
  Extended payload length = 70000
```

小さいメッセージではヘッダーが短く、大きいメッセージでは追加の長さ情報が付きます。

---

## 8. MASK: クライアント送信データは必ずマスクされる

ブラウザなどのクライアントからサーバーへ送るWebSocketフレームは、必ずマスクされます。

```
クライアント -> サーバー: MASK = 1
サーバー -> クライアント: MASK = 0
```

マスクは暗号化ではありません。
4バイトの `Masking-key` を使って、ペイロードの各バイトにXORをかけるだけです。

```text
decoded[i] = encoded[i] XOR maskingKey[i % 4]
```

目的はセキュリティ上の事故を避けるためで、通信内容を秘密にするためではありません。
秘密にしたい場合は `wss://` を使います。

通常、アプリケーションコードでマスク処理を書く必要はありません。
ブラウザやWebSocketライブラリが自動で処理します。

---

## 9. フレーム例: "Hi" を送る

サーバーからクライアントへ `"Hi"` というテキストを送る場合を考えます。

ペイロード:

```text
H  = 0x48
i  = 0x69
```

サーバー送信なのでマスクなしです。

```text
FIN = 1
opcode = 0x1  // text
MASK = 0
Payload len = 2
Payload data = 0x48 0x69
```

先頭2バイトはこうなります。

```text
0x81 0x02 0x48 0x69
```

内訳:

```text
0x81 = 1000 0001
       FIN=1, opcode=0001(text)

0x02 = 0000 0010
       MASK=0, payload length=2
```

クライアントからサーバーへ `"Hi"` を送る場合は、同じ内容でも `MASK=1` になり、4バイトのマスクキーとマスク済みペイロードが付きます。

---

## 10. テキストとバイナリ

WebSocketはテキストとバイナリの両方を送れます。

### テキスト

```javascript
ws.send(JSON.stringify({
  type: "input",
  direction: "up"
}));
```

メリット:

- 読みやすい
- DevToolsで確認しやすい
- 学習・デバッグに向いている

デメリット:

- フィールド名などのオーバーヘッドがある
- 数値も文字列表現になる

### バイナリ

```javascript
const buffer = new ArrayBuffer(2);
const view = new DataView(buffer);
view.setUint8(0, 1); // type
view.setUint8(1, 0); // direction
ws.send(buffer);
```

メリット:

- データ量を小さくしやすい
- パースが速い場合がある

デメリット:

- 人間が読みにくい
- バージョン管理や互換性設計が難しい
- デバッグが難しい

この教材では、理解しやすさを優先してJSONテキストを使っています。

---

## 11. ping / pong

WebSocketには、接続が生きているか確認するための制御フレームがあります。

| フレーム | opcode | 役割 |
|---|---:|---|
| ping | `0x9` | 相手が生きているか確認する |
| pong | `0xA` | pingへの応答 |

ブラウザはサーバーからpingを受け取ると、自動的にpongを返します。
アプリケーションのJavaScriptでpongを手書きする必要はありません。

Goサーバー側では、Step 10 で以下のような heartbeat を扱います。

```go
conn.SetPongHandler(func(string) error {
    conn.SetReadDeadline(time.Now().Add(pongWait))
    return nil
})

conn.WriteControl(websocket.PingMessage, nil, deadline)
```

ping/pongはアプリケーションのゲームデータではなく、接続管理用の制御フレームです。

---

## 12. closeフレーム

WebSocket接続を閉じるときは、TCP接続をいきなり切るのではなく、通常はcloseフレームを送ります。

closeフレームのopcodeは `0x8` です。
ペイロードには close code と理由文字列を含められます。

よく使われるclose code:

| code | 意味 |
|---:|---|
| `1000` | 正常終了 |
| `1001` | going away。ページ遷移やサーバー停止など |
| `1002` | プロトコルエラー |
| `1003` | 受け入れられないデータ種別 |
| `1006` | 異常終了。実際のcloseフレームには入らない特殊コード |
| `1008` | ポリシー違反 |
| `1009` | メッセージが大きすぎる |
| `1011` | サーバー内部エラー |

ブラウザ側では次のように閉じられます。

```javascript
ws.close(1000, "normal close");
```

---

## 13. 分割メッセージとFIN

大きなメッセージは複数フレームに分割できます。

```text
1個目:
  FIN = 0
  opcode = text
  payload = "Hel"

2個目:
  FIN = 0
  opcode = continuation
  payload = "lo "

3個目:
  FIN = 1
  opcode = continuation
  payload = "World"
```

受信側は、`FIN = 1` のフレームまでをつなげて1つのメッセージとして扱います。

アプリケーションコードでは、多くの場合ライブラリが分割と再構築を隠蔽します。
`ReadMessage()` で読めるのは、フレームではなく再構築済みのメッセージです。

---

## 14. 拡張機能と圧縮

フレームヘッダーには `RSV1`、`RSV2`、`RSV3` という予約ビットがあります。
通常は0ですが、拡張機能を使うと意味を持つことがあります。

代表例は `permessage-deflate` です。
これはWebSocketメッセージを圧縮する拡張です。

圧縮のメリット:

- JSONなどテキストデータの通信量を減らせる

圧縮のデメリット:

- CPUを使う
- 小さいメッセージでは効果が薄い
- リアルタイムゲームでは遅延要因になる場合がある
- 設定を誤るとセキュリティ上の注意点が増える

まずは圧縮なしでサイズと頻度を測り、必要になってから検討するのが安全です。

---

## 15. サブプロトコル

WebSocketには `Sec-WebSocket-Protocol` というヘッダーがあります。
これは「WebSocketの上でどのアプリケーションプロトコルを話すか」を決めるための仕組みです。

例:

```http
Sec-WebSocket-Protocol: chat.v1, game.v2
```

サーバーは対応するものを1つ選んで返します。

```http
Sec-WebSocket-Protocol: game.v2
```

この教材ではサブプロトコルを使わず、JSON内の `type` フィールドでメッセージ種別を分けています。

---

## 16. Originと認証

ブラウザからのWebSocket接続には `Origin` ヘッダーが付きます。
サーバーはこれを見て、許可したWebサイトからの接続かを確認できます。

この教材のGoコードでは学習用に以下のような設定を使っています。

```go
CheckOrigin: func(r *http.Request) bool { return true }
```

これは全Originを許可する開発用設定です。
本番では、許可するOriginを限定してください。

認証が必要な場合は、例えば以下の方法があります。

- Cookieを使う
- 最初のHTTP Upgradeリクエストで認証情報を確認する
- URLのqueryに短命トークンを付ける
- 接続直後の最初のメッセージで認証する

ただし、URLに長寿命の秘密情報を入れるとログに残る危険があります。

---

## 17. HTTP/2やHTTP/3との関係

基本のWebSocket仕様はHTTP/1.1のUpgradeを前提にしています。
現在はHTTP/2上でWebSocket相当の接続を扱う仕様もありますが、学習段階ではまずHTTP/1.1 Upgradeとして理解すれば十分です。

アプリケーションコードでは、ブラウザの `new WebSocket(...)` とサーバーライブラリが下の細部を処理します。

---

## 18. この教材で使うメッセージ設計との関係

この教材では、WebSocketのペイロードにJSONを入れています。

例:

```json
{
  "type": "input",
  "direction": "up"
}
```

これはWebSocket仕様そのものではなく、「WebSocket上に載せるアプリケーション独自のプロトコル」です。

階層で見るとこうなります。

```text
TCP
  └── WebSocketフレーム
        └── Text payload
              └── JSON
                    └── { "type": "input", "direction": "up" }
```

WebSocketは「どう運ぶか」を決めます。
JSONの `type` や `direction` は「何を意味するか」をアプリ側で決めています。

---

## 19. DevToolsで確認するポイント

ChromeやFirefoxの開発者ツールでWebSocket通信を確認できます。

1. DevToolsを開く
2. Networkタブを開く
3. `WS` フィルターを選ぶ
4. `/ws` の接続をクリックする
5. Messages / Frames を見る

確認するとよい項目:

- 接続時に `101 Switching Protocols` が返っているか
- 送受信しているメッセージの中身
- `type: "input"` や `type: "state"` が期待通りか
- メッセージ頻度が多すぎないか
- 切断時にcloseが発生しているか

DevToolsではアプリケーションメッセージを見やすく表示してくれます。
実際のマスクキーやフレームヘッダーの全バイトまで常に見えるわけではありません。

---

## 20. まとめ

WebSocketを理解するときは、次の3層を分けて考えると整理しやすくなります。

```text
1. 接続確立
   HTTP Upgrade handshake

2. 転送方式
   WebSocket frame
   opcode / FIN / MASK / payload length / ping / pong / close

3. アプリケーション設計
   JSON message
   type: input / state / stats
   サーバー権威型の状態管理
```

この教材で直接書くのは主に3層目です。
1層目と2層目は、ブラウザや `gorilla/websocket` が処理してくれます。
ただし、遅延、切断、ping/pong、データ量の問題を理解するには、下のプロトコル仕様を知っておくと判断しやすくなります。
