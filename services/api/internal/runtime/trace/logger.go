package trace

import (
	"context"
	"fmt"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
)

// Logger writes RunStep records to the database.
type Logger struct{}

func NewLogger() *Logger { return &Logger{} }

func (l *Logger) Log(ctx context.Context, step *domain.RunStep) error {
	return fmt.Errorf("trace logger: Log not implemented")
}
