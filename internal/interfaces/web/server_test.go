package web

import (
	"errors"
	"strings"
	"testing"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/discovery/ports"
	"seo/internal/evaluation/domain/opportunity"
	"seo/internal/shared/kernel"
)

func timePoint(day int) time.Time {
	return time.Unix(1700000000+int64(day)*86400, 0)
}

func TestParseKey(t *testing.T) {
	key, ok := parseKey("ai agent|US|topic")
	if !ok {
		t.Fatal("valid key rejected")
	}
	if key.Term.String() != "ai agent" || key.Country != kernel.Country("US") || key.Entity != kernel.EntityTopic {
		t.Fatalf("key = %+v", key)
	}

	for _, raw := range []string{
		"",                        // empty
		"ai agent",                // one part
		"ai agent|US",             // two parts
		"ai agent|US|topic|extra", // four parts
		"|US|topic",               // empty term
		"ai agent||topic",         // empty country
		"ai agent|US|",            // empty entity
		"ai agent|NOT_A_COUNTRY|topic",
		"ai agent|US|NOT_AN_ENTITY",
	} {
		if _, ok := parseKey(raw); ok {
			t.Fatalf("parseKey(%q) accepted a malformed key", raw)
		}
	}
}

func TestSplitTerms(t *testing.T) {
	got := splitTerms(" ai agent\nbrowser agent\r\n, ;coding assistant,, ")
	want := []string{"ai agent", "browser agent", "coding assistant"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("splitTerms = %q, want %q", got, want)
	}
	if out := splitTerms("  \n , ; "); len(out) != 0 {
		t.Fatalf("splitTerms(blank) = %q, want none", out)
	}
}

func TestFriendlyErrorMapsDomainErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{"not validated", hotsearch.ErrNotValidated, "趋势"},
		{"too broad", hotsearch.ErrTooBroad, "宽泛"},
		{"depth", hotsearch.ErrDepthExceeded, "深度"},
		{"transition", hotsearch.ErrInvalidTransition, "状态"},
		{"not found", hotsearch.ErrNotFound, "找不到"},
		{"reason", hotsearch.ErrReasonRequired, "原因"},
		{"not eligible", opportunity.ErrNotEligible, "机会条件"},
		{"gate", opportunity.ErrGateFailed, "证据"},
		{"duplicate evidence", opportunity.ErrDuplicateEvidence, "已存在"},
		{"evidence empty", opportunity.ErrEvidenceEmpty, "证据"},
		{"note", opportunity.ErrNoteRequired, "拒绝原因"},
		{"closed", opportunity.ErrClosed, "最终决策"},
		{"rate limited", ports.ErrRateLimited, "429"},
		{"consent", ports.ErrConsentRequired, "同意"},
		{"captcha", ports.ErrCaptchaRequired, "人机验证"},
		{"unavailable", ports.ErrUnavailable, "代理"},
		{"unexpected", ports.ErrUnexpectedResponse, "无法识别"},
		{"wrapped keeps the mapping", fmtWrap(hotsearch.ErrTooBroad), "宽泛"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := friendlyError(tc.err); !strings.Contains(got, tc.contains) {
				t.Fatalf("friendlyError = %q, want it to contain %q", got, tc.contains)
			}
		})
	}

	t.Run("unknown error falls back to its text", func(t *testing.T) {
		err := errors.New("boom details")
		if got := friendlyError(err); got != "boom details" {
			t.Fatalf("friendlyError = %q, want the raw text", got)
		}
	})
}

func fmtWrap(err error) error {
	return hotsearchWrapped{err}
}

// hotsearchWrapped simulates fmt.Errorf("%w") around a domain sentinel.
type hotsearchWrapped struct{ inner error }

func (w hotsearchWrapped) Error() string { return "wrapped: " + w.inner.Error() }
func (w hotsearchWrapped) Unwrap() error { return w.inner }

func TestStateAndVerdictLabels(t *testing.T) {
	if got := stateLabel(hotsearch.StateDiscovered); got != "待验证" {
		t.Fatalf("stateLabel = %q", got)
	}
	if got := stateLabel(hotsearch.State("MYSTERY")); got != "MYSTERY" {
		t.Fatalf("unknown state must fall back to the code, got %q", got)
	}
	if got := reasonLabel(hotsearch.ReasonThinLongWindow); !strings.Contains(got, "采样点不足") {
		t.Fatalf("thin long window label missing: %q", got)
	}
	if got := reasonLabel("SOMETHING_NEW"); got != "SOMETHING_NEW" {
		t.Fatalf("unknown reason must fall back to the code, got %q", got)
	}
}

func TestDisplaySamplesKeepsShortSeries(t *testing.T) {
	obs := sampleObservation(t, 10)
	got := displaySamples(obs)
	if len(got) != 10 {
		t.Fatalf("len = %d, want the full 10-point series", len(got))
	}
}

func TestDisplaySamplesAveragesLongSeries(t *testing.T) {
	obs := sampleObservation(t, 48)
	got := displaySamples(obs)
	if len(got) != 24 {
		t.Fatalf("len = %d, want 24 bars", len(got))
	}
	// The source climbs 1..48, so no averaged bar may exceed the source max.
	for i, v := range got {
		if v.Int() > 48 {
			t.Fatalf("bar %d = %d exceeds source max", i, v.Int())
		}
	}
}

func sampleObservation(t *testing.T, n int) hotsearch.Observation {
	t.Helper()
	values := make([]int, n)
	for i := range values {
		values[i] = i + 1
	}
	obs, err := hotsearch.NewObservation(hotsearch.Window30D, timePoint(0), timePoint(1), values)
	if err != nil {
		t.Fatal(err)
	}
	return obs
}
