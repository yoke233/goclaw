# Backend Architecture Guidance

## Document Version: 1.0
**Date**: 2025-02-09
**Owner**: architect
**Status**: Official Guidance for backend teammate

---

## Current Implementation Status

### ✅ Already Implemented

**WebSocket Gateway** (`/Users/chaoyuepan/ai/goclaw/gateway/`):
- `server.go`: HTTP + WebSocket servers on separate ports
- `protocol.go`: JSON-RPC 2.0 protocol implementation
- `handler.go`: Method registry with system, agent, channel, and browser methods

**Key Design Decisions Already Made**:
1. **Separate ports**: HTTP on configured port, WebSocket on 18789
2. **JSON-RPC protocol**: Implemented in `gateway/protocol.go`
3. **Method registry**: Pattern for registering and calling methods
4. **Connection management**: Connection pool with heartbeat

---

## Task #2: WebSocket Gateway - Guidance

### Q1: Port Configuration

**Current State**: ✅ Correctly implemented
- HTTP server: Configured port (from config)
- WebSocket server: Port 18789 (hardcoded in `wsConfig`)

**Recommendation**: Keep the current design
- Separate ports allows independent scaling
- WebSocket on 18789 matches openclaw convention
- Consider making WebSocket port configurable in `config/schema.go`:

```go
// In config/schema.go
type GatewayConfig struct {
    Host         string        `mapstructure:"host" json:"host"`
    Port         int           `mapstructure:"port" json:"port"`
    WebSocketPort int          `mapstructure:"websocket_port" json:"websocket_port"` // ADD THIS
    ReadTimeout  time.Duration `mapstructure:"read_timeout" json:"read_timeout"`
    WriteTimeout time.Duration `mapstructure:"write_timeout" json:"write_timeout"`
}
```

### Q2: Protocol Package Location

**Current State**: ✅ Correctly implemented
- JSON-RPC protocol in `gateway/protocol.go`
- Clean separation: protocol definitions in one file

**Recommendation**: Keep the current design
- Protocol is gateway-specific, not a separate package
- If protocol grows >500 lines, consider `gateway/protocol/` directory
- Current structure is appropriate for the scope

### Q3: Server Integration

**Current State**: ✅ Correctly implemented
- `server.go` contains both HTTP and WebSocket servers
- `startHTTPServer()` and `startWebSocketServer()` methods
- Clean separation in same file

**Recommendation**: Keep the current design
- Single server struct manages both
- Shared configuration and lifecycle
- Easy to coordinate shutdown

---

## Task #6: Provider Failover - Guidance

### Q1: Auth Rotation Architecture

**Recommendation**: Create a new provider wrapper

**Rationale**:
- Provider interface remains clean
- Failover logic centralized in one place
- Easy to add new providers without modifying existing code
- Matches openclaw's approach

**Implementation**:

```go
// providers/failover.go
package providers

import (
    "context"
    "sync"
    "time"
)

// FailoverProvider 多提供商故障转移
type FailoverProvider struct {
    profiles       []*AuthProfile
    errorClassifier *ErrorClassifier
    retryPolicy    RetryPolicy
    mu             sync.RWMutex
}

// AuthProfile 认证配置
type AuthProfile struct {
    ID            string
    Provider      string  // "anthropic", "openai", "openrouter"
    APIKey        string
    BaseURL       string
    Priority      int
    CooldownUntil time.Time
    FailureCount  int
    LastError     string
}

// NewFailoverProvider 创建故障转移提供商
func NewFailoverProvider(profiles []*AuthProfile) (*FailoverProvider, error) {
    // Sort by priority
    sortProfiles(profiles)

    return &FailoverProvider{
        profiles:       profiles,
        errorClassifier: NewErrorClassifier(),
        retryPolicy:    NewDefaultRetryPolicy(nil),
    }, nil
}

// Chat 实现Provider接口
func (f *FailoverProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
    var lastErr error

    // Try each profile
    for _, profile := range f.getActiveProfiles() {
        // Check cooldown
        if profile.CooldownUntil.After(time.Now()) {
            continue
        }

        // Create provider instance
        provider, err := f.createProvider(profile)
        if err != nil {
            continue
        }

        // Try request
        resp, err := provider.Chat(ctx, messages, tools, options...)
        if err == nil {
            // Success - reset failure count
            f.resetFailure(profile)
            return resp, nil
        }

        lastErr = err

        // Classify error
        reason := f.errorClassifier.Classify(err.Error())

        // Handle failover
        switch reason {
        case FailoverAuth, FailoverQuota:
            f.markFailed(profile, 5*time.Minute)
        case FailoverRateLimit:
            f.markFailed(profile, 1*time.Minute)
        case FailoverTimeout:
            // Retry with same provider
            continue
        default:
            // Unknown error - try next profile
            continue
        }
    }

    return nil, lastErr
}
```

**Configuration Integration**:

```go
// config/schema.go
type ProvidersConfig struct {
    Anthropic  AnthropicProviderConfig  `mapstructure:"anthropic" json:"anthropic"`
    OpenAI     OpenAIProviderConfig     `mapstructure:"openai" json:"openai"`
    OpenRouter OpenRouterProviderConfig `mapstructure:"openrouter" json:"openrouter"`

    // NEW: Multi-profile support
    Profiles []ProviderProfileConfig `mapstructure:"profiles" json:"profiles"`
    Failover FailoverConfig          `mapstructure:"failover" json:"failover"`
}

type ProviderProfileConfig struct {
    ID       string `mapstructure:"id" json:"id"`
    Provider string `mapstructure:"provider" json:"provider"`
    APIKey   string `mapstructure:"api_key" json:"api_key"`
    Priority int    `mapstructure:"priority" json:"priority"`
}

type FailoverConfig struct {
    Enabled       bool     `mapstructure:"enabled" json:"enabled"`
    Cooldown      int      `mapstructure:"cooldown_seconds" json:"cooldown_seconds"`
    ProviderOrder []string `mapstructure:"provider_order" json:"provider_order"`
}
```

### Q2: Circuit Breaker State Persistence

**Recommendation**: In-memory only (no persistence)

**Rationale**:
- Cooldown periods are short (minutes, not hours)
- State rebuilds quickly on restart
- Avoids complexity of distributed state
- Matches openclaw's approach

**Implementation**:

```go
// In FailoverProvider struct
type FailoverProvider struct {
    profiles       []*AuthProfile
    errorClassifier *ErrorClassifier
    retryPolicy    RetryPolicy
    mu             sync.RWMutex

    // Optional: Add metrics
    metrics *FailoverMetrics
}

type FailoverMetrics struct {
    TotalRequests    int64
    SuccessfulCalls  int64
    FailedCalls      int64
    FailoverCount    int64
    CooldownCount    int64
}
```

**If persistence is needed later**:
- Add Redis support for distributed deployments
- Use SQLite for single-server persistence
- Key: `failover:{profile_id}` with TTL

### Q3: Failover Behavior

**Recommendation**: Automatic failover with per-request override

**Rationale**:
- Default behavior should be automatic (zero configuration)
- Allow manual provider selection for specific requests
- Debug mode to disable failover

**Implementation**:

```go
// providers/base.go - Add new option
type ChatOptions struct {
    Model         string
    Temperature   float64
    MaxTokens     int
    Stream        bool

    // NEW: Failover control
    ProviderID    string  // Force specific provider (skip failover)
    DisableFailover bool  // Disable automatic failover
}

// New option constructor
func WithProvider(providerID string) ChatOption {
    return func(o *ChatOptions) {
        o.ProviderID = providerID
    }
}

func WithoutFailover() ChatOption {
    return func(o *ChatOptions) {
        o.DisableFailover = true
    }
}

// Usage
provider.Chat(ctx, messages, tools,
    WithProvider("anthropic-primary"),  // Force specific profile
    WithoutFailover(),                  // Disable failover
)
```

**Configuration**:

```json
{
  "providers": {
    "failover": {
      "enabled": true,
      "auto_rotate": true,
      "cooldown_seconds": 300,
      "provider_order": ["anthropic", "openai", "openrouter"]
    }
  }
}
```

---

## Integration with Existing Components

### Message Bus Integration

The WebSocket gateway already integrates with `bus.MessageBus`:

```go
// In handler.go - agent method
h.registry.Register("agent", func(sessionID string, params map[string]interface{}) (interface{}, error) {
    msg := &bus.InboundMessage{
        Channel:   "websocket",
        SenderID:  sessionID,
        ChatID:    sessionID,
        Content:   content,
        Timestamp: time.Now(),
    }

    if err := h.bus.PublishInbound(context.Background(), msg); err != nil {
        return nil, fmt.Errorf("failed to publish message: %w", err)
    }

    return map[string]interface{}{
        "status": "queued",
        "msg_id": msg.ID,
    }, nil
})
```

### Provider Factory Integration

Modify `providers/factory.go` to create failover provider:

```go
// providers/factory.go
func NewProvider(cfg *config.Config) (Provider, error) {
    // Check if multi-profile mode
    if len(cfg.Providers.Profiles) > 0 {
        return NewFailoverProviderFromConfig(cfg)
    }

    // Legacy single-provider mode
    providerType, model, err := determineProvider(cfg)
    if err != nil {
        return nil, err
    }

    switch providerType {
    case ProviderTypeOpenAI:
        return NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.BaseURL, model)
    // ... other cases
    }
}

func NewFailoverProviderFromConfig(cfg *config.Config) (Provider, error) {
    profiles := make([]*AuthProfile, 0, len(cfg.Providers.Profiles))

    for _, p := range cfg.Profiles.Profiles {
        profiles = append(profiles, &AuthProfile{
            ID:       p.ID,
            Provider: p.Provider,
            APIKey:   p.APIKey,
            Priority: p.Priority,
        })
    }

    return NewFailoverProvider(profiles)
}
```

---

## Testing Strategy

### Unit Tests

```go
// providers/failover_test.go
func TestFailoverProvider_Rotation(t *testing.T) {
    profiles := []*AuthProfile{
        {ID: "primary", Provider: "anthropic", APIKey: "key1", Priority: 1},
        {ID: "secondary", Provider: "openai", APIKey: "key2", Priority: 2},
    }

    provider, _ := NewFailoverProvider(profiles)

    // Mock primary to fail
    // Test that secondary is used
}

func TestFailoverProvider_Cooldown(t *testing.T) {
    // Test that failed profiles are skipped
    // Test that cooldown expires
}
```

### Integration Tests

```go
// gateway/integration_test.go
func TestWebSocketGateway_AuthFlow(t *testing.T) {
    // Test WebSocket connection
    // Test authentication
    // Test method calls
}
```

---

## Next Steps

### Immediate (Week 1)
1. Review existing WebSocket implementation
2. Add configurable WebSocket port to config
3. Implement failover provider structure
4. Add auth profile configuration schema

### Short-term (Week 2)
1. Implement failover logic with error classification
2. Add metrics and monitoring
3. Write unit tests for failover
4. Integration tests with existing providers

### Medium-term (Week 3-4)
1. Add circuit breaker pattern
2. Implement health checks for providers
3. Add retry with exponential backoff
4. Documentation and examples

---

## Files to Modify/Create

### Existing Files to Modify:
- `config/schema.go` - Add failover configuration
- `providers/factory.go` - Add failover provider creation
- `providers/base.go` - Add failover control options

### New Files to Create:
- `providers/failover.go` - Failover provider implementation
- `providers/failover_test.go` - Unit tests
- `providers/error_classifier.go` - Enhanced error classification
- `docs/failover.md` - Documentation

---

## Open Questions?

If you have questions about:
1. Specific implementation details
2. Integration with existing components
3. Testing strategies
4. Configuration options

Please ask and I'll provide detailed guidance.

---

**Document Version**: 1.0
**Last Updated**: 2025-02-09
**Owner**: architect
