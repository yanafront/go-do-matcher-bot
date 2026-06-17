package bot

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/anadubesko/go-do-matcher-bot/internal/config"
	"github.com/anadubesko/go-do-matcher-bot/internal/handlers"
	"github.com/anadubesko/go-do-matcher-bot/internal/repository"
	"github.com/anadubesko/go-do-matcher-bot/internal/scheduler"
	"github.com/anadubesko/go-do-matcher-bot/internal/services"
	"go.uber.org/zap"
)

type Bot struct {
	api     *tgbotapi.BotAPI
	handler *handlers.Handler
	cfg     *config.Config
	log     *zap.Logger
}

func New(cfg *config.Config, repo *repository.Repository, log *zap.Logger) (*Bot, *scheduler.Scheduler, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, nil, err
	}

	users := services.NewUserService(repo)
	vacancies := services.NewVacancyService(repo)
	apps := services.NewApplicationService(repo)
	reviews := services.NewReviewService(repo)
	sessions := services.NewSessionService(repo)
	matches := services.NewMatchService(repo, cfg.MatchThreshold)

	h := handlers.New(api, users, vacancies, apps, reviews, sessions, log)
	sched := scheduler.New(matches, h, cfg.MatchInterval, log)

	return &Bot{
		api:     api,
		handler: h,
		cfg:     cfg,
		log:     log,
	}, sched, nil
}

func (b *Bot) API() *tgbotapi.BotAPI {
	return b.api
}

func (b *Bot) HandleUpdate(ctx context.Context, upd tgbotapi.Update) {
	b.handler.HandleUpdate(ctx, upd)
}
