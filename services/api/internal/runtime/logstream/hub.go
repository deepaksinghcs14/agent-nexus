package logstream

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"
)

const SourceAPI = "api"

type Entry struct {
	Time    time.Time      `json:"ts"`
	Source  string         `json:"source"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

type Hub struct {
	mu          sync.Mutex
	subscribers map[chan Entry]struct{}
}

func NewHub() *Hub {
	return &Hub{subscribers: map[chan Entry]struct{}{}}
}

func (h *Hub) Subscribe() chan Entry {
	ch := make(chan Entry, 256)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan Entry) {
	h.mu.Lock()
	if _, ok := h.subscribers[ch]; ok {
		delete(h.subscribers, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *Hub) Publish(entry Entry) {
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	if entry.Source == "" {
		entry.Source = SourceAPI
	}
	if entry.Level == "" {
		entry.Level = "info"
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
}

type Handler struct {
	next   slog.Handler
	hub    *Hub
	source string
}

func NewHandler(hub *Hub, level slog.Leveler, source string) slog.Handler {
	return &Handler{
		next:   slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}),
		hub:    hub,
		source: source,
	}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	err := h.next.Handle(ctx, record)
	if h.hub != nil {
		attrs := map[string]any{}
		record.Attrs(func(attr slog.Attr) bool {
			attrs[attr.Key] = attrValue(attr.Value)
			return true
		})
		h.hub.Publish(Entry{
			Time:    record.Time.UTC(),
			Source:  h.source,
			Level:   record.Level.String(),
			Message: record.Message,
			Attrs:   attrs,
		})
	}
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{next: h.next.WithAttrs(attrs), hub: h.hub, source: h.source}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name), hub: h.hub, source: h.source}
}

func attrValue(value slog.Value) any {
	switch value.Kind() {
	case slog.KindAny:
		return value.Any()
	case slog.KindBool:
		return value.Bool()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindString:
		return value.String()
	case slog.KindTime:
		return value.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindGroup:
		out := map[string]any{}
		for _, attr := range value.Group() {
			out[attr.Key] = attrValue(attr.Value)
		}
		return out
	case slog.KindLogValuer:
		return attrValue(value.Resolve())
	default:
		b, err := json.Marshal(value.Any())
		if err != nil {
			return value.String()
		}
		return json.RawMessage(b)
	}
}
