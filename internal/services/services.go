package services

import (
	"context"
	"fmt"
	"time"

	"github.com/anadubesko/go-do-matcher-bot/internal/matching"
	"github.com/anadubesko/go-do-matcher-bot/internal/models"
	"github.com/anadubesko/go-do-matcher-bot/internal/repository"
	"github.com/google/uuid"
)

type MatchNotification struct {
	MatchID       string
	CandidateID   int64
	CandidateUUID string
	Title         string
	Description   string
	City          string
	Salary        int
	Score         float64
	VacancyID     string
}

type MatchService struct {
	repo      *repository.Repository
	threshold float64
}

func NewMatchService(repo *repository.Repository, threshold float64) *MatchService {
	return &MatchService{repo: repo, threshold: threshold}
}

func (s *MatchService) Run(ctx context.Context) (created int, err error) {
	candidates, err := s.repo.ListCandidates(ctx)
	if err != nil {
		return 0, err
	}
	vacancies, err := s.repo.ListOpenVacancies(ctx)
	if err != nil {
		return 0, err
	}
	for _, v := range vacancies {
		for _, c := range candidates {
			exists, err := s.repo.MatchExists(ctx, v.ID, c.ID)
			if err != nil {
				return created, err
			}
			if exists {
				continue
			}
			score := matching.Match(c, v)
			if score < s.threshold {
				continue
			}
			if err := s.repo.CreateMatch(ctx, &models.Match{
				VacancyID:   v.ID,
				CandidateID: c.ID,
				Score:       score,
				Sent:        false,
			}); err != nil {
				return created, err
			}
			created++
		}
	}
	return created, nil
}

func (s *MatchService) Pending(ctx context.Context) ([]models.MatchView, error) {
	return s.repo.ListPendingMatches(ctx)
}

func (s *MatchService) Claim(ctx context.Context, matchID uuid.UUID) (*models.MatchView, error) {
	return s.repo.ClaimUnsentMatch(ctx, matchID)
}

type ApplicationService struct {
	repo *repository.Repository
}

func NewApplicationService(repo *repository.Repository) *ApplicationService {
	return &ApplicationService{repo: repo}
}

func (s *ApplicationService) Apply(ctx context.Context, vacancyID, candidateID uuid.UUID) (*models.ApplicationView, error) {
	v, err := s.repo.GetVacancy(ctx, vacancyID)
	if err != nil {
		return nil, err
	}
	if v == nil || v.Status != models.VacancyOpen {
		return nil, fmt.Errorf("vacancy is not open")
	}
	if !v.Collecting {
		return nil, fmt.Errorf("vacancy is not collecting")
	}
	employer, err := s.repo.GetUserByID(ctx, v.EmployerID)
	if err != nil {
		return nil, err
	}
	if employer != nil && employer.HiringPaused {
		return nil, fmt.Errorf("employer paused hiring")
	}
	candidate, err := s.repo.GetUserByID(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	if candidate != nil && !candidate.SearchActive {
		return nil, fmt.Errorf("candidate search paused")
	}
	if _, err := s.repo.CreateApplication(ctx, vacancyID, candidateID); err != nil {
		return nil, err
	}
	apps, err := s.repo.ListVacancyApplications(ctx, vacancyID)
	if err != nil {
		return nil, err
	}
	var result *models.ApplicationView
	for _, a := range apps {
		if a.CandidateID == candidateID {
			result = &a
			break
		}
	}
	if result == nil {
		return nil, fmt.Errorf("application not found")
	}
	if err := s.maybeAutoPauseVacancy(ctx, v); err != nil {
		return result, err
	}
	return result, nil
}

func (s *ApplicationService) maybeAutoPauseVacancy(ctx context.Context, v *models.Vacancy) error {
	count, err := s.repo.CountActiveApplications(ctx, v.ID)
	if err != nil {
		return err
	}
	if count < models.ApplicationLimit(v.NeededCount) {
		return nil
	}
	return s.repo.SetVacancyCollecting(ctx, v.ID, false)
}

func (s *ApplicationService) Accept(ctx context.Context, applicationID uuid.UUID) (*models.ApplicationView, error) {
	app, err := s.repo.GetApplication(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if app.Status == models.AppAccepted {
		return app, nil
	}
	if app.Status != models.AppSent {
		return nil, fmt.Errorf("application cannot be accepted")
	}
	if err := s.repo.UpdateApplicationStatus(ctx, applicationID, models.AppAccepted); err != nil {
		return nil, err
	}
	app.Status = models.AppAccepted
	return app, nil
}

func (s *ApplicationService) Hire(ctx context.Context, applicationID uuid.UUID) (*models.ApplicationView, *models.Vacancy, error) {
	app, err := s.repo.GetApplication(ctx, applicationID)
	if err != nil {
		return nil, nil, err
	}
	if app.Status != models.AppAccepted && app.Status != models.AppHired {
		return nil, nil, fmt.Errorf("application must be accepted first")
	}
	if app.Status == models.AppHired {
		v, err := s.repo.GetVacancy(ctx, app.VacancyID)
		return app, v, err
	}
	if err := s.repo.UpdateApplicationStatus(ctx, applicationID, models.AppHired); err != nil {
		return nil, nil, err
	}
	v, err := s.repo.IncrementFilledCount(ctx, app.VacancyID)
	if err != nil {
		return nil, nil, err
	}
	if err := s.repo.IncrementJobsCompleted(ctx, app.CandidateID); err != nil {
		return nil, nil, err
	}
	if err := s.repo.IncrementJobsCompleted(ctx, app.EmployerID); err != nil {
		return nil, nil, err
	}
	app.Status = models.AppHired
	return app, v, nil
}

func (s *ApplicationService) Get(ctx context.Context, applicationID uuid.UUID) (*models.ApplicationView, error) {
	return s.repo.GetApplication(ctx, applicationID)
}

func (s *ApplicationService) Reject(ctx context.Context, applicationID uuid.UUID) error {
	return s.repo.UpdateApplicationStatus(ctx, applicationID, models.AppRejected)
}

func (s *ApplicationService) ListByVacancy(ctx context.Context, vacancyID uuid.UUID) ([]models.ApplicationView, error) {
	return s.repo.ListVacancyApplications(ctx, vacancyID)
}

type VacancyService struct {
	repo *repository.Repository
}

func NewVacancyService(repo *repository.Repository) *VacancyService {
	return &VacancyService{repo: repo}
}

func (s *VacancyService) Create(ctx context.Context, v *models.Vacancy) error {
	v.Status = models.VacancyOpen
	if v.NeededCount <= 0 {
		v.NeededCount = 1
	}
	return s.repo.CreateVacancy(ctx, v)
}

func (s *VacancyService) Close(ctx context.Context, vacancyID uuid.UUID) error {
	return s.repo.CloseVacancy(ctx, vacancyID)
}

func (s *VacancyService) ListEmployer(ctx context.Context, employerID uuid.UUID) ([]models.Vacancy, error) {
	return s.repo.ListEmployerVacancies(ctx, employerID)
}

func (s *VacancyService) Get(ctx context.Context, id uuid.UUID) (*models.Vacancy, error) {
	return s.repo.GetVacancy(ctx, id)
}

func (s *VacancyService) MaybeResumeCollecting(ctx context.Context, vacancyID uuid.UUID) (bool, error) {
	v, err := s.repo.GetVacancy(ctx, vacancyID)
	if err != nil || v == nil {
		return false, err
	}
	if v.Status != models.VacancyOpen || v.Collecting || v.FilledCount >= v.NeededCount {
		return false, nil
	}
	if err := s.repo.SetVacancyCollecting(ctx, vacancyID, true); err != nil {
		return false, err
	}
	return true, nil
}

func (s *VacancyService) CountHired(ctx context.Context, vacancyID uuid.UUID) (int, error) {
	return s.repo.CountHiredApplications(ctx, vacancyID)
}

type UserService struct {
	repo *repository.Repository
}

func NewUserService(repo *repository.Repository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) GetByTgID(ctx context.Context, tgID int64) (*models.User, error) {
	return s.repo.GetUserByTgID(ctx, tgID)
}

func (s *UserService) Save(ctx context.Context, u *models.User) error {
	return s.repo.UpsertUser(ctx, u)
}

func (s *UserService) IncrementMatchesReceived(ctx context.Context, userID uuid.UUID) error {
	return s.repo.IncrementMatchesReceived(ctx, userID)
}

func (s *VacancyService) SetCollecting(ctx context.Context, vacancyID uuid.UUID, collecting bool) error {
	return s.repo.SetVacancyCollecting(ctx, vacancyID, collecting)
}

func (s *VacancyService) ActiveApplicationCount(ctx context.Context, vacancyID uuid.UUID) (int, error) {
	return s.repo.CountActiveApplications(ctx, vacancyID)
}

func (s *UserService) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	return s.repo.GetUserByID(ctx, id)
}

func (s *UserService) SetSearchActive(ctx context.Context, userID uuid.UUID, active bool) error {
	return s.repo.SetSearchActive(ctx, userID, active)
}

func (s *UserService) SetHiringPaused(ctx context.Context, userID uuid.UUID, paused bool) error {
	return s.repo.SetHiringPaused(ctx, userID, paused)
}

func (s *UserService) ToggleSearch(ctx context.Context, tgID int64) (bool, error) {
	u, err := s.repo.GetUserByTgID(ctx, tgID)
	if err != nil || u == nil {
		return false, err
	}
	next := !u.SearchActive
	if err := s.repo.SetSearchActive(ctx, u.ID, next); err != nil {
		return false, err
	}
	return next, nil
}

func (s *UserService) ToggleHiringPaused(ctx context.Context, tgID int64) (bool, error) {
	u, err := s.repo.GetUserByTgID(ctx, tgID)
	if err != nil || u == nil {
		return false, err
	}
	next := !u.HiringPaused
	if err := s.repo.SetHiringPaused(ctx, u.ID, next); err != nil {
		return false, err
	}
	return next, nil
}

func (s *UserService) Rating(ctx context.Context, userID uuid.UUID) (float64, int, error) {
	return s.repo.AverageRating(ctx, userID)
}

type ReviewService struct {
	repo *repository.Repository
}

func NewReviewService(repo *repository.Repository) *ReviewService {
	return &ReviewService{repo: repo}
}

func (s *ReviewService) Create(ctx context.Context, review *models.Review) error {
	return s.repo.CreateReview(ctx, review)
}

func (s *ReviewService) Has(ctx context.Context, fromUserID, toUserID, vacancyID uuid.UUID) (bool, error) {
	return s.repo.HasReview(ctx, fromUserID, toUserID, vacancyID)
}

type SessionService struct {
	repo *repository.Repository
}

func NewSessionService(repo *repository.Repository) *SessionService {
	return &SessionService{repo: repo}
}

func (s *SessionService) Get(ctx context.Context, tgID int64) (*models.Session, error) {
	return s.repo.GetSession(ctx, tgID)
}

func (s *SessionService) Save(ctx context.Context, session *models.Session) error {
	return s.repo.SaveSession(ctx, session)
}

func (s *SessionService) Clear(ctx context.Context, tgID int64) error {
	return s.repo.ClearSession(ctx, tgID)
}

func (s *SessionService) SetStep(ctx context.Context, tgID int64, step string, data map[string]string) error {
	if data == nil {
		data = map[string]string{}
	}
	return s.repo.SaveSession(ctx, &models.Session{
		TgID:      tgID,
		Step:      step,
		Data:      data,
		UpdatedAt: time.Now().UTC(),
	})
}
