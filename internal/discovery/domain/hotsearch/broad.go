package hotsearch

import "strings"

// Broad reasons returned by SuggestTooBroad. Presentation maps them to text.
const (
	BroadReasonSingleToken = "SINGLE_TOKEN"
	BroadReasonDispersed   = "DISPERSED_CHILDREN"
)

// SuggestTooBroad implements BR-21. It only advises; closing a keyword stays a
// human decision (BR-04).
//
// Two signals matter:
//   - a single-token term has no modifier to express intent;
//   - children that share no modifier are about different things, so the parent
//     does not name one need.
//
// A single token is not enough on its own: a five-word brand also has one
// token. The advice needs at least MinBroadChildren children.
const (
	MinBroadChildren  = 5
	MaxDispersedRatio = 0.3
)

func SuggestTooBroad(term string, childTerms []string) (bool, []string) {
	parent := tokenSet(term)
	if len(childTerms) < MinBroadChildren {
		return false, nil
	}

	reasons := make([]string, 0, 2)
	if len(parent) == 1 {
		reasons = append(reasons, BroadReasonSingleToken)
	}

	if dispersed(childTerms, parent) {
		reasons = append(reasons, BroadReasonDispersed)
	}
	return len(reasons) > 0, reasons
}

func tokenSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, token := range tokens(strings.ToLower(s)) {
		out[token] = true
	}
	return out
}

// dispersed reports whether fewer than MaxDispersedRatio of children share a
// modifier with another child. Parent tokens do not count as modifiers.
func dispersed(childTerms []string, parent map[string]bool) bool {
	freq := map[string]int{}
	childTokens := make([][]string, 0, len(childTerms))
	for _, child := range childTerms {
		var modifiers []string
		for _, token := range tokens(strings.ToLower(child)) {
			if parent[token] {
				continue
			}
			modifiers = append(modifiers, token)
			freq[token]++
		}
		childTokens = append(childTokens, modifiers)
	}

	shared := 0
	for _, modifiers := range childTokens {
		for _, token := range modifiers {
			if freq[token] >= 2 {
				shared++
				break
			}
		}
	}
	ratio := float64(shared) / float64(len(childTerms))
	return ratio < MaxDispersedRatio
}
