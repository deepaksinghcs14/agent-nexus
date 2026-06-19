package memory

import "testing"

func TestParseMemoryCandidatesBlankOutput(t *testing.T) {
	candidates, err := parseMemoryCandidates("   ")
	if err != nil {
		t.Fatalf("parseMemoryCandidates returned error: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %d", len(candidates))
	}
}

func TestParseMemoryCandidatesFencedObject(t *testing.T) {
	raw := "```json\n{\"memories\":[{\"content\":\"prefers concise updates\",\"importance_score\":0.9,\"reason\":\"stable preference\"}]}\n```"
	candidates, err := parseMemoryCandidates(raw)
	if err != nil {
		t.Fatalf("parseMemoryCandidates returned error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Content != "prefers concise updates" {
		t.Fatalf("unexpected candidate content: %q", candidates[0].Content)
	}
}

func TestParseMemoryCandidatesWrappedObject(t *testing.T) {
	raw := "Here is the JSON:\n{\"memories\":[{\"content\":\"uses Agent Nexus\",\"importance_score\":0.8,\"reason\":\"product context\"}]}"
	candidates, err := parseMemoryCandidates(raw)
	if err != nil {
		t.Fatalf("parseMemoryCandidates returned error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}

func TestParseMemoryCandidatesArray(t *testing.T) {
	raw := "[{\"content\":\"likes board-ready recommendations\",\"importance_score\":0.86,\"reason\":\"response preference\"}]"
	candidates, err := parseMemoryCandidates(raw)
	if err != nil {
		t.Fatalf("parseMemoryCandidates returned error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}

func TestParseMemoryCandidatesInvalidNonEmptyOutput(t *testing.T) {
	if _, err := parseMemoryCandidates("not json"); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}
