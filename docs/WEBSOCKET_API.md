# WebSocket Gateway API Documentation

## Overview

The goclaw WebSocket gateway provides real-time bidirectional communication with the agent system. It uses a JSON-RPC 2.0-like protocol for structured method calls and responses.

## Connection

### Default Endpoint
```
ws://localhost:18789/ws
```

### Connection with Authentication
```
ws://localhost:18789/ws?token=YOUR_AUTH_TOKEN
```

Or using Authorization header:
```
Authorization: Bearer YOUR_AUTH_TOKEN
```

### Welcome Message

Upon successful connection, the server sends a welcome message:

```json
{
  "jsonrpc": "2.0",
  "method": "connected",
  "params": {
    "session_id": "uuid-string",
    "version": "1.0"
  }
}
```

## Protocol

### Request Format

All requests follow JSON-RPC 2.0 format:

```json
{
  "jsonrpc": "2.0",
  "id": "request-id",
  "method": "method.name",
  "params": {
    "param1": "value1",
    "param2": "value2"
  }
}
```

### Response Format

```json
{
  "jsonrpc": "2.0",
  "id": "request-id",
  "result": {
    // Response data
  }
}
```

### Error Response

```json
{
  "jsonrpc": "2.0",
  "id": "request-id",
  "error": {
    "code": -32601,
    "message": "Method not found",
    "data": "Additional error details"
  }
}
```

## Available Methods

### System Methods

#### health
Health check endpoint.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "method": "health",
  "params": {}
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "result": {
    "status": "ok",
    "timestamp": 1234567890,
    "version": "1.0"
  }
}
```

#### config.get
Get configuration value.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "2",
  "method": "config.get",
  "params": {
    "key": "model"
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "2",
  "result": {
    "key": "model",
    "value": "gpt-4"
  }
}
```

#### config.set
Set configuration value.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "3",
  "method": "config.set",
  "params": {
    "key": "temperature",
    "value": 0.7
  }
}
```

#### logs.get
Get recent logs.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "4",
  "method": "logs.get",
  "params": {
    "lines": 100
  }
}
```

### Agent Methods

#### agent
Send a message to the agent (non-blocking).

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "5",
  "method": "agent",
  "params": {
    "content": "Hello, how are you?"
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "5",
  "result": {
    "status": "queued",
    "msg_id": "msg-uuid"
  }
}
```

#### agent.wait
Send a message and wait for response.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "6",
  "method": "agent.wait",
  "params": {
    "content": "What is the weather?",
    "timeout": 30
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "6",
  "result": {
    "status": "waiting",
    "msg_id": "msg-uuid",
    "timeout": "30s"
  }
}
```

### Session Methods

#### sessions.list
List all sessions.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "7",
  "method": "sessions.list",
  "params": {}
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "7",
  "result": [
    {
      "key": "telegram:user123",
      "message_count": 45,
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-02T12:00:00Z"
    }
  ]
}
```

#### sessions.get
Get session details.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "8",
  "method": "sessions.get",
  "params": {
    "key": "telegram:user123"
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "8",
  "result": {
    "key": "telegram:user123",
    "messages": [...],
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-02T12:00:00Z",
    "metadata": {}
  }
}
```

#### sessions.clear
Clear session messages.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "9",
  "method": "sessions.clear",
  "params": {
    "key": "telegram:user123"
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "9",
  "result": {
    "status": "cleared",
    "key": "telegram:user123"
  }
}
```

### Channel Methods

#### channels.status
Get channel status.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "10",
  "method": "channels.status",
  "params": {
    "channel": "telegram"
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "10",
  "result": {
    "name": "telegram",
    "enabled": true
  }
}
```

#### channels.list
List all channels.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "11",
  "method": "channels.list",
  "params": {}
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "11",
  "result": {
    "channels": ["telegram", "whatsapp", "feishu"]
  }
}
```

#### send
Send message to a channel.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "12",
  "method": "send",
  "params": {
    "channel": "telegram",
    "chat_id": "user123",
    "content": "Hello from WebSocket!"
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "12",
  "result": {
    "status": "sent",
    "msg_id": "msg-uuid",
    "channel": "telegram",
    "chat_id": "user123"
  }
}
```

#### chat.send
Send chat message (alias for send).

### Browser Methods

#### browser.request
Execute browser action.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "13",
  "method": "browser.request",
  "params": {
    "action": "navigate",
    "url": "https://example.com"
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "13",
  "result": {
    "status": "executed",
    "action": "navigate",
    "result": "browser action executed"
  }
}
```

## Notifications

The server can send unsolicited notifications to clients:

### message.outbound
Broadcast when an outbound message is sent.

```json
{
  "jsonrpc": "2.0",
  "method": "message.outbound",
  "params": {
    "data": {
      "channel": "telegram",
      "chat_id": "user123",
      "content": "Response from agent",
      "timestamp": "2024-01-01T00:00:00Z"
    }
  }
}
```

## Heartbeat

The server sends ping messages every 30 seconds. Clients must respond with pong messages to maintain the connection.

## Error Codes

| Code | Description |
|------|-------------|
| -32700 | Parse error |
| -32600 | Invalid request |
| -32601 | Method not found |
| -32602 | Invalid params |
| -32603 | Internal error |

## Configuration

### WebSocket Configuration

Add to your `config.json`:

```json
{
  "gateway": {
    "host": "0.0.0.0",
    "port": 8080,
    "websocket": {
      "host": "0.0.0.0",
      "port": 18789,
      "path": "/ws",
      "enable_auth": false,
      "auth_token": "",
      "ping_interval": "30s",
      "pong_timeout": "60s",
      "read_timeout": "60s",
      "write_timeout": "10s"
    }
  }
}
```

### TLS Configuration

For secure WebSocket connections (wss://):

```json
{
  "gateway": {
    "websocket": {
      "enable_tls": true,
      "cert_file": "/path/to/cert.pem",
      "key_file": "/path/to/key.pem"
    }
  }
}
```

## Example Client Code

### JavaScript (Browser)

```javascript
const ws = new WebSocket('ws://localhost:18789/ws');

ws.onopen = () => {
  console.log('Connected to goclaw WebSocket');

  // Send a request
  ws.send(JSON.stringify({
    jsonrpc: '2.0',
    id: '1',
    method: 'health',
    params: {}
  }));
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  console.log('Received:', message);

  if (message.method === 'connected') {
    console.log('Session ID:', message.params.session_id);
  }
};

ws.onerror = (error) => {
  console.error('WebSocket error:', error);
};

ws.onclose = () => {
  console.log('WebSocket connection closed');
};
```

### Python

```python
import asyncio
import websockets
import json

async def goclaw_client():
    uri = "ws://localhost:18789/ws"
    async with websockets.connect(uri) as websocket:
        # Send health check
        request = {
            "jsonrpc": "2.0",
            "id": "1",
            "method": "health",
            "params": {}
        }
        await websocket.send(json.dumps(request))

        # Receive response
        response = await websocket.recv()
        data = json.loads(response)
        print(f"Received: {data}")

asyncio.run(goclaw_client())
```

### Go

```go
package main

import (
    "log"
    "net/url"
    "github.com/gorilla/websocket"
)

func main() {
    u := url.URL{Scheme: "ws", Host: "localhost:18789", Path: "/ws"}
    log.Printf("Connecting to %s", u.String())

    c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
    if err != nil {
        log.Fatal("dial:", err)
    }
    defer c.Close()

    // Send health check
    request := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      "1",
        "method":  "health",
        "params":  map[string]interface{}{},
    }
    if err := c.WriteJSON(request); err != nil {
        log.Fatal("write:", err)
    }

    // Read response
    var response map[string]interface{}
    if err := c.ReadJSON(&response); err != nil {
        log.Fatal("read:", err)
    }
    log.Printf("Received: %v", response)
}
```

## Troubleshooting

### Connection Issues

1. **Verify server is running:**
   ```bash
   curl http://localhost:18789/health
   ```

2. **Check firewall settings:**
   Ensure port 18789 is not blocked.

3. **Verify authentication:**
   If auth is enabled, include valid token.

### Message Not Received

1. **Check message format:**
   Ensure JSON-RPC format is correct.

2. **Verify method name:**
   Method names are case-sensitive.

3. **Check parameters:**
   Required parameters must be included.

### Connection Drops

1. **Implement ping/pong:**
   Clients should respond to ping messages.

2. **Increase timeout:**
   Adjust `pong_timeout` in configuration.

3. **Check network stability:**
   Intermittent network issues can cause disconnections.
