package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleCandidate = "candidate"
	RoleEmployer  = "employer"

	VacancyOpen   = "open"
	VacancyClosed = "closed"

	AppSent      = "sent"
	AppAccepted  = "accepted"
	AppRejected  = "rejected"
	AppHired     = "hired"

	MatchThreshold = 60.0
)

type User struct {
	ID         uuid.UUID
	TgID       int64
	Username   string
	Role       string
	Name       string
	City       string
	Phone      string
	DesiredJob string
	CreatedAt  time.Time
}

type Vacancy struct {
	ID          uuid.UUID
	EmployerID  uuid.UUID
	Title       string
	Description string
	City        string
	Salary      int
	NeededCount int
	FilledCount int
	Status      string
	StartDate   *time.Time
	CreatedAt   time.Time
}

type Application struct {
	ID          uuid.UUID
	VacancyID   uuid.UUID
	CandidateID uuid.UUID
	Status      string
	CreatedAt   time.Time
}

type ApplicationView struct {
	Application
	CandidateName   string
	CandidateCity   string
	CandidatePhone  string
	CandidateJob    string
	CandidateTgID   int64
	CandidateUser   string
	VacancyTitle    string
	EmployerID      uuid.UUID
	EmployerTgID    int64
}

type Match struct {
	ID          uuid.UUID
	VacancyID   uuid.UUID
	CandidateID uuid.UUID
	Score       float64
	Sent        bool
	CreatedAt   time.Time
}

type MatchView struct {
	Match
	VacancyTitle       string
	VacancyDescription string
	VacancyCity        string
	VacancySalary      int
	CandidateTgID      int64
	EmployerTgID       int64
}

type Review struct {
	ID         uuid.UUID
	FromUserID uuid.UUID
	ToUserID   uuid.UUID
	VacancyID  uuid.UUID
	Rating     int
	Text       string
	CreatedAt  time.Time
}

type Session struct {
	TgID      int64
	Step      string
	Data      map[string]string
	UpdatedAt time.Time
}
