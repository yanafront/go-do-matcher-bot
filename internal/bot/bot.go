package bot

import (
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/anadubesko/go-do-matcher-bot/internal/config"
	"github.com/anadubesko/go-do-matcher-bot/internal/match"
	"github.com/anadubesko/go-do-matcher-bot/internal/store"
	"go.uber.org/zap"
)

const (
	stateAskName  = "ask_name"
	stateAskQuery = "ask_query"
	stateReady    = "ready"
)

type Bot struct {
	api *tgbotapi.BotAPI
	cfg *config.Config
	st  *store.Store
	log *zap.Logger
}

func New(cfg *config.Config, st *store.Store, log *zap.Logger) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}
	return &Bot{api: api, cfg: cfg, st: st, log: log}, nil
}

func (b *Bot) API() *tgbotapi.BotAPI {
	return b.api
}

func (b *Bot) HandleUpdate(upd tgbotapi.Update) {
	if upd.ChannelPost != nil {
		b.handleChannelPost(upd.ChannelPost)
		return
	}
	if upd.Message == nil {
		return
	}
	if upd.Message.Chat.IsChannel() {
		return
	}
	b.handlePrivateMessage(upd.Message)
}

func (b *Bot) handlePrivateMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			b.start(chatID)
			return
		case "profile":
			b.askProfile(chatID)
			return
		case "stop":
			b.stop(chatID)
			return
		case "help":
			b.send(chatID, "Команды:\n/start — заново\n/profile — сменить имя или должность\n/stop — отписаться")
			return
		}
	}

	user, ok := b.st.GetUser(chatID)
	if !ok || user.State == "" {
		b.start(chatID)
		return
	}

	switch user.State {
	case stateAskName:
		name := trimName(text)
		if name == "" {
			b.send(chatID, "Напиши, пожалуйста, как тебя зовут 🙂")
			return
		}
		user.Name = name
		user.State = stateAskQuery
		_ = b.st.SaveUser(user)
		b.send(chatID, fmt.Sprintf("Приятно познакомиться, %s! 👋\n\nКакую должность ищешь?\nНапример: продавец, официант, водитель, кассир", name))
	case stateAskQuery:
		query := strings.TrimSpace(text)
		if len([]rune(query)) < 2 {
			b.send(chatID, "Напиши должность чуть подробнее — хотя бы одно слово 🙂")
			return
		}
		user.Query = query
		user.State = stateReady
		user.Active = true
		_ = b.st.SaveUser(user)
		b.send(chatID, fmt.Sprintf("Отлично, %s! Буду присылать вакансии по запросу «%s» из канала @%s.\n\n/profile — изменить\n/stop — отписаться", user.Name, query, b.cfg.ChannelUsername))
		b.sendRecentMatches(chatID, user)
	case stateReady:
		b.send(chatID, "Я уже знаю твой запрос. Хочешь изменить — /profile\nОтписаться — /stop")
	default:
		b.start(chatID)
	}
}

func (b *Bot) start(chatID int64) {
	u := store.User{
		ChatID: chatID,
		State:  stateAskName,
		Active: true,
	}
	_ = b.st.SaveUser(u)
	b.send(chatID, "Привет! 👋 Я подбираю вакансии из нашего канала под твой запрос.\n\nКак тебя зовут?")
}

func (b *Bot) askProfile(chatID int64) {
	u, ok := b.st.GetUser(chatID)
	if !ok {
		b.start(chatID)
		return
	}
	u.State = stateAskName
	u.Query = ""
	_ = b.st.SaveUser(u)
	b.send(chatID, "Давай обновим профиль. Как тебя зовут?")
}

func (b *Bot) stop(chatID int64) {
	u, ok := b.st.GetUser(chatID)
	if ok {
		u.Active = false
		u.State = stateReady
		_ = b.st.SaveUser(u)
	}
	b.send(chatID, "Хорошо, больше не буду присылать вакансии. Вернуться — /start")
}

func (b *Bot) handleChannelPost(msg *tgbotapi.Message) {
	text := messageText(msg)
	if text == "" {
		return
	}
	added, err := b.st.AddVacancy(store.Vacancy{
		ChannelMsgID: msg.MessageID,
		Text:         text,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		b.log.Warn("save vacancy", zap.Error(err))
		return
	}
	if !added {
		return
	}
	b.log.Info("new vacancy", zap.Int("msg_id", msg.MessageID))
	b.notifyMatchingUsers(msg.MessageID, text)
}

func (b *Bot) notifyMatchingUsers(channelMsgID int, text string) {
	for _, u := range b.st.ActiveUsers() {
		if !match.Fits(u.Query, text) {
			continue
		}
		if b.st.WasSent(u.ChatID, channelMsgID) {
			continue
		}
		if err := b.sendVacancy(u.ChatID, u.Name, channelMsgID, text); err != nil {
			b.log.Warn("send vacancy",
				zap.Int64("user", u.ChatID),
				zap.Int("msg_id", channelMsgID),
				zap.Error(err),
			)
			continue
		}
		_ = b.st.MarkSent(u.ChatID, channelMsgID)
	}
}

func (b *Bot) sendRecentMatches(chatID int64, user store.User) {
	items := b.st.RecentVacancies(b.cfg.MaxHistory)
	var sent int
	for _, v := range items {
		if !match.Fits(user.Query, v.Text) {
			continue
		}
		if b.st.WasSent(chatID, v.ChannelMsgID) {
			continue
		}
		if err := b.sendVacancy(chatID, user.Name, v.ChannelMsgID, v.Text); err != nil {
			continue
		}
		_ = b.st.MarkSent(chatID, v.ChannelMsgID)
		sent++
		if sent >= 5 {
			break
		}
		time.Sleep(400 * time.Millisecond)
	}
	if sent == 0 {
		b.send(chatID, "Пока нет подходящих вакансий в последних публикациях — пришлю, как только появится новая!")
	}
}

func (b *Bot) sendVacancy(chatID int64, name string, channelMsgID int, text string) error {
	body := text
	if len([]rune(body)) > 3500 {
		body = string([]rune(body)[:3500]) + "…"
	}
	msg := fmt.Sprintf("%s, кажется, это тебе подходит 🎯\n\n%s\n\n👉 %s",
		name, body, b.cfg.ChannelLink(channelMsgID))
	return b.send(chatID, msg)
}

func (b *Bot) send(chatID int64, text string) error {
	m := tgbotapi.NewMessage(chatID, text)
	m.DisableWebPagePreview = false
	_, err := b.api.Send(m)
	return err
}

func messageText(msg *tgbotapi.Message) string {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		text = strings.TrimSpace(msg.Caption)
	}
	return text
}

func trimName(s string) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > 64 {
		s = string([]rune(s)[:64])
	}
	return s
}
