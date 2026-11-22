package hotsearch

import (
	"strings"
	"unicode"
)

// Intent is the likely purpose behind a query (BR-17).
type Intent string

// Intent values.
const (
	IntentUnknown       Intent = "UNKNOWN"
	IntentInformational Intent = "INFORMATIONAL"
	IntentCommercial    Intent = "COMMERCIAL"
	IntentTransactional Intent = "TRANSACTIONAL"
	IntentNavigational  Intent = "NAVIGATIONAL"
)

// ClassifyIntent applies BR-17 with word lists. Navigational wins because a
// branded destination is not an unmet need. Informational is the fallback only
// when a question word is present; otherwise the intent stays unknown.
func ClassifyIntent(term string) Intent {
	t := strings.ToLower(term)
	switch {
	case containsAnyWord(t, navigationalWords...) || IsBrandMention(t):
		return IntentNavigational
	case containsAnyWord(t, transactionalWords...):
		return IntentTransactional
	case containsAnyWord(t, commercialWords...):
		return IntentCommercial
	case containsAnyWord(t, informationalWords...):
		return IntentInformational
	default:
		return IntentUnknown
	}
}

var (
	navigationalWords = []string{
		"login", "log in", "sign in", "download", "app", "official", "官网", "登录", "下载", "入口",
	}
	transactionalWords = []string{
		"buy", "price", "pricing", "cost", "cheap", "discount", "coupon", "free trial",
		"subscribe", "subscription", "purchase", "价格", "购买", "折扣", "订阅", "多少钱",
	}
	commercialWords = []string{
		"best", "top", "review", "reviews", "vs", "versus", "compare", "comparison",
		"alternative", "alternatives", "recommend", "ranking", "推荐", "对比", "测评", "排行",
	}
	informationalWords = []string{
		"how", "what", "why", "when", "where", "tutorial", "guide", "example", "examples",
		"meaning", "definition", "works", "教程", "怎么", "如何", "是什么", "原理",
	}
)

// knownBrands is reference data for BR-18. It is deliberately small and kept in
// the domain because it is business vocabulary, not infrastructure.
var knownBrands = []string{
	"openai", "chatgpt", "gpt", "claude", "anthropic", "gemini", "bard", "grok", "xai",
	"copilot", "perplexity", "midjourney", "runway", "sora", "kimi", "moonshot",
	"deepseek", "qwen", "tongyi", "llama", "mistral", "notion", "figma", "canva",
	"suno", "elevenlabs", "cursor", "windsurf", "replit", "vercel", "github copilot",
	"meta ai", "google ai", "apple intelligence", "doubao", "wenxin", "glm",
}

// IsBrandMention reports whether the term names a known product (BR-18 input).
func IsBrandMention(term string) bool {
	t := strings.ToLower(term)
	for _, brand := range knownBrands {
		if strings.Contains(brand, " ") {
			if strings.Contains(t, brand) {
				return true
			}
			continue
		}
		for _, token := range tokens(t) {
			if token == brand {
				return true
			}
		}
	}
	return false
}

// NonLatinRatio is the share of non-ASCII letters in the term. A high value
// warns that the chosen market returned another language (BR-19).
func NonLatinRatio(term string) float64 {
	total, nonLatin := 0, 0
	for _, r := range term {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if r > 127 {
			nonLatin++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(nonLatin) / float64(total)
}

func containsAnyWord(term string, words ...string) bool {
	for _, word := range words {
		if strings.ContainsAny(word, " ") {
			if strings.Contains(term, word) {
				return true
			}
			continue
		}
		if hasCJK(word) {
			if strings.Contains(term, word) {
				return true
			}
			continue
		}
		for _, token := range tokens(term) {
			if token == word {
				return true
			}
		}
	}
	return false
}

func tokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
