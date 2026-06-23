package scheduler

import (
	"context"
	"time"

	"github.com/anadubesko/go-do-matcher-bot/internal/services"
	"go.uber.org/zap"
)

type Notifier interface {
	SendMatch(ctx context.Context, match services.MatchNotification) error
}

type Scheduler struct {
	matches  *services.MatchService
	notifier Notifier
	interval time.Duration
	log      *zap.Logger
}

func New(matches *services.MatchService, notifier Notifier, interval time.Duration, log *zap.Logger) *Scheduler {
	return &Scheduler{
		matches:  matches,
		notifier: notifier,
		interval: interval,
		log:      log,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	created, err := s.matches.Run(ctx)
	if err != nil {
		s.log.Warn("match run", zap.Error(err))
		return
	}
	if created > 0 {
		s.log.Info("matches created", zap.Int("count", created))
	}

	pending, err := s.matches.Pending(ctx)
	if err != nil {
		s.log.Warn("list pending matches", zap.Error(err))
		return
	}
	for _, m := range pending {
		claimed, err := s.matches.Claim(ctx, m.ID)
		if err != nil {
			s.log.Warn("claim match", zap.Error(err))
			continue
		}
		if claimed == nil {
			continue
		}
		if err := s.notifier.SendMatch(ctx, services.MatchNotification{
			MatchID:       claimed.ID.String(),
			CandidateID:   claimed.CandidateTgID,
			CandidateUUID: claimed.CandidateID.String(),
			Title:         claimed.VacancyTitle,
			Description:   claimed.VacancyDescription,
			City:          claimed.VacancyCity,
			Salary:        claimed.VacancySalary,
			Score:         claimed.Score,
			VacancyID:     claimed.VacancyID.String(),
		}); err != nil {
			s.log.Warn("send match",
				zap.String("match_id", claimed.ID.String()),
				zap.Int64("candidate", claimed.CandidateTgID),
				zap.Error(err),
			)
			continue
		}
		s.log.Info("match sent",
			zap.String("match_id", claimed.ID.String()),
			zap.Int64("candidate", claimed.CandidateTgID),
			zap.Float64("score", claimed.Score),
		)
	}
}
