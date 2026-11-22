package kernel

import "testing"

func TestNewTermNormalizes(t *testing.T) {
	tests := map[string]string{
		"  AI   Agent ":  "ai agent",
		"Browser\tAgent": "browser agent",
		"AI Agent":       "ai agent",
		"  AI Agent  ":   "ai agent",
	}
	for in, want := range tests {
		got, err := NewTerm(in)
		if err != nil {
			t.Fatalf("NewTerm(%q): %v", in, err)
		}
		if string(got) != want {
			t.Fatalf("NewTerm(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := NewTerm("   "); err == nil {
		t.Fatal("blank term should be rejected")
	}
}

func TestCountryUpper(t *testing.T) {
	got, err := NewCountry(" us ")
	if err != nil || got != "US" {
		t.Fatalf("NewCountry = %q, %v", got, err)
	}
	if _, err := NewCountry(""); err == nil {
		t.Fatal("empty country should be rejected")
	}
}

func TestChooseEntityTypePrefersTopicBR08(t *testing.T) {
	if got := ChooseEntityType(true); got != EntityTopic {
		t.Fatalf("topic available: got %s", got)
	}
	if got := ChooseEntityType(false); got != EntitySearchTerm {
		t.Fatalf("topic unavailable: got %s", got)
	}
}

func TestParseEntityType(t *testing.T) {
	for _, raw := range []string{"topic", " TOPIC "} {
		got, err := ParseEntityType(raw)
		if err != nil || got != EntityTopic {
			t.Fatalf("ParseEntityType(%q) = %s, %v", raw, got, err)
		}
	}
	if _, err := ParseEntityType("nope"); err == nil {
		t.Fatal("unknown entity type should be rejected")
	}
}
