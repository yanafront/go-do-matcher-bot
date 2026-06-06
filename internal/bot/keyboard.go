package bot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

const (
	btnStart   = "▶️ Начать"
	btnProfile = "👤 Профиль"
	btnStop    = "⏹ Отписаться"
	btnHelp    = "❓ Помощь"
)

func menuActive() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnProfile),
			tgbotapi.NewKeyboardButton(btnStop),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnHelp),
		),
	)
	kb.ResizeKeyboard = true
	return kb
}

func menuStopped() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnStart),
			tgbotapi.NewKeyboardButton(btnHelp),
		),
	)
	kb.ResizeKeyboard = true
	return kb
}

func menuHidden() tgbotapi.ReplyKeyboardRemove {
	return tgbotapi.NewRemoveKeyboard(true)
}

func isButtonAction(text string) bool {
	switch text {
	case btnStart, btnProfile, btnStop, btnHelp:
		return true
	default:
		return false
	}
}
