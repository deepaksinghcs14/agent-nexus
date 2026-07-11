package handler

import "testing"

func TestParseOwnerNaturalCommand(t *testing.T) {
	cases := []struct {
		body, wantIntent, wantTarget string
		wantOK                       bool
	}{
		{"Enable auto reply to Rajat", "start_contact", "Rajat", true},
		{"auto reply to Rajat Chahar", "start_contact", "Rajat Chahar", true},
		{"Disable auto reply to Rajat", "stop_contact", "Rajat", true},
		{"turn off auto reply to Rajat", "stop_contact", "Rajat", true},
		{"stop auto-reply to Rajat", "stop_contact", "Rajat", true},
		{"stop replying to Mom", "stop_contact", "Mom", true},
		{"start replying to Mom on whatsapp", "start_contact", "Mom", true},
		{"enable bot mode", "enable_bot_mode", "", true},
		{"stop assistant", "stop_assistant", "", true},
		{"how is the weather today", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.body, func(t *testing.T) {
			intent, target, ok := parseOwnerNaturalCommand(c.body)
			if ok != c.wantOK || intent != c.wantIntent || target != c.wantTarget {
				t.Errorf("parseOwnerNaturalCommand(%q) = (%q, %q, %v), want (%q, %q, %v)",
					c.body, intent, target, ok, c.wantIntent, c.wantTarget, c.wantOK)
			}
		})
	}
}
