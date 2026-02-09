# Developer Contribution Guide

Thank you for your interest in contributing to goclaw! This guide will help you get started.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Submitting Changes](#submitting-changes)
- [Code Review Process](#code-review-process)

## Getting Started

### Prerequisites

- Go 1.21 or higher
- Git
- Docker (for sandbox testing)
- A code editor (VS Code, GoLand, etc.)

### Fork and Clone

```bash
# Fork the repository on GitHub
# Clone your fork
git clone https://github.com/YOUR_USERNAME/goclaw.git
cd goclaw

# Add upstream remote
git remote add upstream https://github.com/smallnest/dogclaw/goclaw.git
```

## Development Setup

### Install Dependencies

```bash
go mod download
go mod tidy
```

### Build

```bash
go build -o goclaw .
./goclaw --help
```

### Run Tests

```bash
go test ./...
go test -race ./...
```

### Install Development Tools

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install goimports
go install golang.org/x/tools/cmd/goimports@latest

# Install mockgen (for generating mocks)
go install github.com/golang/mock/mockgen@latest
```

## Project Structure

```
goclaw/
├── agent/              # Agent core logic
│   ├── loop.go         # Agent loop
│   ├── context.go      # Context builder
│   ├── memory.go       # Memory management
│   ├── skills.go       # Skills loader
│   └── tools/          # Tool implementations
├── channels/           # Messaging channels
│   ├── base.go         # Channel interface
│   ├── telegram.go     # Telegram
│   ├── whatsapp.go     # WhatsApp
│   └── ...
├── gateway/            # WebSocket gateway
│   ├── server.go       # Server implementation
│   ├── protocol.go     # JSON-RPC protocol
│   └── handler.go      # Request handlers
├── provider/           # Provider failover
│   ├── failover.go     # Failover logic
│   ├── rotation.go     # Auth rotation
│   └── circuit.go      # Circuit breaker
├── providers/          # LLM providers
│   ├── base.go         # Provider interface
│   ├── openai.go       # OpenAI
│   ├── anthropic.go    # Anthropic
│   └── openrouter.go   # OpenRouter
├── config/             # Configuration
│   ├── schema.go       # Config structs
│   └── loader.go       # Config loader
├── session/            # Session management
│   └── manager.go      # Session manager
├── bus/                # Message bus
│   ├── events.go       # Event definitions
│   └── queue.go        # Message queue
├── cli/                # Command-line interface
│   └── root.go         # CLI commands
├── docs/               # Documentation
├── tests/              # Tests
│   └── integration/    # Integration tests
└── main.go             # Entry point
```

## Coding Standards

### Go Conventions

Follow [Effective Go](https://golang.org/doc/effective_go) and the standard Go code style:

```go
// Good
func ProcessMessage(msg *Message) error {
    if msg == nil {
        return fmt.Errorf("message cannot be nil")
    }
    // Process message
    return nil
}

// Bad
func process_message(message *MESSAGE) error {
    //...
}
```

### Package Names

Use short, lowercase package names:

```go
package gateway    // Good
package Gateway    // Bad
package gw         // Bad (too abbreviated)
```

### Error Handling

Always handle errors:

```go
// Good
result, err := someFunction()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}

// Bad
result, _ := someFunction()  // Never ignore errors!
```

### Context Usage

Accept context as first parameter:

```go
func (p *Provider) Chat(ctx context.Context, messages []Message) (*Response, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
        // Proceed with operation
    }
}
```

### Interface Design

Keep interfaces small:

```go
// Good - focused interface
type Sender interface {
    Send(msg Message) error
}

// Bad - god interface
type Manager interface {
    Send(msg Message) error
    Receive() (Message, error)
    Close() error
    Status() string
    // ... many more methods
}
```

### Struct Initialization

Use named parameters for complex structs:

```go
// Good
cfg := &WebSocketConfig{
    Host:         "0.0.0.0",
    Port:         18789,
    EnableAuth:   true,
    PingInterval: 30 * time.Second,
}

// Bad for many fields
cfg := &WebSocketConfig{"0.0.0.0", 18789, true, "", 30*time.Second, ...}
```

## Testing

### Test Organization

Place unit tests next to source code:

```
providers/
├── openai.go
├── openai_test.go
└── openrouter.go
```

Place integration tests in `tests/integration/`:

```
tests/integration/
├── gateway_test.go
├── failover_test.go
└── e2e_test.go
```

### Test Naming

Use descriptive names:

```go
// Good
func TestProviderFailover_OnRateLimit_ShouldUseFallback(t *testing.T)

// Bad
func TestFailover(t *testing.T)
```

### Table-Driven Tests

For multiple test cases:

```go
func TestValidateConfig(t *testing.T) {
    tests := []struct {
        name    string
        config  *Config
        wantErr bool
    }{
        {"valid config", &Config{...}, false},
        {"missing api key", &Config{...}, true},
        {"invalid port", &Config{...}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateConfig(tt.config)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

## Documentation

### Code Comments

Document exported functions and types:

```go
// Chat sends messages to the LLM provider and returns the response.
//
// The messages slice should contain the conversation history, with the
// most recent message last. Tools can be provided for function calling.
//
// Context can be used to cancel long-running requests.
func (p *Provider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
    // Implementation
}
```

### Package Documentation

Each package should have a doc.go:

```go
// Package gateway implements a WebSocket gateway for real-time
// communication with the goclaw agent system.
//
// The gateway uses a JSON-RPC 2.0-like protocol and supports:
//   - Agent method invocation
//   - Session management
//   - Channel operations
//   - Browser automation
//
// Example usage:
//
//   server := gateway.NewServer(cfg, bus, channelMgr, sessionMgr)
//   server.Start(ctx)
package gateway
```

### README for Features

Add README.md for complex features:

```
provider/
├── README.md
├── failover.go
├── rotation.go
└── circuit.go
```

## Submitting Changes

### Branch Strategy

1. Create a feature branch from `master`:
   ```bash
   git checkout master
   git pull upstream master
   git checkout -b feature/my-feature
   ```

2. Make your changes

3. Commit with clear messages:
   ```bash
   git commit -m "Add WebSocket authentication support"
   ```

### Commit Message Format

```
<type>: <subject>

<body>

<footer>
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `test`: Tests only
- `refactor`: Code refactoring
- `perf`: Performance improvement

Example:
```
feat(gateway): Add TLS support for WebSocket connections

- Add cert_file and key_file configuration options
- Implement TLS server configuration
- Update documentation with TLS examples

Closes #123
```

### Pull Request Process

1. Push your branch:
   ```bash
   git push origin feature/my-feature
   ```

2. Create a pull request on GitHub

3. Fill out the PR template:
   - Description of changes
   - Testing performed
   - Breaking changes (if any)
   - Documentation updates

4. Request review from maintainers

5. Address review feedback

6. Once approved, your PR will be merged

## Code Review Process

### Review Checklist

 reviewers will check:

- [ ] Code follows project conventions
- [ ] Tests are included and passing
- [ ] Documentation is updated
- [ ] No breaking changes (or documented)
- [ ] Performance impact considered
- [ ] Security implications reviewed
- [ ] Error handling is proper

### Responding to Feedback

- Address all review comments
- Mark comments as resolved
- Push new commits to your branch
- Request re-review when ready

### Common Review Feedback

**Add Tests:**
```go
// Before
func (p *Provider) Chat(...) {...}

// After
func (p *Provider) Chat(...) {...}

func TestProviderChat(t *testing.T) {
    // Test implementation
}
```

**Handle Errors:**
```go
// Before
result, _ := someFunction()

// After
result, err := someFunction()
if err != nil {
    return nil, fmt.Errorf("failed to get result: %w", err)
}
```

**Add Context:**
```go
// Before
func (p *Provider) Chat(messages []Message) (*Response, error)

// After
func (p *Provider) Chat(ctx context.Context, messages []Message) (*Response, error)
```

## Performance Guidelines

### Avoid Premature Optimization

Profile before optimizing:

```bash
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

### Use Channels Wisely

```go
// Good - buffered channel for async work
ch := make(chan Result, 100)

// Bad - unbuffered channel for high throughput
ch := make(chan Result)  // May cause blocking
```

### Pool Resources

```go
// Reuse buffers
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 0, 1024)
    },
}

func process() {
    buf := bufferPool.Get().([]byte)
    defer bufferPool.Put(buf[:0])
    // Use buf
}
```

## Security Considerations

### Input Validation

Always validate input:

```go
func ValidateToken(token string) error {
    if token == "" {
        return fmt.Errorf("token cannot be empty")
    }
    if len(token) < 32 {
        return fmt.Errorf("token too short")
    }
    return nil
}
```

### Secret Management

Never log secrets:

```go
// Bad
log.Printf("Connecting with API key: %s", apiKey)

// Good
log.Printf("Connecting with API key: %s...", maskKey(apiKey))
```

### Dependency Updates

Regularly update dependencies:

```bash
go get -u ./...
go mod tidy
```

Check for vulnerabilities:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## Getting Help

### Resources

- [Go Documentation](https://golang.org/doc/)
- [Effective Go](https://golang.org/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

### Community

- GitHub Issues: Report bugs and request features
- GitHub Discussions: Ask questions and share ideas
- Pull Requests: Contribute code

### Contact Maintainers

For security issues, please contact maintainers directly rather than creating a public issue.

## License

By contributing to goclaw, you agree that your contributions will be licensed under the MIT License.
