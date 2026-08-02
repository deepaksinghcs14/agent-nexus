package handler

// WhatsApp media ingestion: an inbound image/voice note is attached to the
// LLM request for the triggering turn only. It must never round-trip back
// out of conversation history — the DB row for that turn only ever holds
// the placeholder/caption text (no media column), so a later turn reloading
// the same row via the history query gets text-only again.

import (
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/google/uuid"
)

func TestLoopMediaAttachedToTriggeringMessageOnly(t *testing.T) {
	fx := newLoopFixture(t, []fakeTurn{
		{deltas: []string{"I see an image"}},
		{deltas: []string{"no image this time"}},
	}, "")

	img := provider.MediaBlock{MIMEType: "image/jpeg", Data: []byte("fake-jpeg-bytes")}
	fx.run(invokeOpts{MediaImages: []provider.MediaBlock{img}})

	reqs := fx.fake.recorded()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	last := lastUserMessage(t, reqs[0].Messages)
	if len(last.Images) != 1 || string(last.Images[0].Data) != "fake-jpeg-bytes" || last.Images[0].MIMEType != "image/jpeg" {
		t.Fatalf("triggering message missing the image: %+v", last)
	}

	// Second turn, same conversation, no media on this call: the fixture's
	// pre-inserted-message pattern means executeRun reloads history from the
	// DB, which must NOT resurrect the first turn's image bytes.
	fx.runID = newRunID(t, fx)
	fx.run(invokeOpts{})

	reqs = fx.fake.recorded()
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	for i, m := range reqs[1].Messages {
		if len(m.Images) != 0 || len(m.Audio) != 0 {
			t.Fatalf("second turn's request replayed media on message %d (role %s): %+v", i, m.Role, m)
		}
	}
}

func TestDecodeInboundMedia(t *testing.T) {
	items := []inboundMediaItem{
		{Type: "image", MimeType: "image/jpeg", DataBase64: "aGVsbG8="}, // "hello"
		{Type: "audio", MimeType: "audio/ogg", DataBase64: "d29ybGQ="},  // "world"
		{Type: "image", MimeType: "image/png", DataBase64: "not-valid-base64!!"},
		{Type: "document", MimeType: "application/pdf", DataBase64: "aGVsbG8="},
	}
	images, audio := decodeInboundMedia(items)
	if len(images) != 1 || string(images[0].Data) != "hello" || images[0].MIMEType != "image/jpeg" {
		t.Fatalf("images = %+v, want one valid jpeg block", images)
	}
	if len(audio) != 1 || string(audio[0].Data) != "world" || audio[0].MIMEType != "audio/ogg" {
		t.Fatalf("audio = %+v, want one valid ogg block", audio)
	}
}

func lastUserMessage(t *testing.T, msgs []provider.Message) provider.Message {
	t.Helper()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i]
		}
	}
	t.Fatal("no user message found")
	return provider.Message{}
}

// newRunID inserts a fresh 'pending'->'running' run row on the fixture's
// existing conversation and a matching user-role message, mirroring what
// dispatchGatewayRun does for each new inbound turn, then points the
// fixture at it so a second fx.run() call exercises a genuinely new turn.
func newRunID(t *testing.T, fx *loopFixture) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := fx.pool.Exec(t.Context(),
		`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user','test input')`,
		uuid.NewString(), fx.convID); err != nil {
		t.Fatalf("insert second-turn message: %v", err)
	}
	if _, err := fx.pool.Exec(t.Context(),
		`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,'test input','running')`,
		id, fx.ws, fx.agent.ID, fx.convID, fx.uid); err != nil {
		t.Fatalf("insert second run: %v", err)
	}
	return id
}
