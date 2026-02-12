package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/smallnest/goclaw/config"
)

// MemsearchSearchManager memsearch 后端实现
type MemsearchSearchManager struct {
	cfg       config.MemsearchConfig
	workspace string
}

// NewMemsearchSearchManager 创建 memsearch 搜索管理器
func NewMemsearchSearchManager(cfg config.MemsearchConfig, workspace string) (MemorySearchManager, error) {
	cmd := cfg.Command
	if strings.TrimSpace(cmd) == "" {
		cmd = "memsearch"
	}

	if err := checkMemsearchAvailable(cmd, cfg); err != nil {
		return nil, err
	}

	return &MemsearchSearchManager{
		cfg:       cfg,
		workspace: workspace,
	}, nil
}

// Search 执行搜索
func (m *MemsearchSearchManager) Search(ctx context.Context, query string, opts SearchOptions) ([]*SearchResult, error) {
	args := []string{"search", query, "--json-output"}

	if opts.Limit > 0 {
		args = append(args, "--top-k", fmt.Sprintf("%d", opts.Limit))
	}

	args = append(args, m.buildCommonArgs()...)

	output, err := runMemsearchCommand(ctx, m.cfg.Command, args, m.cfg)
	if err != nil {
		return nil, err
	}

	var items []memsearchResult
	if err := json.Unmarshal(output, &items); err != nil {
		return nil, fmt.Errorf("failed to parse memsearch output: %w", err)
	}

	results := make([]*SearchResult, 0, len(items))
	for _, item := range items {
		if item.Score < opts.MinScore {
			continue
		}

		sr := &SearchResult{
			VectorEmbedding: VectorEmbedding{
				ID:     item.ChunkHash,
				Text:   item.Content,
				Source: "",
				Type:   "",
				Metadata: MemoryMetadata{
					FilePath:   item.Source,
					LineNumber: item.StartLine,
				},
			},
			Score:       item.Score,
			MatchedText: item.Heading,
		}

		results = append(results, sr)
	}

	return results, nil
}

// Add 添加记忆（写入 Markdown 并触发索引）
func (m *MemsearchSearchManager) Add(ctx context.Context, text string, source MemorySource, memType MemoryType, metadata MemoryMetadata) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("text is required")
	}

	if err := appendDailyNote(m.workspace, text); err != nil {
		return err
	}

	memoryDir := filepath.Join(m.workspace, "memory")
	_, err := runMemsearchCommand(ctx, m.cfg.Command, append([]string{"index", memoryDir}, m.buildCommonArgs()...), m.cfg)
	return err
}

// GetStatus 获取状态
func (m *MemsearchSearchManager) GetStatus() map[string]interface{} {
	status := make(map[string]interface{})
	status["backend"] = "memsearch"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := append([]string{"stats"}, m.buildCommonArgs()...)
	output, err := runMemsearchCommand(ctx, m.cfg.Command, args, m.cfg)
	if err != nil {
		status["error"] = err.Error()
		return status
	}

	raw := strings.TrimSpace(string(output))
	status["raw"] = raw

	if total, ok := parseMemsearchStats(raw); ok {
		status["total_count"] = total
	}

	return status
}

// Close 关闭
func (m *MemsearchSearchManager) Close() error {
	return nil
}

// buildCommonArgs builds shared CLI args for memsearch
func (m *MemsearchSearchManager) buildCommonArgs() []string {
	args := []string{}

	if strings.TrimSpace(m.cfg.Provider) != "" {
		args = append(args, "--provider", m.cfg.Provider)
	}
	if strings.TrimSpace(m.cfg.Model) != "" {
		args = append(args, "--model", m.cfg.Model)
	}
	if strings.TrimSpace(m.cfg.Collection) != "" {
		args = append(args, "--collection", m.cfg.Collection)
	}
	if strings.TrimSpace(m.cfg.MilvusURI) != "" {
		args = append(args, "--milvus-uri", m.cfg.MilvusURI)
	}
	if strings.TrimSpace(m.cfg.MilvusToken) != "" {
		args = append(args, "--milvus-token", m.cfg.MilvusToken)
	}

	return args
}

type memsearchResult struct {
	Content      string  `json:"content"`
	Source       string  `json:"source"`
	Heading      string  `json:"heading"`
	ChunkHash    string  `json:"chunk_hash"`
	HeadingLevel int     `json:"heading_level"`
	StartLine    int     `json:"start_line"`
	EndLine      int     `json:"end_line"`
	Score        float64 `json:"score"`
}

func runMemsearchCommand(ctx context.Context, command string, args []string, memCfg config.MemsearchConfig) ([]byte, error) {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		cmd = "memsearch"
	}

	c := exec.CommandContext(ctx, cmd, args...)
	c.Env = BuildMemsearchEnv(memCfg)
	output, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("memsearch command failed: %w, output: %s", err, string(output))
	}
	return output, nil
}

func checkMemsearchAvailable(command string, memCfg config.MemsearchConfig) error {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		cmd = "memsearch"
	}

	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Errorf("memsearch command not found: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := runMemsearchCommand(ctx, cmd, []string{"--version"}, memCfg)
	return err
}

func parseMemsearchStats(output string) (int, bool) {
	re := regexp.MustCompile(`(?i)total\\s+indexed\\s+chunks\\s*:\\s*(\\d+)`)
	match := re.FindStringSubmatch(output)
	if len(match) != 2 {
		return 0, false
	}

	var total int
	_, err := fmt.Sscanf(match[1], "%d", &total)
	if err != nil {
		return 0, false
	}
	return total, true
}

func appendDailyNote(workspace, text string) error {
	if strings.TrimSpace(workspace) == "" {
		return fmt.Errorf("workspace is required")
	}

	memoryDir := filepath.Join(workspace, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return err
	}

	filename := filepath.Join(memoryDir, time.Now().Format("2006-01-02")+".md")
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err == nil && info.Size() > 0 {
		if _, err := file.WriteString("\n\n"); err != nil {
			return err
		}
	}

	_, err = file.WriteString(strings.TrimSpace(text))
	return err
}

// BuildMemsearchEnv builds environment variables for memsearch based on goclaw config.
func BuildMemsearchEnv(memCfg config.MemsearchConfig) []string {
	env := os.Environ()
	root := config.Get()
	if root == nil {
		return env
	}

	provider := normalizeProvider(memCfg.Provider, "openai")
	env = applyProviderEnv(env, provider, root.Providers)

	llmProvider := normalizeProvider(memCfg.Compact.LLMProvider, "openai")
	if !sameProvider(llmProvider, provider) {
		env = applyProviderEnv(env, llmProvider, root.Providers)
	}

	return env
}

func normalizeProvider(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func sameProvider(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func applyProviderEnv(env []string, provider string, providers config.ProvidersConfig) []string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		if strings.TrimSpace(providers.OpenAI.APIKey) != "" {
			env = upsertEnv(env, "OPENAI_API_KEY", providers.OpenAI.APIKey)
		}
		if strings.TrimSpace(providers.OpenAI.BaseURL) != "" {
			env = upsertEnv(env, "OPENAI_BASE_URL", providers.OpenAI.BaseURL)
		}
	case "anthropic":
		if strings.TrimSpace(providers.Anthropic.APIKey) != "" {
			env = upsertEnv(env, "ANTHROPIC_API_KEY", providers.Anthropic.APIKey)
		}
	}

	return env
}

func upsertEnv(env []string, key, value string) []string {
	if strings.TrimSpace(value) == "" {
		return env
	}
	upperKey := strings.ToUpper(key)
	for i, kv := range env {
		idx := strings.IndexByte(kv, '=')
		if idx <= 0 {
			continue
		}
		if strings.ToUpper(kv[:idx]) == upperKey {
			env[i] = key + "=" + value
			return env
		}
	}
	return append(env, key+"="+value)
}
