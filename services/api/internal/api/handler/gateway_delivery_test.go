package handler

import "testing"

func TestCompletionReply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		output          string
		sentToOtherPeer bool
		want            string
	}{
		{
			name:   "normal agent response is preserved",
			output: "Here is the answer you requested.",
			want:   "Here is the answer you requested.",
		},
		{
			name:            "cross-contact send receives concise confirmation",
			output:          "I sent Aayushi: hello",
			sentToOtherPeer: true,
			want:            "Message delivered.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := completionReply(tt.output, tt.sentToOtherPeer); got != tt.want {
				t.Fatalf("completionReply() = %q, want %q", got, tt.want)
			}
		})
	}
}
