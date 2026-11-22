package main

import "testing"

func TestResolveRemoteTarget(t *testing.T) {
	always := func(string) bool { return true }
	never := func(string) bool { return false }

	cases := []struct {
		value    string
		probe    func(string) bool
		wantURL  string
		wantUsed bool
	}{
		{"", never, "", false},
		{"off", always, "", false},
		{"none", always, "", false},
		{"auto", always, DefaultRemoteURL, true},
		{"auto", never, "", false},
		{"http://127.0.0.1:9333", never, "http://127.0.0.1:9333", true},
	}
	for _, tc := range cases {
		url, used := resolveRemoteTarget(tc.value, tc.probe)
		if url != tc.wantURL || used != tc.wantUsed {
			t.Fatalf("resolveRemoteTarget(%q) = %q,%v want %q,%v", tc.value, url, used, tc.wantURL, tc.wantUsed)
		}
	}
}
