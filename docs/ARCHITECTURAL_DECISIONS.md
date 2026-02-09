# Architectural Decisions

## Document Version: 1.0
**Date**: 2025-02-09
**Owner**: architect
**Status**: Official

This document records official architectural decisions for the goclaw project to guide implementation teams.

---

## Decision 1: Vector Memory Database

**Status**: ✅ APPROVED

**Choice**: SQLite with sqlite-vec extension

**Rationale**:
1. **Alignment with openclaw**: openclaw uses SQLite-vec, maintaining consistency with the reference architecture
2. **Simplicity**: Single-file database with no external dependencies
3. **Sufficient for Use Case**: Personal AI assistant doesn't require distributed database scalability
4. **Performance**: In-memory operations for typical workloads (<100K messages)
5. **Low Infrastructure**: No additional services to deploy, monitor, or maintain
6. **Migration Path**: Can migrate to PostgreSQL+pgvector later if needed

**Technical Specification**:
- Use `github.com/asg017/sqlite-vec-go` for Go bindings
- Store embeddings as BLOB in `memory_chunks_vec` table
- FTS5 for keyword search
- Configurable vector dimensions (default: 1536 for OpenAI)

**Migration Strategy**:
- Implement database schema versioning
- Provide migration script for future PostgreSQL upgrade
- Abstract database layer for portability

---

## Decision 2: Embedding Provider Priority

**Status**: ✅ APPROVED

**Choice**: Multi-provider with OpenAI as primary, Ollama/local as fallback

**Rationale**:
1. **Reliability**: OpenAI API provides consistent, production-ready embeddings
2. **Performance**: Faster inference compared to local models
3. **Privacy Option**: Local models via Ollama for sensitive data
4. **Cost Control**: Automatic fallback prevents API overages
5. **Alignment**: Matches openclaw's multi-provider failover approach

**Technical Specification**:

```go
type EmbeddingProvider interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
}

type EmbeddingConfig struct {
    Primary   string  // "openai", "gemini", "ollama"
    Fallback  string  // Fallback provider
    Model     string
    APIToken  string
    LocalPath string  // For Ollama
}
```

**Implementation Priority**:
1. **Phase 1**: OpenAI embeddings (`text-embedding-3-small`)
2. **Phase 2**: Gemini embeddings (`embedding-001`)
3. **Phase 3**: Ollama/local embeddings (`nomic-embed-text`)

**Failover Logic**:
- Primary provider fails → fallback provider
- API rate limits → switch provider with cooldown
- Network errors → retry with exponential backoff

---

## Decision 3: Session Tree Depth Limits

**Status**: ✅ APPROVED

**Choice**: Configurable maximum depth with soft warnings

**Rationale**:
1. **Flexibility**: Different use cases require different depths
2. **UX**: Prevent overwhelming users with deep trees
3. **Performance**: Limit traversal complexity
4. **Safety**: Prevent infinite recursion bugs

**Technical Specification**:

```go
type TreeConfig struct {
    MaxDepth           int           // Default: 10
    WarningDepth       int           // Default: 7
    AllowCircularRef   bool          // Default: false
}

type Branch struct {
    ID         string
    ParentID   *string
    Depth      int
    MessageIDs []string
}
```

**Configuration**:
```json
{
  "sessions": {
    "tree": {
      "max_depth": 10,
      "warning_depth": 7,
      "allow_circular_references": false
    }
  }
}
```

**Validation Rules**:
1. Reject branch creation at depth > MaxDepth
2. Log warning when depth > WarningDepth
3. Detect circular references (A → B → A)
4. Provide branch pruning utilities

**UI Guidance**:
- Display depth indicator in session tree view
- Show warnings prominently
- Provide "flatten" operation for deep trees

---

## Decision 4: Backward Compatibility

**Status**: ✅ APPROVED

**Choice**: Maintain full backward compatibility with existing JSONL sessions

**Rationale**:
1. **User Data**: Existing sessions contain valuable conversation history
2. **Trust**: No data loss during upgrade
3. **Migration**: Gradual adoption of new features
4. **Testing**: Dual-format validation catches bugs

**Technical Specification**:

**File Format Evolution**:
```
Legacy JSONL (v1):
{"role": "user", "content": "...", "timestamp": "..."}
{"role": "assistant", "content": "...", "timestamp": "..."}

Enhanced JSONL (v2):
{"_type": "metadata", "version": 2, "tree_enabled": true}
{"role": "user", "content": "...", "timestamp": "...", "id": "msg-1", "parent_id": null}
{"role": "assistant", "content": "...", "timestamp": "...", "id": "msg-2", "parent_id": "msg-1"}
```

**Migration Strategy**:

1. **Detection**: Check for `_type` field to determine version
2. **Auto-Migration**: On load, add missing IDs and parent_id
3. **Opt-In**: New features require explicit config enable
4. **Validation**: Verify data integrity on load

**Configuration**:
```json
{
  "sessions": {
    "format": "auto",  // "legacy", "v2", "auto"
    "auto_migrate": true,
    "tree_enabled": false  // Opt-in for new features
  }
}
```

**Compatibility Matrix**:

| Feature | Legacy Format | V2 Format |
|---------|--------------|-----------|
| Basic chat | ✅ | ✅ |
| Tool calls | ✅ | ✅ |
| Media | ✅ | ✅ |
| Branching | ❌ | ✅ |
| Parent IDs | ❌ | ✅ |
| Message IDs | Auto-generated | Preserved |

**Migration Tool**:
```bash
# Migrate all sessions to v2 format
goclaw sessions migrate --to v2

# Validate migration
goclaw sessions validate
```

---

## Decision 5: WebSocket Authentication

**Status**: ✅ APPROVED

**Choice**: JWT-based auth with Tailscale support

**Rationale**:
1. **Stateless**: JWT tokens don't require server-side session storage
2. **Scalability**: Easy to scale horizontally
3. **Security**: Token expiration and refresh mechanism
4. **Tailscale**: Zero-config LAN access for trusted networks

**Technical Specification**:

```go
type AuthConfig struct {
    Mode           string  // "token", "password", "tailscale"
    JWTSecret      string
    TokenExpiry    time.Duration  // Default: 24h
    RefreshExpiry  time.Duration  // Default: 30d
    TailscaleEnabled bool
}

type TokenClaims struct {
    UserID      string
    Permissions []string
    ExpiresAt   time.Time
    jwt.RegisteredClaims
}
```

**Authentication Flow**:
1. Client requests token via POST /auth/token
2. Server validates credentials (password or Tailscale)
3. Server issues JWT with 24h expiry
4. Client includes JWT in WebSocket connection
5. Server validates JWT on each connection

**Tailscale Integration**:
- Skip auth for connections from Tailscale IPs
- Extract user info from Tailscale headers
- Map Tailscale identities to user IDs

---

## Decision 6: Streaming Response Chunking

**Status**: ✅ APPROVED

**Choice**: Paragraph-based chunking with configurable size

**Rationale**:
1. **Readability**: Natural breaks improve reading experience
2. **UX**: Faster time-to-first-token
3. **Alignment**: Matches openclaw's block chunker
4. **Flexibility**: Configurable for different channels

**Technical Specification**:

```go
type ChunkConfig struct {
    MinChunkSize   int           // Default: 50 chars
    MaxChunkSize   int           // Default: 500 chars
    PreferParagraphs bool        // Default: true
    Delimiters     []string      // ["\n\n", "\n", ". ", "? ", "! "]
}
```

**Chunking Algorithm**:
1. Scan for paragraph breaks (`\n\n`)
2. If no paragraph break, scan for sentence breaks
3. If sentence exceeds MaxChunkSize, force break
4. Never break below MinChunkSize

**Channel-Specific Settings**:
```json
{
  "streaming": {
    "default": {
      "min_chunk_size": 50,
      "max_chunk_size": 500,
      "prefer_paragraphs": true
    },
    "telegram": {
      "max_chunk_size": 4096  // Telegram limit
    },
    "whatsapp": {
      "max_chunk_size": 1000
    }
  }
}
```

---

## Implementation Guidelines

### Phase 1 (Weeks 1-2): Foundation
- Implement Decision 4 (Backward Compatibility)
- Add migration utilities
- Validate data integrity

### Phase 2 (Weeks 3-4): Reliability
- Implement Decision 5 (WebSocket Auth)
- Add JWT token management
- Integrate Tailscale support

### Phase 3 (Weeks 5-6): Sessions
- Implement Decision 3 (Tree Depth)
- Add branch management
- Implement pruning utilities

### Phase 4 (Weeks 7-8): Memory
- Implement Decision 1 (SQLite-vec)
- Implement Decision 2 (Embedding Providers)
- Add vector indexing
- Implement search

### Phase 5 (Weeks 9-10): Streaming
- Implement Decision 6 (Chunking)
- Add streaming over WebSocket
- Integrate with channels

---

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2025-02-09 | Initial decisions | architect |

---

## Appendix: Openclaw Alignment

These decisions align with openclaw's architecture:

| Decision | openclaw Reference | goclaw Implementation |
|----------|-------------------|----------------------|
| SQLite-vec | `memory/manager.ts` | `memory/database.go` |
| Multi-provider embeddings | `memory/embeddings-*.ts` | `memory/embeddings.go` |
| Tree sessions | `sessions/session-utils.ts` | `session/branch.go` |
| JWT auth | `gateway/auth.ts` | `gateway/auth.go` |
| Block chunking | `gateway/pi-embedded-block-chunker.ts` | `gateway/stream.go` |

---

**Next Steps**: Implementation teams should reference these decisions when building features. Questions or requests for changes should be directed to the architect.
