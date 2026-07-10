package handler

import "testing"

func TestRenderWorkflowTemplate(t *testing.T) {
	sessionOut := `{"status":"success","branch":"nexus/WF-1","summary":"did the thing","cost_usd":1.2}`

	cases := []struct {
		name, tpl, input, original, want string
	}{
		{"plain input", "{{input}}", "hello", "orig", "hello"},
		{"original input", "{{original_input}}", "hello", "orig", "orig"},
		{"dotted path string leaf", "{{input.branch}}", sessionOut, "", "nexus/WF-1"},
		{"dotted path non-string leaf", "{{input.cost_usd}}", sessionOut, "", "1.2"},
		{"missing path renders empty", "{{input.nope}}", sessionOut, "", ""},
		{"non-JSON input path renders empty", "{{input.branch}}", "not json", "", ""},
		{"dotted path on original_input", "{{original_input.branch}}", "x", sessionOut, "nexus/WF-1"},
		{"mixed", "head={{input.branch}} all={{input}}", sessionOut, "", "head=nexus/WF-1 all=" + sessionOut},
		{"nested path", "{{input.a.b}}", `{"a":{"b":"deep"}}`, "", "deep"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := renderWorkflowTemplate(c.tpl, c.input, c.original); got != c.want {
				t.Errorf("renderWorkflowTemplate(%q) = %q, want %q", c.tpl, got, c.want)
			}
		})
	}
}
