package handler

import (
	"strings"
	"testing"
)

func TestDistillRepoMap(t *testing.T) {
	summary := "Fixed the bug and pushed.\n\n### Repo Map\n- cmd/ entrypoint\n- internal/ business logic\n\n### Next Steps\nignore this"
	got := distillRepoMap(summary)
	if !strings.Contains(got, "cmd/ entrypoint") || !strings.Contains(got, "internal/ business logic") {
		t.Errorf("distilled map missing expected lines:\n%s", got)
	}
	if strings.Contains(got, "Next Steps") || strings.Contains(got, "ignore this") {
		t.Errorf("distilled map should stop before the next heading:\n%s", got)
	}

	if got := distillRepoMap("Fixed the bug, no map section here."); got != "" {
		t.Errorf("summary with no marker should distill to nothing, got %q", got)
	}
	if got := distillRepoMap("### Repo Map\n   \n"); got != "" {
		t.Errorf("blank map body should distill to nothing, got %q", got)
	}
}
