package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"seo/internal/discovery/application/command/expandkeyword"
	"seo/internal/discovery/application/command/judgetrend"
	"seo/internal/discovery/application/command/marktoobroad"
	"seo/internal/discovery/application/command/openverification"
	"seo/internal/discovery/application/command/promote"
	"seo/internal/discovery/application/command/recordobservation"
	"seo/internal/discovery/application/command/startexploration"
	"seo/internal/discovery/application/query"
	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/discovery/ports"
	"seo/internal/evaluation/application/attacheevidence"
	"seo/internal/evaluation/application/createopportunity"
	"seo/internal/evaluation/application/decideopportunity"
	evalquery "seo/internal/evaluation/application/query"
	"seo/internal/infrastructure/sqlite"
	"seo/internal/infrastructure/trends"
	"seo/internal/interfaces/web"
)

func main() {
	dbPath := flag.String("db", "seo.db", "SQLite database file (use :memory: for a throwaway run)")
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	gatewayName := flag.String("gateway", "fake", "trends data source: fake or browser")
	chromePath := flag.String("chrome", "", "Chrome binary for the browser gateway (default: auto-detect)")
	headless := flag.Bool("headless", true, "run Chrome without a window")
	trendsBase := flag.String("trends-base", trends.DefaultBaseURL, "Trends origin (change only for a mock)")
	trendsProxy := flag.String("trends-proxy", "", "HTTP proxy for Chrome, for example http://127.0.0.1:7890")
	chromeRemote := flag.String("chrome-remote", "auto", "attach target: auto, off, or http://127.0.0.1:9222")
	minInterval := flag.Duration("min-interval", 3*time.Second, "minimum delay between Trends calls")
	trendSamples := flag.Int("trends-samples", 1, "fetch each window N times and keep the median (3 reduces noise)")
	flag.Parse()

	db, err := sqlite.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := sqlite.NewKeywordRepository(db)
	oppRepo := sqlite.NewOpportunityRepository(db)
	expansion := hotsearch.DefaultExpansionPolicy()
	trend := hotsearch.DefaultTrendPolicy()

	gateway, stopGateway, err := buildGateway(*gatewayName, *chromePath, *headless, *trendsBase, *trendsProxy, *chromeRemote, *minInterval, *trendSamples)
	if err != nil {
		log.Fatalf("build gateway: %v", err)
	}
	defer stopGateway()

	app := &web.App{
		Start:     startexploration.New(repo, nil),
		Expand:    expandkeyword.New(repo, gateway, expansion, nil),
		Observe:   recordobservation.New(repo, gateway, nil),
		Assess:    judgetrend.New(repo, trend),
		Advance:   promote.New(repo, expansion),
		Broad:     marktoobroad.New(repo),
		Verify:    openverification.New(gateway),
		Dashboard: query.NewDashboard(repo),
		Advice:    query.NewAdvisor(repo),

		CreateOpportunity: createopportunity.New(repo, oppRepo, nil),
		AttachEvidence:    attacheevidence.New(oppRepo, nil),
		DecideOpportunity: decideopportunity.New(repo, oppRepo, nil),
		Opportunities:     evalquery.NewLookup(oppRepo),
	}
	server, err := web.New(app)
	if err != nil {
		log.Fatalf("build web server: %v", err)
	}

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()

	log.Printf("SEO trend explorer on http://%s (db=%s, gateway=%s)", *addr, *dbPath, *gatewayName)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}

// DefaultRemoteURL is the debug endpoint that auto mode looks for.
const DefaultRemoteURL = "http://127.0.0.1:9222"

// resolveRemoteTarget turns the -chrome-remote value into an attach decision.
// "auto" attaches only when a debug Chrome answers on DefaultRemoteURL.
func resolveRemoteTarget(value string, reachable func(string) bool) (string, bool) {
	switch value {
	case "", "off", "none":
		return "", false
	case "auto":
		if reachable(DefaultRemoteURL) {
			return DefaultRemoteURL, true
		}
		return "", false
	default:
		return value, true
	}
}

// probeChrome reports whether a CDP endpoint answers.
func probeChrome(url string) bool {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(strings.TrimRight(url, "/") + "/json/version")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// buildGateway selects the TrendsGateway adapter at startup. The rest of the
// program only sees the port.
func buildGateway(name, chromePath string, headless bool, baseURL, proxy, remoteValue string, minInterval time.Duration, repeat int) (ports.TrendsGateway, func(), error) {
	switch name {
	case "", "fake":
		return trends.NewFake(), func() {}, nil
	case "browser":
		remoteURL, attached := resolveRemoteTarget(remoteValue, probeChrome)
		if attached {
			log.Printf("chrome: attached to %s", remoteURL)
		} else {
			log.Printf("chrome: no debug endpoint at %s, launching a local browser", DefaultRemoteURL)
		}
		path := chromePath
		if path == "" {
			path = trends.DefaultChromePath()
		}
		gateway, err := trends.NewBrowser(trends.BrowserOptions{
			ChromePath:  path,
			Headless:    headless,
			BaseURL:     baseURL,
			Proxy:       proxy,
			RemoteURL:   remoteURL,
			MinInterval: minInterval,
			Repeat:      repeat,
			Timeout:     45 * time.Second,
		})
		if err != nil {
			return nil, nil, err
		}
		return gateway, func() { _ = gateway.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("unknown gateway %q (use fake or browser)", name)
	}
}
