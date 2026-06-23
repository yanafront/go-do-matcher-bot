package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/anadubesko/go-do-matcher-bot/internal/models"
	"github.com/google/uuid"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetUserByTgID(ctx context.Context, tgID int64) (*models.User, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, tg_id, username, role, name, city, phone, desired_job, matches_received, jobs_completed, search_active, hiring_paused, created_at
FROM users WHERE tg_id = $1
`, tgID)
	return scanUser(row)
}

func (r *Repository) GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, tg_id, username, role, name, city, phone, desired_job, matches_received, jobs_completed, search_active, hiring_paused, created_at
FROM users WHERE id = $1
`, id)
	return scanUser(row)
}

func (r *Repository) UpsertUser(ctx context.Context, u *models.User) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO users (id, tg_id, username, role, name, city, phone, desired_job, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (tg_id) DO UPDATE SET
	username = EXCLUDED.username,
	role = CASE WHEN EXCLUDED.role <> '' THEN EXCLUDED.role ELSE users.role END,
	name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE users.name END,
	city = CASE WHEN EXCLUDED.city <> '' THEN EXCLUDED.city ELSE users.city END,
	phone = CASE WHEN EXCLUDED.phone <> '' THEN EXCLUDED.phone ELSE users.phone END,
	desired_job = CASE WHEN EXCLUDED.desired_job <> '' THEN EXCLUDED.desired_job ELSE users.desired_job END
`, u.ID, u.TgID, u.Username, u.Role, u.Name, u.City, u.Phone, u.DesiredJob, u.CreatedAt)
	return err
}

func (r *Repository) ListCandidates(ctx context.Context) ([]models.User, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, tg_id, username, role, name, city, phone, desired_job, matches_received, jobs_completed, search_active, hiring_paused, created_at
FROM users
WHERE role = 'candidate' AND name <> '' AND city <> '' AND desired_job <> '' AND phone <> '' AND search_active = TRUE
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

func (r *Repository) ListOpenVacancies(ctx context.Context) ([]models.Vacancy, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT v.id, v.employer_id, v.title, v.description, v.city, v.salary, v.needed_count, v.filled_count, v.status, v.collecting, v.start_date, v.created_at
FROM vacancies v
JOIN users u ON u.id = v.employer_id
WHERE v.status = 'open' AND v.collecting = TRUE AND u.hiring_paused = FALSE
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVacancies(rows)
}

func (r *Repository) GetVacancy(ctx context.Context, id uuid.UUID) (*models.Vacancy, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, employer_id, title, description, city, salary, needed_count, filled_count, status, collecting, start_date, created_at
FROM vacancies WHERE id = $1
`, id)
	return scanVacancy(row)
}

func (r *Repository) CreateVacancy(ctx context.Context, v *models.Vacancy) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	if v.Status == "" {
		v.Status = models.VacancyOpen
	}
	v.Collecting = true
	_, err := r.db.ExecContext(ctx, `
INSERT INTO vacancies (id, employer_id, title, description, city, salary, needed_count, filled_count, status, collecting, start_date, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
`, v.ID, v.EmployerID, v.Title, v.Description, v.City, v.Salary, v.NeededCount, v.FilledCount, v.Status, v.Collecting, v.StartDate, v.CreatedAt)
	return err
}

func (r *Repository) ListEmployerVacancies(ctx context.Context, employerID uuid.UUID) ([]models.Vacancy, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, employer_id, title, description, city, salary, needed_count, filled_count, status, collecting, start_date, created_at
FROM vacancies WHERE employer_id = $1 ORDER BY created_at DESC
`, employerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVacancies(rows)
}

func (r *Repository) CloseVacancy(ctx context.Context, vacancyID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE vacancies SET status = 'closed', collecting = FALSE WHERE id = $1`, vacancyID)
	return err
}

func (r *Repository) IncrementFilledCount(ctx context.Context, vacancyID uuid.UUID) (*models.Vacancy, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var v models.Vacancy
	row := tx.QueryRowContext(ctx, `
UPDATE vacancies SET filled_count = filled_count + 1
WHERE id = $1 AND status = 'open'
RETURNING id, employer_id, title, description, city, salary, needed_count, filled_count, status, collecting, start_date, created_at
`, vacancyID)
	v, err = scanVacancyRow(row)
	if err != nil {
		return nil, err
	}
	if v.FilledCount >= v.NeededCount {
		if _, err := tx.ExecContext(ctx, `UPDATE vacancies SET status = 'closed', collecting = FALSE WHERE id = $1`, vacancyID); err != nil {
			return nil, err
		}
		v.Status = models.VacancyClosed
		v.Collecting = false
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *Repository) MatchExists(ctx context.Context, vacancyID, candidateID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM matches WHERE vacancy_id = $1 AND candidate_id = $2)
`, vacancyID, candidateID).Scan(&exists)
	return exists, err
}

func (r *Repository) CreateMatch(ctx context.Context, m *models.Match) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO matches (id, vacancy_id, candidate_id, score, sent, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (vacancy_id, candidate_id) DO NOTHING
`, m.ID, m.VacancyID, m.CandidateID, m.Score, m.Sent, m.CreatedAt)
	return err
}

func (r *Repository) ClaimUnsentMatch(ctx context.Context, matchID uuid.UUID) (*models.MatchView, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `
UPDATE matches SET sent = TRUE
WHERE id = $1 AND sent = FALSE
RETURNING id, vacancy_id, candidate_id, score, sent, created_at
`, matchID)

	var m models.Match
	if err := row.Scan(&m.ID, &m.VacancyID, &m.CandidateID, &m.Score, &m.Sent, &m.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	view, err := r.loadMatchViewTx(ctx, tx, m)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return view, nil
}

func (r *Repository) ListPendingMatches(ctx context.Context) ([]models.MatchView, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT m.id, m.vacancy_id, m.candidate_id, m.score, m.sent, m.created_at,
       v.title, v.description, v.city, v.salary,
       c.tg_id, e.tg_id
FROM matches m
JOIN vacancies v ON v.id = m.vacancy_id
JOIN users c ON c.id = m.candidate_id AND c.search_active = TRUE
JOIN users e ON e.id = v.employer_id
WHERE m.sent = FALSE AND v.status = 'open' AND v.collecting = TRUE AND e.hiring_paused = FALSE
ORDER BY m.score DESC, m.created_at ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.MatchView
	for rows.Next() {
		var mv models.MatchView
		if err := rows.Scan(
			&mv.ID, &mv.VacancyID, &mv.CandidateID, &mv.Score, &mv.Sent, &mv.CreatedAt,
			&mv.VacancyTitle, &mv.VacancyDescription, &mv.VacancyCity, &mv.VacancySalary,
			&mv.CandidateTgID, &mv.EmployerTgID,
		); err != nil {
			return nil, err
		}
		out = append(out, mv)
	}
	return out, rows.Err()
}

func (r *Repository) CreateApplication(ctx context.Context, vacancyID, candidateID uuid.UUID) (*models.Application, error) {
	app := &models.Application{
		ID:          uuid.New(),
		VacancyID:   vacancyID,
		CandidateID: candidateID,
		Status:      models.AppSent,
		CreatedAt:   time.Now().UTC(),
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO applications (id, vacancy_id, candidate_id, status, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (vacancy_id, candidate_id) DO UPDATE SET status = 'sent'
`, app.ID, app.VacancyID, app.CandidateID, app.Status, app.CreatedAt)
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, vacancy_id, candidate_id, status, created_at
FROM applications WHERE vacancy_id = $1 AND candidate_id = $2
`, vacancyID, candidateID)
	return scanApplication(row)
}

func (r *Repository) GetApplication(ctx context.Context, id uuid.UUID) (*models.ApplicationView, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT a.id, a.vacancy_id, a.candidate_id, a.status, a.created_at,
       c.name, c.city, c.phone, c.desired_job, c.tg_id, c.username,
       v.title, v.employer_id, e.tg_id
FROM applications a
JOIN users c ON c.id = a.candidate_id
JOIN vacancies v ON v.id = a.vacancy_id
JOIN users e ON e.id = v.employer_id
WHERE a.id = $1
`, id)
	var av models.ApplicationView
	err := row.Scan(
		&av.ID, &av.VacancyID, &av.CandidateID, &av.Status, &av.CreatedAt,
		&av.CandidateName, &av.CandidateCity, &av.CandidatePhone, &av.CandidateJob,
		&av.CandidateTgID, &av.CandidateUser,
		&av.VacancyTitle, &av.EmployerID, &av.EmployerTgID,
	)
	if err != nil {
		return nil, err
	}
	return &av, nil
}

func (r *Repository) ListVacancyApplications(ctx context.Context, vacancyID uuid.UUID) ([]models.ApplicationView, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT a.id, a.vacancy_id, a.candidate_id, a.status, a.created_at,
       c.name, c.city, c.phone, c.desired_job, c.tg_id, c.username,
       v.title, v.employer_id, e.tg_id
FROM applications a
JOIN users c ON c.id = a.candidate_id
JOIN vacancies v ON v.id = a.vacancy_id
JOIN users e ON e.id = v.employer_id
WHERE a.vacancy_id = $1 AND a.status IN ('sent', 'accepted', 'hired')
ORDER BY a.created_at DESC
`, vacancyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ApplicationView
	for rows.Next() {
		var av models.ApplicationView
		if err := rows.Scan(
			&av.ID, &av.VacancyID, &av.CandidateID, &av.Status, &av.CreatedAt,
			&av.CandidateName, &av.CandidateCity, &av.CandidatePhone, &av.CandidateJob,
			&av.CandidateTgID, &av.CandidateUser,
			&av.VacancyTitle, &av.EmployerID, &av.EmployerTgID,
		); err != nil {
			return nil, err
		}
		out = append(out, av)
	}
	return out, rows.Err()
}

func (r *Repository) CountHiredApplications(ctx context.Context, vacancyID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM applications WHERE vacancy_id = $1 AND status = 'hired'
`, vacancyID).Scan(&n)
	return n, err
}

func (r *Repository) UpdateApplicationStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE applications SET status = $2 WHERE id = $1`, id, status)
	return err
}

func (r *Repository) CreateReview(ctx context.Context, review *models.Review) error {
	if review.ID == uuid.Nil {
		review.ID = uuid.New()
	}
	if review.CreatedAt.IsZero() {
		review.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO reviews (id, from_user_id, to_user_id, vacancy_id, rating, text, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (from_user_id, to_user_id, vacancy_id) DO UPDATE SET rating = EXCLUDED.rating, text = EXCLUDED.text
`, review.ID, review.FromUserID, review.ToUserID, review.VacancyID, review.Rating, review.Text, review.CreatedAt)
	return err
}

func (r *Repository) AverageRating(ctx context.Context, userID uuid.UUID) (float64, int, error) {
	var avg sql.NullFloat64
	var count int
	err := r.db.QueryRowContext(ctx, `
SELECT COALESCE(AVG(rating)::float, 0), COUNT(*)
FROM reviews WHERE to_user_id = $1
`, userID).Scan(&avg, &count)
	if err != nil {
		return 0, 0, err
	}
	if !avg.Valid {
		return 0, count, nil
	}
	return avg.Float64, count, nil
}

func (r *Repository) HasReview(ctx context.Context, fromUserID, toUserID, vacancyID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM reviews WHERE from_user_id = $1 AND to_user_id = $2 AND vacancy_id = $3)
`, fromUserID, toUserID, vacancyID).Scan(&exists)
	return exists, err
}

func (r *Repository) GetSession(ctx context.Context, tgID int64) (*models.Session, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT tg_id, step, data, updated_at FROM bot_sessions WHERE tg_id = $1
`, tgID)
	var s models.Session
	var raw []byte
	if err := row.Scan(&s.TgID, &s.Step, &raw, &s.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	s.Data = map[string]string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &s.Data)
	}
	return &s, nil
}

func (r *Repository) SaveSession(ctx context.Context, s *models.Session) error {
	raw, err := json.Marshal(s.Data)
	if err != nil {
		return err
	}
	s.UpdatedAt = time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `
INSERT INTO bot_sessions (tg_id, step, data, updated_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (tg_id) DO UPDATE SET step = EXCLUDED.step, data = EXCLUDED.data, updated_at = EXCLUDED.updated_at
`, s.TgID, s.Step, raw, s.UpdatedAt)
	return err
}

func (r *Repository) ClearSession(ctx context.Context, tgID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM bot_sessions WHERE tg_id = $1`, tgID)
	return err
}

func (r *Repository) loadMatchViewTx(ctx context.Context, tx *sql.Tx, m models.Match) (*models.MatchView, error) {
	row := tx.QueryRowContext(ctx, `
SELECT m.id, m.vacancy_id, m.candidate_id, m.score, m.sent, m.created_at,
       v.title, v.description, v.city, v.salary,
       c.tg_id, e.tg_id
FROM matches m
JOIN vacancies v ON v.id = m.vacancy_id
JOIN users c ON c.id = m.candidate_id
JOIN users e ON e.id = v.employer_id
WHERE m.id = $1
`, m.ID)
	var mv models.MatchView
	err := row.Scan(
		&mv.ID, &mv.VacancyID, &mv.CandidateID, &mv.Score, &mv.Sent, &mv.CreatedAt,
		&mv.VacancyTitle, &mv.VacancyDescription, &mv.VacancyCity, &mv.VacancySalary,
		&mv.CandidateTgID, &mv.EmployerTgID,
	)
	if err != nil {
		return nil, err
	}
	return &mv, nil
}

func (r *Repository) SetSearchActive(ctx context.Context, userID uuid.UUID, active bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET search_active = $2 WHERE id = $1`, userID, active)
	return err
}

func (r *Repository) SetHiringPaused(ctx context.Context, userID uuid.UUID, paused bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET hiring_paused = $2 WHERE id = $1`, userID, paused)
	return err
}

func (r *Repository) SetVacancyCollecting(ctx context.Context, vacancyID uuid.UUID, collecting bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE vacancies SET collecting = $2 WHERE id = $1`, vacancyID, collecting)
	return err
}

func (r *Repository) CountActiveApplications(ctx context.Context, vacancyID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM applications
WHERE vacancy_id = $1 AND status IN ('sent', 'accepted')
`, vacancyID).Scan(&n)
	return n, err
}

func (r *Repository) IncrementMatchesReceived(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE users SET matches_received = matches_received + 1 WHERE id = $1
`, userID)
	return err
}

func (r *Repository) IncrementJobsCompleted(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE users SET jobs_completed = jobs_completed + 1 WHERE id = $1
`, userID)
	return err
}

func scanUser(row *sql.Row) (*models.User, error) {
	var u models.User
	err := row.Scan(
		&u.ID, &u.TgID, &u.Username, &u.Role, &u.Name, &u.City, &u.Phone, &u.DesiredJob,
		&u.MatchesReceived, &u.JobsCompleted, &u.SearchActive, &u.HiringPaused, &u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func scanUsers(rows *sql.Rows) ([]models.User, error) {
	var out []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.TgID, &u.Username, &u.Role, &u.Name, &u.City, &u.Phone, &u.DesiredJob,
			&u.MatchesReceived, &u.JobsCompleted, &u.SearchActive, &u.HiringPaused, &u.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func scanVacancy(row *sql.Row) (*models.Vacancy, error) {
	v, err := scanVacancyRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func scanVacancyRow(row interface {
	Scan(dest ...any) error
}) (models.Vacancy, error) {
	var v models.Vacancy
	var start sql.NullTime
	err := row.Scan(&v.ID, &v.EmployerID, &v.Title, &v.Description, &v.City, &v.Salary, &v.NeededCount, &v.FilledCount, &v.Status, &v.Collecting, &start, &v.CreatedAt)
	if err != nil {
		return v, err
	}
	if start.Valid {
		t := start.Time
		v.StartDate = &t
	}
	return v, nil
}

func scanVacancies(rows *sql.Rows) ([]models.Vacancy, error) {
	var out []models.Vacancy
	for rows.Next() {
		v, err := scanVacancyRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanApplication(row *sql.Row) (*models.Application, error) {
	var a models.Application
	err := row.Scan(&a.ID, &a.VacancyID, &a.CandidateID, &a.Status, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
