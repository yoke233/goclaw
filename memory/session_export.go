package memory

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/smallnest/goclaw/session"
)

// ExportSessionsToMarkdown exports JSONL sessions to Markdown files.
func ExportSessionsToMarkdown(sessionDir, exportDir string, retentionDays int, redact bool) (int, error) {
	if sessionDir == "" || exportDir == "" {
		return 0, fmt.Errorf("sessionDir and exportDir are required")
	}

	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create export directory: %w", err)
	}

	files, err := os.ReadDir(sessionDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read session directory: %w", err)
	}

	exported := 0
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		filePath := filepath.Join(sessionDir, file.Name())
		sessionKey := strings.TrimSuffix(file.Name(), ".jsonl")

		createdAt, messages, err := readSessionJSONL(filePath)
		if err != nil {
			continue
		}

		if retentionDays > 0 && time.Since(createdAt) > time.Duration(retentionDays)*24*time.Hour {
			continue
		}

		content := buildSessionMarkdown(sessionKey, createdAt, messages, filePath, redact)
		outPath := filepath.Join(exportDir, sessionKey+".md")
		if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
			continue
		}

		exported++
	}

	return exported, nil
}

// ExportSessionJSONLToMarkdown exports a single JSONL session file to Markdown.
// Returns the output path on success.
func ExportSessionJSONLToMarkdown(jsonlPath, exportDir string, redact bool) (string, error) {
	if strings.TrimSpace(jsonlPath) == "" || strings.TrimSpace(exportDir) == "" {
		return "", fmt.Errorf("jsonlPath and exportDir are required")
	}

	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	sessionKey := strings.TrimSuffix(filepath.Base(jsonlPath), ".jsonl")
	createdAt, messages, err := readSessionJSONL(jsonlPath)
	if err != nil {
		return "", err
	}

	content := buildSessionMarkdown(sessionKey, createdAt, messages, jsonlPath, redact)
	outPath := filepath.Join(exportDir, sessionKey+".md")
	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		return "", err
	}

	return outPath, nil
}

// PruneSessionJSONL deletes JSONL session files older than retentionDays.
func PruneSessionJSONL(sessionDir string, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	files, err := os.ReadDir(sessionDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read session directory: %w", err)
	}

	removed := 0
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		filePath := filepath.Join(sessionDir, file.Name())
		createdAt, _, err := readSessionJSONL(filePath)
		if err != nil {
			continue
		}

		if time.Since(createdAt) > time.Duration(retentionDays)*24*time.Hour {
			if err := os.Remove(filePath); err == nil {
				removed++
			}
		}
	}

	return removed, nil
}

type sessionMetadata struct {
	Type      string    `json:"_type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func readSessionJSONL(filePath string) (time.Time, []session.Message, error) {
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

		var meta sessionMetadata
		if err := json.Unmarshal([]byte(line), &meta); err == nil && meta.Type == "metadata" {
			if !meta.CreatedAt.IsZero() {
				createdAt = meta.CreatedAt
			}
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
		if info, err := os.Stat(filePath); err == nil {
			createdAt = info.ModTime()
		} else {
			createdAt = time.Now()
		}
	}

	return createdAt, messages, nil
}

func buildSessionMarkdown(sessionKey string, createdAt time.Time, messages []session.Message, jsonlPath string, redact bool) string {
	var sb strings.Builder

	sb.WriteString("# Session: ")
	sb.WriteString(sessionKey)
	sb.WriteString("\n")
	sb.WriteString("Date: ")
	sb.WriteString(createdAt.Format("2006-01-02 15:04:05"))
	sb.WriteString("\n\n---\n\n")

	for _, msg := range messages {
		sb.WriteString("## ")
		sb.WriteString(strings.Title(msg.Role))
		sb.WriteString("\n\n")

		turnID := buildTurnID(msg)
		sb.WriteString("<!-- session:")
		sb.WriteString(sessionKey)
		sb.WriteString(" turn:")
		sb.WriteString(turnID)
		sb.WriteString(" transcript:")
		sb.WriteString(jsonlPath)
		sb.WriteString(" -->\n")

		if !msg.Timestamp.IsZero() {
			sb.WriteString("*Time:* ")
			sb.WriteString(msg.Timestamp.Format("2006-01-02 15:04:05"))
			sb.WriteString("\n\n")
		}

		content := strings.TrimSpace(msg.Content)
		if redact {
			content = sanitizeText(content)
		}
		sb.WriteString(content)

		if len(msg.ToolCalls) > 0 {
			sb.WriteString("\n\nTools:\n")
			for _, tc := range msg.ToolCalls {
				sb.WriteString("- ")
				sb.WriteString(tc.Name)
				sb.WriteString("\n")
			}
		}

		sb.WriteString("\n\n")
	}

	return sb.String()
}

func buildTurnID(msg session.Message) string {
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

// sanitizeText applies optional redaction.
func sanitizeText(text string) string {
	text = redactAPIKeys(text)
	text = redactPasswords(text)
	text = redactEmails(text)
	text = redactPhoneNumbers(text)
	return text
}

var (
	apiKeyPattern   = regexp.MustCompile(`(?i)(api[_-]?key|apikey|secret|token)["\s:=]+([a-zA-Z0-9_\-]{16,})`)
	passwordPattern = regexp.MustCompile(`(?i)(password|passwd|pwd)["\s:=]+([^\s]+)`)
	emailPattern    = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phonePattern    = regexp.MustCompile(`1[3-9]\d{9}`)
)

func redactAPIKeys(text string) string {
	return apiKeyPattern.ReplaceAllString(text, `$1 [REDACTED_API_KEY]`)
}

func redactPasswords(text string) string {
	return passwordPattern.ReplaceAllString(text, `$1 [REDACTED_PASSWORD]`)
}

func redactEmails(text string) string {
	return emailPattern.ReplaceAllString(text, "[REDACTED_EMAIL]")
}

func redactPhoneNumbers(text string) string {
	return phonePattern.ReplaceAllString(text, "[REDACTED_PHONE]")
}
