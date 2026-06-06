package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/anadubesko/go-do-matcher-bot/internal/bot"
	"github.com/anadubesko/go-do-matcher-bot/internal/config"
	"github.com/anadubesko/go-do-matcher-bot/internal/store"
	"go.uber.org/zap"
)

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("config", zap.Error(err))
	}

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatal("store", zap.Error(err))
	}
	defer st.Close()

	b, err := bot.New(cfg, st, log)
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

	if cfg.WebhookURL != "" {
		runWebhook(ctx, cfg, b, log)
		return
	}
	runPolling(ctx, b, log)
}

func runPolling(ctx context.Context, b *bot.Bot, log *zap.Logger) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	u.AllowedUpdates = []string{"message", "channel_post"}

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
			b.HandleUpdate(upd)
		}
	}
}

func runWebhook(ctx context.Context, cfg *config.Config, b *bot.Bot, log *zap.Logger) {
	wh, err := tgbotapi.NewWebhook(cfg.WebhookURL)
	if err != nil {
		log.Fatal("webhook", zap.Error(err))
	}
	wh.AllowedUpdates = []string{"message", "channel_post"}
	if _, err := b.API().Request(wh); err != nil {
		log.Fatal("set webhook", zap.Error(err))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		var upd tgbotapi.Update
		if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		b.HandleUpdate(upd)
		w.WriteHeader(http.StatusOK)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Info("webhook mode", zap.String("url", cfg.WebhookURL), zap.String("port", port))

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("http", zap.Error(err))
	}
}
