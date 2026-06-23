package handlers

import "fmt"

import "github.com/anadubesko/go-do-matcher-bot/internal/models"

func scorePercent(score float64) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return int(score + 0.5)
}

func formatRating(avg float64, count int) string {
	if count == 0 {
		return "пока без отзывов"
	}
	return fmt.Sprintf("⭐ %.1f · %d отз.", avg, count)
}

func formatSalary(amount int) string {
	if amount <= 0 {
		return "по договорённости"
	}
	return fmt.Sprintf("%d BYN", amount)
}

func tgLink(username string, tgID int64) string {
	if username != "" {
		return "@" + username
	}
	return fmt.Sprintf("tg://user?id=%d", tgID)
}

func isSingleHire(needed int) bool {
	return needed <= 1
}

func teamLabel(needed int) string {
	if isSingleHire(needed) {
		return "1 человек"
	}
	return fmt.Sprintf("смена · %d чел.", needed)
}

func applicantsHeader(v *models.Vacancy) string {
	if isSingleHire(v.NeededCount) {
		return fmt.Sprintf(
			"<b>«%s»</b> · ищем 1 человека\n\nПонравился кандидат? <b>Принять</b> → контакты → <b>Нанять — готово</b>",
			escape(v.Title),
		)
	}
	return fmt.Sprintf(
		"<b>«%s»</b>\n👥 Смена: %d из %d набрано\n\nРазбирай отклики — нанимай по одному.",
		escape(v.Title), v.FilledCount, v.NeededCount,
	)
}

func vacancyPublishedText(needed int) string {
	if isSingleHire(needed) {
		return "Вакансия опубликована! 🚀\n\nКак только придёт подходящий отклик — прими его и нажми «Нанять». Всё займёт пару кликов."
	}
	return "Вакансия опубликована! 🚀\n\nНанимай по одному — прогресс виден в «Мои вакансии». При большом потоке откликов набор встанет на паузу сам."
}
