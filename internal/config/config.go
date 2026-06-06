package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	BotToken        string
	ChannelUsername string
	WebhookURL      string
	DataDir         string
	MaxHistory      int
}

func Load() (*Config, error) {
	cfg := &Config{
		BotToken:        strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		ChannelUsername: normalizeChannel(os.Getenv("CHANNEL_USERNAME")),
		WebhookURL:      strings.TrimSpace(os.Getenv("WEBHOOK_URL")),
		DataDir:         strings.TrimSpace(os.Getenv("DATA_DIR")),
		MaxHistory:      30,
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if v := os.Getenv("MAX_HISTORY"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			cfg.MaxHistory = n
		}
	}
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is required")
	}
	if cfg.ChannelUsername == "" {
		return nil, fmt.Errorf("CHANNEL_USERNAME is required (e.g. goDoMinsk)")
	}
	if cfg.WebhookURL == "" {
		if base := strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"); base != "" {
			cfg.WebhookURL = base + "/webhook"
		} else if domain := strings.TrimSpace(os.Getenv("RAILWAY_PUBLIC_DOMAIN")); domain != "" {
			cfg.WebhookURL = "https://" + domain + "/webhook"
		}
	}
	return cfg, nil
}

func normalizeChannel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://t.me/")
	s = strings.TrimPrefix(s, "http://t.me/")
	s = strings.TrimPrefix(s, "t.me/")
	s = strings.TrimPrefix(s, "@")
	return s
}

func (c *Config) ChannelLink(messageID int) string {
	return fmt.Sprintf("https://t.me/%s/%d", c.ChannelUsername, messageID)
}
