package handler

import "testing"

func TestFabricatesActionLog(t *testing.T) {
	cases := []struct {
		reply string
		want  bool
	}{
		{`[actions taken: native_launch_repo_session({"repo":"x/y"})]` + "\nDone, PR opened!", true},
		{"I did the work.\n[actions taken: native_create_pull_request(...)]", true},
		{"The pull request is open at https://github.com/x/y/pull/1", false},
		{"I list past actions taken by the team below.", false},
		{"", false},
	}
	for _, c := range cases {
		if got := fabricatesActionLog(c.reply); got != c.want {
			t.Errorf("fabricatesActionLog(%q) = %v, want %v", c.reply, got, c.want)
		}
	}
}
