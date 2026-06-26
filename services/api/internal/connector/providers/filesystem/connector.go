package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
)

// Connector indexes files from a local directory.
type Connector struct{}

func New() *Connector { return &Connector{} }

// FetchStream implements connector.StreamProvider.
// Filesystem walks are fast so checkpointing is by file path for consistency with other providers.
func (c *Connector) FetchStream(ctx context.Context, cfg map[string]any, opts connector.FetchOpts, emit func(connector.Document) error) error {
	rootPath, _ := cfg["path"].(string)
	if rootPath == "" {
		return fmt.Errorf("filesystem: path is required")
	}

	log := slog.With("connector", "filesystem", "root", rootPath)

	// Load resume checkpoint.
	var cp struct {
		LastPath string `json:"last_path"`
	}
	if len(opts.Checkpoint) > 2 {
		_ = json.Unmarshal(opts.Checkpoint, &cp)
	}
	resumeFrom := cp.LastPath
	if resumeFrom != "" {
		log.Info("resuming from checkpoint", "last_path", resumeFrom)
	}
	pastResume := resumeFrom == ""

	processed := 0
	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		rel, _ := filepath.Rel(rootPath, path)

		// Skip files until we reach the resume point.
		if !pastResume {
			if rel == resumeFrom {
				pastResume = true
			}
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			log.Warn("skipping unreadable file", "path", path, "error", readErr)
			return nil
		}

		if err := emit(connector.Document{
			Source:           "filesystem",
			SourceDocumentID: rel,
			Title:            d.Name(),
			URL:              path,
			Content:          string(data),
		}); err != nil {
			return err
		}

		processed++
		if opts.SaveCheckpoint != nil && processed%20 == 0 {
			opts.SaveCheckpoint(map[string]any{"last_path": rel})
		}
		return nil
	})

	if err == nil {
		log.Info("filesystem sync complete", "files_processed", processed)
	}
	return err
}

// Fetch implements connector.Provider via FetchStream.
func (c *Connector) Fetch(ctx context.Context, cfg map[string]any) ([]connector.Document, error) {
	var docs []connector.Document
	err := c.FetchStream(ctx, cfg, connector.FetchOpts{}, func(doc connector.Document) error {
		docs = append(docs, doc)
		return nil
	})
	return docs, err
}
