package paginate

import (
	"encoding/base64"
	"fmt"
)

// Cursor encodes/decodes a cursor for cursor-based pagination.
type Cursor struct {
	After string
	Limit int
}

const defaultLimit = 50

func ParseCursor(after string, limit int) (*Cursor, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > 200 {
		limit = 200
	}
	c := &Cursor{Limit: limit}
	if after != "" {
		decoded, err := base64.StdEncoding.DecodeString(after)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		c.After = string(decoded)
	}
	return c, nil
}

func EncodeCursor(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}
