package trends

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/discovery/ports"
	"seo/internal/shared/kernel"
)

func TestTrendTime(t *testing.T) {
	want := map[hotsearch.Window]string{
		hotsearch.Window7D:  "now 7-d",
		hotsearch.Window30D: "today 1-m",
		hotsearch.Window90D: "today 3-m",
		hotsearch.Window12M: "today 12-m",
		hotsearch.Window5Y:  "today 5-y",
	}
	for window, expected := range want {
		if got := trendTime(window); got != expected {
			t.Fatalf("%s -> %q, want %q", window, got, expected)
		}
	}
}

func TestGeoFor(t *testing.T) {
	if got := geoFor(kernel.CountryWorldwide); got != "" {
		t.Fatalf("worldwide = %q, want empty", got)
	}
	if got := geoFor("us"); got != "US" {
		t.Fatalf("us = %q, want US", got)
	}
}

func TestClassifyLanding(t *testing.T) {
	base := "https://trends.google.com"
	if err := classifyLanding(base, "https://trends.google.com/trends/explore?q=x"); err != nil {
		t.Fatalf("same host: %v", err)
	}
	if err := classifyLanding("http://127.0.0.1:9999", "http://127.0.0.1:9999/trends/explore"); err != nil {
		t.Fatalf("mock host: %v", err)
	}
	err := classifyLanding(base, "https://www.google.com/sorry/index?continue=x")
	if !errors.Is(err, ports.ErrCaptchaRequired) {
		t.Fatalf("captcha err = %v", err)
	}
	err = classifyLanding(base, "https://consent.google.com/ml?continue=x")
	if !errors.Is(err, ports.ErrConsentRequired) {
		t.Fatalf("consent err = %v", err)
	}
}

func TestClassifyRelatedBR02(t *testing.T) {
	cases := []struct {
		formatted string
		value     float64
		source    hotsearch.Source
		breakout  bool
	}{
		{"Breakout", 4500, hotsearch.SourceBreakout, true},
		{"+180%", 180, hotsearch.SourceRising, false},
		{"", 5000, hotsearch.SourceBreakout, true},
		{"100", 100, hotsearch.SourceTop, false},
		{"", 250, hotsearch.SourceRising, false},
	}
	for _, tc := range cases {
		source, _, breakout := classifyRelated(tc.formatted, tc.value)
		if source != tc.source || breakout != tc.breakout {
			t.Fatalf("classifyRelated(%q,%v) = %s/%v, want %s/%v",
				tc.formatted, tc.value, source, breakout, tc.source, tc.breakout)
		}
	}
}

func TestToRelatedResultKeepsSources(t *testing.T) {
	payload := browserPayload{}
	add := func(q string, v float64, f string) {
		payload.Related = append(payload.Related, struct {
			Q string  `json:"q"`
			V float64 `json:"v"`
			F string  `json:"f"`
		}{Q: q, V: v, F: f})
	}
	// Rising first: the provider does not guarantee list order.
	add("agent memory", 4500, "Breakout")
	add("browser agent", 180, "+180%")
	add("ai agent framework", 100, "100")
	add("", 10, "+10%")
	add("browser agent", 100, "100") // duplicate weaker top entry must not downgrade

	result := toRelatedResult(payload)
	if len(result.Queries) != 3 {
		t.Fatalf("queries = %d, want 3", len(result.Queries))
	}
	byTerm := map[string]int{}
	for i, q := range result.Queries {
		byTerm[q.Term.String()] = i
	}
	breakout := result.Queries[byTerm["agent memory"]]
	if !breakout.Breakout || breakout.Source != hotsearch.SourceBreakout || breakout.RisePct != 5000 {
		t.Fatalf("breakout = %+v", breakout)
	}
	rising := result.Queries[byTerm["browser agent"]]
	if rising.Source != hotsearch.SourceRising || rising.RisePct != 180 {
		t.Fatalf("rising must survive duplicate top entry: %+v", rising)
	}
	top := result.Queries[byTerm["ai agent framework"]]
	if top.Source != hotsearch.SourceTop {
		t.Fatalf("top = %+v", top)
	}
}

func TestMergeSeriesMedian(t *testing.T) {
	runs := []ports.InterestSeries{
		{Window: hotsearch.Window30D, Values: []int{10, 20, 30}},
		{Window: hotsearch.Window30D, Values: []int{12, 18, 33}},
		{Window: hotsearch.Window30D, Values: []int{11, 25, 30}},
	}
	got, err := mergeSeries(runs)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{11, 20, 30}
	if got.Window != hotsearch.Window30D {
		t.Fatalf("window = %s", got.Window)
	}
	for i := range want {
		if got.Values[i] != want[i] {
			t.Fatalf("median = %v, want %v", got.Values, want)
		}
	}
	one, err := mergeSeries(runs[:1])
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Values) != 3 {
		t.Fatalf("single run changed: %+v", one)
	}
}

// TestMergeSeriesRejectsShapeMismatch pins the guard: runs sampled at
// different granularities cannot be merged position by position, so the
// mismatch must surface as an error instead of a silently truncated series.
func TestMergeSeriesRejectsShapeMismatch(t *testing.T) {
	runs := []ports.InterestSeries{
		{Window: hotsearch.Window30D, Values: []int{10, 20, 30}},
		{Window: hotsearch.Window30D, Values: []int{10, 20}},
	}
	if _, err := mergeSeries(runs); err == nil {
		t.Fatal("expected shape mismatch error")
	}
}

func TestDownsamplePreservesShape(t *testing.T) {
	values := make([]int, 120)
	for i := range values {
		values[i] = i
	}
	out := downsample(values, 24)
	if len(out) != 24 {
		t.Fatalf("len = %d, want 24", len(out))
	}
	if out[0] > 10 {
		t.Fatalf("first bucket = %d, want low", out[0])
	}
	if out[len(out)-1] < 100 {
		t.Fatalf("last bucket = %d, want high", out[len(out)-1])
	}
	short := downsample([]int{10, 20}, 24)
	if len(short) != 2 || short[0] != 10 || short[1] != 20 {
		t.Fatalf("short series changed: %v", short)
	}
}

func TestToInterestSeries(t *testing.T) {
	payload := browserPayload{}
	for i, v := range []float64{10, 18.4, 27, 36} {
		payload.Series = append(payload.Series, struct {
			T int64   `json:"t"`
			V float64 `json:"v"`
		}{T: int64(1700000000 + i*86400), V: v})
	}
	series, err := toInterestSeries(hotsearch.Window30D, payload, 24)
	if err != nil {
		t.Fatal(err)
	}
	if series.Window != hotsearch.Window30D {
		t.Fatalf("window = %s", series.Window)
	}
	if len(series.Values) != 4 || series.Values[1] != 18 {
		t.Fatalf("values = %v", series.Values)
	}
	if !series.Start.Before(series.End) {
		t.Fatalf("start %v end %v", series.Start, series.End)
	}

	tooShort := browserPayload{}
	tooShort.Series = append(tooShort.Series, payload.Series[0])
	if _, err := toInterestSeries(hotsearch.Window30D, tooShort, 24); err == nil {
		t.Fatal("single point should be rejected")
	}
}

// mockTrends serves the internal endpoint shapes so the browser path can be
// tested without Google. The payloads mirror the real response structure,
// including the `)]}'` anti-JSON-hijack prefix.
func mockTrends(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	page := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><h1>mock trends</h1></body></html>")
	}
	mux.HandleFunc("/trends/explore", page)
	mux.HandleFunc("/trends/api/explore", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("req") == "" {
			http.Error(w, "missing req", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `)]}'`+"\n"+`{"widgets":[
			{"id":"TIMESERIES","token":"ts-token","request":{"time":"today 12-m","maxSearches":5}},
			{"id":"RELATED_QUERIES","token":"rq-token","request":{"time":"today 12-m","maxSearches":5}}
		]}`)
	})
	mux.HandleFunc("/trends/api/widgetdata/multiline", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "ts-token" {
			http.Error(w, "bad token", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `)]}'`+"\n"+`{"default":{"timelineData":[
			{"time":"1700000000","value":[10]},
			{"time":"1700086400","value":[18]},
			{"time":"1700172800","value":[27]},
			{"time":"1700259200","value":[36]}
		]}}`)
	})
	mux.HandleFunc("/trends/api/widgetdata/relatedsearches", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "rq-token" {
			http.Error(w, "bad token", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `)]}',`+"\n"+`{"default":{"rankedList":[
			{"rankedKeyword":[
				{"query":"agent memory","value":4500,"formattedValue":"Breakout"},
				{"query":"browser agent","value":180,"formattedValue":"+180%"}
			]},
			{"rankedKeyword":[{"query":"ai agent framework","value":100,"formattedValue":"100"}]}
		]}}`)
	})
	return httptest.NewServer(mux)
}

func TestBrowserGatewayAgainstMock(t *testing.T) {
	if testing.Short() {
		t.Skip("browser test skipped in -short mode")
	}
	chrome := DefaultChromePath()
	if chrome == "" {
		t.Skip("chrome not found; set CHROME_PATH")
	}

	server := mockTrends(t)
	defer server.Close()

	gateway, err := NewBrowser(BrowserOptions{
		ChromePath:  chrome,
		Headless:    true,
		BaseURL:     server.URL,
		Timeout:     30 * time.Second,
		MinInterval: 0,
	})
	if err != nil {
		t.Fatalf("new browser: %v", err)
	}
	defer func() { _ = gateway.Close() }()

	ctx := context.Background()
	related, err := gateway.RelatedQueries(ctx, ports.RelatedRequest{
		Term:    mustTerm(t, "ai agent"),
		Country: kernel.CountryWorldwide,
		Entity:  kernel.EntityTopic,
	})
	if err != nil {
		t.Fatalf("related queries: %v", err)
	}
	if len(related.Queries) != 3 {
		t.Fatalf("related = %+v", related.Queries)
	}
	byTerm := map[string]ports.DiscoveredTerm{}
	for _, q := range related.Queries {
		byTerm[q.Term.String()] = q
	}
	if byTerm["ai agent framework"].Source != hotsearch.SourceTop {
		t.Fatalf("top misclassified: %+v", byTerm["ai agent framework"])
	}
	if !byTerm["agent memory"].Breakout {
		t.Fatalf("breakout not flagged: %+v", byTerm["agent memory"])
	}
	if byTerm["browser agent"].Source != hotsearch.SourceRising {
		t.Fatalf("rising misclassified: %+v", byTerm["browser agent"])
	}

	series, err := gateway.InterestOverTime(ctx, ports.InterestRequest{
		Term:    mustTerm(t, "ai agent"),
		Country: kernel.CountryWorldwide,
		Entity:  kernel.EntityTopic,
		Window:  hotsearch.Window30D,
	})
	if err != nil {
		t.Fatalf("interest over time: %v", err)
	}
	if len(series.Values) != 4 || series.Values[0] != 10 || series.Values[3] != 36 {
		t.Fatalf("series = %v", series.Values)
	}
}

// TestBrowserGatewayRemoteMock proves the attach path works against a Chrome
// started by the operator. Set CHROME_REMOTE to its debug URL.
func TestBrowserGatewayRemoteMock(t *testing.T) {
	remoteURL := os.Getenv("CHROME_REMOTE")
	if remoteURL == "" {
		t.Skip("set CHROME_REMOTE=http://127.0.0.1:9222")
	}
	server := mockTrends(t)
	defer server.Close()

	gateway, err := NewBrowser(BrowserOptions{
		RemoteURL:   remoteURL,
		BaseURL:     server.URL,
		Timeout:     30 * time.Second,
		MinInterval: 0,
	})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	defer func() { _ = gateway.Close() }()

	related, err := gateway.RelatedQueries(context.Background(), ports.RelatedRequest{
		Term:    mustTerm(t, "ai agent"),
		Country: kernel.CountryWorldwide,
		Entity:  kernel.EntityTopic,
	})
	if err != nil {
		t.Fatalf("related queries: %v", err)
	}
	if len(related.Queries) != 3 {
		t.Fatalf("related = %+v", related.Queries)
	}
}

// TestBrowserGatewayLive is opt-in because Google rate-limits and blocks
// anonymous bulk access.
func TestBrowserGatewayLive(t *testing.T) {
	if os.Getenv("TRENDS_LIVE") != "1" {
		t.Skip("set TRENDS_LIVE=1 to call the real provider")
	}
	chrome := DefaultChromePath()
	if chrome == "" {
		t.Skip("chrome not found")
	}
	gateway, err := NewBrowser(BrowserOptions{
		ChromePath:  chrome,
		Headless:    true,
		Timeout:     60 * time.Second,
		MinInterval: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("new browser: %v", err)
	}
	defer func() { _ = gateway.Close() }()

	ctx := context.Background()
	series, err := gateway.InterestOverTime(ctx, ports.InterestRequest{
		Term:    mustTerm(t, "ai agent"),
		Country: kernel.CountryWorldwide,
		Entity:  kernel.EntityTopic,
		Window:  hotsearch.Window12M,
	})
	if err != nil {
		if strings.Contains(err.Error(), "rate limit") || strings.Contains(err.Error(), "429") {
			t.Skipf("provider rate limited this egress IP: %v", err)
		}
		t.Fatalf("live call: %v", err)
	}
	if len(series.Values) < 2 {
		t.Fatalf("live series too short: %v", series.Values)
	}
	t.Logf("live points=%d first=%d last=%d", len(series.Values), series.Values[0], series.Values[len(series.Values)-1])
}

func mustTerm(t *testing.T, s string) kernel.Term {
	t.Helper()
	term, err := kernel.NewTerm(s)
	if err != nil {
		t.Fatal(err)
	}
	return term
}
