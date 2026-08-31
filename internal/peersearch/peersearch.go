package peersearch

import (
	"strings"

	"tailscale.com/tailcfg"
)

// fieldWeights apply to Fields in order: display name, hostname, FQDN, addresses.
var fieldWeights = []int{4, 3, 2, 1}

// Fields returns the strings that peer search matches against: display
// name, hostname, FQDN, and Tailscale addresses.
func Fields(peer tailcfg.NodeView) []string {
	if !peer.Valid() {
		return nil
	}
	host := ""
	if hi := peer.Hostinfo(); hi.Valid() {
		host = hi.Hostname()
	}
	var addrs []string
	for _, pfx := range peer.Addresses().All() {
		addrs = append(addrs, pfx.Addr().String())
	}
	return []string{
		peer.DisplayName(true),
		host,
		strings.TrimSuffix(peer.Name(), "."),
		strings.Join(addrs, " "),
	}
}

// Score reports how well peer matches query. Query tokens are
// whitespace-separated and all must match. The boolean is false if any
// token matches no field.
func Score(peer tailcfg.NodeView, query string) (int, bool) {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return 0, true
	}
	fields := Fields(peer)
	total := 0
	for _, tok := range tokens {
		best := -1
		for i, field := range fields {
			s := scoreToken(field, tok)
			if s < 0 {
				continue
			}
			weight := 1
			if i < len(fieldWeights) {
				weight = fieldWeights[i]
			}
			s *= weight
			if s > best {
				best = s
			}
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
	t := strings.ToLower(text)
	q := strings.ToLower(token)
	if t == q {
		return 10000
	}
	if strings.HasPrefix(t, q) {
		return 8000 - len(t)
	}
	if i := strings.Index(t, q); i >= 0 {
		return 6000 - i*10 - len(t)
	}
	s, ok := subsequenceScore(t, q)
	if !ok {
		return -1
	}
	return s
}

func subsequenceScore(text, query string) (int, bool) {
	tr := []rune(text)
	qr := []rune(query)
	if len(qr) == 0 {
		return 0, true
	}
	ti := 0
	score := 4000
	for qi, q := range qr {
		found := -1
		for i := ti; i < len(tr); i++ {
			if tr[i] == q {
				found = i
				break
			}
		}
		if found < 0 {
			return 0, false
		}
		if qi == 0 {
			score -= found * 15
		} else if gap := found - ti; gap > 0 {
			score -= gap * 10
		} else {
			score += 25
		}
		ti = found + 1
	}
	score -= (len(tr) - len(qr)) * 2
	if score < 0 {
		score = 0
	}
	return score, true
}
