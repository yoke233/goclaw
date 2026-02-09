# Testing Guide

This guide covers testing strategies for goclaw, including unit tests, integration tests, and end-to-end tests.

## Test Structure

```
tests/
├── integration/
│   ├── gateway_test.go      # WebSocket gateway tests
│   ├── failover_test.go     # Provider failover tests
│   └── e2e_test.go          # End-to-end flow tests
├── unit/                    # Unit tests (co-located with source)
└── fixtures/                # Test fixtures and data
```

## Running Tests

### Run All Tests

```bash
go test ./...
```

### Run Specific Package

```bash
go test ./gateway/...
go test ./provider/...
```

### Run Integration Tests

```bash
go test ./tests/integration/...
```

### Run with Coverage

```bash
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Run Specific Test

```bash
go test -run TestGatewayWebSocketConnection ./tests/integration/
```

### Verbose Output

```bash
go test -v ./tests/integration/
```

### Skip Long Tests

```bash
go test -short ./...
```

## Unit Tests

Unit tests should be co-located with the source code.

### Example: Provider Test

```go
// providers/openai_test.go
package providers

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestNewOpenAIProvider(t *testing.T) {
    tests := []struct {
        name    string
        apiKey  string
        baseURL string
        wantErr bool
    }{
        {
            name:    "valid provider",
            apiKey:  "test-key",
            baseURL: "https://api.openai.com/v1",
            wantErr: false,
        },
        {
            name:    "missing api key",
            apiKey:  "",
            baseURL: "https://api.openai.com/v1",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            provider, err := NewOpenAIProvider(tt.apiKey, tt.baseURL, "gpt-4")
            if tt.wantErr {
                assert.Error(t, err)
                assert.Nil(t, provider)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, provider)
            }
        })
    }
}
```

### Example: Message Bus Test

```go
// bus/messagebus_test.go
package bus

import (
    "context"
    "testing"
    "time"
)

func TestMessageBusPublishConsume(t *testing.T) {
    bus := NewMessageBus(10)
    ctx := context.Background()

    msg := &InboundMessage{
        Channel:  "test",
        Content:  "test message",
    }

    // Publish message
    err := bus.PublishInbound(ctx, msg)
    if err != nil {
        t.Fatalf("Failed to publish: %v", err)
    }

    // Consume message
    received, err := bus.ConsumeInbound(ctx)
    if err != nil {
        t.Fatalf("Failed to consume: %v", err)
    }

    if received.Content != msg.Content {
        t.Errorf("Expected %s, got %s", msg.Content, received.Content)
    }
}
```

## Integration Tests

Integration tests test multiple components working together.

### Gateway Integration Tests

Located in `tests/integration/gateway_test.go`:

- **TestGatewayWebSocketConnection**: Tests WebSocket connection establishment
- **TestGatewayRPCMethods**: Tests JSON-RPC method calls
- **TestGatewaySessionMethods**: Tests session-related methods
- **TestGatewayChannelMethods**: Tests channel methods
- **TestGatewayAuthentication**: Tests token-based authentication
- **TestGatewayHeartbeat**: Tests ping-pong mechanism
- **TestGatewayBroadcastOutbound**: Tests message broadcasting

### Failover Integration Tests

Located in `tests/integration/failover_test.go`:

- **TestProviderFailover**: Tests automatic failover
- **TestProviderRotation**: Tests profile rotation
- **TestProviderCooldown**: Tests cooldown mechanism
- **TestCircuitBreaker**: Tests circuit breaker behavior
- **TestErrorClassification**: Tests error classification

### E2E Tests

Located in `tests/integration/e2e_test.go`:

- **TestE2EConversationFlow**: Tests complete conversation flow
- **TestE2ESessionBranching**: Tests session branching/merging
- **TestE2EMemoryIndexing**: Tests memory indexing and search
- **TestE2EFailoverScenario**: Tests failover in realistic scenario

## Test Fixtures

### Mock Provider

```go
// tests/fixtures/mock_provider.go
package fixtures

type MockProvider struct {
    Response string
    Error    error
}

func (m *MockProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
    if m.Error != nil {
        return nil, m.Error
    }
    return &Response{Content: m.Response}, nil
}
```

### Test Messages

```go
// tests/fixtures/messages.go
package fixtures

var TestMessages = []Message{
    {Role: "user", Content: "Hello"},
    {Role: "assistant", Content: "Hi there!"},
    {Role: "user", Content: "How are you?"},
}
```

## Best Practices

### 1. Table-Driven Tests

```go
func TestErrorClassifier(t *testing.T) {
    classifier := NewErrorClassifier()

    tests := []struct {
        name     string
        error    string
        expected FailoverReason
    }{
        {"auth error", "invalid api key", FailoverReasonAuth},
        {"rate limit", "429 too many requests", FailoverReasonRateLimit},
        {"timeout", "context deadline exceeded", FailoverReasonTimeout},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := classifier.ClassifyError(errors.New(tt.error))
            if result != tt.expected {
                t.Errorf("Expected %v, got %v", tt.expected, result)
            }
        })
    }
}
```

### 2. Test Helpers

```go
// tests/helpers.go
package tests

func SetupTestGateway(t *testing.T) (*Gateway, *bus.MessageBus, func()) {
    messageBus := bus.NewMessageBus(100)
    sessionMgr, _ := session.NewManager(t.TempDir())
    channelMgr := channels.NewManager(messageBus)

    cfg := &config.GatewayConfig{
        Host:         "127.0.0.1",
        Port:         18000 + rand.Intn(1000),
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
    }

    server := NewServer(cfg, messageBus, channelMgr, sessionMgr)

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

    if err := server.Start(ctx); err != nil {
        t.Fatalf("Failed to start server: %v", err)
    }

    cleanup := func() {
        server.Stop()
        cancel()
    }

    return server, messageBus, cleanup
}
```

### 3. Context Management

```go
func TestWithTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // Use ctx in test
    result, err := SomeOperation(ctx)
    if err != nil {
        t.Fatalf("Operation failed: %v", err)
    }
}
```

### 4. Temporary Directories

```go
func TestWithTempDir(t *testing.T) {
    tempDir := t.TempDir()

    // Use tempDir for test files
    sessionMgr, err := session.NewManager(tempDir)
    if err != nil {
        t.Fatalf("Failed to create session manager: %v", err)
    }

    // tempDir is automatically cleaned up
}
```

## Coverage Goals

Target >80% code coverage:

```bash
# Check coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total

# View HTML report
go tool cover -html=coverage.out -o coverage.html
```

## Benchmarking

### Write Benchmarks

```go
func BenchmarkProviderChat(b *testing.B) {
    provider := setupTestProvider()
    ctx := context.Background()
    messages := []Message{{Role: "user", Content: "test"}}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := provider.Chat(ctx, messages, nil)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

### Run Benchmarks

```bash
go test -bench=. -benchmem
go test -bench=BenchmarkProviderChat -benchtime=10s
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run tests
        run: |
          go test -v -race -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out
      - name: Upload coverage
        uses: codecov/codecov-action@v3
```

## Debugging Tests

### Verbose Logging

```bash
LOG_LEVEL=debug go test -v ./tests/integration/
```

### Test with Delve

```bash
dlv test ./tests/integration/ -test.run TestGatewayWebSocketConnection
```

### Print Statements

```go
t.Logf("Current state: %+v", state)
t.Helper() // Mark as helper function
```

## Common Pitfalls

### 1. Race Conditions

```go
// Bad
go func() {
    result := someOperation()
    results = append(results, result) // Data race!
}()

// Good
go func() {
    result := someOperation()
    mu.Lock()
    results = append(results, result)
    mu.Unlock()
}()
```

### 2. Leaked Goroutines

```go
func TestWithGoroutines(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel() // Always cancel context

    go func() {
        <-ctx.Done()
        cleanup()
    }()
}
```

### 3. Unhandled Errors

```go
// Bad
_ = someFunction()

// Good
if err := someFunction(); err != nil {
    t.Fatalf("Unexpected error: %v", err)
}
```

## Resources

- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Table Driven Tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [Testify Assertions](https://github.com/stretchr/testify)
