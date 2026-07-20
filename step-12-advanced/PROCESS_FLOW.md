# Step 12 処理フロー図

このファイルは `step-12-advanced` の処理フローをまとめたものです。

`step-12-advanced` は実装コードではなく、1台構成を超えるための設計トピックを扱う README 中心の step です。

## 1台構成から複数サーバー構成へ

```mermaid
flowchart TD
    A[同時接続数が増える] --> B{1台で処理できるか}
    B -- はい --> C[単一 Go サーバーで継続]
    B -- いいえ --> D[複数サーバー構成を検討]
    D --> E[ロードバランサーを置く]
    E --> F{部屋の担当をどう決めるか}
    F --> G[部屋ごとに担当サーバーを固定]
    G --> H[同じ部屋のプレイヤーを同じサーバーへ誘導]
    H --> I[部屋内 broadcast はサーバー内で完結]
    F --> J[サーバーをまたぐ情報共有が必要]
    J --> K[Redis Pub/Sub などを利用]
```

## Redis Pub/Sub の概念フロー

```mermaid
flowchart LR
    A[Server 1] -->|publish: room/global event| B[(Redis Pub/Sub)]
    B -->|subscribe| C[Server 2]
    B -->|subscribe| D[Server 3]
    C --> E[Server 2 配下の clients へ通知]
    D --> F[Server 3 配下の clients へ通知]
```

## 通信方式選択フロー

```mermaid
flowchart TD
    A[リアルタイム通信が必要] --> B{双方向通信か}
    B -- いいえ --> C[Server-Sent Events]
    B -- はい --> D{順序保証と信頼性が重要か}
    D -- はい --> E[WebSocket]
    D -- いいえ --> F{低遅延を最優先するか}
    F -- はい --> G[WebRTC DataChannel]
    F -- 将来選択肢 --> H[QUIC / HTTP/3]
```

## 重要ポイント

- WebSocket は接続が特定サーバーに固定されるため、通常の HTTP よりスケール設計に注意が必要。
- 蛇ゲームのように部屋内だけで完結する処理は、部屋ごとに担当サーバーを固定すると設計しやすい。
- Redis Pub/Sub は、部屋をまたぐ通知・全体人数・ランキング・サーバー間イベント共有に向いている。
- WebSocket はチャットや蛇ゲームには十分だが、FPS など極端な低遅延用途では WebRTC DataChannel なども候補になる。
