package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
)

// Connector indexes files from a local directory.
type Connector struct{}

func New() *Connector { return &Connector{} }

func (c *Connector) Fetch(ctx context.Context, cfg map[string]any) ([]connector.Document, error) {
	rootPath, _ := cfg["path"].(string)
	if rootPath == "" {
		return nil, fmt.Errorf("filesystem connector: path is required")
	}

	var docs []connector.Document
	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil // skip unreadable files
		}
		rel, _ := filepath.Rel(rootPath, path)
		docs = append(docs, connector.Document{
			Source:           "filesystem",
			SourceDocumentID: rel,
			Title:            d.Name(),
			URL:              path,
			Content:          string(data),
		})
		return nil
	})
	return docs, err
}
