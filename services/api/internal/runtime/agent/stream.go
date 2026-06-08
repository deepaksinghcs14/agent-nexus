package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// SSEEvent is a single server-sent event payload.
type SSEEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

// Emitter writes SSE events to an http.ResponseWriter.
type Emitter struct {
	w http.ResponseWriter
}

func NewEmitter(w http.ResponseWriter) *Emitter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return &Emitter{w: w}
}

func (e *Emitter) Send(event SSEEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("sse emitter: marshal: %w", err)
	}
	_, err = fmt.Fprintf(e.w, "data: %s\n\n", data)
	if f, ok := e.w.(http.Flusher); ok {
		f.Flush()
	}
	return err
}
