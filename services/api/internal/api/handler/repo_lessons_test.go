package handler

import (
	"strings"
	"testing"
)

func TestDistillReviewLessons(t *testing.T) {
	verdict := `Here is my review: {"verdict":"block","blocking_issues":[{"file":"a.go","issue":"error swallowed","suggestion":"log it"}],"non_blocking_notes":["prefer stdlib"],"pr_description_notes":"x"}`
	got := distillReviewLessons("PROJ-1", verdict)
	for _, want := range []string{"PROJ-1", "[a.go] error swallowed — log it", "- prefer stdlib"} {
		if !strings.Contains(got, want) {
			t.Errorf("distilled lessons missing %q:\n%s", want, got)
		}
	}

	if got := distillReviewLessons("PROJ-1", `{"verdict":"approve","blocking_issues":[],"non_blocking_notes":[]}`); got != "" {
		t.Errorf("empty verdict should distill to nothing, got %q", got)
	}
	if got := distillReviewLessons("PROJ-1", "not json at all"); got != "" {
		t.Errorf("non-JSON summary should distill to nothing, got %q", got)
	}
}

func TestAppendLessonsCap(t *testing.T) {
	if got := appendLessons("", "new block"); got != "new block" {
		t.Errorf("got %q", got)
	}
	if got := appendLessons("old", "new"); got != "new\n\nold" {
		t.Errorf("got %q", got)
	}
	old := strings.Repeat("- old line\n", 1000)
	got := appendLessons(old, "newest")
	if len(got) > maxRepoLessons {
		t.Errorf("cap not enforced: %d > %d", len(got), maxRepoLessons)
	}
	if !strings.HasPrefix(got, "newest") {
		t.Errorf("newest block must survive truncation")
	}
	if strings.HasSuffix(got, "- old") { // partial line
		t.Errorf("truncation left a dangling partial line")
	}
}
