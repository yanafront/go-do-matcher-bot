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
			b.help(chatID)
			return
		}
	}

	if text == btnStart {
		b.start(chatID)
		return
	}
	if text == btnProfile {
		b.askProfile(chatID)
		return
	}
	if text == btnStop {
		b.stop(chatID)
		return
	}
	if text == btnHelp {
		b.help(chatID)
		return
	}

	user, ok := b.st.GetUser(chatID)
	if !ok || user.State == "" {
		b.start(chatID)
		return
	}

	switch user.State {
	case stateAskName:
		if isButtonAction(text) {
			return
		}
		name := trimName(text)
		if name == "" {
			b.sendText(chatID, "Напиши, пожалуйста, как тебя зовут 🙂", menuHidden())
			return
		}
		user.Name = name
		user.State = stateAskQuery
		_ = b.st.SaveUser(user)
		b.sendText(chatID, fmt.Sprintf("Приятно познакомиться, %s! 👋\n\nКакую должность ищешь?\nНапример: продавец, официант, водитель, кассир", name), menuHidden())
	case stateAskQuery:
		if isButtonAction(text) {
			return
		}
		query := strings.TrimSpace(text)
		if len([]rune(query)) < 2 {
			b.sendText(chatID, "Напиши должность чуть подробнее — хотя бы одно слово 🙂", menuHidden())
			return
		}
		user.Query = query
		user.State = stateReady
		user.Active = true
		_ = b.st.SaveUser(user)
		b.log.Info("user registered",
			zap.Int64("chat_id", chatID),
			zap.String("name", user.Name),
			zap.String("query", query),
		)
		b.sendText(chatID, fmt.Sprintf("Отлично, %s! Буду присылать вакансии по запросу «%s» из канала @%s.", user.Name, query, b.cfg.ChannelUsername), menuActive())
		b.sendRecentMatches(chatID, user)
	case stateReady:
		if user.Active {
			b.sendText(chatID, "Используй кнопки ниже 👇", menuActive())
		} else {
			b.sendText(chatID, "Ты отписан. Нажми «Начать», чтобы снова получать вакансии.", menuStopped())
		}
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
	b.sendText(chatID, "Привет! 👋 Я подбираю вакансии из нашего канала под твой запрос.\n\nКак тебя зовут?", menuHidden())
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
	b.sendText(chatID, "Давай обновим профиль. Как тебя зовут?", menuHidden())
}

func (b *Bot) stop(chatID int64) {
	u, ok := b.st.GetUser(chatID)
	if ok {
		u.Active = false
		u.State = stateReady
		_ = b.st.SaveUser(u)
	}
	b.sendText(chatID, "Хорошо, больше не буду присылать вакансии.", menuStopped())
}

func (b *Bot) help(chatID int64) {
	u, ok := b.st.GetUser(chatID)
	text := "Кнопки:\n▶️ Начать — подписаться на вакансии\n👤 Профиль — сменить имя или должность\n⏹ Отписаться — перестать получать вакансии"
	if ok && u.Active && u.State == stateReady {
		b.sendText(chatID, text, menuActive())
		return
	}
	b.sendText(chatID, text, menuStopped())
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
	users := b.st.ActiveUsers()
	var matched, sent int
	for _, u := range users {
		if !match.Fits(u.Query, text) {
			continue
		}
		matched++
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
		sent++
	}
	b.log.Info("vacancy processed",
		zap.Int("msg_id", channelMsgID),
		zap.Int("active_users", len(users)),
		zap.Int("matched", matched),
		zap.Int("sent", sent),
	)
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
		b.sendText(chatID, "Пока нет подходящих вакансий в последних публикациях — пришлю, как только появится новая!", menuActive())
	}
}

func (b *Bot) sendVacancy(chatID int64, name string, channelMsgID int, text string) error {
	body := text
	if len([]rune(body)) > 3500 {
		body = string([]rune(body)[:3500]) + "…"
	}
	msg := fmt.Sprintf("%s, кажется, это тебе подходит 🎯\n\n%s\n\n👉 %s",
		name, body, b.cfg.ChannelLink(channelMsgID))
	return b.sendText(chatID, msg, menuActive())
}

func (b *Bot) sendText(chatID int64, text string, kb interface{}) error {
	m := tgbotapi.NewMessage(chatID, text)
	m.DisableWebPagePreview = false
	if kb != nil {
		m.ReplyMarkup = kb
	}
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
