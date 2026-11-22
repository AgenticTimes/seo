package hotsearch

import "testing"

func TestSuggestTooBroad(t *testing.T) {
	dispersedAI := []string{
		"ai coding agent", "browser agent", "computer use",
		"personal ai assistant", "agent memory", "ai agent framework",
	}
	focused := []string{
		"browser automation", "web agent", "ai web research",
		"browser use api", "website interaction automation",
	}

	cases := []struct {
		name     string
		term     string
		children []string
		suggest  bool
		reasons  []string
	}{
		{"single token with a shared theme", "ai", dispersedAI, true, []string{BroadReasonSingleToken}},
		{"single token and dispersed", "ai", []string{
			"coding tool", "browser automation", "computer vision",
			"personal assistant", "memory service", "framework docs",
		}, true, []string{BroadReasonSingleToken, BroadReasonDispersed}},
		{"multi token but dispersed", "ai agent", dispersedAI, true, []string{BroadReasonDispersed}},
		{"focused children", "browser agent", focused, false, nil},
		{"too few children", "agent memory", dispersedAI[:4], false, nil},
		{"single token but few children", "openhands", []string{"openhands ai", "openhands github"}, false, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			suggest, reasons := SuggestTooBroad(tc.term, tc.children)
			if suggest != tc.suggest {
				t.Fatalf("suggest = %v, want %v (reasons %v)", suggest, tc.suggest, reasons)
			}
			if len(reasons) != len(tc.reasons) {
				t.Fatalf("reasons = %v, want %v", reasons, tc.reasons)
			}
			for i := range reasons {
				if reasons[i] != tc.reasons[i] {
					t.Fatalf("reasons = %v, want %v", reasons, tc.reasons)
				}
			}
		})
	}
}
