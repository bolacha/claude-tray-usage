package parser

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UsageEntry represents a single assistant message usage record.
type UsageEntry struct {
	Project   string
	SessionID string
	Model     string
	Timestamp time.Time

	InputTokens            int64
	OutputTokens           int64
	CacheCreationTokens    int64
	CacheReadTokens        int64
	WebSearchRequests      int64
	WebFetchRequests       int64
}

type jsonlMessage struct {
	Type      string    `json:"type"`
	Timestamp string    `json:"timestamp"`
	Message   *msgBody  `json:"message,omitempty"`
}

type msgBody struct {
	Model string   `json:"model"`
	Role  string   `json:"role"`
	Usage *msgUsage `json:"usage,omitempty"`
}

type msgUsage struct {
	InputTokens             int64 `json:"input_tokens"`
	OutputTokens            int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens    int64 `json:"cache_read_input_tokens"`
	ServerToolUse           *struct {
		WebSearchRequests int64 `json:"web_search_requests"`
		WebFetchRequests  int64 `json:"web_fetch_requests"`
	} `json:"server_tool_use,omitempty"`
}

// ClaudeDataDir returns ~/.claude path.
func ClaudeDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// ParseAll walks ~/.claude/projects and parses all session JSONL files.
func ParseAll() ([]UsageEntry, error) {
	projectsDir := filepath.Join(ClaudeDataDir(), "projects")
	var entries []UsageEntry

	err := filepath.WalkDir(projectsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Derive project name from parent directory name.
		parentDir := filepath.Base(filepath.Dir(path))
		projectName := dirNameToProject(parentDir)
		sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

		fileEntries, err := parseFile(path, projectName, sessionID)
		if err != nil {
			return nil // skip broken files
		}
		entries = append(entries, fileEntries...)
		return nil
	})
	return entries, err
}

func dirNameToProject(dir string) string {
	// Claude stores project dirs as URL-encoded paths like -Users-foo-dev-myproject
	name := strings.TrimPrefix(dir, "-")
	parts := strings.Split(name, "-")
	if len(parts) == 0 {
		return dir
	}
	return parts[len(parts)-1]
}

func parseFile(path, project, sessionID string) ([]UsageEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []UsageEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var msg jsonlMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Type != "assistant" || msg.Message == nil || msg.Message.Usage == nil {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, msg.Timestamp)
		u := msg.Message.Usage

		entry := UsageEntry{
			Project:             project,
			SessionID:           sessionID,
			Model:               msg.Message.Model,
			Timestamp:           ts,
			InputTokens:         u.InputTokens,
			OutputTokens:        u.OutputTokens,
			CacheCreationTokens: u.CacheCreationInputTokens,
			CacheReadTokens:     u.CacheReadInputTokens,
		}
		if u.ServerToolUse != nil {
			entry.WebSearchRequests = u.ServerToolUse.WebSearchRequests
			entry.WebFetchRequests = u.ServerToolUse.WebFetchRequests
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}
