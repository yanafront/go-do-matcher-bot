package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/anadubesko/go-do-matcher-bot/internal/bot"
	"github.com/anadubesko/go-do-matcher-bot/internal/config"
	"github.com/anadubesko/go-do-matcher-bot/internal/db"
	"github.com/anadubesko/go-do-matcher-bot/internal/repository"
	"go.uber.org/zap"
)

func main() {
	loadDotEnv(".env")

	log, _ := zap.NewProduction()
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("config", zap.Error(err))
	}

	database, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("database", zap.Error(err))
	}
	defer database.Close()

	repo := repository.New(database.DB)
	b, sched, err := bot.New(cfg, repo, log)
	if err != nil {
		log.Fatal("bot", zap.Error(err))
	}

	me, err := b.API().GetMe()
	if err != nil {
		log.Fatal("getMe", zap.Error(err))
	}
	log.Info("bot started", zap.String("username", me.UserName))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go sched.Run(ctx)

	if _, err := b.API().Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: false}); err != nil {
		log.Warn("delete webhook", zap.Error(err))
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	u.AllowedUpdates = []string{"message", "callback_query"}

	updates := b.API().GetUpdatesChan(u)
	log.Info("polling mode")

	for {
		select {
		case <-ctx.Done():
			b.API().StopReceivingUpdates()
			return
		case upd, ok := <-updates:
			if !ok {
				return
			}
			b.HandleUpdate(ctx, upd)
		}
	}
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		val = strings.Trim(val, `"'`)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
