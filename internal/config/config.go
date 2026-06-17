package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BotToken       string
	DatabaseURL    string
	MatchInterval  time.Duration
	MatchThreshold float64
}

func Load() (*Config, error) {
	intervalSec := 90
	if v := strings.TrimSpace(os.Getenv("MATCH_INTERVAL_SEC")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 60 || n > 180 {
			return nil, fmt.Errorf("MATCH_INTERVAL_SEC must be between 60 and 180")
		}
		intervalSec = n
	}

	threshold := 60.0
	if v := strings.TrimSpace(os.Getenv("MATCH_THRESHOLD")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f > 100 {
			return nil, fmt.Errorf("MATCH_THRESHOLD must be between 0 and 100")
		}
		threshold = f
	}

	cfg := &Config{
		BotToken:       strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		DatabaseURL:    strings.TrimSpace(os.Getenv("DATABASE_URL")),
		MatchInterval:  time.Duration(intervalSec) * time.Second,
		MatchThreshold: threshold,
	}
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is required")
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}
