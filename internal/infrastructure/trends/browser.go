package trends

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/discovery/ports"
	"seo/internal/shared/kernel"
)

// DefaultBaseURL is the public Trends origin.
const DefaultBaseURL = "https://trends.google.com"

const browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// BrowserOptions configures the chromedp-backed gateway.
type BrowserOptions struct {
	// ChromePath is the browser binary. Empty means: detect a standard path,
	// then let chromedp search PATH.
	ChromePath string
	// Headless hides the window. Default true.
	Headless bool
	// BaseURL overrides the Trends origin. Tests point it at a local mock.
	BaseURL string
	// Proxy is an HTTP proxy URL, for example http://127.0.0.1:7890.
	Proxy string
	// RemoteURL attaches to a Chrome that already runs, for example
	// http://127.0.0.1:9222. When set, ChromePath, Headless and Proxy are
	// ignored, and the existing profile and session are reused.
	RemoteURL string
	// MinInterval is the minimum delay between provider calls.
	MinInterval time.Duration
	// Repeat is how many times each window is fetched. The median of the runs
	// reduces sampling noise (BR-16). 1 disables repeats.
	Repeat int
	// Timeout bounds one tab's work.
	Timeout time.Duration
	// MaxPoints is the fallback cap when a window has no explicit cap.
	MaxPoints int
	// MaxPointsByWindow caps each window independently. Seasonality needs a
	// longer 5Y series than the UI needs, so one global cap is not enough.
	MaxPointsByWindow map[hotsearch.Window]int
}

// DefaultMaxPointsByWindow keeps enough resolution for BR-12/13 while bounding
// storage: daily for short windows, weekly for the five-year window.
func DefaultMaxPointsByWindow() map[hotsearch.Window]int {
	return map[hotsearch.Window]int{
		hotsearch.Window7D:  7,
		hotsearch.Window30D: 30,
		hotsearch.Window90D: 90,
		hotsearch.Window12M: 52,
		hotsearch.Window5Y:  260,
	}
}

// maxPoints returns the cap for one window.
func (o BrowserOptions) maxPoints(w hotsearch.Window) int {
	if v, ok := o.MaxPointsByWindow[w]; ok && v > 0 {
		return v
	}
	return o.MaxPoints
}

func withBrowserDefaults(opts BrowserOptions) BrowserOptions {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 45 * time.Second
	}
	if opts.MaxPoints <= 0 {
		opts.MaxPoints = 24
	}
	if opts.MaxPointsByWindow == nil {
		opts.MaxPointsByWindow = DefaultMaxPointsByWindow()
	}
	if opts.MinInterval < 0 {
		opts.MinInterval = 0
	}
	return opts
}

// Browser is a TrendsGateway that drives a real Chrome tab through CDP.
//
// It loads the Explore page to obtain the session, then calls the page's own
// internal endpoints from inside the page. That keeps cookies and consent
// handling in the browser, and avoids fragile SVG scraping.
type Browser struct {
	opts       BrowserOptions
	allocCtx   context.Context
	browserCtx context.Context
	cancel     context.CancelFunc

	mu           sync.Mutex
	lastCall     time.Time
	verifyCancel context.CancelFunc
}

// NewBrowser starts Chrome and returns a gateway. Call Close when done.
func NewBrowser(opts BrowserOptions) (*Browser, error) {
	opts = withBrowserDefaults(opts)

	allocOpts := make([]chromedp.ExecAllocatorOption, 0, len(chromedp.DefaultExecAllocatorOptions)+8)
	allocOpts = append(allocOpts, chromedp.DefaultExecAllocatorOptions[:]...)
	allocOpts = append(allocOpts,
		chromedp.Flag("headless", opts.Headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.UserAgent(browserUserAgent),
	)
	if opts.ChromePath != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(opts.ChromePath))
	}
	if opts.Proxy != "" {
		allocOpts = append(allocOpts, chromedp.ProxyServer(opts.Proxy))
	}

	var allocCtx context.Context
	var allocCancel context.CancelFunc
	if opts.RemoteURL != "" {
		allocCtx, allocCancel = remote.NewAllocator(context.Background(), opts.RemoteURL)
	} else {
		allocCtx, allocCancel = chromedp.NewExecAllocator(context.Background(), allocOpts...)
	}
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	cancel := func() {
		browserCancel()
		allocCancel()
	}

	// A local browser allocates on the first Run. Do not wrap it in a timeout:
	// the browser lives as long as this context. A remote browser only needs a
	// health check; navigating here would leave an extra blank tab.
	if opts.RemoteURL != "" {
		if err := probeRemoteVersion(opts.RemoteURL); err != nil {
			cancel()
			return nil, fmt.Errorf("trends: attach to %s: %w", opts.RemoteURL, err)
		}
	} else if err := chromedp.Do(browserCtx, chromedp.Navigate("about:blank")); err != nil {
		cancel()
		return nil, fmt.Errorf("trends: start chrome: %w", err)
	}

	return &Browser{
		opts:       opts,
		allocCtx:   allocCtx,
		browserCtx: browserCtx,
		cancel:     cancel,
	}, nil
}

// probeRemoteVersion checks a CDP HTTP endpoint without opening a tab.
func probeRemoteVersion(base string) error {
	if strings.HasPrefix(base, "ws://") || strings.HasPrefix(base, "wss://") {
		return nil
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(strings.TrimRight(base, "/") + "/json/version")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// Close stops the browser.
func (b *Browser) Close() error {
	if b == nil || b.cancel == nil {
		return nil
	}
	b.mu.Lock()
	if b.verifyCancel != nil {
		b.verifyCancel()
		b.verifyCancel = nil
	}
	b.mu.Unlock()
	b.cancel()
	b.cancel = nil
	return nil
}

// VerificationVisible reports whether the operator can see the browser window.
func (b *Browser) VerificationVisible() bool {
	if b == nil {
		return false
	}
	return b.opts.RemoteURL != "" || !b.opts.Headless
}

// OpenVerification opens the Explore page and keeps the tab. After a challenge,
// the operator solves it in that tab and retries in the UI.
func (b *Browser) OpenVerification(ctx context.Context, req ports.VerificationRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	tabCtx, tabCancel := chromedp.NewContext(b.browserCtx)
	runCtx, runCancel := context.WithTimeout(tabCtx, 30*time.Second)
	err := chromedp.Do(runCtx, chromedp.Navigate(b.exploreURL(req.Term, req.Country, hotsearch.Window12M)))
	runCancel()
	if err != nil {
		tabCancel()
		return fmt.Errorf("trends: open verification page: %w", err)
	}
	b.adoptVerificationTab(tabCancel)
	return nil
}

// closeVerificationTab drops the retained challenge tab. The caller must hold b.mu.
func (b *Browser) closeVerificationTab() {
	if b.verifyCancel != nil {
		b.verifyCancel()
		b.verifyCancel = nil
	}
}

// adoptVerificationTab keeps one verification tab alive. The caller must hold b.mu.
func (b *Browser) adoptVerificationTab(cancel context.CancelFunc) {
	if b.verifyCancel != nil {
		b.verifyCancel()
	}
	b.verifyCancel = cancel
}

// RelatedQueries fetches top and rising queries for one term.
func (b *Browser) RelatedQueries(ctx context.Context, req ports.RelatedRequest) (ports.RelatedResult, error) {
	payload, err := b.query(ctx, req.Term, req.Country, req.Entity, hotsearch.Window12M, false, true)
	if err != nil {
		return ports.RelatedResult{}, err
	}
	return toRelatedResult(payload), nil
}

// InterestOverTime fetches one window's series. With Repeat > 1 it fetches the
// window several times and keeps the median at each position, because the
// provider returns sampled values.
func (b *Browser) InterestOverTime(ctx context.Context, req ports.InterestRequest) (ports.InterestSeries, error) {
	repeat := b.opts.Repeat
	if repeat < 1 {
		repeat = 1
	}
	series := make([]ports.InterestSeries, 0, repeat)
	for i := 0; i < repeat; i++ {
		payload, err := b.query(ctx, req.Term, req.Country, req.Entity, req.Window, true, false)
		if err != nil {
			return ports.InterestSeries{}, err
		}
		s, err := toInterestSeries(req.Window, payload, b.opts.maxPoints(req.Window))
		if err != nil {
			return ports.InterestSeries{}, err
		}
		series = append(series, s)
	}
	return mergeSeries(series)
}

// mergeSeries computes a point-wise median across repeated runs. Runs whose
// sampled shapes disagree cannot be merged position by position — the values
// would belong to different points of the series — so that is an error, not a
// silent truncation.
func mergeSeries(runs []ports.InterestSeries) (ports.InterestSeries, error) {
	if len(runs) == 0 {
		return ports.InterestSeries{}, nil
	}
	if len(runs) == 1 {
		return runs[0], nil
	}
	n := len(runs[0].Values)
	for _, run := range runs[1:] {
		if len(run.Values) != n {
			return ports.InterestSeries{}, fmt.Errorf("trends: repeated fetches disagree on shape (%d vs %d points); retry", n, len(run.Values))
		}
	}
	out := ports.InterestSeries{
		Window: runs[0].Window,
		Start:  runs[0].Start,
		End:    runs[0].End,
		Values: make([]int, n),
	}
	buf := make([]int, len(runs))
	for i := 0; i < n; i++ {
		for j, run := range runs {
			buf[j] = run.Values[i]
		}
		out.Values[i] = medianInt(buf)
	}
	return out, nil
}

// medianInt is the true median. For an even count it rounds the average of the
// two middle samples, so the estimate does not lean toward the upper half.
func medianInt(values []int) int {
	cp := append([]int(nil), values...)
	sort.Ints(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}

func (b *Browser) query(ctx context.Context, term kernel.Term, country kernel.Country, entity kernel.EntityType, window hotsearch.Window, wantSeries, wantRelated bool) (browserPayload, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.pace(ctx); err != nil {
		return browserPayload{}, err
	}

	tabCtx, tabCancel := chromedp.NewContext(b.browserCtx)
	kept := false
	defer func() {
		if !kept {
			tabCancel()
		}
	}()
	runCtx, runCancel := context.WithTimeout(tabCtx, b.opts.Timeout)
	defer runCancel()

	exploreURL := b.exploreURL(term, country, window)
	if err := chromedp.Do(runCtx,
		chromedp.Navigate(exploreURL),
		chromedp.WaitReady("body"),
	); err != nil {
		return browserPayload{}, fmt.Errorf("trends: open explore page: %w", err)
	}

	cfg := scriptConfig{
		Term:        term.String(),
		Geo:         geoFor(country),
		Time:        trendTime(window),
		WantSeries:  wantSeries,
		WantRelated: wantRelated,
	}
	rawCfg, err := json.Marshal(cfg)
	if err != nil {
		return browserPayload{}, err
	}
	script := fmt.Sprintf(browserScript, string(rawCfg))

	// Detect a redirect away from the provider before running the fetch. A
	// CAPTCHA or consent page makes every relative fetch fail, and the raw
	// "Failed to fetch" hides the real cause.
	href, err := chromedp.Run(runCtx, chromedp.Evaluate[string](`location.href`, chromedp.EvalAwaitPromise))
	if err != nil {
		return browserPayload{}, classifyBrowserError(err)
	}
	if err := classifyLanding(b.opts.BaseURL, href); err != nil {
		if errors.Is(err, ports.ErrCaptchaRequired) || errors.Is(err, ports.ErrConsentRequired) {
			// Keep the tab so a visible browser shows the challenge.
			b.adoptVerificationTab(tabCancel)
			kept = true
		}
		return browserPayload{}, err
	}

	raw, err := chromedp.Run(runCtx, chromedp.Evaluate[string](script, chromedp.EvalAwaitPromise))
	if err != nil {
		return browserPayload{}, classifyBrowserError(err)
	}
	var payload browserPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return browserPayload{}, fmt.Errorf("%w: decode payload: %v", ports.ErrUnexpectedResponse, err)
	}
	if wantSeries && len(payload.Series) == 0 {
		return browserPayload{}, fmt.Errorf("%w: empty series for %q", ports.ErrUnexpectedResponse, term)
	}
	if wantRelated && len(payload.Related) == 0 {
		return browserPayload{}, fmt.Errorf("%w: empty related queries for %q", ports.ErrUnexpectedResponse, term)
	}
	// A successful call means the session works again; drop the old challenge tab.
	b.closeVerificationTab()
	return payload, nil
}

func (b *Browser) exploreURL(term kernel.Term, country kernel.Country, window hotsearch.Window) string {
	q := make([]string, 0, 3)
	q = append(q, "hl=en-US", "q="+urlQuery(term.String()), "date="+urlQuery(trendTime(window)))
	if geo := geoFor(country); geo != "" {
		q = append(q, "geo="+geo)
	}
	return strings.TrimRight(b.opts.BaseURL, "/") + "/trends/explore?" + strings.Join(q, "&")
}

// pace enforces the minimum interval between provider calls.
func (b *Browser) pace(ctx context.Context) error {
	if b.opts.MinInterval <= 0 || b.lastCall.IsZero() {
		b.lastCall = time.Now()
		return nil
	}
	wait := time.Until(b.lastCall.Add(b.opts.MinInterval))
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	b.lastCall = time.Now()
	return nil
}

type scriptConfig struct {
	Term        string `json:"term"`
	Geo         string `json:"geo"`
	Time        string `json:"time"`
	WantSeries  bool   `json:"wantSeries"`
	WantRelated bool   `json:"wantRelated"`
}

type browserPayload struct {
	Series []struct {
		T int64   `json:"t"`
		V float64 `json:"v"`
	} `json:"series"`
	Related []struct {
		Q string  `json:"q"`
		V float64 `json:"v"`
		F string  `json:"f"`
	} `json:"related"`
}

// browserScript runs inside the Trends page, so fetch keeps the session cookie.
const browserScript = `(async () => {
  const cfg = %s;
  const api = async (path, params) => {
    const r = await fetch(path + "?" + new URLSearchParams(params).toString(), {
      credentials: "include",
      headers: { accept: "application/json, text/plain, */*" }
    });
    const text = await r.text();
    if (!r.ok) { throw new Error("HTTP " + r.status); }
    let body = text;
    if (body.startsWith(")]}'")) {
      body = body.slice(4);
      // Google sometimes guards JSONP with a comma: )]}',\n{...}
      body = body.replace(/^\s*,/, "").trim();
    }
    try { return JSON.parse(body); }
    catch (e) { throw new Error("NON_JSON " + body.slice(0, 120)); }
  };
  const req = {
    comparisonItem: [{ keyword: cfg.term, geo: cfg.geo, time: cfg.time }],
    category: 0,
    property: ""
  };
  const explore = await api("/trends/api/explore", { hl: "en-US", tz: "0", req: JSON.stringify(req) });
  const out = { series: [], related: [] };
  for (const w of (explore.widgets || [])) {
    if (cfg.wantSeries && w.id === "TIMESERIES") {
      const d = await api("/trends/api/widgetdata/multiline", {
        hl: "en-US", tz: "0", req: JSON.stringify(w.request), token: w.token
      });
      const points = ((d.default || {}).timelineData) || [];
      out.series = points.map(p => ({ t: Number(p.time), v: Number((p.value || [0])[0]) }));
    }
    if (cfg.wantRelated && w.id === "RELATED_QUERIES") {
      const d = await api("/trends/api/widgetdata/relatedsearches", {
        hl: "en-US", tz: "0", req: JSON.stringify(w.request), token: w.token
      });
      const lists = ((d.default || {}).rankedList) || [];
      // The provider does not guarantee list order. Flatten and classify in Go.
      out.related = lists.flatMap(list => (list.rankedKeyword || []).map(k => ({
        q: String(k.query || ""), v: Number(k.value || 0), f: String(k.formattedValue || "")
      })));
    }
  }
  return JSON.stringify(out);
})()`

// trendTime maps our window to the provider's date token.
func trendTime(window hotsearch.Window) string {
	switch window {
	case hotsearch.Window7D:
		return "now 7-d"
	case hotsearch.Window30D:
		return "today 1-m"
	case hotsearch.Window90D:
		return "today 3-m"
	case hotsearch.Window12M:
		return "today 12-m"
	case hotsearch.Window5Y:
		return "today 5-y"
	default:
		return "today 12-m"
	}
}

// geoFor maps our market to the provider's geo code. Worldwide is empty.
func geoFor(country kernel.Country) string {
	if country == "" || country == kernel.CountryWorldwide {
		return ""
	}
	return strings.ToUpper(string(country))
}

func toRelatedResult(payload browserPayload) ports.RelatedResult {
	type entry struct {
		term     kernel.Term
		source   hotsearch.Source
		rise     int
		breakout bool
	}
	best := make(map[string]entry, len(payload.Related))
	order := make([]string, 0, len(payload.Related))
	for _, item := range payload.Related {
		term, err := kernel.NewTerm(item.Q)
		if err != nil {
			continue
		}
		source, rise, breakout := classifyRelated(item.F, item.V)
		key := term.String()
		next := entry{term: term, source: source, rise: rise, breakout: breakout}
		prev, seen := best[key]
		if !seen {
			best[key] = next
			order = append(order, key)
			continue
		}
		if sourceRank(next.source) > sourceRank(prev.source) {
			best[key] = next
		}
	}
	out := ports.RelatedResult{Queries: make([]ports.DiscoveredTerm, 0, len(order))}
	for _, key := range order {
		item := best[key]
		out.Queries = append(out.Queries, ports.DiscoveredTerm{
			Term:     item.term,
			Source:   item.source,
			RisePct:  item.rise,
			Breakout: item.breakout,
		})
	}
	return out
}

// classifyRelated decides the source from the entry itself, not its list index.
// Top entries are 0-100 indices with a numeric label. Rising entries carry a
// percent label or a value above 100. Breakout wins over both (BR-02).
func classifyRelated(formatted string, value float64) (hotsearch.Source, int, bool) {
	lower := strings.ToLower(formatted)
	if strings.Contains(lower, "breakout") || value >= 5000 {
		return hotsearch.SourceBreakout, 5000, true
	}
	if strings.Contains(formatted, "%") || value > 100 {
		return hotsearch.SourceRising, risePct(value, false), false
	}
	return hotsearch.SourceTop, 0, false
}

func sourceRank(source hotsearch.Source) int {
	switch source {
	case hotsearch.SourceBreakout:
		return 2
	case hotsearch.SourceRising:
		return 1
	default:
		return 0
	}
}

func risePct(value float64, breakout bool) int {
	if breakout {
		return 5000
	}
	if value < 0 {
		return 0
	}
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(math.Round(value))
}

func toInterestSeries(window hotsearch.Window, payload browserPayload, maxPoints int) (ports.InterestSeries, error) {
	values := make([]int, 0, len(payload.Series))
	for _, point := range payload.Series {
		values = append(values, clampIndex(point.V))
	}
	if len(values) < 2 {
		return ports.InterestSeries{}, fmt.Errorf("%w: %d points for %s", ports.ErrUnexpectedResponse, len(values), window)
	}
	start := time.Unix(payload.Series[0].T, 0).UTC()
	end := time.Unix(payload.Series[len(payload.Series)-1].T, 0).UTC()
	return ports.InterestSeries{
		Window: window,
		Start:  start,
		End:    end,
		Values: downsample(values, maxPoints),
	}, nil
}

func clampIndex(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return int(math.Round(v))
}

// downsample averages buckets so long series keep their shape at low resolution.
func downsample(values []int, maxPoints int) []int {
	if maxPoints <= 0 || len(values) <= maxPoints {
		out := make([]int, len(values))
		copy(out, values)
		return out
	}
	out := make([]int, maxPoints)
	for i := 0; i < maxPoints; i++ {
		start := i * len(values) / maxPoints
		end := (i + 1) * len(values) / maxPoints
		if end <= start {
			end = start + 1
		}
		sum := 0
		for _, v := range values[start:end] {
			sum += v
		}
		out[i] = int(math.Round(float64(sum) / float64(end-start)))
	}
	return out
}

// classifyLanding rejects pages that Google redirected us to. The provider sends
// automated traffic to a CAPTCHA, and consent interstitials have their own host.
// Both make the in-page fetch meaningless.
func classifyLanding(baseURL, href string) error {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	landed, err := url.Parse(href)
	if err != nil {
		return nil
	}
	if landed.Host == base.Host {
		return nil
	}
	if strings.Contains(href, "google.com/sorry") {
		return fmt.Errorf("%w: landed on %s", ports.ErrCaptchaRequired, href)
	}
	return fmt.Errorf("%w: landed on %s", ports.ErrConsentRequired, href)
}

func classifyBrowserError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "HTTP 429"):
		return fmt.Errorf("%w: %s", ports.ErrRateLimited, msg)
	case strings.Contains(msg, "NON_JSON"):
		return fmt.Errorf("%w: %s", ports.ErrConsentRequired, msg)
	case strings.Contains(msg, "Failed to fetch"):
		return fmt.Errorf("%w: %s", ports.ErrUnavailable, msg)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("trends: browser call timed out: %w", err)
	default:
		return fmt.Errorf("trends: browser call failed: %w", err)
	}
}

// DefaultChromePath finds a local Chrome. An empty result is not fatal because
// chromedp can also search PATH.
func DefaultChromePath() string {
	candidates := []string{
		os.Getenv("CHROME_PATH"),
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// Ensure Browser satisfies the port at compile time.
var _ ports.TrendsGateway = (*Browser)(nil)

// urlQuery escapes a query value. Kept local so callers cannot forget it.
func urlQuery(s string) string { return url.QueryEscape(s) }
