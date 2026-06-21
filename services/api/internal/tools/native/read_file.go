package native

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

type ReadFileTool struct {
	storagePath string
}

func NewReadFileTool(storagePath string) *ReadFileTool {
	return &ReadFileTool{storagePath: storagePath}
}

func (t *ReadFileTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Relative file path within the storage directory"},
		},
		"required": []string{"path"},
	})
	return domain.Tool{
		Name:             "read_file",
		Description:      "Read the contents of a file from the storage directory",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *ReadFileTool) Execute(input map[string]any) (any, error) {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("read_file: path is required")
	}
	fullPath := filepath.Join(t.storagePath, path)
	if !strings.HasPrefix(filepath.Clean(fullPath)+string(filepath.Separator), filepath.Clean(t.storagePath)+string(filepath.Separator)) {
		return nil, fmt.Errorf("read_file: path escapes storage root")
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read_file: %w", err)
	}
	return map[string]any{"content": string(data)}, nil
}
