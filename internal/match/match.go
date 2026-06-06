package match

import (
	"strings"
	"unicode"
)

func Fits(query, text string) bool {
	query = normalize(query)
	text = normalize(text)
	if query == "" || text == "" {
		return false
	}
	if strings.Contains(text, query) {
		return true
	}
	for _, word := range keywords(query) {
		if len([]rune(word)) < 3 {
			continue
		}
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func keywords(query string) []string {
	query = strings.NewReplacer(",", " ", ";", " ", "/", " ", "|", " ", " или ", " ").Replace(query)
	parts := strings.Fields(query)
	var out []string
	seen := make(map[string]bool)
	for _, p := range parts {
		p = normalize(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
