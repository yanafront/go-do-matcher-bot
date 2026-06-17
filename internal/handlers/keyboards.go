package handlers

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func roleKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Соискатель", "role:candidate"),
			tgbotapi.NewInlineKeyboardButtonData("Работодатель", "role:employer"),
		),
	)
}

func candidateMatchKeyboard(vacancyID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Apply", "apply:"+vacancyID),
			tgbotapi.NewInlineKeyboardButtonData("Skip", "skip:"+vacancyID),
		),
	)
}

func employerMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Create vacancy", "menu:create_vacancy"),
			tgbotapi.NewInlineKeyboardButtonData("View applicants", "menu:my_vacancies"),
		),
	)
}

func vacancyActionsKeyboard(vacancyID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("View applicants", "menu:applicants:"+vacancyID),
			tgbotapi.NewInlineKeyboardButtonData("Close vacancy", "close:"+vacancyID),
		),
	)
}

func applicantActionsKeyboard(applicationID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Hire", "hire:"+applicationID),
			tgbotapi.NewInlineKeyboardButtonData("Reject", "reject:"+applicationID),
		),
	)
}

func reviewRatingKeyboard(vacancyID, toUserID string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := 1; i <= 10; i += 5 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("1", "review:"+vacancyID+":"+toUserID+":1"),
			tgbotapi.NewInlineKeyboardButtonData("2", "review:"+vacancyID+":"+toUserID+":2"),
			tgbotapi.NewInlineKeyboardButtonData("3", "review:"+vacancyID+":"+toUserID+":3"),
			tgbotapi.NewInlineKeyboardButtonData("4", "review:"+vacancyID+":"+toUserID+":4"),
			tgbotapi.NewInlineKeyboardButtonData("5", "review:"+vacancyID+":"+toUserID+":5"),
		}
		rows = append(rows, row)
	}
	row2 := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("6", "review:"+vacancyID+":"+toUserID+":6"),
		tgbotapi.NewInlineKeyboardButtonData("7", "review:"+vacancyID+":"+toUserID+":7"),
		tgbotapi.NewInlineKeyboardButtonData("8", "review:"+vacancyID+":"+toUserID+":8"),
		tgbotapi.NewInlineKeyboardButtonData("9", "review:"+vacancyID+":"+toUserID+":9"),
		tgbotapi.NewInlineKeyboardButtonData("10", "review:"+vacancyID+":"+toUserID+":10"),
	}
	rows = append(rows, row2)
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Skip review", "skip_review:"+vacancyID+":"+toUserID),
	))
	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func contactKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButtonContact("Отправить телефон"),
		),
	)
}

func removeKeyboard() tgbotapi.ReplyKeyboardRemove {
	return tgbotapi.NewRemoveKeyboard(true)
}
