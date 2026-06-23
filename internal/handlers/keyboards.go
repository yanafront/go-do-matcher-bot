package handlers

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func roleKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔍 Ищу работу", "role:candidate"),
			tgbotapi.NewInlineKeyboardButtonData("💼 Нанимаю", "role:employer"),
		),
	)
}

func candidateMenuKeyboard(searchActive bool) tgbotapi.InlineKeyboardMarkup {
	toggle := tgbotapi.NewInlineKeyboardButtonData("⏸ Пауза поиска", "menu:toggle_search")
	if !searchActive {
		toggle = tgbotapi.NewInlineKeyboardButtonData("▶️ Возобновить поиск", "menu:toggle_search")
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(toggle),
	)
}

func candidateMatchKeyboard(vacancyID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👎 Пропустить", "skip:"+vacancyID),
			tgbotapi.NewInlineKeyboardButtonData("🟢 Откликнуться", "apply:"+vacancyID),
		),
	)
}

func employerMenuKeyboard(hiringPaused bool) tgbotapi.InlineKeyboardMarkup {
	toggle := tgbotapi.NewInlineKeyboardButtonData("⏸ Пауза всех вакансий", "menu:toggle_hiring")
	if hiringPaused {
		toggle = tgbotapi.NewInlineKeyboardButtonData("▶️ Возобновить набор", "menu:toggle_hiring")
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👤 Одна вакансия", "menu:create_one"),
			tgbotapi.NewInlineKeyboardButtonData("👥 Смена", "menu:create_vacancy"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Мои вакансии", "menu:my_vacancies"),
		),
		tgbotapi.NewInlineKeyboardRow(toggle),
	)
}

func neededCountKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👤 Один человек", "needed:1"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("2", "needed:2"),
			tgbotapi.NewInlineKeyboardButtonData("3", "needed:3"),
			tgbotapi.NewInlineKeyboardButtonData("5", "needed:5"),
			tgbotapi.NewInlineKeyboardButtonData("10", "needed:10"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("20", "needed:20"),
			tgbotapi.NewInlineKeyboardButtonData("50", "needed:50"),
			tgbotapi.NewInlineKeyboardButtonData("✏️ Другое", "needed:custom"),
		),
	)
}

func vacancyActionsKeyboard(vacancyID string, collecting bool, closed bool, neededCount int) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	if isSingleHire(neededCount) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Отклики", "menu:applicants:"+vacancyID),
		))
	} else {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Отклики", "menu:applicants:"+vacancyID),
			tgbotapi.NewInlineKeyboardButtonData("✅ Смена", "menu:hired:"+vacancyID),
		))
	}
	if !closed {
		toggle := tgbotapi.NewInlineKeyboardButtonData("⏸ Пауза", "pause_vac:"+vacancyID)
		if !collecting {
			toggle = tgbotapi.NewInlineKeyboardButtonData("▶️ Возобновить", "resume_vac:"+vacancyID)
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			toggle,
			tgbotapi.NewInlineKeyboardButtonData("🔴 Закрыть", "close:"+vacancyID),
		))
	}
	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func applicationKeyboard(status, applicationID string, singleHire bool) tgbotapi.InlineKeyboardMarkup {
	switch status {
	case "sent":
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔴 Отклонить", "reject:"+applicationID),
				tgbotapi.NewInlineKeyboardButtonData("🟢 Принять", "accept:"+applicationID),
			),
		)
	case "accepted":
		hireLabel := "✅ Нанять"
		if singleHire {
			hireLabel = "✅ Нанять — готово"
		}
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔴 Не подошёл", "reject:"+applicationID),
				tgbotapi.NewInlineKeyboardButtonData(hireLabel, "hire:"+applicationID),
			),
		)
	default:
		return tgbotapi.InlineKeyboardMarkup{}
	}
}

func hiredRateKeyboard(vacancyID, candidateID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⭐ Оценить", "rate_prompt:"+vacancyID+":"+candidateID),
		),
	)
}

func reviewRatingKeyboard(vacancyID, toUserID string) tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("1⭐", "review:"+vacancyID+":"+toUserID+":1"),
			tgbotapi.NewInlineKeyboardButtonData("2⭐", "review:"+vacancyID+":"+toUserID+":2"),
			tgbotapi.NewInlineKeyboardButtonData("3⭐", "review:"+vacancyID+":"+toUserID+":3"),
			tgbotapi.NewInlineKeyboardButtonData("4⭐", "review:"+vacancyID+":"+toUserID+":4"),
			tgbotapi.NewInlineKeyboardButtonData("5⭐", "review:"+vacancyID+":"+toUserID+":5"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("6⭐", "review:"+vacancyID+":"+toUserID+":6"),
			tgbotapi.NewInlineKeyboardButtonData("7⭐", "review:"+vacancyID+":"+toUserID+":7"),
			tgbotapi.NewInlineKeyboardButtonData("8⭐", "review:"+vacancyID+":"+toUserID+":8"),
			tgbotapi.NewInlineKeyboardButtonData("9⭐", "review:"+vacancyID+":"+toUserID+":9"),
			tgbotapi.NewInlineKeyboardButtonData("10⭐", "review:"+vacancyID+":"+toUserID+":10"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("Пропустить", "skip_review:"+vacancyID+":"+toUserID),
		},
	}
	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func contactKeyboard() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButtonContact("📱 Отправить телефон"),
		),
	)
	kb.ResizeKeyboard = true
	kb.OneTimeKeyboard = true
	return kb
}

func removeKeyboard() tgbotapi.ReplyKeyboardRemove {
	return tgbotapi.NewRemoveKeyboard(true)
}
