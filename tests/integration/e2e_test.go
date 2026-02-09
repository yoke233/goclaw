package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/smallnest/dogclaw/goclaw/agent"
	"github.com/smallnest/dogclaw/goclaw/agent/tools"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/channels"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/providers"
	"github.com/smallnest/dogclaw/goclaw/session"
)

// TestE2EConversationFlow tests a complete conversation flow
func TestE2EConversationFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Setup
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager("/tmp/test-e2e-sessions")
	channelMgr := channels.NewManager(messageBus)

	// Create mock provider
	provider := &mockE2EProvider{}

	// Create agent loop
	toolRegistry := tools.NewRegistry()
	contextBuilder := agent.NewContextBuilder()
	memory := agent.NewMemoryStore(sessionMgr)

	loopCfg := &agent.Config{
		Bus:          messageBus,
		Provider:     provider,
		SessionMgr:   sessionMgr,
		Memory:       memory,
		Context:      contextBuilder,
		Tools:        toolRegistry,
		SkillsLoader: nil,
		Subagents:    nil,
		Workspace:    "/tmp/test-e2e",
		MaxIteration: 5,
	}

	loop, err := agent.NewLoop(loopCfg)
	if err != nil {
		t.Fatalf("Failed to create agent loop: %v", err)
	}

	// Start agent loop
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Failed to start agent loop: %v", err)
	}
	defer loop.Stop()

	// Send inbound message
	inboundMsg := &bus.InboundMessage{
		Channel:   "test",
		SenderID:  "test-user",
		ChatID:    "test-chat",
		Content:   "Hello, how are you?",
		Timestamp: time.Now(),
	}

	if err := messageBus.PublishInbound(ctx, inboundMsg); err != nil {
		t.Fatalf("Failed to publish inbound message: %v", err)
	}

	// Wait for response
	time.Sleep(1 * time.Second)

	// Verify session was created/updated
	sess, err := sessionMgr.GetOrCreate("test:test-chat")
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if len(sess.Messages) < 2 {
		t.Errorf("Expected at least 2 messages in session, got: %d", len(sess.Messages))
	}

	t.Logf("Session has %d messages", len(sess.Messages))
	t.Log("E2E conversation flow test passed")
}

// TestE2ESessionBranching tests session branching and merging
func TestE2ESessionBranching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	ctx := context.Background()
	sessionMgr, _ := session.NewManager("/tmp/test-e2e-branching")

	// Create parent session
	parent, _ := sessionMgr.GetOrCreate("parent-session")
	parent.AddMessage(session.Message{
		Role:      "user",
		Content:   "Initial message",
		Timestamp: time.Now(),
	})

	// Create branch
	branch := sessionMgr.Branch(parent, "experiment-branch")

	// Add different messages to branch
	branch.AddMessage(session.Message{
		Role:      "user",
		Content:   "Branch-specific message",
		Timestamp: time.Now(),
	})

	// Parent should be unchanged
	if len(parent.Messages) != 1 {
		t.Errorf("Expected parent to have 1 message, got: %d", len(parent.Messages))
	}

	// Branch should have 2 messages
	if len(branch.Messages) != 2 {
		t.Errorf("Expected branch to have 2 messages, got: %d", len(branch.Messages))
	}

	// Merge branch back to parent
	sessionMgr.Merge(parent, branch)

	// Parent should now have messages from both
	if len(parent.Messages) < 2 {
		t.Errorf("Expected parent to have at least 2 messages after merge, got: %d", len(parent.Messages))
	}

	t.Log("E2E session branching/merging test passed")
}

// TestE2EMemoryIndexing tests memory indexing and search
func TestE2EMemoryIndexing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	ctx := context.Background()
	sessionMgr, _ := session.NewManager("/tmp/test-e2e-memory")

	// Create session with multiple messages
	sess, _ := sessionMgr.GetOrCreate("memory-test-session")
	messages := []session.Message{
		{Role: "user", Content: "What is machine learning?", Timestamp: time.Now()},
		{Role: "assistant", Content: "Machine learning is a subset of AI...", Timestamp: time.Now()},
		{Role: "user", Content: "How does neural network work?", Timestamp: time.Now()},
		{Role: "assistant", Content: "Neural networks work by...", Timestamp: time.Now()},
	}

	for _, msg := range messages {
		sess.AddMessage(msg)
	}

	// Index memories
	memory := agent.NewMemoryStore(sessionMgr)
	err := memory.IndexSession(ctx, sess)
	if err != nil {
		t.Fatalf("Failed to index session: %v", err)
	}

	// Search for relevant memories
	results, err := memory.Search(ctx, "AI and machine learning", 5)
	if err != nil {
		t.Fatalf("Failed to search memories: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected search results, got none")
	}

	t.Logf("Found %d relevant memories", len(results))
	t.Log("E2E memory indexing test passed")
}

// TestE2EFailoverScenario tests failover in realistic scenario
func TestE2EFailoverScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	ctx := context.Background()
	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager("/tmp/test-e2e-failover")

	// Create rotation provider with primary and fallback
	errorClassifier := agent.NewErrorClassifier()
	rotation := provider.NewRotationProvider(
		provider.RotationStrategyRoundRobin,
		30*time.Second,
		errorClassifier,
	)

	// Add primary that will fail
	primary := &mockProvider{
		shouldFail: true,
		failReason: agent.FailoverReasonRateLimit,
	}

	// Add fallback that will succeed
	fallback := &mockProvider{
		shouldFail: false,
		response:   "Fallback response",
	}

	rotation.AddProfile("primary", primary, "key1", 1)
	rotation.AddProfile("fallback", fallback, "key2", 2)

	// Create agent with rotation provider
	toolRegistry := tools.NewRegistry()
	contextBuilder := agent.NewContextBuilder()
	memory := agent.NewMemoryStore(sessionMgr)

	loopCfg := &agent.Config{
		Bus:          messageBus,
		Provider:     rotation,
		SessionMgr:   sessionMgr,
		Memory:       memory,
		Context:      contextBuilder,
		Tools:        toolRegistry,
		MaxIteration: 3,
	}

	loop, err := agent.NewLoop(loopCfg)
	if err != nil {
		t.Fatalf("Failed to create agent loop: %v", err)
	}

	testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer testCancel()

	if err := loop.Start(testCtx); err != nil {
		t.Fatalf("Failed to start agent loop: %v", err)
	}
	defer loop.Stop()

	// Send message that will trigger failover
	inboundMsg := &bus.InboundMessage{
		Channel:   "test",
		SenderID:  "test-user",
		ChatID:    "failover-test",
		Content:   "Test failover",
		Timestamp: time.Now(),
	}

	messageBus.PublishInbound(testCtx, inboundMsg)

	// Wait for processing
	time.Sleep(2 * time.Second)

	// Verify session was created despite primary failure
	sess, err := sessionMgr.GetOrCreate("test:failover-test")
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	// Should have received a response from fallback
	if len(sess.Messages) < 2 {
		t.Errorf("Expected at least 2 messages, got: %d", len(sess.Messages))
	}

	// Verify primary is in cooldown
	status, _ := rotation.GetProfileStatus("primary")
	if !status["in_cooldown"].(bool) {
		t.Error("Expected primary profile to be in cooldown")
	}

	t.Log("E2E failover scenario test passed")
}

// Mock provider for E2E testing
type mockE2EProvider struct {
	responseCount int
}

func (m *mockE2EProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, options ...providers.ChatOption) (*providers.Response, error) {
	m.responseCount++

	// Simulate realistic response
	responses := []string{
		"Hello! I'm doing well, thank you for asking.",
		"Based on our conversation, I'd suggest...",
		"Let me help you with that.",
	}

	response := responses[m.responseCount%len(responses)]

	return &providers.Response{
		Content: response,
		Usage: providers.Usage{
			PromptTokens:     50,
			CompletionTokens: 30,
			TotalTokens:      80,
		},
	}, nil
}

func (m *mockE2EProvider) ChatWithTools(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, options ...providers.ChatOption) (*providers.Response, error) {
	return m.Chat(ctx, messages, tools, options...)
}

func (m *mockE2EProvider) Close() error {
	return nil
}
