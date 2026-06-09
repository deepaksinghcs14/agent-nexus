package connector

import (
	"context"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

// Document is the normalised output from any connector provider.
type Document struct {
	Source           string
	SourceDocumentID string
	Title            string
	URL              string
	Author           string
	Content          string
	Metadata         map[string]any
}

// Provider fetches documents from an external source.
type Provider interface {
	Fetch(ctx context.Context, cfg map[string]any) ([]Document, error)
}

// Connector wraps domain.Connector with its resolved provider.
type Connector struct {
	Meta     domain.Connector
	Provider Provider
}
