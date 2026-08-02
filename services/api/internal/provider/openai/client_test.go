package openai

import (
	"strings"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

// A user message carrying an inline image must produce a content-part array
// (text part + image_url part with a base64 data: URL) instead of the plain
// string content OpenAI's chat completions API takes for text-only turns.
func TestBuildParamsUserImagePart(t *testing.T) {
	req := provider.CompletionRequest{
		Model: "gpt-4o",
		Messages: []provider.Message{
			{Role: "user", Content: "what is this?", Images: []provider.MediaBlock{
				{MIMEType: "image/png", Data: []byte("fake-png-bytes")},
			}},
		},
	}
	params := buildParams(req)

	if len(params.Messages) != 1 || params.Messages[0].OfUser == nil {
		t.Fatalf("expected one user message: %+v", params.Messages)
	}
	parts := params.Messages[0].OfUser.Content.OfArrayOfContentParts
	if len(parts) != 2 {
		t.Fatalf("content parts = %d, want 2 (text + image): %+v", len(parts), parts)
	}
	if parts[0].OfText == nil || parts[0].OfText.Text != "what is this?" {
		t.Fatalf("first part not the text part: %+v", parts[0])
	}
	if parts[1].OfImageURL == nil {
		t.Fatalf("second part not an image_url part: %+v", parts[1])
	}
	url := parts[1].OfImageURL.ImageURL.URL
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("image url = %q, want a data: URL with the mime type", url)
	}
}

// A user message with no images must keep sending plain string content —
// the image-part path must not change plain-text behavior.
func TestBuildParamsUserTextOnlyUnaffected(t *testing.T) {
	req := provider.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
	}
	params := buildParams(req)
	if len(params.Messages) != 1 || params.Messages[0].OfUser == nil {
		t.Fatalf("expected one user message: %+v", params.Messages)
	}
	content := params.Messages[0].OfUser.Content
	if content.OfString.Value != "hello" {
		t.Fatalf("content string = %q, want hello", content.OfString.Value)
	}
	if len(content.OfArrayOfContentParts) != 0 {
		t.Fatal("no content parts should be built when Images is empty")
	}
}
