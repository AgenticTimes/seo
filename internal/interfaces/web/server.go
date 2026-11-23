// Package web is the primary (P) adapter: server-rendered pages that trigger
// every discovery use case with one button each. No business rule lives here.
package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"seo/internal/discovery/application/command/discardkeyword"
	"seo/internal/discovery/application/command/expandkeyword"
	"seo/internal/discovery/application/command/judgetrend"
	"seo/internal/discovery/application/command/marktoobroad"
	"seo/internal/discovery/application/command/openverification"
	"seo/internal/discovery/application/command/promote"
	"seo/internal/discovery/application/command/recordobservation"
	"seo/internal/discovery/application/command/reopenkeyword"
	"seo/internal/discovery/application/command/startexploration"
	"seo/internal/discovery/application/query"
	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/discovery/ports"
	"seo/internal/evaluation/application/attacheevidence"
	"seo/internal/evaluation/application/createopportunity"
	"seo/internal/evaluation/application/decideopportunity"
	evalquery "seo/internal/evaluation/application/query"
	"seo/internal/evaluation/domain/opportunity"
	"seo/internal/shared/kernel"
)

//go:embed templates/*.html static/*
var assets embed.FS

// App wires the use cases the page can trigger.
type App struct {
	Start     *startexploration.Handler
	Expand    *expandkeyword.Handler
	Observe   *recordobservation.Handler
	Assess    *judgetrend.Handler
	Advance   *promote.Handler
	Broad     *marktoobroad.Handler
	Discard   *discardkeyword.Handler
	Reopen    *reopenkeyword.Handler
	Verify    *openverification.Handler
	Dashboard *query.DashboardHandler
	Advice    *query.Advisor

	CreateOpportunity *createopportunity.Handler
	AttachEvidence    *attacheevidence.Handler
	DecideOpportunity *decideopportunity.Handler
	Opportunities     *evalquery.Lookup
}

// Server renders the console.
type Server struct {
	app       *App
	templates *template.Template
}

// New parses templates and validates wiring.
func New(app *App) (*Server, error) {
	if app == nil || app.Start == nil || app.Expand == nil || app.Observe == nil ||
		app.Assess == nil || app.Advance == nil || app.Broad == nil ||
		app.Discard == nil || app.Reopen == nil ||
		app.Verify == nil || app.Dashboard == nil || app.Advice == nil ||
		app.CreateOpportunity == nil || app.AttachEvidence == nil ||
		app.DecideOpportunity == nil || app.Opportunities == nil {
		return nil, fmt.Errorf("web: incomplete app wiring")
	}
	templates, err := template.New("").Funcs(funcs()).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("web: parse templates: %w", err)
	}
	return &Server{app: app, templates: templates}, nil
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("POST /explore", s.explore)
	mux.HandleFunc("POST /expand", s.expand)
	mux.HandleFunc("POST /observe", s.observe)
	mux.HandleFunc("POST /assess", s.assess)
	mux.HandleFunc("POST /advance", s.advance)
	mux.HandleFunc("POST /broad", s.markBroad)
	mux.HandleFunc("POST /discard", s.discard)
	mux.HandleFunc("POST /reopen", s.reopen)
	mux.HandleFunc("POST /verify", s.openVerify)
	mux.HandleFunc("POST /opportunity/create", s.createOpportunity)
	mux.HandleFunc("POST /opportunity/evidence", s.attachEvidence)
	mux.HandleFunc("POST /opportunity/decide", s.decideOpportunity)
	return mux
}

type pageData struct {
	View            query.Dashboard
	Selected        *hotsearch.Keyword
	CanExpand       bool
	CanAdvance      bool
	CanDiscard      bool
	CanReopen       bool
	AllowSeed       bool
	Closed          bool
	Captcha         bool
	CanVerify       bool
	VerifyVisible   bool
	BroadAdvice     query.Advice
	Opportunity     *opportunity.Opportunity
	CanCreate       bool
	OpportunityHint string
	Country         string
	Entity          string
	Msg             string
	Err             string
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()

	view, err := s.app.Dashboard.Execute(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := pageData{
		View:          view,
		Country:       string(kernel.CountryWorldwide),
		Entity:        string(kernel.EntityTopic),
		Msg:           r.URL.Query().Get("msg"),
		Err:           r.URL.Query().Get("err"),
		Captcha:       r.URL.Query().Get("captcha") == "1",
		CanVerify:     s.app.Verify.Available(),
		VerifyVisible: s.app.Verify.Visible(),
	}
	if key, ok := parseKey(r.URL.Query().Get("k")); ok {
		for _, candidate := range view.All {
			if candidate.Key().Equal(key) {
				data.Selected = candidate
				break
			}
		}
	}
	if data.Selected != nil {
		data.CanExpand = data.Selected.CanBeExpanded()
		data.AllowSeed = data.Selected.Verdict().AllowsSeed()
		data.Closed = data.Selected.State().IsTerminal()
		data.CanDiscard = !data.Closed
		switch data.Selected.State() {
		case hotsearch.StateValidated, hotsearch.StateSeedCandidate, hotsearch.StateDiscarded:
			data.CanReopen = !data.Selected.TooBroad()
		}
		// A root seed is already the origin of the graph; there is nothing to
		// promote. Only discovered children can become next-round seeds.
		data.CanAdvance = data.AllowSeed &&
			data.Selected.Depth() > 0 &&
			data.Selected.State() != hotsearch.StateSeedCandidate &&
			!data.Closed
		data.Country = string(data.Selected.Key().Country)
		data.Entity = string(data.Selected.Key().Entity)
		if !data.Closed && !data.Selected.TooBroad() {
			advice, err := s.app.Advice.Execute(ctx, data.Selected.Key())
			if err != nil {
				log.Printf("web: advice %s: %v", data.Selected.Key(), err)
			} else {
				data.BroadAdvice = advice
			}
		}
		opp, err := s.app.Opportunities.Execute(ctx, data.Selected.Key())
		if err != nil {
			log.Printf("web: opportunity %s: %v", data.Selected.Key(), err)
		} else {
			data.Opportunity = opp
		}
		if data.Opportunity == nil {
			data.CanCreate, data.OpportunityHint = opportunityEligibility(data.Selected, data.Selected.Signals())
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "index", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) explore(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()

	terms := splitTerms(r.FormValue("terms"))
	if len(terms) == 0 {
		redirect(w, r, nil, "", "请至少输入一个种子词")
		return
	}
	res, err := s.app.Start.Execute(ctx, startexploration.Command{
		Terms:   terms,
		Country: r.FormValue("country"),
		Entity:  r.FormValue("entity"),
	})
	if err != nil {
		s.fail(w, r, nil, err)
		return
	}
	var key *hotsearch.Key
	if len(res.Created) > 0 {
		key = &res.Created[0]
	} else if len(res.Skipped) > 0 {
		key = &res.Skipped[0]
	}
	redirect(w, r, key, fmt.Sprintf("种子新增 %d，已存在 %d", len(res.Created), len(res.Skipped)), "")
}

func (s *Server) expand(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 150*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	res, err := s.app.Expand.Execute(ctx, expandkeyword.Command{
		Term:    string(key.Term),
		Country: string(key.Country),
		Entity:  string(key.Entity),
	})
	if err != nil {
		s.fail(w, r, &key, err)
		return
	}
	redirect(w, r, &key, fmt.Sprintf("扩展完成：新增 %d，已存在 %d，超出深度 %d",
		len(res.Created), len(res.Skipped), len(res.DepthLimited)), "")
}

func (s *Server) observe(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 240*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	res, err := s.app.Observe.Execute(ctx, recordobservation.Command{
		Term:    string(key.Term),
		Country: string(key.Country),
		Entity:  string(key.Entity),
	})
	if err != nil {
		s.fail(w, r, &key, err)
		return
	}
	redirect(w, r, &key, fmt.Sprintf("已记录 %d 个时间窗", len(res.Recorded)), "")
}

func (s *Server) assess(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	res, err := s.app.Assess.Execute(ctx, judgetrend.Command{
		Term:    string(key.Term),
		Country: string(key.Country),
		Entity:  string(key.Entity),
	})
	if err != nil {
		s.fail(w, r, &key, err)
		return
	}
	msg := "判定结果：" + verdictLabel(res.Verdict)
	if len(res.Missing) > 0 {
		msg += fmt.Sprintf("（缺少 %s 窗口数据）", joinWindows(res.Missing))
	}
	if res.ChildCount > 0 {
		msg += fmt.Sprintf("；子词品牌占比 %.0f%%（%d/%d）", res.BrandShare*100, int(res.BrandShare*float64(res.ChildCount)+0.5), res.ChildCount)
		if res.Crowded {
			msg += "，注意红海入口"
		}
	}
	redirect(w, r, &key, msg, "")
}

func (s *Server) advance(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	if _, err := s.app.Advance.Execute(ctx, promote.Command{
		Term:    string(key.Term),
		Country: string(key.Country),
		Entity:  string(key.Entity),
	}); err != nil {
		s.fail(w, r, &key, err)
		return
	}
	redirect(w, r, &key, "已进入下一轮种子队列", "")
}

func (s *Server) createOpportunity(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	res, err := s.app.CreateOpportunity.Execute(ctx, createopportunity.Command{
		Term: string(key.Term), Country: string(key.Country), Entity: string(key.Entity),
	})
	if err != nil {
		s.fail(w, r, &key, err)
		return
	}
	msg := "机会已创建，先补充外部证据再批准"
	if !res.Created {
		msg = "机会已存在"
	}
	redirect(w, r, &key, msg, "")
}

func (s *Server) attachEvidence(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	res, err := s.app.AttachEvidence.Execute(ctx, attacheevidence.Command{
		Term:    string(key.Term),
		Country: string(key.Country),
		Entity:  string(key.Entity),
		Source:  r.FormValue("source"),
		URL:     strings.TrimSpace(r.FormValue("url")),
		Summary: strings.TrimSpace(r.FormValue("summary")),
	})
	if err != nil {
		s.fail(w, r, &key, err)
		return
	}
	redirect(w, r, &key, fmt.Sprintf("已记录 %d 条外部证据", res.Count), "")
}

func (s *Server) decideOpportunity(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	res, err := s.app.DecideOpportunity.Execute(ctx, decideopportunity.Command{
		Term:    string(key.Term),
		Country: string(key.Country),
		Entity:  string(key.Entity),
		Approve: r.FormValue("approve") == "1",
		Note:    strings.TrimSpace(r.FormValue("note")),
	})
	if err != nil {
		s.fail(w, r, &key, err)
		return
	}
	msg := "机会已拒绝"
	if res.State == opportunity.StateApproved {
		msg = "机会已批准"
	}
	redirect(w, r, &key, msg, "")
}

// opportunityEligibility explains why a keyword cannot open an opportunity.
func opportunityEligibility(keyword *hotsearch.Keyword, signals hotsearch.Signals) (bool, string) {
	if keyword == nil {
		return false, ""
	}
	if keyword.TooBroad() || keyword.State().IsTerminal() {
		return false, "该词已关闭，不能创建机会。"
	}
	if signals.Intent == hotsearch.IntentNavigational {
		return false, "导航型意图（品牌/登录/下载），不做机会。"
	}
	if !keyword.Verdict().AllowsSeed() {
		return false, fmt.Sprintf("当前判定为「%s」，需要新兴 / 持续增长 / 季节性增长。", verdictLabel(keyword.Verdict()))
	}
	return true, ""
}

func (s *Server) discard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	if _, err := s.app.Discard.Execute(ctx, discardkeyword.Command{
		Term: string(key.Term), Country: string(key.Country), Entity: string(key.Entity),
		Reason: strings.TrimSpace(r.FormValue("reason")),
	}); err != nil {
		s.fail(w, r, &key, err)
		return
	}
	redirect(w, r, &key, "已放弃该词", "")
}

func (s *Server) reopen(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	if _, err := s.app.Reopen.Execute(ctx, reopenkeyword.Command{
		Term: string(key.Term), Country: string(key.Country), Entity: string(key.Entity),
	}); err != nil {
		s.fail(w, r, &key, err)
		return
	}
	redirect(w, r, &key, "已重新打开，请重新判定趋势", "")
}

func (s *Server) openVerify(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 45*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	res, err := s.app.Verify.Execute(ctx, openverification.Command{
		Term:    string(key.Term),
		Country: string(key.Country),
		Entity:  string(key.Entity),
	})
	if err != nil {
		s.fail(w, r, &key, err)
		return
	}
	msg := "验证页已打开。请在 Chrome 中完成验证，然后重新点原来的那一步。"
	if !res.Visible {
		msg = "验证页已打开，但当前浏览器不可见。请改用 -chrome-remote 连接有头 Chrome。"
	}
	redirect(w, r, &key, msg, "")
}

func (s *Server) markBroad(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()

	key, ok := formKey(r)
	if !ok {
		redirect(w, r, nil, "", "无效的关键词")
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if _, err := s.app.Broad.Execute(ctx, marktoobroad.Command{
		Term:    string(key.Term),
		Country: string(key.Country),
		Entity:  string(key.Entity),
		Reason:  reason,
	}); err != nil {
		s.fail(w, r, &key, err)
		return
	}
	redirect(w, r, &key, "已标记为过于宽泛并关闭", "")
}

func formKey(r *http.Request) (hotsearch.Key, bool) {
	return parseKey(r.FormValue("k"))
}

func parseKey(raw string) (hotsearch.Key, bool) {
	parts := strings.Split(raw, "|")
	if len(parts) != 3 {
		return hotsearch.Key{}, false
	}
	term, err := kernel.NewTerm(parts[0])
	if err != nil {
		return hotsearch.Key{}, false
	}
	country, err := kernel.NewCountry(parts[1])
	if err != nil {
		return hotsearch.Key{}, false
	}
	entity, err := kernel.ParseEntityType(parts[2])
	if err != nil {
		return hotsearch.Key{}, false
	}
	key, err := hotsearch.NewKey(term, country, entity)
	if err != nil {
		return hotsearch.Key{}, false
	}
	return key, true
}

func redirect(w http.ResponseWriter, r *http.Request, key *hotsearch.Key, msg, errMsg string) {
	redirectWith(w, r, key, msg, errMsg, nil)
}

func redirectWith(w http.ResponseWriter, r *http.Request, key *hotsearch.Key, msg, errMsg string, extra url.Values) {
	q := url.Values{}
	if key != nil {
		q.Set("k", key.String())
	}
	if msg != "" {
		q.Set("msg", msg)
	}
	if errMsg != "" {
		q.Set("err", errMsg)
	}
	for name, values := range extra {
		for _, value := range values {
			q.Set(name, value)
		}
	}
	target := "/"
	if encoded := q.Encode(); encoded != "" {
		target += "?" + encoded
	}
	if key != nil {
		target += "#detail"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func splitTerms(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func funcs() template.FuncMap {
	return template.FuncMap{
		"stateLabel":     stateLabel,
		"stateClass":     stateClass,
		"sourceLabel":    sourceLabel,
		"sourceClass":    sourceClass,
		"verdictLabel":   verdictLabel,
		"verdictClass":   verdictClass,
		"changePct":      changePct,
		"barHeight":      barHeight,
		"indent":         indent,
		"allowsSeed":     func(v hotsearch.Verdict) bool { return v.AllowsSeed() },
		"joinKeys":       joinKeys,
		"intentLabel":    intentLabel,
		"displaySamples": displaySamples,
		"pct0":           func(v float64) string { return fmt.Sprintf("%.0f%%", v*100) },
		"broadReason":    broadReasonLabel,
		"reasonLabel":    reasonLabel,
		"oppState":       oppStateLabel,
		"oppStateClass":  oppStateClass,
		"oppOpen":        func(state opportunity.State) bool { return !state.IsClosed() },
		"evidenceSource": evidenceSourceLabel,
		"signedPct":      func(v float64) string { return fmt.Sprintf("%+.1f%%", v*100) },
		"hiddenKey":      hiddenKey,
		"dict":           dict,
		"sameKey":        sameKey,
	}
}

func stateLabel(s hotsearch.State) string {
	switch s {
	case hotsearch.StateDiscovered:
		return "待验证"
	case hotsearch.StateValidated:
		return "已验证"
	case hotsearch.StateSeedCandidate:
		return "种子候选"
	case hotsearch.StateTooBroad:
		return "过于宽泛"
	case hotsearch.StateDiscarded:
		return "已放弃"
	default:
		return string(s)
	}
}

func stateClass(s hotsearch.State) string {
	switch s {
	case hotsearch.StateDiscovered:
		return "text-bg-secondary"
	case hotsearch.StateValidated:
		return "text-bg-info"
	case hotsearch.StateSeedCandidate:
		return "text-bg-success"
	case hotsearch.StateTooBroad:
		return "text-bg-warning"
	case hotsearch.StateDiscarded:
		return "text-bg-dark"
	default:
		return "text-bg-light"
	}
}

func sourceLabel(s hotsearch.Source) string {
	switch s {
	case hotsearch.SourceSeed:
		return "种子"
	case hotsearch.SourceTop:
		return "热门"
	case hotsearch.SourceRising:
		return "上升"
	case hotsearch.SourceBreakout:
		return "暴涨"
	default:
		return string(s)
	}
}

func sourceClass(s hotsearch.Source) string {
	switch s {
	case hotsearch.SourceRising:
		return "text-bg-success"
	case hotsearch.SourceBreakout:
		return "text-bg-warning"
	case hotsearch.SourceTop:
		return "text-bg-secondary"
	default:
		return "text-bg-dark"
	}
}

func verdictLabel(v hotsearch.Verdict) string {
	switch v {
	case hotsearch.VerdictInsufficient:
		return "数据不足"
	case hotsearch.VerdictDeclining:
		return "下降"
	case hotsearch.VerdictStable:
		return "平稳"
	case hotsearch.VerdictShortLived:
		return "短期波动"
	case hotsearch.VerdictEmerging:
		return "新兴"
	case hotsearch.VerdictSustained:
		return "持续增长"
	case hotsearch.VerdictLongTerm:
		return "长期赛道"
	default:
		return "未判定"
	}
}

func verdictClass(v hotsearch.Verdict) string {
	switch v {
	case hotsearch.VerdictEmerging:
		return "text-bg-success"
	case hotsearch.VerdictSustained:
		return "text-bg-primary"
	case hotsearch.VerdictLongTerm:
		return "text-bg-info"
	case hotsearch.VerdictShortLived:
		return "text-bg-warning"
	case hotsearch.VerdictDeclining:
		return "text-bg-danger"
	case hotsearch.VerdictStable:
		return "text-bg-secondary"
	default:
		return "text-bg-light"
	}
}

func changePct(o hotsearch.Observation) string {
	pct := o.ChangePct() * 100
	sign := ""
	if pct > 0 {
		sign = "+"
	}
	return fmt.Sprintf("%s%.0f%%", sign, pct)
}

func barHeight(i hotsearch.Index) int {
	if v := i.Int(); v > 4 {
		return v
	}
	return 4
}

func indent(depth int) string {
	if depth <= 0 {
		return "0px"
	}
	return fmt.Sprintf("%dpx", depth*18)
}

func joinKeys(keys []hotsearch.Key) string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, string(key.Term))
	}
	return strings.Join(out, "，")
}

func joinWindows(windows []hotsearch.Window) string {
	out := make([]string, 0, len(windows))
	for _, w := range windows {
		out = append(out, string(w))
	}
	return strings.Join(out, "、")
}

// contextWithTimeout ties downstream work to the request lifetime.
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

// hiddenKey renders the three identity fields as hidden inputs.
func hiddenKey(k hotsearch.Key) template.HTML {
	return template.HTML(fmt.Sprintf(`<input type="hidden" name="k" value="%s">`,
		template.HTMLEscapeString(k.String())))
}

// dict builds a map for passing two values into a nested template.
func dict(pairs ...any) map[string]any {
	out := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out[fmt.Sprint(pairs[i])] = pairs[i+1]
	}
	return out
}

// sameKey compares a row against the current selection.
func sameKey(a, b *hotsearch.Keyword) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Key().Equal(b.Key())
}

// friendlyError maps domain error codes to operator-facing Chinese. The domain
// stays language-neutral; translation belongs to the presentation adapter.
func friendlyError(err error) string {
	switch {
	case errors.Is(err, hotsearch.ErrNotValidated):
		return "趋势还没判定为「新兴」或「持续增长」。先点 ④ 记录观测，再点 ⑤ 判定趋势。"
	case errors.Is(err, hotsearch.ErrTooBroad):
		return "该词已被标记为过于宽泛，不能再推进。"
	case errors.Is(err, hotsearch.ErrDepthExceeded):
		return "已达到最大扩展深度（2 层），不能再往下扩展。"
	case errors.Is(err, hotsearch.ErrInvalidTransition):
		return "当前状态不允许这个操作：种子本身无需入队，或该词已关闭。"
	case errors.Is(err, hotsearch.ErrNotFound):
		return "找不到该关键词，请刷新页面后重试。"
	case errors.Is(err, hotsearch.ErrReasonRequired):
		return "请填写原因。"
	case errors.Is(err, opportunity.ErrNotEligible):
		return "该词不满足机会条件：意图为导航型，或趋势未达到新兴 / 持续增长 / 季节性增长。"
	case errors.Is(err, opportunity.ErrGateFailed):
		return "批准未通过：需要至少一条外部证据，且趋势合格、词不过宽。"
	case errors.Is(err, opportunity.ErrDuplicateEvidence):
		return "这条证据已存在。"
	case errors.Is(err, opportunity.ErrEvidenceEmpty):
		return "证据至少要填写链接或摘要。"
	case errors.Is(err, opportunity.ErrNoteRequired):
		return "请填写拒绝原因。"
	case errors.Is(err, opportunity.ErrClosed):
		return "该机会已有最终决策，不能再修改。"
	case errors.Is(err, ports.ErrRateLimited):
		return "Google Trends 返回 429（限流）。请稍后再试，或更换出口 IP / 代理。"
	case errors.Is(err, ports.ErrConsentRequired):
		return "Google Trends 返回了非数据页面（同意页 / 拦截页 / 限流页）。请先在 Chrome 里打开 trends.google.com 完成同意；若持续失败，说明出口 IP 被限流。"
	case errors.Is(err, ports.ErrCaptchaRequired):
		return "Google 要求人机验证（检测到异常流量）。请打开有头 Chrome 访问 trends.google.com 完成验证，再重试；或更换出口 IP。"
	case errors.Is(err, ports.ErrUnavailable):
		return "浏览器无法访问 Google Trends（网络 / 代理 / 被拦截）。请检查代理连通性和出口 IP 后重试。"
	case errors.Is(err, ports.ErrUnexpectedResponse):
		return "Google Trends 返回了无法识别的数据（可能已改版），请查看服务端日志。"
	default:
		return err.Error()
	}
}

// fail logs the raw adapter error and shows a friendly message.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, key *hotsearch.Key, err error) {
	log.Printf("web: %s %s: %v", r.Method, r.URL.Path, err)
	extra := url.Values{}
	if errors.Is(err, ports.ErrCaptchaRequired) {
		extra.Set("captcha", "1")
	}
	redirectWith(w, r, key, "", friendlyError(err), extra)
}

func intentLabel(intent hotsearch.Intent) string {
	switch intent {
	case hotsearch.IntentInformational:
		return "信息型"
	case hotsearch.IntentCommercial:
		return "商业调研"
	case hotsearch.IntentTransactional:
		return "交易型"
	case hotsearch.IntentNavigational:
		return "导航型"
	default:
		return "未识别"
	}
}

// displaySamples averages a long series down for the sparkline. The stored
// series keeps full resolution for BR-12; only the view is reduced.
func displaySamples(o hotsearch.Observation) []hotsearch.Index {
	samples := o.Samples()
	const maxBars = 24
	if len(samples) <= maxBars {
		return samples
	}
	out := make([]hotsearch.Index, maxBars)
	for i := 0; i < maxBars; i++ {
		start := i * len(samples) / maxBars
		end := (i + 1) * len(samples) / maxBars
		if end <= start {
			end = start + 1
		}
		sum := 0
		for _, v := range samples[start:end] {
			sum += v.Int()
		}
		avg, err := hotsearch.NewIndex(sum / (end - start))
		if err != nil {
			avg = 0
		}
		out[i] = avg
	}
	return out
}

// broadReasonLabel maps BR-21 reason codes to operator text.
func broadReasonLabel(reason string) string {
	switch reason {
	case hotsearch.BroadReasonSingleToken:
		return "单个词，没有修饰语说明意图"
	case hotsearch.BroadReasonDispersed:
		return "子词之间没有共同主题"
	default:
		return reason
	}
}

// oppStateLabel maps evaluation states to operator text.
func oppStateLabel(state opportunity.State) string {
	switch state {
	case opportunity.StateCandidate:
		return "候选"
	case opportunity.StateEvidence:
		return "已收集证据"
	case opportunity.StateApproved:
		return "已批准"
	case opportunity.StateRejected:
		return "已拒绝"
	default:
		return string(state)
	}
}

func oppStateClass(state opportunity.State) string {
	switch state {
	case opportunity.StateApproved:
		return "text-bg-success"
	case opportunity.StateRejected:
		return "text-bg-danger"
	case opportunity.StateEvidence:
		return "text-bg-info"
	default:
		return "text-bg-secondary"
	}
}

func evidenceSourceLabel(source opportunity.SourceType) string {
	switch source {
	case opportunity.SourceReddit:
		return "Reddit"
	case opportunity.SourceCompetitor:
		return "竞品"
	case opportunity.SourceReview:
		return "评论"
	case opportunity.SourceSearch:
		return "搜索量"
	default:
		return "其他"
	}
}

// reasonLabel maps verdict reason codes to operator text.
func reasonLabel(reason string) string {
	switch reason {
	case hotsearch.ReasonNo30D:
		return "缺少 30D 数据，无法判断当前动量"
	case hotsearch.ReasonNoisy:
		return "30D 采样点不足，噪声大"
	case hotsearch.ReasonDeclining30D:
		return "30D 下降"
	case hotsearch.ReasonFlat30D:
		return "30D 平稳"
	case hotsearch.ReasonSpike:
		return "出现单次尖峰，90D 未持续"
	case hotsearch.ReasonSeasonal:
		return "季节性强，改看去季节后的结构增长"
	case hotsearch.ReasonSeasonalGrowth:
		return "去季节后仍有增长"
	case hotsearch.ReasonMissing90D:
		return "缺少 90D 数据"
	case hotsearch.ReasonMissing12M:
		return "缺少 12M 数据"
	case hotsearch.ReasonThinLongWindow:
		return "90D / 12M 采样点不足，无法判定方向"
	case hotsearch.ReasonLongTerm:
		return "5Y 长期上涨，属于长期赛道而非新机会"
	case hotsearch.ReasonShortLived:
		return "30D 上涨但 90D 未持续"
	case hotsearch.ReasonStable90D:
		return "90D 未涨，但 12M 在涨"
	case hotsearch.ReasonSustained:
		return "30D / 90D / 12M 同步上涨"
	case hotsearch.ReasonEmerging:
		return "30D / 90D 上涨，12M 平稳"
	default:
		return reason
	}
}
