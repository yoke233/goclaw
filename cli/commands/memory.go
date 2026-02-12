package commands

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/memory"
	"github.com/smallnest/goclaw/memory/qmd"
	"github.com/smallnest/goclaw/session"
	"github.com/spf13/cobra"
)

// MemoryCmd 记忆管理命令
var MemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage goclaw memory",
	Long:  `View status, index, and search memory stores. Uses memsearch backend.`,
}

// memoryStatusCmd 显示记忆状态
var memoryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory index statistics",
	Long:  `Display statistics about the memory store including backend type, collections, and documents.`,
	Run:   runMemoryStatus,
}

// memoryIndexCmd 重新索引记忆文件
var memoryIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Reindex memory files",
	Long:  `Rebuild the memory index from configured sources.`,
	Run:   runMemoryIndex,
}

// memorySearchCmd 语义搜索记忆
var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search over memory",
	Long:  `Perform semantic search over stored memories using the configured backend.`,
	Args:  cobra.ExactArgs(1),
	Run:   runMemorySearch,
}

// memoryBackendCmd 查看当前后端
var memoryBackendCmd = &cobra.Command{
	Use:   "backend",
	Short: "Show current memory backend",
	Long:  `Display the current memory backend configuration.`,
	Run:   runMemoryBackend,
}

// memoryWatchCmd 监听并自动索引
var memoryWatchCmd = &cobra.Command{
	Use:   "watch <paths...>",
	Short: "Watch paths and auto-index markdown changes",
	Args:  cobra.MinimumNArgs(1),
	Run:   runMemoryWatch,
}

// memoryCompactCmd 压缩索引内容
var memoryCompactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact indexed memory into a summary",
	Run:   runMemoryCompact,
}

// memoryExpandCmd 展开 chunk
var memoryExpandCmd = &cobra.Command{
	Use:   "expand <chunk_hash>",
	Short: "Expand a chunk to show full context",
	Args:  cobra.ExactArgs(1),
	Run:   runMemoryExpand,
}

// memoryTranscriptCmd 查看 JSONL 会话
var memoryTranscriptCmd = &cobra.Command{
	Use:   "transcript <jsonl_path>",
	Short: "View conversation turns from a JSONL transcript",
	Args:  cobra.ExactArgs(1),
	Run:   runMemoryTranscript,
}

// memoryResetCmd 删除索引
var memoryResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Drop all indexed data from the collection",
	Run:   runMemoryReset,
}

var (
	memorySearchLimit    int
	memorySearchMinScore float64
	memorySearchJSON     bool

	memoryIndexForce bool

	memoryWatchDebounce int

	memoryCompactSource      string
	memoryCompactOutputDir   string
	memoryCompactLLMProvider string
	memoryCompactLLMModel    string
	memoryCompactPrompt      string
	memoryCompactPromptFile  string

	memoryExpandLines   int
	memoryExpandJSON    bool
	memoryExpandSection bool

	memoryTranscriptTurn    string
	memoryTranscriptContext int
	memoryTranscriptJSON    bool

	memoryResetYes bool
)

func init() {
	MemoryCmd.AddCommand(memoryStatusCmd)
	MemoryCmd.AddCommand(memoryIndexCmd)
	MemoryCmd.AddCommand(memorySearchCmd)
	MemoryCmd.AddCommand(memoryBackendCmd)
	MemoryCmd.AddCommand(memoryWatchCmd)
	MemoryCmd.AddCommand(memoryCompactCmd)
	MemoryCmd.AddCommand(memoryExpandCmd)
	MemoryCmd.AddCommand(memoryTranscriptCmd)
	MemoryCmd.AddCommand(memoryResetCmd)

	memorySearchCmd.Flags().IntVarP(&memorySearchLimit, "limit", "n", 10, "Maximum number of results")
	memorySearchCmd.Flags().Float64Var(&memorySearchMinScore, "min-score", 0.0, "Minimum similarity score (0-1)")
	memorySearchCmd.Flags().BoolVar(&memorySearchJSON, "json", false, "Output in JSON format")

	memoryIndexCmd.Flags().BoolVar(&memoryIndexForce, "force", false, "Force re-index of all chunks")

	memoryWatchCmd.Flags().IntVar(&memoryWatchDebounce, "debounce-ms", 0, "Debounce delay in milliseconds")

	memoryCompactCmd.Flags().StringVar(&memoryCompactSource, "source", "", "Only compact chunks from this specific source file")
	memoryCompactCmd.Flags().StringVar(&memoryCompactOutputDir, "output-dir", "", "Directory to write the compact summary into")
	memoryCompactCmd.Flags().StringVar(&memoryCompactLLMProvider, "llm-provider", "", "LLM provider for summarization")
	memoryCompactCmd.Flags().StringVar(&memoryCompactLLMModel, "llm-model", "", "Override LLM model")
	memoryCompactCmd.Flags().StringVar(&memoryCompactPrompt, "prompt", "", "Custom prompt template string")
	memoryCompactCmd.Flags().StringVar(&memoryCompactPromptFile, "prompt-file", "", "Custom prompt template file")

	memoryExpandCmd.Flags().IntVarP(&memoryExpandLines, "lines", "n", 0, "Show N lines around the chunk instead of full section")
	memoryExpandCmd.Flags().BoolVar(&memoryExpandJSON, "json", false, "Output in JSON format")
	memoryExpandCmd.Flags().BoolVar(&memoryExpandSection, "section", true, "Show full section (default true)")

	memoryTranscriptCmd.Flags().StringVarP(&memoryTranscriptTurn, "turn", "t", "", "Target turn ID prefix")
	memoryTranscriptCmd.Flags().IntVarP(&memoryTranscriptContext, "context", "c", 3, "Number of turns before and after target")
	memoryTranscriptCmd.Flags().BoolVar(&memoryTranscriptJSON, "json", false, "Output in JSON format")

	memoryResetCmd.Flags().BoolVarP(&memoryResetYes, "yes", "y", false, "Skip confirmation prompt")
}

// getWorkspace 获取工作区路径
func getWorkspace() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	cfg, err := config.Load("")
	if err != nil {
		// 使用默认工作区
		return filepath.Join(home, ".goclaw", "workspace"), nil
	}

	if cfg.Workspace.Path != "" {
		return cfg.Workspace.Path, nil
	}

	return filepath.Join(home, ".goclaw", "workspace"), nil
}

// getSearchManager 获取搜索管理器
func getSearchManager() (memory.MemorySearchManager, error) {
	workspace, err := getWorkspace()
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load("")
	if err != nil {
		cfg = &config.Config{}
	}

	return memory.GetMemorySearchManager(cfg.Memory, workspace)
}

// runMemoryStatus 执行记忆状态命令
func runMemoryStatus(cmd *cobra.Command, args []string) {
	mgr, err := getSearchManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create search manager: %v\n", err)
		os.Exit(1)
	}
	defer mgr.Close()

	status := mgr.GetStatus()

	fmt.Println("Memory Status")
	fmt.Println("=============")

	if backend, ok := status["backend"].(string); ok {
		fmt.Printf("Backend: %s\n", backend)
	}

	if totalCount, ok := status["total_count"].(int); ok {
		fmt.Printf("Total Indexed Chunks: %d\n", totalCount)
	}

	if raw, ok := status["raw"].(string); ok && raw != "" {
		fmt.Printf("\nRaw Output:\n%s\n", raw)
	}

	if errMsg, ok := status["error"].(string); ok && errMsg != "" {
		fmt.Printf("\nError: %s\n", errMsg)
	}
}

// runMemoryBackend 显示当前后端
func runMemoryBackend(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Printf("Backend: memsearch (default)\n")
		return
	}

	backend := cfg.Memory.Backend
	if backend == "" {
		backend = "memsearch"
	}

	fmt.Printf("Backend: %s\n", backend)

	if backend == "memsearch" {
		ms := cfg.Memory.Memsearch
		if strings.TrimSpace(ms.Command) == "" {
			ms.Command = "memsearch"
		}
		fmt.Printf("  Command: %s\n", ms.Command)
		if ms.Provider != "" {
			fmt.Printf("  Provider: %s\n", ms.Provider)
		}
		if ms.Model != "" {
			fmt.Printf("  Model: %s\n", ms.Model)
		}
		if ms.MilvusURI != "" {
			fmt.Printf("  Milvus URI: %s\n", ms.MilvusURI)
		}
		if ms.Collection != "" {
			fmt.Printf("  Collection: %s\n", ms.Collection)
		}
		if ms.Sessions.Enabled {
			fmt.Printf("  Sessions Export: %s\n", ms.Sessions.ExportDir)
			fmt.Printf("  Sessions Retention Days: %d\n", ms.Sessions.RetentionDays)
		}
	}
}

// runMemoryIndex 执行记忆索引命令
func runMemoryIndex(cmd *cobra.Command, args []string) {
	workspace, err := getWorkspace()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get workspace: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load("")
	if err != nil {
		cfg = &config.Config{}
	}

	ms := resolveMemsearchConfig(cfg)

	if err := ensureMemsearchAvailable(ms); err != nil {
		fmt.Fprintf(os.Stderr, "memsearch not available: %v\n", err)
		os.Exit(1)
	}

	paths := make([]string, 0, 2)
	if len(args) > 0 {
		paths = append(paths, args...)
	} else {
		memoryDir := filepath.Join(workspace, "memory")
		_ = os.MkdirAll(memoryDir, 0755)
		paths = append(paths, memoryDir)
	}

	// Export sessions and apply retention
	if ms.Sessions.Enabled {
		sessionDir, err := qmd.FindSessionDir(workspace)
		if err != nil {
			sessionDir = defaultSessionDir()
		}

		if _, err := memory.PruneSessionJSONL(sessionDir, ms.Sessions.RetentionDays); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to prune JSONL sessions: %v\n", err)
		}

		if ms.Sessions.ExportDir == "" {
			home, _ := os.UserHomeDir()
			ms.Sessions.ExportDir = filepath.Join(home, ".goclaw", "sessions", "export")
		}
		ms.Sessions.ExportDir = expandHomeDir(ms.Sessions.ExportDir)

		if _, err := memory.ExportSessionsToMarkdown(sessionDir, ms.Sessions.ExportDir, ms.Sessions.RetentionDays, ms.Sessions.Redact); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to export sessions: %v\n", err)
		} else {
			paths = append(paths, ms.Sessions.ExportDir)
		}
	}

	indexArgs := []string{"index"}
	indexArgs = append(indexArgs, paths...)
	if memoryIndexForce {
		indexArgs = append(indexArgs, "--force")
	}
	indexArgs = append(indexArgs, buildMemsearchCommonArgs(ms, true)...)

	if err := runMemsearchStreaming(ms, indexArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Index failed: %v\n", err)
		os.Exit(1)
	}
}

// runMemorySearch 执行记忆搜索命令
func runMemorySearch(cmd *cobra.Command, args []string) {
	query := args[0]

	mgr, err := getSearchManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create search manager: %v\n", err)
		os.Exit(1)
	}
	defer mgr.Close()

	// Perform search
	ctx := context.Background()
	opts := memory.DefaultSearchOptions()
	opts.Limit = memorySearchLimit
	opts.MinScore = memorySearchMinScore

	results, err := mgr.Search(ctx, query, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Search failed: %v\n", err)
		os.Exit(1)
	}

	if memorySearchJSON {
		outputSearchResultsJSON(query, results)
		return
	}

	outputSearchResults(query, results)
}

// outputSearchResultsJSON 输出搜索结果为 JSON
func outputSearchResultsJSON(query string, results []*memory.SearchResult) {
	data := struct {
		Query   string                 `json:"query"`
		Count   int                    `json:"count"`
		Results []*memory.SearchResult `json:"results"`
	}{
		Query:   query,
		Count:   len(results),
		Results: results,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(jsonData))
}

// outputSearchResults 输出搜索结果
func outputSearchResults(query string, results []*memory.SearchResult) {
	if len(results) == 0 {
		fmt.Printf("No results found for: %s\n", query)
		return
	}

	fmt.Printf("Search Results for: %s\n", query)
	fmt.Printf("Found %d result(s)\n\n", len(results))

	for i, result := range results {
		fmt.Printf("[%d] Score: %.2f\n", i+1, result.Score)
		if result.Source != "" {
			fmt.Printf("    Source: %s\n", result.Source)
		}
		if result.Type != "" {
			fmt.Printf("    Type: %s\n", result.Type)
		}

		if result.Metadata.FilePath != "" {
			fmt.Printf("    File: %s", result.Metadata.FilePath)
			if result.Metadata.LineNumber > 0 {
				fmt.Printf(":%d", result.Metadata.LineNumber)
			}
			fmt.Println()
		}

		if !result.CreatedAt.IsZero() {
			fmt.Printf("    Created: %s\n", result.CreatedAt.Format(time.RFC3339))
		}

		// Truncate text for display
		text := result.Text
		maxLen := 200
		if len(text) > maxLen {
			text = text[:maxLen] + "..."
		}
		fmt.Printf("    Text: %s\n\n", text)
	}
}

// runMemoryWatch 执行记忆监听命令
func runMemoryWatch(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		cfg = &config.Config{}
	}
	ms := resolveMemsearchConfig(cfg)

	if err := ensureMemsearchAvailable(ms); err != nil {
		fmt.Fprintf(os.Stderr, "memsearch not available: %v\n", err)
		os.Exit(1)
	}

	watchArgs := []string{"watch"}
	watchArgs = append(watchArgs, args...)

	debounce := memoryWatchDebounce
	if debounce <= 0 {
		debounce = ms.Watch.DebounceMs
	}
	if debounce > 0 {
		watchArgs = append(watchArgs, "--debounce-ms", fmt.Sprintf("%d", debounce))
	}

	watchArgs = append(watchArgs, buildMemsearchCommonArgs(ms, true)...)
	if err := runMemsearchStreaming(ms, watchArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Watch failed: %v\n", err)
		os.Exit(1)
	}
}

// runMemoryCompact 执行记忆压缩命令
func runMemoryCompact(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		cfg = &config.Config{}
	}
	ms := resolveMemsearchConfig(cfg)

	if err := ensureMemsearchAvailable(ms); err != nil {
		fmt.Fprintf(os.Stderr, "memsearch not available: %v\n", err)
		os.Exit(1)
	}

	compactArgs := []string{"compact"}
	if memoryCompactSource != "" {
		compactArgs = append(compactArgs, "--source", memoryCompactSource)
	}
	if memoryCompactOutputDir != "" {
		compactArgs = append(compactArgs, "--output-dir", memoryCompactOutputDir)
	}

	llmProvider := firstNonEmpty(memoryCompactLLMProvider, ms.Compact.LLMProvider)
	if llmProvider != "" {
		compactArgs = append(compactArgs, "--llm-provider", llmProvider)
	}

	llmModel := firstNonEmpty(memoryCompactLLMModel, ms.Compact.LLMModel)
	if llmModel != "" {
		compactArgs = append(compactArgs, "--llm-model", llmModel)
	}

	if memoryCompactPrompt != "" {
		compactArgs = append(compactArgs, "--prompt", memoryCompactPrompt)
	}
	if memoryCompactPromptFile != "" {
		compactArgs = append(compactArgs, "--prompt-file", memoryCompactPromptFile)
	}

	compactArgs = append(compactArgs, buildMemsearchCommonArgs(ms, true)...)
	if err := runMemsearchStreaming(ms, compactArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Compact failed: %v\n", err)
		os.Exit(1)
	}
}

// runMemoryExpand 执行记忆展开命令
func runMemoryExpand(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		cfg = &config.Config{}
	}
	ms := resolveMemsearchConfig(cfg)

	if err := ensureMemsearchAvailable(ms); err != nil {
		fmt.Fprintf(os.Stderr, "memsearch not available: %v\n", err)
		os.Exit(1)
	}

	expandArgs := []string{"expand", args[0]}
	if memoryExpandLines > 0 {
		expandArgs = append(expandArgs, "--lines", fmt.Sprintf("%d", memoryExpandLines))
	}
	if !memoryExpandSection {
		expandArgs = append(expandArgs, "--no-section")
	}
	if memoryExpandJSON {
		expandArgs = append(expandArgs, "--json-output")
	}

	expandArgs = append(expandArgs, buildMemsearchCommonArgs(ms, true)...)
	if err := runMemsearchStreaming(ms, expandArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Expand failed: %v\n", err)
		os.Exit(1)
	}
}

// runMemoryTranscript 查看 JSONL 会话
func runMemoryTranscript(cmd *cobra.Command, args []string) {
	jsonlPath := args[0]

	createdAt, messages, err := readTranscriptJSONL(jsonlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Transcript failed: %v\n", err)
		os.Exit(1)
	}

	turns := buildTranscriptTurns(messages)
	if memoryTranscriptTurn == "" {
		if memoryTranscriptJSON {
			outputTranscriptJSON(createdAt, turns)
			return
		}
		outputTranscriptList(turns)
		return
	}

	idx := findTurnIndex(turns, memoryTranscriptTurn)
	if idx < 0 {
		fmt.Fprintf(os.Stderr, "Turn not found: %s\n", memoryTranscriptTurn)
		os.Exit(1)
	}

	start := idx - memoryTranscriptContext
	if start < 0 {
		start = 0
	}
	end := idx + memoryTranscriptContext
	if end >= len(turns) {
		end = len(turns) - 1
	}

	selection := turns[start : end+1]
	if memoryTranscriptJSON {
		outputTranscriptJSON(createdAt, selection)
		return
	}

	outputTranscriptContext(selection, turns[idx].ID)
}

// runMemoryReset 删除索引
func runMemoryReset(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		cfg = &config.Config{}
	}
	ms := resolveMemsearchConfig(cfg)

	if err := ensureMemsearchAvailable(ms); err != nil {
		fmt.Fprintf(os.Stderr, "memsearch not available: %v\n", err)
		os.Exit(1)
	}

	resetArgs := []string{"reset"}
	if memoryResetYes {
		resetArgs = append(resetArgs, "--yes")
	}
	resetArgs = append(resetArgs, buildMemsearchCommonArgs(ms, false)...)

	if err := runMemsearchStreaming(ms, resetArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Reset failed: %v\n", err)
		os.Exit(1)
	}
}

type transcriptTurn struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
}

func readTranscriptJSONL(filePath string) (time.Time, []session.Message, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, nil, err
	}
	defer file.Close()

	var createdAt time.Time
	messages := make([]session.Message, 0)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var meta struct {
			Type      string    `json:"_type"`
			CreatedAt time.Time `json:"created_at"`
		}
		if err := json.Unmarshal([]byte(line), &meta); err == nil && meta.Type == "metadata" {
			createdAt = meta.CreatedAt
			continue
		}

		var msg session.Message
		if err := json.Unmarshal([]byte(line), &msg); err == nil {
			messages = append(messages, msg)
		}
	}

	if err := scanner.Err(); err != nil {
		return time.Time{}, nil, err
	}

	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	return createdAt, messages, nil
}

func buildTranscriptTurns(messages []session.Message) []transcriptTurn {
	turns := make([]transcriptTurn, 0, len(messages))
	for _, msg := range messages {
		turns = append(turns, transcriptTurn{
			ID:        buildTranscriptTurnID(msg),
			Timestamp: msg.Timestamp,
			Role:      msg.Role,
			Content:   msg.Content,
		})
	}
	return turns
}

func buildTranscriptTurnID(msg session.Message) string {
	h := md5.New()
	h.Write([]byte(msg.Role))
	h.Write([]byte("|"))
	h.Write([]byte(msg.Timestamp.Format(time.RFC3339Nano)))
	h.Write([]byte("|"))
	h.Write([]byte(msg.Content))
	sum := hex.EncodeToString(h.Sum(nil))
	if len(sum) > 8 {
		return sum[:8]
	}
	return sum
}

func findTurnIndex(turns []transcriptTurn, prefix string) int {
	for i, t := range turns {
		if strings.HasPrefix(t.ID, prefix) {
			return i
		}
	}
	return -1
}

func outputTranscriptList(turns []transcriptTurn) {
	fmt.Printf("All turns (%d):\n\n", len(turns))
	for _, t := range turns {
		fmt.Printf("  %s  %s  %s\n", t.ID, t.Timestamp.Format("15:04:05"), truncateString(t.Content, 80))
	}
}

func outputTranscriptContext(turns []transcriptTurn, focusID string) {
	for _, t := range turns {
		prefix := " "
		if t.ID == focusID {
			prefix = ">"
		}
		fmt.Printf("%s [%s] %s\n", prefix, t.Timestamp.Format("15:04:05"), t.ID)
		fmt.Printf("%s\n\n", t.Content)
	}
}

func outputTranscriptJSON(createdAt time.Time, turns []transcriptTurn) {
	data := struct {
		CreatedAt time.Time        `json:"created_at"`
		Count     int              `json:"count"`
		Turns     []transcriptTurn `json:"turns"`
	}{
		CreatedAt: createdAt,
		Count:     len(turns),
		Turns:     turns,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(jsonData))
}

func resolveMemsearchConfig(cfg *config.Config) config.MemsearchConfig {
	ms := cfg.Memory.Memsearch
	if strings.TrimSpace(ms.Command) == "" {
		ms.Command = "memsearch"
	}
	if strings.TrimSpace(ms.Collection) == "" {
		ms.Collection = "memsearch_chunks"
	}
	if strings.TrimSpace(ms.MilvusURI) == "" {
		ms.MilvusURI = "~/.memsearch/milvus.db"
	}
	if ms.Watch.DebounceMs == 0 {
		ms.Watch.DebounceMs = 1500
	}
	if ms.Chunking.MaxChunkSize == 0 {
		ms.Chunking.MaxChunkSize = 1500
	}
	if ms.Chunking.OverlapLines == 0 {
		ms.Chunking.OverlapLines = 2
	}
	if ms.Sessions.RetentionDays == 0 {
		ms.Sessions.RetentionDays = 60
	}
	if ms.Context.Limit == 0 {
		ms.Context.Limit = 6
	}
	if ms.Compact.LLMProvider == "" {
		ms.Compact.LLMProvider = "openai"
	}
	return ms
}

func buildMemsearchCommonArgs(cfg config.MemsearchConfig, includeEmbeddingArgs bool) []string {
	args := []string{}
	if includeEmbeddingArgs {
		if strings.TrimSpace(cfg.Provider) != "" {
			args = append(args, "--provider", cfg.Provider)
		}
		if strings.TrimSpace(cfg.Model) != "" {
			args = append(args, "--model", cfg.Model)
		}
	}
	if strings.TrimSpace(cfg.Collection) != "" {
		args = append(args, "--collection", cfg.Collection)
	}
	if strings.TrimSpace(cfg.MilvusURI) != "" {
		args = append(args, "--milvus-uri", expandHomeDir(cfg.MilvusURI))
	}
	if strings.TrimSpace(cfg.MilvusToken) != "" {
		args = append(args, "--milvus-token", cfg.MilvusToken)
	}
	return args
}

func defaultSessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".goclaw", "sessions")
	}
	return filepath.Join(home, ".goclaw", "sessions")
}

func expandHomeDir(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return path
	}
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}

	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return path
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(p, "~/"), "~\\")
		return filepath.Join(home, filepath.FromSlash(rest))
	}

	return path
}

func ensureMemsearchAvailable(cfg config.MemsearchConfig) error {
	cmd := strings.TrimSpace(cfg.Command)
	if cmd == "" {
		cmd = "memsearch"
	}
	if _, err := exec.LookPath(cmd); err != nil {
		return err
	}
	return nil
}

func runMemsearchStreaming(cfg config.MemsearchConfig, args []string) error {
	cmd := strings.TrimSpace(cfg.Command)
	if cmd == "" {
		cmd = "memsearch"
	}
	c := exec.Command(cmd, args...)
	c.Env = memory.BuildMemsearchEnv(cfg)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
