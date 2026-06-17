package matching

import (
	"strings"
	"unicode"

	"github.com/anadubesko/go-do-matcher-bot/internal/models"
)

func Match(candidate models.User, vacancy models.Vacancy) float64 {
	if !cityMatch(candidate.City, vacancy.City) {
		return 0
	}
	score := 50.0
	score += keywordOverlap(candidate.DesiredJob, vacancy.Title+" "+vacancy.Description) * 30
	score += salaryMatch(vacancy.Salary) * 20
	if score > 100 {
		return 100
	}
	return score
}

func cityMatch(a, b string) bool {
	a = normalize(a)
	b = normalize(b)
	if a == "" || b == "" {
		return false
	}
	return a == b
}

func keywordOverlap(query, text string) float64 {
	words := keywords(query)
	if len(words) == 0 {
		return 0
	}
	text = normalize(text)
	if text == "" {
		return 0
	}
	matched := 0
	for _, w := range words {
		if strings.Contains(text, w) {
			matched++
		}
	}
	return float64(matched) / float64(len(words))
}

func salaryMatch(salary int) float64 {
	if salary > 0 {
		return 1
	}
	return 0
}

func keywords(s string) []string {
	s = strings.NewReplacer(",", " ", ";", " ", "/", " ", "|", " ", " или ", " ").Replace(s)
	parts := strings.Fields(normalize(s))
	seen := make(map[string]bool)
	var out []string
	for _, p := range parts {
		if len([]rune(p)) < 3 || seen[p] {
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
