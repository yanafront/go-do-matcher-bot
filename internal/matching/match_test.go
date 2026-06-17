package matching

import (
	"testing"

	"github.com/anadubesko/go-do-matcher-bot/internal/models"
)

func TestMatchCityMismatch(t *testing.T) {
	c := models.User{City: "Минск", DesiredJob: "продавец"}
	v := models.Vacancy{City: "Гомель", Title: "продавец", Salary: 1500}
	if got := Match(c, v); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
}

func TestMatchAboveThreshold(t *testing.T) {
	c := models.User{City: "Минск", DesiredJob: "продавец кассир"}
	v := models.Vacancy{City: "Минск", Title: "продавец", Description: "кассир в магазине", Salary: 2000}
	got := Match(c, v)
	if got < 60 {
		t.Fatalf("expected >= 60, got %v", got)
	}
}
