package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Media 媒体文件
type Media struct {
	Type     string `json:"type"`             // image, video, audio, document
	URL      string `json:"url"`              // 文件URL
	Base64   string `json:"base64,omitempty"` // Base64编码内容
	MimeType string `json:"mimetype"`         // MIME类型
}

// ToolCall 工具调用
type ToolCall struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Params map[string]interface{} `json:"params"`
}

// Message 消息
type Message struct {
	Role       string                 `json:"role"` // user, assistant, system, tool
	Content    string                 `json:"content"`
	Media      []Media                `json:"media,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"` // For tool role
	ToolCalls  []ToolCall             `json:"tool_calls,omitempty"`   // For assistant role
}

// Session 会话
type Session struct {
	Key       string                 `json:"key"`
	Messages  []Message              `json:"messages"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	mu        sync.RWMutex
}

// AddMessage 添加消息
func (s *Session) AddMessage(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// GetHistory 获取历史消息
func (s *Session) GetHistory(maxMessages int) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if maxMessages <= 0 || maxMessages >= len(s.Messages) {
		// 返回所有消息的副本
		result := make([]Message, len(s.Messages))
		copy(result, s.Messages)
		return result
	}

	// 返回最近的消息
	start := len(s.Messages) - maxMessages
	result := make([]Message, maxMessages)
	copy(result, s.Messages[start:])
	return result
}

// Clear 清空消息
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = []Message{}
	s.UpdatedAt = time.Now()
}

// Manager 会话管理器
type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	baseDir  string
}

// NewManager 创建会话管理器
func NewManager(baseDir string) (*Manager, error) {
	// 确保目录存在
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}

	return &Manager{
		sessions: make(map[string]*Session),
		baseDir:  baseDir,
	}, nil
}

// GetOrCreate 获取或创建会话
func (m *Manager) GetOrCreate(key string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查内存缓存
	if session, ok := m.sessions[key]; ok {
		return session, nil
	}

	// 尝试从磁盘加载
	session, err := m.load(key)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		// 文件不存在，创建新会话
		session = &Session{
			Key:       key,
			Messages:  []Message{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Metadata:  make(map[string]interface{}),
		}
	}

	// 添加到缓存
	m.sessions[key] = session
	return session, nil
}

// Save 保存会话
func (m *Manager) Save(session *Session) error {
	session.mu.RLock()
	createdAt := session.CreatedAt
	updatedAt := session.UpdatedAt
	sessionMetadata := make(map[string]interface{}, len(session.Metadata))
	for k, v := range session.Metadata {
		sessionMetadata[k] = v
	}
	messages := make([]Message, len(session.Messages))
	copy(messages, session.Messages)
	session.mu.RUnlock()

	// 确定文件路径
	filePath := m.sessionPath(session.Key)

	// 创建唯一临时文件，避免并发保存时冲突
	tmpPath := fmt.Sprintf("%s.%d.tmp", filePath, time.Now().UnixNano())
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpPath) }()

	// 写入元数据行
	encoder := json.NewEncoder(file)
	metaLine := map[string]interface{}{
		"_type":      "metadata",
		"created_at": createdAt,
		"updated_at": updatedAt,
		"metadata":   sessionMetadata,
	}
	if err := encoder.Encode(metaLine); err != nil {
		_ = file.Close()
		return err
	}

	// 写入消息
	for _, msg := range messages {
		if err := encoder.Encode(msg); err != nil {
			_ = file.Close()
			return err
		}
	}

	// 先关闭文件句柄，再替换目标文件（Windows 对打开句柄的 rename 更严格）
	if err := file.Close(); err != nil {
		return err
	}

	// 尝试替换目标文件，处理 Windows 下偶发占用冲突
	var lastErr error
	for i := 0; i < 5; i++ {
		if err := os.Rename(tmpPath, filePath); err == nil {
			return nil
		} else {
			lastErr = err
		}

		// Windows 上目标文件存在时，先删除再重命名更稳定
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			lastErr = fmt.Errorf("%w (remove target failed: %v)", lastErr, err)
		}
		if err := os.Rename(tmpPath, filePath); err == nil {
			return nil
		} else {
			lastErr = err
		}

		time.Sleep(time.Duration(20*(i+1)) * time.Millisecond)
	}

	if lastErr != nil {
		return lastErr
	}

	return nil
}

// Delete 删除会话
func (m *Manager) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 从缓存中删除
	delete(m.sessions, key)

	// 删除文件
	filePath := m.sessionPath(key)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// List 列出所有会话
func (m *Manager) List() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 读取目录
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		return nil, err
	}

	// 提取会话键
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".jsonl" {
			key := strings.TrimSuffix(entry.Name(), ".jsonl")
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// load 从磁盘加载会话
func (m *Manager) load(key string) (*Session, error) {
	filePath := m.sessionPath(key)

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 创建会话
	session := &Session{
		Key:       key,
		Messages:  []Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	// 解析文件
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}

		// 检查是否为元数据行
		if msgType, ok := raw["_type"].(string); ok && msgType == "metadata" {
			if createdAt, ok := raw["created_at"].(string); ok {
				session.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
			}
			if updatedAt, ok := raw["updated_at"].(string); ok {
				session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
			}
			if metadata, ok := raw["metadata"].(map[string]interface{}); ok {
				session.Metadata = metadata
			}
		} else {
			// 消息行
			data, _ := json.Marshal(raw)
			var msg Message
			if err := json.Unmarshal(data, &msg); err != nil {
				return nil, err
			}
			session.Messages = append(session.Messages, msg)
		}
	}

	return session, nil
}

// sessionPath 获取会话文件路径
func (m *Manager) sessionPath(key string) string {
	// 将 key 中的特殊字符替换为下划线
	safeKey := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, key)

	return filepath.Join(m.baseDir, safeKey+".jsonl")
}

// SessionPath returns the full path to a session JSONL file.
func (m *Manager) SessionPath(key string) string {
	return m.sessionPath(key)
}
