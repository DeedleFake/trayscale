package peersearch

import (
	"iter"
	"strings"
	"unicode/utf8"

	"tailscale.com/tailcfg"
)

// fieldWeights apply to the first fields yielded by Fields: display
// name, hostname, FQDN. Later fields (addresses) use weight 1.
var fieldWeights = []int{4, 3, 2}

// Fields yields the strings that peer search matches against: display
// name, hostname, FQDN, then each Tailscale address.
func Fields(peer tailcfg.NodeView) iter.Seq[string] {
	return func(yield func(string) bool) {
		if !peer.Valid() {
			return
		}
		if !yield(peer.DisplayName(true)) {
			return
		}
		host := ""
		if hi := peer.Hostinfo(); hi.Valid() {
			host = hi.Hostname()
		}
		if !yield(host) {
			return
		}
		if !yield(strings.TrimSuffix(peer.Name(), ".")) {
			return
		}
		for _, pfx := range peer.Addresses().All() {
			if !yield(pfx.Addr().String()) {
				return
			}
		}
	}
}

// Score reports how well peer matches tokens. All tokens must match.
// The boolean is false if any token matches no field.
func Score(peer tailcfg.NodeView, tokens []string) (int, bool) {
	if len(tokens) == 0 {
		return 0, true
	}
	total := 0
	for _, tok := range tokens {
		best := -1
		i := 0
		for field := range Fields(peer) {
			s := scoreToken(field, tok)
			if s >= 0 {
				weight := 1
				if i < len(fieldWeights) {
					weight = fieldWeights[i]
				}
				s *= weight
				if s > best {
					best = s
				}
			}
			i++
		}
		if best < 0 {
			return 0, false
		}
		total += best
	}
	return total, true
}

func scoreToken(text, token string) int {
	if token == "" {
		return 0
	}
	if strings.EqualFold(text, token) {
		return 10000
	}
	n := utf8.RuneCountInString(text)
	pos := 0
	for i := range text {
		if hasPrefixFold(text[i:], token) {
			if pos == 0 {
				return 8000 - n
			}
			return 6000 - pos*10 - n
		}
		pos++
	}
	s, ok := subsequenceScore(text, token)
	if !ok {
		return -1
	}
	return s
}

func hasPrefixFold(s, prefix string) bool {
	skip := utf8.RuneCountInString(prefix)
	for i := range s {
		if skip == 0 {
			return strings.EqualFold(s[:i], prefix)
		}
		skip--
	}
	return skip == 0 && strings.EqualFold(s, prefix)
}

func subsequenceScore(text, query string) (int, bool) {
	if query == "" {
		return 0, true
	}

	score := 4000
	start := 0
	first := true
	rest := text
	for qrest := query; qrest != ""; {
		qchunk, qnext := splitRune(qrest)
		qrest = qnext

		found := -1
		rel := 0
		search := rest
		for search != "" {
			chunk, next := splitRune(search)
			if strings.EqualFold(chunk, qchunk) {
				found = rel
				rest = next
				break
			}
			search = next
			rel++
		}
		if found < 0 {
			return 0, false
		}
		abs := start + found
		if first {
			score -= abs * 15
			first = false
		} else if found > 0 {
			score -= found * 10
		} else {
			score += 25
		}
		start = abs + 1
	}

	score -= (utf8.RuneCountInString(text) - utf8.RuneCountInString(query)) * 2
	if score < 0 {
		score = 0
	}
	return score, true
}

func splitRune(s string) (first, rest string) {
	_, n := utf8.DecodeRuneInString(s)
	return s[:n], s[n:]
}
