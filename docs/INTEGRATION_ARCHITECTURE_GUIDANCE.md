# Integration Architecture Guidance

## Document Version: 1.0
**Date**: 2025-02-09
**Owner**: architect
**Status**: Official Guidance for integration teammate

---

## Current Implementation Status

### ✅ Already Implemented

**Browser CDP Tools** (`/Users/chaoyuepan/ai/goclaw/agent/tools/`):
- `browser.go`: Browser tool with CDP operations
- `browser_session.go`: Persistent browser session manager
- Full CDP integration using `github.com/mafredri/cdp`

**New Channels** (`/Users/chaoyuepan/ai/goclaw/channels/`):
- `discord.go`: Discord channel implementation
- `slack.go`: Slack channel implementation
- `googlechat.go`: Google Chat channel implementation
- `teams.go`: Microsoft Teams channel implementation

**Key Observations**:
- Browser tool already has comprehensive CDP support
- All new channels follow the established pattern from `telegram.go`
- Streaming support exists in provider options but not implemented

---

## Task #4: Browser CDP Enhancement - Guidance

### Current State: ✅ Strong Foundation

The browser tool already implements:
- Navigate to URLs
- Take screenshots
- Execute JavaScript
- Get page content
- Persistent browser sessions
- Connection to existing Chrome instances

### Recommendations:

**1. PDF Generation - YES, Add It**

**Rationale**:
- Matches openclaw's capabilities
- Useful for report generation, archiving
- CDP already supports `Page.printToPDF`

**Implementation**:

```go
// In agent/tools/browser.go

// BrowserToPDF Generate PDF from current page or URL
func (b *BrowserTool) BrowserToPDF(ctx context.Context, params map[string]interface{}) (string, error) {
    var urlStr string
    var landscape bool
    var printBackground bool

    if u, ok := params["url"].(string); ok {
        urlStr = u
    }
    if l, ok := params["landscape"].(bool); ok {
        landscape = l
    }
    if p, ok := params["print_background"].(bool); ok {
        printBackground = p
    }

    logger.Info("Browser generating PDF",
        zap.String("url", urlStr),
        zap.Bool("landscape", landscape))

    sessionMgr := GetBrowserSession()
    if !sessionMgr.IsReady() {
        return "", fmt.Errorf("browser session not ready")
    }

    client, err := sessionMgr.GetClient()
    if err != nil {
        return "", fmt.Errorf("failed to get browser client: %w", err)
    }

    // Navigate if URL provided
    if urlStr != "" {
        if _, err := client.Page.Navigate(ctx, page.NewNavigateArgs(urlStr)); err != nil {
            return "", fmt.Errorf("failed to navigate: %w", err)
        }
        // Wait for load
        domContentLoaded, err := client.Page.DOMContentEventFired(ctx)
        if err == nil {
            defer domContentLoaded.Close()
            _, _ = domContentLoaded.Recv()
        }
    }

    // Generate PDF
    pdfArgs := page.NewPrintToPDFArgs().
        SetLandscape(landscape).
        SetPrintBackground(printBackground).
        SetDisplayHeaderFooter(false).
        SetPreferCSSPageSize(true)

    pdf, err := client.Page.PrintToPDF(ctx, pdfArgs)
    if err != nil {
        return "", fmt.Errorf("failed to generate PDF: %w", err)
    }

    // Save PDF
    filename := fmt.Sprintf("page_%d.pdf", time.Now().Unix())
    filepath := b.outputDir + string(os.PathSeparator) + filename
    if err := os.WriteFile(filepath, pdf.Data, 0644); err != nil {
        return "", fmt.Errorf("failed to save PDF: %w", err)
    }

    return fmt.Sprintf("PDF saved to: %s\nSize: %d bytes", filepath, len(pdf.Data)), nil
}
```

**2. Structured Data Extraction - YES, Add Utilities**

**Rationale**:
- LLMs benefit from structured data over raw HTML
- Common use case: product pages, articles, documentation
- Reduces token usage vs raw HTML

**Implementation**:

```go
// In agent/tools/browser.go

// BrowserExtract Extract structured data from page
func (b *BrowserTool) BrowserExtract(ctx context.Context, params map[string]interface{}) (string, error) {
    selector := ""
    extractType := "text" // text, html, markdown, schema

    if s, ok := params["selector"].(string); ok {
        selector = s
    }
    if e, ok := params["type"].(string); ok {
        extractType = e
    }

    sessionMgr := GetBrowserSession()
    if !sessionMgr.IsReady() {
        return "", fmt.Errorf("browser session not ready")
    }

    client, err := sessionMgr.GetClient()
    if err != nil {
        return "", fmt.Errorf("failed to get browser client: %w", err)
    }

    // Get document
    doc, err := client.DOM.GetDocument(ctx, nil)
    if err != nil {
        return "", fmt.Errorf("failed to get document: %w", err)
    }

    var result string

    switch extractType {
    case "html":
        // Extract HTML
        if selector != "" {
            nodeID, err := b.querySelector(ctx, client, selector, doc.Root.NodeID)
            if err != nil {
                return "", fmt.Errorf("selector not found: %w", err)
            }
            outerHTML, err := client.DOM.GetOuterHTML(ctx, &dom.GetOuterHTMLArgs{
                NodeID: &nodeID,
            })
            if err != nil {
                return "", fmt.Errorf("failed to get HTML: %w", err)
            }
            result = outerHTML.OuterHTML
        } else {
            outerHTML, err := client.DOM.GetOuterHTML(ctx, &dom.GetOuterHTMLArgs{
                NodeID: &doc.Root.NodeID,
            })
            if err != nil {
                return "", fmt.Errorf("failed to get HTML: %w", err)
            }
            result = outerHTML.OuterHTML
        }

    case "text":
        // Extract text content
        if selector != "" {
            nodeID, err := b.querySelector(ctx, client, selector, doc.Root.NodeID)
            if err != nil {
                return "", fmt.Errorf("selector not found: %w", err)
            }
            result, err = b.extractText(ctx, client, nodeID)
        } else {
            result, err = b.extractText(ctx, client, doc.Root.NodeID)
        }

    case "schema":
        // Extract JSON-LD structured data
        result, err = b.extractStructuredData(ctx, client)

    default:
        return "", fmt.Errorf("unsupported extract type: %s", extractType)
    }

    return result, err
}

// Helper: Query selector
func (b *BrowserTool) querySelector(ctx context.Context, client *cdp.Client, selector string, rootNodeID dom.NodeID) (dom.NodeID, error) {
    result, err := client.DOM.PerformSearch(ctx, &dom.PerformSearchArgs{
        Query: selector,
    })
    if err != nil {
        return 0, err
    }
    defer client.DOM.DiscardSearchResults(ctx, &dom.DiscardSearchResultsArgs{
        SearchID: result.SearchID,
    })

    results, err := client.DOM.GetSearchResults(ctx, &dom.GetSearchResultsArgs{
        SearchID:   result.SearchID,
        FromIndex:  0,
        ToIndex:    1,
    })
    if err != nil {
        return 0, err
    }

    if len(results.NodeIds) == 0 {
        return 0, fmt.Errorf("no results for selector: %s", selector)
    }

    return results.NodeIds[0], nil
}

// Helper: Extract text content
func (b *BrowserTool) extractText(ctx context.Context, client *cdp.Client, nodeID dom.NodeID) (string, error) {
    // Use JavaScript to extract clean text
    evalArgs := runtime.NewEvaluateArgs(fmt.Sprintf(`
        (function() {
            const node = domLookup(%d);
            if (!node) return '';
            // Remove script and style elements
            const clone = node.cloneNode(true);
            clone.querySelectorAll('script, style').forEach(el => el.remove());
            return clone.innerText || clone.textContent;
        })();
    `, nodeID))

    result, err := client.Runtime.Evaluate(ctx, evalArgs)
    if err != nil {
        return "", fmt.Errorf("failed to evaluate: %w", err)
    }

    if result.Result.Type == "string" {
        return result.Result.Value.(string), nil
    }

    return "", fmt.Errorf("unexpected result type: %s", result.Result.Type)
}

// Helper: Extract structured data (JSON-LD, microdata)
func (b *BrowserTool) extractStructuredData(ctx context.Context, client *cdp.Client) (string, error) {
    evalArgs := runtime.NewEvaluateArgs(`
        (function() {
            const data = [];

            // Extract JSON-LD
            document.querySelectorAll('script[type="application/ld+json"]').forEach(el => {
                try {
                    data.push(JSON.parse(el.textContent));
                } catch(e) {}
            });

            // Extract microdata
            const items = document.querySelectorAll('[itemscope]');
            items.forEach(item => {
                const obj = {};
                item.querySelectorAll('[itemprop]').forEach(prop => {
                    obj[prop.getAttribute('itemprop')] = prop.getAttribute('content') || prop.textContent;
                });
                if (Object.keys(obj).length > 0) {
                    data.push(obj);
                }
            });

            return JSON.stringify(data, null, 2);
        })();
    `)

    result, err := client.Runtime.Evaluate(ctx, evalArgs)
    if err != nil {
        return "", fmt.Errorf("failed to evaluate: %w", err)
    }

    if result.Result.Type == "string" {
        return result.Result.Value.(string), nil
    }

    return "[]", nil
}
```

**3. Chrome Extension Protocol - SKIP for Now**

**Rationale**:
- Not in openclaw's current feature set
- Complex implementation with limited use cases
- Better to focus on core features first
- Can add later if specific use case emerges

**Alternative**: Use existing CDP commands for all browser automation needs.

---

## Task #5: Streaming Response Support - Guidance

### Current State: ⚠️ Interface Exists, No Implementation

The `Stream` field exists in `ChatOptions` but no actual streaming.

### Architecture Recommendations:

**1. Provider Interface Enhancement - Add Streaming Callbacks**

**Recommendation**: Extend provider interface with streaming support

```go
// providers/base.go

type StreamCallback func(chunk string) error

type Provider interface {
    // Existing methods...
    Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error)

    // NEW: Streaming chat
    ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, callback StreamCallback, options ...ChatOption) (*Response, error)

    Close() error
}

// Update ChatOptions
type ChatOptions struct {
    Model         string
    Temperature   float64
    MaxTokens     int
    Stream        bool
    StreamCallback StreamCallback  // NEW: Streaming callback
}
```

**Implementation Example (OpenAI)**:

```go
// providers/openai.go

func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, callback StreamCallback, options ...ChatOption) (*Response, error) {
    opts := &ChatOptions{
        Model:         p.model,
        Temperature:   0.7,
        MaxTokens:     4096,
        Stream:        true,
        StreamCallback: callback,
    }

    for _, opt := range options {
        opt(opts)
    }

    // Convert messages
    langchainMessages := ConvertToLangChainMessages(messages)
    langchainTools := ConvertToLangChainTools(tools)

    // Generate streaming completion
    var fullContent strings.Builder

    err := p.llm.GenerateContent(ctx, langchainMessages,
        llms.WithStreamingFunc(ctx, func(ctx context.Context, chunk []byte) error {
            fullContent.Write(chunk)
            if opts.StreamCallback != nil {
                return opts.StreamCallback(string(chunk))
            }
            return nil
        }),
        llms.WithModel(opts.Model),
        llms.WithTemperature(opts.Temperature),
        llms.WithMaxTokens(opts.MaxTokens),
    )

    if err != nil {
        return nil, err
    }

    return &Response{
        Content: fullContent.String(),
        Usage: Usage{
            // Estimate usage
            TotalTokens: len(fullContent.String()) / 4,
        },
    }, nil
}
```

**2. Message Bus - No Changes Needed**

**Rationale**:
- Message bus handles discrete messages, not streams
- Streaming happens at provider level
- Bus carries final results, not intermediate chunks

**Alternative**: Create streaming event bus for real-time updates

```go
// bus/events.go - NEW streaming events

type StreamingEvent struct {
    SessionID string
    Chunk     string
    Done      bool
    Error     error
}

type StreamingEventBus struct {
    subscribers map[string]chan StreamingEvent
    mu          sync.RWMutex
}

func (b *StreamingEventBus) Subscribe(sessionID string) chan StreamingEvent {
    b.mu.Lock()
    defer b.mu.Unlock()

    ch := make(chan StreamingEvent, 100)
    b.subscribers[sessionID] = ch
    return ch
}

func (b *StreamingEventBus) Publish(sessionID string, event StreamingEvent) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    if ch, ok := b.subscribers[sessionID]; ok {
        select {
        case ch <- event:
        default:
            // Channel full, drop event
        }
    }
}
```

**3. Channel Handling - Progressive Updates**

**Recommendation**: Channels implement progressive message updates

```go
// channels/base.go

type StreamingMessage struct {
    ChatID      string
    Content     strings.Builder
    MessageID   string
    mu          sync.Mutex
    Done        bool
}

func (c *BaseChannelImpl) SendStreaming(chatID string, callback func() (string, bool, error)) error {
    stream := &StreamingMessage{
        ChatID:    chatID,
        Content:   strings.Builder{},
        Done:      false,
    }

    // Send initial message (or typing indicator)
    messageID, err := c.sendInitial(chatID)
    if err != nil {
        return err
    }
    stream.MessageID = messageID

    // Stream updates
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    for {
        chunk, done, err := callback()
        if err != nil {
            return err
        }

        stream.mu.Lock()
        stream.Content.WriteString(chunk)
        stream.Done = done
        currentContent := stream.Content.String()
        stream.mu.Unlock()

        // Update message
        if err := c.updateMessage(chatID, messageID, currentContent); err != nil {
            logger.Warn("Failed to update streaming message", zap.Error(err))
        }

        if done {
            break
        }

        <-ticker.C
    }

    return nil
}

// Channel-specific implementations
func (c *TelegramChannel) updateMessage(chatID, messageID, content string) error {
    // Telegram supports editing messages
    msg := telegrambot.NewEditMessageText(chatID, messageID, content)
    _, err := c.bot.Send(msg)
    return err
}

func (c *DiscordChannel) updateMessage(chatID, messageID, content string) error {
    // Discord supports editing messages
    msgID, _ := strconv.ParseInt(messageID, 10, 64)
    _, err := c.session.ChannelMessageEdit(c.session.State.ChannelID(chatID), msgID, content)
    return err
}
```

**4. Integration with Agent Loop**

```go
// agent/loop.go

func (l *Loop) processMessage(ctx context.Context, msg *bus.InboundMessage) {
    // ... existing code ...

    // Check if streaming is enabled
    if l.shouldStream(msg) {
        err := l.processWithStreaming(ctx, msg, sess)
        // Handle streaming response
    } else {
        err := l.processNormal(ctx, msg, sess)
        // Handle normal response
    }
}

func (l *Loop) processWithStreaming(ctx context.Context, msg *bus.InboundMessage, sess *session.Session) error {
    var fullContent strings.Builder

    callback := func(chunk string) error {
        fullContent.WriteString(chunk)
        // Send to streaming event bus or channel
        return l.streamingBus.Publish(msg.SessionKey(), bus.StreamingEvent{
            SessionID: msg.SessionKey(),
            Chunk:     chunk,
            Done:      false,
        })
    }

    response, err := l.provider.ChatStream(ctx, messages, tools, callback, opts)
    if err != nil {
        return err
    }

    // Mark streaming as complete
    l.streamingBus.Publish(msg.SessionKey(), bus.StreamingEvent{
        SessionID: msg.SessionKey(),
        Done:      true,
    })

    // Save final response to session
    sess.AddMessage(session.Message{
        Role:    "assistant",
        Content: fullContent.String(),
        // ...
    })

    return nil
}
```

---

## Task #8: New Channels - Guidance

### Current State: ✅ Already Implemented

All requested channels have been implemented:
- `discord.go`: Discord channel
- `slack.go`: Slack channel
- `googlechat.go`: Google Chat channel
- `teams.go`: Microsoft Teams channel

### Pattern Consistency: ✅ Excellent

All channels follow the established pattern from `telegram.go`:
1. Embed `*BaseChannelImpl`
2. Implement `Start()` method
3. Channel-specific receive loop
4. Message handling
5. Configuration structure

### Recommendations:

**1. Pattern Consistency - YES, Follow Exactly**

All channels should maintain the current pattern:

```go
// Standard channel pattern
type NewChannel struct {
    *BaseChannelImpl
    client  *platform.Client
    token   string
}

type NewChannelConfig struct {
    BaseChannelConfig
    Token string `mapstructure:"token" json:"token"`
}

func NewNewChannel(cfg NewChannelConfig, bus *bus.MessageBus) (*NewChannel, error) {
    // Validate and create channel
    return &NewChannel{
        BaseChannelImpl: NewBaseChannelImpl("newchannel", cfg.BaseChannelConfig, bus),
        // ...
    }, nil
}

func (c *NewChannel) Start(ctx context.Context) error {
    if err := c.BaseChannelImpl.Start(ctx); err != nil {
        return err
    }

    // Start platform-specific receive loop
    go c.receiveUpdates(ctx)

    return nil
}
```

**2. Additional Channels - Defer to Future**

The current set covers major platforms. Additional channels (Matrix, Signal, etc.) can be added later based on user demand.

**3. Testing Each Channel**

```go
// channels/newchannel_test.go

func TestNewChannel_MessageHandling(t *testing.T) {
    // Test message parsing
    // Test outbound message sending
    // Test error handling
}

func TestNewChannel_AllowedUsers(t *testing.T) {
    // Test allowlist functionality
    // Test permission checking
}
```

---

## Implementation Priority

### Week 1: Browser Enhancement
1. Add `BrowserToPDF` method
2. Add `BrowserExtract` method with structured data extraction
3. Add helper methods (querySelector, extractText, extractStructuredData)
4. Update tool registry

### Week 2: Streaming Infrastructure
1. Update `Provider` interface with `ChatStream`
2. Implement `ChatStream` in OpenAI provider
3. Add streaming event bus to `bus/` package
4. Update agent loop for streaming support

### Week 3: Channel Streaming
1. Add `SendStreaming` method to `BaseChannelImpl`
2. Implement `updateMessage` for each channel
3. Test streaming across all channels
4. Add configuration for streaming toggle

### Week 4: Integration & Testing
1. End-to-end streaming tests
2. Browser tool tests
3. Channel integration tests
4. Documentation

---

## Files to Modify/Create

### Browser Enhancement
- `agent/tools/browser.go` - Add PDF and extraction methods
- `agent/tools/registry.go` - Register new tools

### Streaming Support
- `providers/base.go` - Add streaming interface
- `providers/openai.go` - Implement streaming
- `providers/anthropic.go` - Implement streaming
- `bus/events.go` - Add streaming event types
- `agent/loop.go` - Integrate streaming

### Channel Streaming
- `channels/base.go` - Add streaming methods
- `channels/telegram.go` - Implement message editing
- `channels/discord.go` - Implement message editing
- `channels/slack.go` - Implement message editing

---

## Configuration Examples

```json
{
  "tools": {
    "browser": {
      "enabled": true,
      "headless": true,
      "timeout": 30,
      "features": {
        "pdf_generation": true,
        "structured_extraction": true
      }
    }
  },
  "streaming": {
    "enabled": true,
    "chunk_interval_ms": 500,
    "update_strategy": "progressive"
  }
}
```

---

## Questions?

If you need clarification on:
1. Specific streaming implementation details
2. Browser tool enhancement priorities
3. Channel-specific streaming behaviors
4. Testing strategies

Please ask and I'll provide detailed guidance.

---

**Document Version**: 1.0
**Last Updated**: 2025-02-09
**Owner**: architect
