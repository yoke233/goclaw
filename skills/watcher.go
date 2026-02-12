package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// SkillsChangeEvent represents a skill change event
type SkillsChangeEvent struct {
	WorkspaceDir string
	Reason       string // "watch", "manual", "remote-node"
	ChangedPath  string
}

// SkillsWatchState manages the state of a skills watcher
type SkillsWatchState struct {
	Watcher     *fsnotify.Watcher
	PathsKey    string
	DebounceMs  int
	Timer       *time.Timer
	PendingPath string
	Mutex       sync.Mutex
}

// Default ignored patterns for skills watching
var DefaultSkillsWatchIgnored = []*regexp.Regexp{
	regexp.MustCompile(`(^|[\\/])\.git([\\/]|$)`),
	regexp.MustCompile(`(^|[\\/])node_modules([\\/]|$)`),
	regexp.MustCompile(`(^|[\\/])dist([\\/]|$)`),
}

// Global state for skills watching
type SkillsWatchManager struct {
	Listeners         map[string]func(SkillsChangeEvent)
	WorkspaceVersions map[string]int64
	Watchers          map[string]*SkillsWatchState
	GlobalVersion     int64
	Mutex             sync.RWMutex
}

var globalWatchManager = &SkillsWatchManager{
	Listeners:         make(map[string]func(SkillsChangeEvent)),
	WorkspaceVersions: make(map[string]int64),
	Watchers:          make(map[string]*SkillsWatchState),
}

// RegisterSkillsChangeListener registers a listener for skills change events
func RegisterSkillsChangeListener(listener func(event SkillsChangeEvent)) func() {
	globalWatchManager.Mutex.Lock()
	defer globalWatchManager.Mutex.Unlock()

	// Generate unique ID for listener
	listenerID := fmt.Sprintf("%d", time.Now().UnixNano())
	globalWatchManager.Listeners[listenerID] = listener

	// Return unsubscribe function
	return func() {
		globalWatchManager.Mutex.Lock()
		delete(globalWatchManager.Listeners, listenerID)
		globalWatchManager.Mutex.Unlock()
	}
}

// BumpSkillsSnapshotVersion bumps the version counter and notifies listeners
func BumpSkillsSnapshotVersion(params BumpVersionParams) int64 {
	globalWatchManager.Mutex.Lock()
	defer globalWatchManager.Mutex.Unlock()

	reason := params.Reason
	if reason == "" {
		reason = "manual"
	}

	var newVersion int64
	event := SkillsChangeEvent{
		Reason:      reason,
		ChangedPath: params.ChangedPath,
	}

	if params.WorkspaceDir != "" {
		// Bump workspace-specific version
		current := globalWatchManager.WorkspaceVersions[params.WorkspaceDir]
		newVersion = bumpVersion(current)
		globalWatchManager.WorkspaceVersions[params.WorkspaceDir] = newVersion
		event.WorkspaceDir = params.WorkspaceDir
	} else {
		// Bump global version
		globalWatchManager.GlobalVersion = bumpVersion(globalWatchManager.GlobalVersion)
		newVersion = globalWatchManager.GlobalVersion
	}

	// Notify all listeners
	go emitSkillsChangeEvent(event)

	return newVersion
}

// BumpVersionParams configures version bumping
type BumpVersionParams struct {
	WorkspaceDir string
	Reason       string
	ChangedPath  string
}

// GetSkillsSnapshotVersion returns the current snapshot version
func GetSkillsSnapshotVersion(workspaceDir string) int64 {
	globalWatchManager.Mutex.RLock()
	defer globalWatchManager.Mutex.RUnlock()

	if workspaceDir == "" {
		return globalWatchManager.GlobalVersion
	}

	local := globalWatchManager.WorkspaceVersions[workspaceDir]
	return maxInt64(globalWatchManager.GlobalVersion, local)
}

// EnsureSkillsWatcher ensures a skills watcher is running for a workspace
func EnsureSkillsWatcher(params EnsureWatcherParams) error {
	workspaceDir := params.WorkspaceDir
	if workspaceDir == "" {
		return nil
	}

	// Check if watching is enabled
	watchEnabled := true
	if params.Config != nil {
		skillsConfig := getSkillsConfig(params.Config)
		watchEnabled = skillsConfig.Load.Watch
	}

	// Get debounce settings
	debounceMs := 250
	if params.Config != nil {
		skillsConfig := getSkillsConfig(params.Config)
		if skillsConfig.Load.WatchDebounceMs > 0 {
			debounceMs = skillsConfig.Load.WatchDebounceMs
		}
	}

	// Stop existing watcher if needed
	globalWatchManager.Mutex.Lock()
	defer globalWatchManager.Mutex.Unlock()

	existing, exists := globalWatchManager.Watchers[workspaceDir]
	if !watchEnabled {
		if exists {
			delete(globalWatchManager.Watchers, workspaceDir)
			if existing.Timer != nil {
				existing.Timer.Stop()
			}
			if existing.Watcher != nil {
				_ = existing.Watcher.Close()
			}
		}
		return nil
	}

	// Resolve watch paths
	watchPaths := resolveWatchPaths(workspaceDir, params.Config)
	if len(watchPaths) == 0 {
		return nil
	}

	pathsKey := strings.Join(watchPaths, "|")

	// Check if existing watcher is already watching the same paths
	if exists && existing.PathsKey == pathsKey && existing.DebounceMs == debounceMs {
		return nil
	}

	// Stop existing watcher
	if exists {
		delete(globalWatchManager.Watchers, workspaceDir)
		if existing.Timer != nil {
			existing.Timer.Stop()
		}
		if existing.Watcher != nil {
			_ = existing.Watcher.Close()
		}
	}

	// Create new watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	// Add watch paths
	for _, path := range watchPaths {
		if err := watcher.Add(path); err != nil {
			logger.Warn("Failed to watch path", zap.String("path", path), zap.Error(err))
		}
	}

	state := &SkillsWatchState{
		Watcher:    watcher,
		PathsKey:   pathsKey,
		DebounceMs: debounceMs,
	}

	// Schedule event processing
	go processWatcherEvents(state, workspaceDir)

	globalWatchManager.Watchers[workspaceDir] = state
	return nil
}

// EnsureWatcherParams configures skills watching
type EnsureWatcherParams struct {
	WorkspaceDir string
	Config       *config.Config
}

// resolveWatchPaths resolves the paths that should be watched for skill changes
func resolveWatchPaths(workspaceDir string, cfg *config.Config) []string {
	var paths []string

	if workspaceDir != "" {
		workspaceSkills := filepath.Join(workspaceDir, "skills")
		if exists(workspaceSkills) {
			paths = append(paths, workspaceSkills)
		}
	}

	// Add managed skills directory
	home, err := config.ResolveUserHomeDir()
	if err == nil {
		managedSkills := filepath.Join(home, ".goclaw", "skills")
		if exists(managedSkills) {
			paths = append(paths, managedSkills)
		}
	}

	// Add extra directories from config
	if cfg != nil && cfg.Skills != nil {
		skillsConfig := getSkillsConfig(cfg)
		for _, extraDir := range skillsConfig.Load.ExtraDirs {
			if extraDir != "" && exists(extraDir) {
				paths = append(paths, extraDir)
			}
		}
	}

	// Add plugin skill directories
	var skillsConfig map[string]interface{}
	if cfg != nil {
		skillsConfig = cfg.Skills
	}
	pluginDirs := scanPluginSkillDirs(workspaceDir, skillsConfig)
	for _, pluginDir := range pluginDirs {
		if exists(pluginDir) {
			paths = append(paths, pluginDir)
		}
	}

	return paths
}

// processWatcherEvents processes events from the filesystem watcher
func processWatcherEvents(state *SkillsWatchState, workspaceDir string) {
	logger.Info("Starting skills watcher", zap.String("workspace", workspaceDir))

	defer func() {
		if r := recover(); r != nil {
			logger.Error("Skills watcher panicked", zap.Any("recover", r))
		}
		if state.Watcher != nil {
			_ = state.Watcher.Close()
		}
	}()

	for {
		select {
		case event, ok := <-state.Watcher.Events:
			if !ok {
				return
			}

			// Filter ignored paths
			if isIgnoredPath(event.Name) {
				continue
			}

			// Only process .md files
			if !strings.HasSuffix(event.Name, ".md") {
				continue
			}

			// Skip SKILL.md files that aren't in skill directories
			if filepath.Base(event.Name) == "SKILL.md" {
				if !isSkillDirectory(filepath.Dir(event.Name)) {
					continue
				}
			}

			logger.Debug("Skills file changed",
				zap.String("workspace", workspaceDir),
				zap.String("path", event.Name),
				zap.String("op", event.Op.String()),
			)

			// Schedule debounced bump
			state.Mutex.Lock()
			state.PendingPath = event.Name
			if state.Timer != nil {
				state.Timer.Stop()
			}
			state.Timer = time.AfterFunc(time.Duration(state.DebounceMs)*time.Millisecond, func() {
				state.Mutex.Lock()
				pendingPath := state.PendingPath
				state.PendingPath = ""
				state.Timer = nil
				state.Mutex.Unlock()

				BumpSkillsSnapshotVersion(BumpVersionParams{
					WorkspaceDir: workspaceDir,
					Reason:       "watch",
					ChangedPath:  pendingPath,
				})
			})
			state.Mutex.Unlock()

		case err, ok := <-state.Watcher.Errors:
			if !ok {
				return
			}
			logger.Warn("Skills watcher error",
				zap.String("workspace", workspaceDir),
				zap.Error(err),
			)
		}
	}
}

// isIgnoredPath checks if a path should be ignored
func isIgnoredPath(path string) bool {
	for _, pattern := range DefaultSkillsWatchIgnored {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}

// isSkillDirectory checks if a directory looks like a skill directory
func isSkillDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	// Check if there's a SKILL.md file
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() == "SKILL.md" {
			return true
		}
	}

	return false
}

// exists checks if a path exists
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// emitSkillsChangeEvent notifies all listeners of a skills change event
func emitSkillsChangeEvent(event SkillsChangeEvent) {
	globalWatchManager.Mutex.RLock()
	defer globalWatchManager.Mutex.RUnlock()

	for _, listener := range globalWatchManager.Listeners {
		go func(l func(SkillsChangeEvent), e SkillsChangeEvent) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Skills change listener panicked", zap.Any("recover", r))
				}
			}()
			l(e)
		}(listener, event)
	}
}

// bumpVersion increments a version number ensuring monotonic increase
func bumpVersion(current int64) int64 {
	now := time.Now().UnixMilli()
	if now <= current {
		return current + 1
	}
	return now
}

// maxInt64 returns the maximum of two int64 values
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// StopAllWatchers stops all active skills watchers
func StopAllWatchers() {
	globalWatchManager.Mutex.Lock()
	defer globalWatchManager.Mutex.Unlock()

	for workspace, state := range globalWatchManager.Watchers {
		logger.Info("Stopping skills watcher", zap.String("workspace", workspace))
		delete(globalWatchManager.Watchers, workspace)
		if state.Timer != nil {
			state.Timer.Stop()
		}
		if state.Watcher != nil {
			_ = state.Watcher.Close()
		}
	}
}

// GetActiveWatchers returns the number of active watchers
func GetActiveWatchers() int {
	globalWatchManager.Mutex.RLock()
	defer globalWatchManager.Mutex.RUnlock()
	return len(globalWatchManager.Watchers)
}

// Initialize skills watching on application startup
func init() {
	// Gracefully shutdown watchers on application exit
	go func() {
		<-context.Background().Done()
		StopAllWatchers()
	}()
}
