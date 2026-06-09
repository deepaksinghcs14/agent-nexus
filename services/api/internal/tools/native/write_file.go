package native

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

type WriteFileTool struct {
	storagePath string
}

func NewWriteFileTool(storagePath string) *WriteFileTool {
	return &WriteFileTool{storagePath: storagePath}
}

func (t *WriteFileTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Relative file path within the storage directory"},
			"content": map[string]any{"type": "string", "description": "Content to write to the file"},
		},
		"required": []string{"path", "content"},
	})
	return domain.Tool{
		Name:             "write_file",
		Description:      "Write content to a file in the storage directory",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		RiskLevel:        "high",
		RequiresApproval: true,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *WriteFileTool) Execute(input map[string]any) (any, error) {
	path, _ := input["path"].(string)
	content, _ := input["content"].(string)
	if path == "" {
		return nil, fmt.Errorf("write_file: path is required")
	}
	fullPath := filepath.Join(t.storagePath, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
		return nil, fmt.Errorf("write_file: mkdir: %w", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0640); err != nil {
		return nil, fmt.Errorf("write_file: %w", err)
	}
	return map[string]any{"written": true, "path": path}, nil
}
