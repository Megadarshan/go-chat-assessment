# Simple Go Chat App

This is a basic chat server implemented in Go using Server-Sent Events (SSE) and minimalistic HTML UI. Clients may join, broadcast messages, and exit the chat. The server will automatically disconnect inactive users with a timeout.

## ⚙️ Features

- Live chat using SSE
- Removal of idle clients automatically (after 30 seconds of inactivity)
- Message history when reconnecting (only when it was last disconnect)
- Integrated static HTML UI
- Simple REST endpoints for interaction

## 🔗 API Endpoints

- GET /join?id=CLIENT_ID - Join the chat
- GET /send?id=CLIENT_ID&message=TEXT - Send message
- GET /leave?id=CLIENT_ID - Leave the chat
- GET /messages?id=CLIENT_ID - Stream messages (SSE)

## ⚠️ Known Bug / Limitation

- If a user is dropped because they've been inactive, the front-end will not automatically reconnect. Rejoining or Reloading the page might be required.
- When a new client joins, the client's chat window displays `You joined the chat` twice. Need to tweak this in the UI section.
- Clients don't get messages sent while they were offline.
- The client buffer channel (MsgChan) is fixed at 100 for now. If too many messages come in rapidly, messages may be dropped for slow clients.

## ✅ To Do

- User typing indicators
- Persistent user list display on UI
