package anthropic

import (
	"encoding/base64"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

// A user message carrying an inline image must produce a text block plus an
// image block with the raw bytes base64-encoded — the wire format the
// Anthropic Messages API requires.
func TestBuildParamsUserImageBlock(t *testing.T) {
	req := provider.CompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []provider.Message{
			{Role: "user", Content: "what is this?", Images: []provider.MediaBlock{
				{MIMEType: "image/jpeg", Data: []byte("fake-jpeg-bytes")},
			}},
		},
	}
	params := buildParams(req)

	if len(params.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(params.Messages))
	}
	blocks := params.Messages[0].Content
	if len(blocks) != 2 {
		t.Fatalf("content blocks = %d, want 2 (text + image): %+v", len(blocks), blocks)
	}
	if blocks[0].OfText == nil || blocks[0].OfText.Text != "what is this?" {
		t.Fatalf("first block not the text block: %+v", blocks[0])
	}
	if blocks[1].OfImage == nil {
		t.Fatalf("second block not an image block: %+v", blocks[1])
	}
	wantData := base64.StdEncoding.EncodeToString([]byte("fake-jpeg-bytes"))
	gotData := blocks[1].OfImage.Source.GetData()
	if gotData == nil || *gotData != wantData {
		t.Fatalf("image data = %v, want %q", gotData, wantData)
	}
	gotMediaType := blocks[1].OfImage.Source.GetMediaType()
	if gotMediaType == nil || *gotMediaType != "image/jpeg" {
		t.Fatalf("image media type = %v, want image/jpeg", gotMediaType)
	}
}

// A user message with no images must keep producing a single text block —
// the image-block path must not change plain-text behavior.
func TestBuildParamsUserTextOnlyUnaffected(t *testing.T) {
	req := provider.CompletionRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
	}
	params := buildParams(req)
	if len(params.Messages) != 1 || len(params.Messages[0].Content) != 1 {
		t.Fatalf("expected exactly one text block, got: %+v", params.Messages)
	}
	if params.Messages[0].Content[0].OfImage != nil {
		t.Fatal("no image should be present when Images is empty")
	}
}
