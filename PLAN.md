# goclaw Implementation Plan

## Executive Summary

This plan outlines the architecture and implementation roadmap for enhancing goclaw (Go-based AI agent framework) to achieve feature parity with openclaw (TypeScript-based personal AI assistant). The plan is organized by priority phases with clear technical specifications, tech stack decisions, and integration points.

## Current State Analysis

### goclaw Current Capabilities
- **Channels**: 5 channels (Telegram, WhatsApp, Feishu, QQ, WeWork)
- **Tools**: 11 tools (FileSystem, Shell, Web, Browser, SmartSearch, Message, Spawn, Skill, Subagent, Filesystem, BrowserSession)
- **Skills**: 56+ skills with OpenClaw/AgentSkills compatibility
- **Providers**: 3 LLM providers (OpenAI, Anthropic, OpenRouter)
- **Session**: JSONL-based persistent sessions
- **Gateway**: Basic HTTP webhook gateway
- **Agent Loop**: Event-driven with error classification and retry logic

### openclaw Reference Capabilities
- **Channels**: 15+ channels with unified WebSocket gateway
- **Memory**: Vector-based semantic search with SQLite-vec
- **Auth**: Multi-profile rotation with failover
- **Session**: Tree-structured sessions with branching
- **Streaming**: Block-based chunking with thinking tag handling
- **Real-time**: WebSocket gateway with bidirectional communication
- **Browser**: Advanced automation with CDP
- **Voice**: TTS/STT integration

## Priority Matrix (Impact vs Complexity)

### High Impact, Low Complexity (Quick Wins)
1. **Error Classification Enhancement** - Better retry/failover decisions
2. **Thinking Tag Handling** - Strip `<thinking>` tags from responses
3. **Final Block Handling** - Strip `<final>` control blocks
4. **Block Chunking** - Intelligent paragraph-based streaming
5. **Owner-Only Tools** - Privileged operation marking
6. **DM vs Group History Limits** - Different history retention

### High Impact, Medium Complexity
1. **Multi-Provider Auth Rotation** - Multiple API keys per provider
2. **Automatic Retry with Backoff** - Per-reason retry strategies
3. **Message Tool Deduplication** - Prevent duplicate sends
4. **Session Manager Caching** - In-memory session cache
5. **Hierarchical Tool Policy** - 5-level policy resolution
6. **WebSocket Gateway** - Real-time bidirectional communication
7. **Vector Memory System** - Semantic search with embeddings

### High Impact, High Complexity
1. **Tree-Structured Sessions** - Conversation branching
2. **Auto-Compaction** - Context overflow handling
3. **Extensions System** - Plugin architecture for session behavior
4. **Advanced Browser Automation** - CDP integration
5. **Vector Memory with Hybrid Search** - FTS + vector search

### Medium Impact, Low Complexity
1. **Sandbox Information** - Docker container metadata
2. **Reply Tags** - Special reply syntax
3. **CLI Reference** - Command help in system prompt
4. **Runtime Metadata** - Session info in prompt
5. **Google Gemini Provider** - Additional LLM support

## Architecture Design

### 1. WebSocket Gateway

#### Purpose
Enable real-time bidirectional communication between clients and the goclaw agent, supporting:
- Live streaming responses
- Real-time tool execution feedback
- Multiple concurrent connections
- Authentication and authorization

#### Tech Stack
- **gorilla/websocket**: WebSocket library for Go
- **jwt-go**: JWT token authentication
- **middleware**: Request authentication and rate limiting

#### Architecture Components

```go
// Gateway Structure
type GatewayServer struct {
    server      *http.Server
    upgrader    *websocket.Upgrader
    clients     map[string]*ClientConnection
    auth        *AuthManager
    bus         *bus.MessageBus
    channelMgr  *channels.Manager
}

type ClientConnection struct {
    ID          string
    AuthToken   string
    UserID      string
    Channel     string
    SendMessage chan []byte
    Subscriptions map[string]bool
}

type AuthManager struct {
    jwtSecret   string
    tokens      map[string]*TokenInfo
    tailscaleMode bool
}

type TokenInfo struct {
    UserID      string
    ExpiresAt   time.Time
    Permissions []string
}
```

#### API Endpoints
- `GET /ws` - WebSocket connection endpoint
- `POST /auth/token` - Generate auth token
- `POST /auth/refresh` - Refresh auth token
- `GET /health` - Health check

#### Message Protocol
```json
{
  "type": "request|response|event|error",
  "id": "unique-message-id",
  "method": "chat|tool|session|status",
  "payload": { ... }
}
```

#### Integration Points
- `bus.MessageBus` - Subscribe to events
- `channels.Manager` - Send outbound messages
- `session.Manager` - Session operations

### 2. Multi-Provider Failover System

#### Purpose
Implement robust multi-provider authentication with automatic rotation and failover:
- Multiple API keys per provider
- Automatic rotation on failure
- Cooldown tracking for failed keys
- Provider-level failover

#### Tech Stack
- **go-redis**: For distributed cooldown tracking (optional)
- **sync**: In-memory state management

#### Architecture Components

```go
type AuthProfile struct {
    ID            string
    Provider      string
    APIKey        string
    Priority      int
    CooldownUntil time.Time
    FailureCount  int
    LastError     string
    RateLimit     *RateLimitInfo
}

type RateLimitInfo struct {
    RequestsMade int
    WindowStart  time.Time
    Limit        int
    Remaining    int
    ResetAt      time.Time
}

type AuthProfileStore struct {
    profiles map[string][]*AuthProfile  // provider -> profiles
    mu       sync.RWMutex
}

type FailoverManager struct {
    profiles       *AuthProfileStore
    errorClassifier *ErrorClassifier
    retryPolicy    RetryPolicy
    maxRetries     int
}
```

#### Failover Logic
1. Try current profile
2. On error, classify failure type
3. If auth/quota error, rotate to next profile
4. Apply cooldown to failed profile
5. If all profiles fail, try next provider
6. Reset cooldown after configured period

#### Configuration Schema
```json
{
  "providers": {
    "anthropic": {
      "profiles": [
        {
          "id": "primary",
          "api_key": "sk-ant-...",
          "priority": 1
        },
        {
          "id": "secondary",
          "api_key": "sk-ant-...",
          "priority": 2
        }
      ],
      "cooldown_seconds": 300,
      "max_retries": 3
    }
  },
  "failover": {
    "provider_order": ["anthropic", "openai", "openrouter"],
    "auto_rotate": true
  }
}
```

#### Integration Points
- `providers/factory.go` - Provider selection
- `agent/error_classifier.go` - Error classification
- `agent/retry_manager.go` - Retry logic

### 3. Vector Memory System

#### Purpose
Implement semantic search over conversation history and knowledge bases:
- Vector embeddings for messages
- Hybrid search (vector + keyword)
- Real-time indexing
- Multi-provider embeddings support

#### Tech Stack
- **sqlite-vec**: Vector similarity search in SQLite
- **github.com/openai/openai-go**: OpenAI embeddings
- **github.com/ggerganov/llama.cpp**: Local embeddings (optional)
- **github.com/google/generative-ai-go**: Gemini embeddings

#### Architecture Components

```go
type MemoryManager struct {
    db           *sql.DB
    embeddings   EmbeddingProvider
    indexer      *DocumentIndexer
    searcher     *SearchManager
    config       MemoryConfig
}

type EmbeddingProvider interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
}

type DocumentIndexer struct {
    chunkSize     int
    chunkOverlap  int
    batchSize     int
    concurrency   int
}

type SearchManager struct {
    vectorWeight  float64
    keywordWeight float64
    topK          int
}

type MemoryConfig struct {
    Provider     string  // "openai", "gemini", "local"
    Model        string
    ChunkSize    int
    ChunkOverlap int
    VectorDims   int
}
```

#### Database Schema
```sql
-- Memory chunks table
CREATE TABLE memory_chunks (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    metadata JSON,
    created_at INTEGER NOT NULL
);

-- Vector table (sqlite-vec)
CREATE TABLE memory_chunks_vec(
    rowid INTEGER PRIMARY KEY,
    embedding BLOB NOT NULL
);

-- FTS table
CREATE VIRTUAL TABLE memory_chunks_fts USING fts5(
    content,
    content_rowid=rowid
);

-- Index for vector similarity search
CREATE INDEX idx_memory_chunks_vec ON memory_chunks_vec(
    vec_distance_cosine(embedding, ?)
);
```

#### Search API
```go
type SearchResult struct {
    Content    string
    Source     string
    Score      float64
    Metadata   map[string]interface{}
}

func (m *MemoryManager) Search(
    ctx context.Context,
    query string,
    topK int,
) ([]SearchResult, error)

func (m *MemoryManager) IndexMessage(
    ctx context.Context,
    msg *session.Message,
) error
```

#### Integration Points
- `session/manager.go` - Index messages on save
- `agent/context.go` - Inject relevant memories
- `agent/tools/smart_search.go` - Enhanced search

### 4. Enhanced Session Management

#### Purpose
Implement advanced session features:
- Tree-structured conversations with branching
- DM vs group chat history limits
- Auto-compaction on context overflow
- Session metadata and tagging

#### Tech Stack
- **database/sql**: Session storage
- **github.com/google/uuid**: Session/message IDs

#### Architecture Components

```go
type EnhancedMessage struct {
    ID          string                 // Unique message ID
    ParentID    *string                // Parent message ID for tree structure
    BranchID    string                 // Branch identifier
    Role        string
    Content     string
    Media       []Media
    Timestamp   time.Time
    Metadata    map[string]interface{}
    ToolCalls   []ToolCall
    TTL         *time.Duration         // Time-to-live for context pruning
}

type EnhancedSession struct {
    ID           string
    Type         SessionType           // "dm" or "group"
    RootMessage  string
    CurrentBranch string
    Branches     map[string]*Branch    // Branch management
    Messages     map[string]*EnhancedMessage
    Metadata     SessionMetadata
}

type SessionType string
const (
    SessionTypeDM    SessionType = "dm"
    SessionTypeGroup SessionType = "group"
)

type Branch struct {
    ID         string
    ParentID   *string
    MessageIDs []string
    CreatedAt  time.Time
}

type SessionMetadata struct {
    Title       string
    Tags        []string
    Pinned      bool
    Archived    bool
    CreatedAt   time.Time
    UpdatedAt   time.Time
    MessageCount int
    TokenCount  int
}
```

#### Configuration
```json
{
  "sessions": {
    "dm_max_history": 100,
    "group_max_history": 50,
    "auto_compaction": {
      "enabled": true,
      "trigger_tokens": 150000,
      "target_tokens": 100000
    },
    "context_pruning": {
      "default_ttl": "24h",
      "min_messages": 10
    }
  }
}
```

#### API
```go
// Branch management
func (s *EnhancedSession) CreateBranch(messageID string) (*Branch, error)
func (s *EnhancedSession) SwitchBranch(branchID string) error
func (s *EnhancedSession) GetBranchHistory(branchID string, limit int) ([]EnhancedMessage, error)

// Compaction
func (m *EnhancedManager) CompactSession(sessionID string) error
func (m *EnhancedManager) SummarizeMessages(messages []EnhancedMessage) (string, error)
```

## Tech Stack Summary

### Core Dependencies
| Component | Library | Purpose |
|-----------|---------|---------|
| WebSocket | gorilla/websocket | Real-time communication |
| JWT | github.com/golang-jwt/jwt | Authentication tokens |
| Database | github.com/mattn/go-sqlite3 | SQLite database |
| Vector | github.com/asg017/sqlite-vec-go | Vector similarity search |
| Embeddings | github.com/openai/openai-go | OpenAI embeddings |
| Embeddings | github.com/google/generative-ai-go | Gemini embeddings |
| HTTP | github.com/imroc/req/v3 | HTTP client |
| Logging | go.uber.org/zap | Structured logging |
| Config | github.com/spf13/viper | Configuration management |

### New Dependencies to Add
```go
require (
    github.com/coder/websocket v1.8.12  // Alternative WebSocket
    github.com/google/uuid v1.6.0       // UUID generation
    github.com/tmc/langchaingo v0.0.0   // LLM abstractions
    github.com/yourbasic/edge v1.0.0    // Priority queues for failover
)
```

## Implementation Phases

### Phase 1: Foundation (Week 1-2)
**Goal**: Quick wins to improve reliability

1. **Error Classification Enhancement**
   - Files: `agent/error_classifier.go`
   - Add patterns for: auth, rate_limit, quota, timeout, context_overflow
   - Implement failover reason classification

2. **Thinking Tag Handling**
   - Files: `agent/context.go`, `agent/loop.go`
   - Strip `<thinking>...</thinking>` tags
   - Support thinking modes: off, on, stream

3. **Final Block Handling**
   - Files: `agent/loop.go`
   - Parse and remove `<final>` blocks
   - Log final block contents

4. **Block Chunking**
   - Files: `bus/outbound.go`, `channels/base.go`
   - Implement paragraph-based chunking
   - Configure chunk size limits

5. **Owner-Only Tools**
   - Files: `agent/tools/registry.go`, `config/schema.go`
   - Add `owner_only` field to tools
   - Configure `owner_ids` in tool policy

### Phase 2: Reliability (Week 3-4)
**Goal**: Robust multi-provider support

1. **Multi-Provider Auth Rotation**
   - Files: `providers/factory.go`, `providers/auth.go`
   - Implement auth profile management
   - Add cooldown tracking
   - Implement automatic rotation

2. **Automatic Retry with Backoff**
   - Files: `agent/retry_manager.go`, `providers/*.go`
   - Add retry middleware in providers
   - Per-reason retry strategies

3. **Message Tool Deduplication**
   - Files: `channels/manager.go`, `agent/tools/message.go`
   - Track sent messages
   - Suppress direct responses after tool send

4. **Session Manager Caching**
   - Files: `session/manager.go`
   - Add in-memory cache
   - Implement TTL-based expiration

### Phase 3: Advanced Session Management (Week 5-6)
**Goal**: Enhanced session capabilities

1. **DM vs Group History Limits**
   - Files: `session/manager.go`, `config/schema.go`
   - Extend session key with conversation type
   - Apply different limits per type

2. **Context Pruning**
   - Files: `agent/context.go`, `session/manager.go`
   - Add TTL field to messages
   - Implement TTL-based filtering

3. **Tree-Structured Sessions**
   - Files: `session/manager.go`, `session/message.go`
   - Add ParentID, BranchID to messages
   - Implement branch creation/switching

4. **Auto-Compaction**
   - Files: `agent/loop.go`, `session/manager.go`
   - Detect context overflow errors
   - Implement summarization compaction

### Phase 4: Real-Time Communication (Week 7-8)
**Goal**: WebSocket gateway

1. **WebSocket Gateway Core**
   - Files: `gateway/server.go`, `gateway/ws.go`
   - Implement WebSocket upgrader
   - Connection management
   - Message routing

2. **Authentication System**
   - Files: `gateway/auth.go`
   - JWT token generation
   - Token validation
   - Permission management

3. **Streaming Support**
   - Files: `agent/loop.go`, `gateway/stream.go`
   - Block-based chunking over WebSocket
   - Real-time tool execution feedback

4. **Control UI Integration**
   - Files: `gateway/ui.go`
   - Serve control UI assets
   - API endpoints for UI

### Phase 5: Vector Memory (Week 9-10)
**Goal**: Semantic search capabilities

1. **Embedding Provider**
   - Files: `memory/embeddings.go`
   - Support OpenAI embeddings
   - Support Gemini embeddings
   - Local embedding support (optional)

2. **Vector Database**
   - Files: `memory/database.go`
   - SQLite with sqlite-vec
   - Schema creation
   - Index management

3. **Document Indexer**
   - Files: `memory/indexer.go`
   - Message chunking
   - Batch embedding
   - Real-time indexing

4. **Search Manager**
   - Files: `memory/search.go`
   - Vector similarity search
   - Hybrid search (vector + FTS)
   - Result ranking

### Phase 6: Enhanced Capabilities (Week 11-12)
**Goal**: Advanced features

1. **Hierarchical Tool Policy**
   - Files: `agent/tools/registry.go`, `config/schema.go`
   - 5-level policy resolution
   - Policy inheritance

2. **System Prompt Enhancements**
   - Files: `agent/context.go`
   - Sandbox information
   - Reply tags support
   - CLI reference
   - Runtime metadata

3. **Google Gemini Provider**
   - Files: `providers/gemini.go`
   - Full Gemini support
   - Handle turn ordering quirks

4. **Extensions System**
   - Files: `agent/extensions.go`
   - Plugin API
   - Extension lifecycle
   - Built-in extensions

## File Structure

```
goclaw/
├── agent/
│   ├── loop.go                 # [MOD] Enhanced with streaming
│   ├── context.go              # [MOD] Memory injection, pruning
│   ├── memory.go               # [MOD] Enhanced memory store
│   ├── error_classifier.go     # [MOD] Enhanced patterns
│   ├── retry_manager.go        # [MOD] Per-reason strategies
│   ├── extensions.go           # [NEW] Extension system
│   └── tools/
│       ├── registry.go         # [MOD] Hierarchical policy
│       └── message.go          # [MOD] Deduplication
├── gateway/
│   ├── server.go               # [MOD] WebSocket support
│   ├── ws.go                   # [NEW] WebSocket handler
│   ├── auth.go                 # [NEW] Authentication
│   ├── stream.go               # [NEW] Streaming support
│   └── ui.go                   # [NEW] Control UI
├── memory/
│   ├── manager.go              # [NEW] Memory manager
│   ├── embeddings.go           # [NEW] Embedding providers
│   ├── database.go             # [NEW] Vector database
│   ├── indexer.go              # [NEW] Document indexing
│   └── search.go               # [NEW] Search manager
├── providers/
│   ├── factory.go              # [MOD] Failover support
│   ├── auth.go                 # [NEW] Profile management
│   ├── base.go                 # [MOD] Retry middleware
│   └── gemini.go               # [NEW] Gemini provider
├── session/
│   ├── manager.go              # [MOD] Enhanced features
│   ├── message.go              # [MOD] Tree structure
│   ├── branch.go               # [NEW] Branch management
│   └── compaction.go           # [NEW] Auto-compaction
├── channels/
│   ├── manager.go              # [MOD] Deduplication
│   └── base.go                 # [MOD] Streaming support
└── config/
    └── schema.go               # [MOD] New configuration options
```

## Testing Strategy

### Unit Tests
- Each component with isolated tests
- Mock external dependencies (LLM APIs, databases)
- >80% code coverage target

### Integration Tests
- Gateway + Channels integration
- Memory indexing + Search
- Failover scenarios
- Session branching/merging

### E2E Tests
- Full conversation flows
- Multi-turn interactions
- Tool execution chains
- Streaming responses

## Migration Path

### Data Migration
1. Session format upgrade (JSON → JSONL with metadata)
2. Memory index initialization
3. Auth profile configuration

### Configuration Migration
1. Legacy config detection
2. Automatic migration with warnings
3. Validation and error reporting

## Dependencies on Other Tasks

- **Task #2 (WebSocket Gateway)**: Must be completed before streaming support
- **Task #3 (Vector Memory)**: Requires embeddings infrastructure
- **Task #6 (Failover)**: Enables vector memory fallback
- **Task #7 (Sessions)**: Tree structure enables advanced features

## Success Criteria

### Phase 1-2
- [ ] Error classification correctly identifies all failover scenarios
- [ ] Multi-provider auth rotation works without manual intervention
- [ ] Message deduplication prevents double-sends
- [ ] Session caching reduces disk I/O by >50%

### Phase 3-4
- [ ] WebSocket gateway handles 100+ concurrent connections
- [ ] Streaming responses deliver <100ms latency
- [ ] Tree-structured sessions support branching and merging
- [ ] Auto-compaction reduces token usage by >30%

### Phase 5-6
- [ ] Vector search returns relevant results (precision >0.8)
- [ ] Memory indexing handles 10K+ messages
- [ ] Extensions system allows runtime customization
- [ ] Hierarchical policy provides granular control

## Documentation Requirements

1. **Architecture Documentation**
   - Component diagrams
   - Data flow diagrams
   - API documentation

2. **User Documentation**
   - Configuration guide
   - Migration guide
   - Troubleshooting

3. **Developer Documentation**
   - Code organization
   - Contributing guidelines
   - Testing practices

## Risk Mitigation

### Technical Risks
- **SQLite-vec compatibility**: Test on target platforms early
- **WebSocket scalability**: Load testing before deployment
- **Embedding costs**: Implement caching and batching

### Operational Risks
- **API key exhaustion**: Monitor usage, implement quotas
- **Session corruption**: Regular backups, validation
- **Memory leaks**: Profiling, monitoring

## Next Steps

1. Review and approve this plan
2. Set up development infrastructure
3. Begin Phase 1 implementation
4. Establish regular sync meetings
5. Create detailed task breakdowns

---

**Document Version**: 1.0
**Last Updated**: 2025-02-09
**Owner**: architect
**Status**: Draft for Review
