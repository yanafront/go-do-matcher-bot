package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/anadubesko/go-do-matcher-bot/internal/models"
	"github.com/anadubesko/go-do-matcher-bot/internal/services"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Handler struct {
	api     *tgbotapi.BotAPI
	users   *services.UserService
	vacancy *services.VacancyService
	apps    *services.ApplicationService
	reviews *services.ReviewService
	session *services.SessionService
	log     *zap.Logger
}

func New(
	api *tgbotapi.BotAPI,
	users *services.UserService,
	vacancy *services.VacancyService,
	apps *services.ApplicationService,
	reviews *services.ReviewService,
	session *services.SessionService,
	log *zap.Logger,
) *Handler {
	return &Handler{
		api:     api,
		users:   users,
		vacancy: vacancy,
		apps:    apps,
		reviews: reviews,
		session: session,
		log:     log,
	}
}

func (h *Handler) HandleUpdate(ctx context.Context, upd tgbotapi.Update) {
	if upd.CallbackQuery != nil {
		h.handleCallback(ctx, upd.CallbackQuery)
		return
	}
	if upd.Message == nil {
		return
	}
	msg := upd.Message
	if msg.Chat == nil || msg.Chat.IsChannel() {
		return
	}
	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			h.handleStart(ctx, msg)
		case "menu":
			h.handleMenu(ctx, msg)
		}
		return
	}
	h.handleMessage(ctx, msg)
}

func (h *Handler) SendMatch(ctx context.Context, n services.MatchNotification) error {
	_ = ctx
	text := fmt.Sprintf(
		"Новая вакансия для тебя (score %.0f)\n\n<b>%s</b>\n%s\n\nГород: %s\nЗарплата: %d",
		n.Score,
		escape(n.Title),
		escape(n.Description),
		escape(n.City),
		n.Salary,
	)
	m := tgbotapi.NewMessage(n.CandidateID, text)
	m.ParseMode = tgbotapi.ModeHTML
	m.ReplyMarkup = candidateMatchKeyboard(n.VacancyID)
	_, err := h.api.Send(m)
	return err
}

func (h *Handler) handleStart(ctx context.Context, msg *tgbotapi.Message) {
	user, _ := h.ensureUser(ctx, msg)
	if user != nil && user.Role != "" && h.profileComplete(user) {
		h.sendRoleHome(ctx, msg.Chat.ID, user)
		return
	}
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepChooseRole, nil)
	h.sendText(msg.Chat.ID, "Добро пожаловать на job marketplace.\nВыбери роль:", roleKeyboard())
}

func (h *Handler) handleMenu(ctx context.Context, msg *tgbotapi.Message) {
	user, err := h.users.GetByTgID(ctx, msg.Chat.ID)
	if err != nil || user == nil || user.Role == "" {
		h.handleStart(ctx, msg)
		return
	}
	h.sendRoleHome(ctx, msg.Chat.ID, user)
}

func (h *Handler) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	if sess == nil || sess.Step == "" {
		user, _ := h.users.GetByTgID(ctx, msg.Chat.ID)
		if user != nil && user.Role != "" {
			h.sendText(msg.Chat.ID, "Используй /menu", nil)
		} else {
			h.handleStart(ctx, msg)
		}
		return
	}

	if msg.Contact != nil && sess.Step == StepCandidatePhone {
		h.finishCandidatePhone(ctx, msg, msg.Contact.PhoneNumber)
		return
	}

	text := strings.TrimSpace(msg.Text)
	switch sess.Step {
	case StepCandidateName:
		h.saveCandidateName(ctx, msg, text)
	case StepCandidateCity:
		h.saveCandidateCity(ctx, msg, text)
	case StepCandidateJob:
		h.saveCandidateJob(ctx, msg, text)
	case StepCandidatePhone:
		h.sendText(msg.Chat.ID, "Нажми кнопку, чтобы отправить телефон.", contactKeyboard())
	case StepEmployerName:
		h.saveEmployerName(ctx, msg, text)
	case StepEmployerCity:
		h.saveEmployerCity(ctx, msg, text)
	case StepEmployerJob:
		h.saveEmployerJob(ctx, msg, text)
	case StepEmployerSalary:
		h.saveEmployerSalary(ctx, msg, text)
	case StepEmployerStart:
		h.saveEmployerStart(ctx, msg, text)
	case StepEmployerNeeded:
		h.saveEmployerNeeded(ctx, msg, text)
	case StepVacancyTitle:
		h.saveVacancyTitle(ctx, msg, text)
	case StepVacancyDesc:
		h.saveVacancyDesc(ctx, msg, text)
	case StepVacancyCity:
		h.saveVacancyCity(ctx, msg, text)
	case StepVacancySalary:
		h.saveVacancySalary(ctx, msg, text)
	case StepVacancyNeeded:
		h.saveVacancyNeeded(ctx, msg, text)
	case StepVacancyStart:
		h.saveVacancyStart(ctx, msg, text)
	case StepReviewText:
		h.saveReviewText(ctx, msg, text)
	default:
		h.handleStart(ctx, msg)
	}
}

func (h *Handler) handleCallback(ctx context.Context, q *tgbotapi.CallbackQuery) {
	if q.Message == nil {
		return
	}
	chatID := q.Message.Chat.ID
	data := q.Data
	_ = h.answer(q.ID, "")

	switch {
	case data == "role:candidate":
		h.startCandidate(ctx, chatID, q.From)
	case data == "role:employer":
		h.startEmployer(ctx, chatID, q.From)
	case strings.HasPrefix(data, "apply:"):
		h.handleApply(ctx, chatID, strings.TrimPrefix(data, "apply:"))
	case strings.HasPrefix(data, "skip:"):
		h.sendText(chatID, "Пропущено.", nil)
	case strings.HasPrefix(data, "hire:"):
		h.handleHire(ctx, chatID, strings.TrimPrefix(data, "hire:"))
	case strings.HasPrefix(data, "reject:"):
		h.handleReject(ctx, chatID, strings.TrimPrefix(data, "reject:"))
	case strings.HasPrefix(data, "close:"):
		h.handleCloseVacancy(ctx, chatID, strings.TrimPrefix(data, "close:"))
	case data == "menu:create_vacancy":
		h.beginVacancyCreation(ctx, chatID)
	case data == "menu:my_vacancies":
		h.showMyVacancies(ctx, chatID)
	case strings.HasPrefix(data, "menu:applicants:"):
		h.showApplicants(ctx, chatID, strings.TrimPrefix(data, "menu:applicants:"))
	case strings.HasPrefix(data, "review:"):
		h.handleReviewRating(ctx, chatID, strings.TrimPrefix(data, "review:"))
	case strings.HasPrefix(data, "skip_review:"):
		_ = h.session.Clear(ctx, chatID)
		h.sendText(chatID, "Отзыв пропущен.", nil)
	}
}

func (h *Handler) startCandidate(ctx context.Context, chatID int64, from *tgbotapi.User) {
	user := &models.User{TgID: chatID, Role: models.RoleCandidate}
	if from != nil {
		user.Username = from.UserName
	}
	_ = h.users.Save(ctx, user)
	_ = h.session.SetStep(ctx, chatID, StepCandidateName, nil)
	h.sendText(chatID, "Как тебя зовут?", nil)
}

func (h *Handler) startEmployer(ctx context.Context, chatID int64, from *tgbotapi.User) {
	user := &models.User{TgID: chatID, Role: models.RoleEmployer}
	if from != nil {
		user.Username = from.UserName
	}
	_ = h.users.Save(ctx, user)
	_ = h.session.SetStep(ctx, chatID, StepEmployerName, nil)
	h.sendText(chatID, "Название компании или твоё имя:", nil)
}

func (h *Handler) saveCandidateName(ctx context.Context, msg *tgbotapi.Message, name string) {
	if name == "" {
		h.sendText(msg.Chat.ID, "Введи имя.", nil)
		return
	}
	user, err := h.users.GetByTgID(ctx, msg.Chat.ID)
	if err != nil || user == nil {
		h.handleStart(ctx, msg)
		return
	}
	user.Name = name
	_ = h.users.Save(ctx, user)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepCandidateCity, nil)
	h.sendText(msg.Chat.ID, "В каком городе ищешь работу?", nil)
}

func (h *Handler) saveCandidateCity(ctx context.Context, msg *tgbotapi.Message, city string) {
	if city == "" {
		h.sendText(msg.Chat.ID, "Введи город.", nil)
		return
	}
	user, err := h.users.GetByTgID(ctx, msg.Chat.ID)
	if err != nil || user == nil {
		h.handleStart(ctx, msg)
		return
	}
	user.City = city
	_ = h.users.Save(ctx, user)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepCandidateJob, nil)
	h.sendText(msg.Chat.ID, "Какую должность ищешь?", nil)
}

func (h *Handler) saveCandidateJob(ctx context.Context, msg *tgbotapi.Message, job string) {
	if job == "" {
		h.sendText(msg.Chat.ID, "Введи должность.", nil)
		return
	}
	user, err := h.users.GetByTgID(ctx, msg.Chat.ID)
	if err != nil || user == nil {
		h.handleStart(ctx, msg)
		return
	}
	user.DesiredJob = job
	_ = h.users.Save(ctx, user)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepCandidatePhone, nil)
	h.sendText(msg.Chat.ID, "Отправь телефон кнопкой ниже.", contactKeyboard())
}

func (h *Handler) finishCandidatePhone(ctx context.Context, msg *tgbotapi.Message, phone string) {
	user, err := h.users.GetByTgID(ctx, msg.Chat.ID)
	if err != nil || user == nil {
		h.handleStart(ctx, msg)
		return
	}
	user.Phone = phone
	_ = h.users.Save(ctx, user)
	_ = h.session.Clear(ctx, msg.Chat.ID)
	h.sendText(msg.Chat.ID, "Профиль готов. Буду присылать подходящие вакансии.", removeKeyboard())
	h.sendRoleHome(ctx, msg.Chat.ID, user)
}

func (h *Handler) saveEmployerName(ctx context.Context, msg *tgbotapi.Message, name string) {
	if name == "" {
		h.sendText(msg.Chat.ID, "Введи название.", nil)
		return
	}
	user, _ := h.users.GetByTgID(ctx, msg.Chat.ID)
	user.Name = name
	_ = h.users.Save(ctx, user)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepEmployerCity, nil)
	h.sendText(msg.Chat.ID, "Город (рекомендуется):", nil)
}

func (h *Handler) saveEmployerCity(ctx context.Context, msg *tgbotapi.Message, city string) {
	user, _ := h.users.GetByTgID(ctx, msg.Chat.ID)
	user.City = city
	_ = h.users.Save(ctx, user)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepEmployerJob, nil)
	h.sendText(msg.Chat.ID, "Кого нанимаешь? (должность)", nil)
}

func (h *Handler) saveEmployerJob(ctx context.Context, msg *tgbotapi.Message, job string) {
	if job == "" {
		h.sendText(msg.Chat.ID, "Введи должность.", nil)
		return
	}
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	if sess.Data == nil {
		sess.Data = map[string]string{}
	}
	sess.Data["job"] = job
	_ = h.session.Save(ctx, sess)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepEmployerSalary, sess.Data)
	h.sendText(msg.Chat.ID, "Зарплата (число):", nil)
}

func (h *Handler) saveEmployerSalary(ctx context.Context, msg *tgbotapi.Message, raw string) {
	salary, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || salary < 0 {
		h.sendText(msg.Chat.ID, "Введи число.", nil)
		return
	}
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	sess.Data["salary"] = strconv.Itoa(salary)
	_ = h.session.Save(ctx, sess)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepEmployerStart, sess.Data)
	h.sendText(msg.Chat.ID, "Дата начала (ГГГГ-ММ-ДД):", nil)
}

func (h *Handler) saveEmployerStart(ctx context.Context, msg *tgbotapi.Message, raw string) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		h.sendText(msg.Chat.ID, "Формат: 2026-06-17", nil)
		return
	}
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	sess.Data["start_date"] = t.Format(time.RFC3339)
	_ = h.session.Save(ctx, sess)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepEmployerNeeded, sess.Data)
	h.sendText(msg.Chat.ID, "Сколько человек нужно?", nil)
}

func (h *Handler) saveEmployerNeeded(ctx context.Context, msg *tgbotapi.Message, raw string) {
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || count <= 0 {
		h.sendText(msg.Chat.ID, "Введи число больше 0.", nil)
		return
	}
	user, _ := h.users.GetByTgID(ctx, msg.Chat.ID)
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	salary, _ := strconv.Atoi(sess.Data["salary"])
	start, _ := time.Parse(time.RFC3339, sess.Data["start_date"])
	v := &models.Vacancy{
		EmployerID:  user.ID,
		Title:       sess.Data["job"],
		Description: fmt.Sprintf("Вакансия от %s", user.Name),
		City:        user.City,
		Salary:      salary,
		NeededCount: count,
		StartDate:   &start,
	}
	if err := h.vacancy.Create(ctx, v); err != nil {
		h.log.Warn("create vacancy", zap.Error(err))
		h.sendText(msg.Chat.ID, "Ошибка создания вакансии.", nil)
		return
	}
	_ = h.session.Clear(ctx, msg.Chat.ID)
	h.sendText(msg.Chat.ID, "Вакансия опубликована.", employerMenuKeyboard())
}

func (h *Handler) beginVacancyCreation(ctx context.Context, chatID int64) {
	_ = h.session.SetStep(ctx, chatID, StepVacancyTitle, map[string]string{})
	h.sendText(chatID, "Название вакансии:", nil)
}

func (h *Handler) saveVacancyTitle(ctx context.Context, msg *tgbotapi.Message, title string) {
	if title == "" {
		h.sendText(msg.Chat.ID, "Введи название.", nil)
		return
	}
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	sess.Data["title"] = title
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepVacancyDesc, sess.Data)
	h.sendText(msg.Chat.ID, "Описание:", nil)
}

func (h *Handler) saveVacancyDesc(ctx context.Context, msg *tgbotapi.Message, desc string) {
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	sess.Data["description"] = desc
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepVacancyCity, sess.Data)
	h.sendText(msg.Chat.ID, "Город:", nil)
}

func (h *Handler) saveVacancyCity(ctx context.Context, msg *tgbotapi.Message, city string) {
	if city == "" {
		h.sendText(msg.Chat.ID, "Введи город.", nil)
		return
	}
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	sess.Data["city"] = city
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepVacancySalary, sess.Data)
	h.sendText(msg.Chat.ID, "Зарплата:", nil)
}

func (h *Handler) saveVacancySalary(ctx context.Context, msg *tgbotapi.Message, raw string) {
	salary, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || salary < 0 {
		h.sendText(msg.Chat.ID, "Введи число.", nil)
		return
	}
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	sess.Data["salary"] = strconv.Itoa(salary)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepVacancyNeeded, sess.Data)
	h.sendText(msg.Chat.ID, "Сколько человек нужно?", nil)
}

func (h *Handler) saveVacancyNeeded(ctx context.Context, msg *tgbotapi.Message, raw string) {
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || count <= 0 {
		h.sendText(msg.Chat.ID, "Введи число больше 0.", nil)
		return
	}
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	sess.Data["needed"] = strconv.Itoa(count)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepVacancyStart, sess.Data)
	h.sendText(msg.Chat.ID, "Дата начала (ГГГГ-ММ-ДД):", nil)
}

func (h *Handler) saveVacancyStart(ctx context.Context, msg *tgbotapi.Message, raw string) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		h.sendText(msg.Chat.ID, "Формат: 2026-06-17", nil)
		return
	}
	user, _ := h.users.GetByTgID(ctx, msg.Chat.ID)
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	salary, _ := strconv.Atoi(sess.Data["salary"])
	needed, _ := strconv.Atoi(sess.Data["needed"])
	v := &models.Vacancy{
		EmployerID:  user.ID,
		Title:       sess.Data["title"],
		Description: sess.Data["description"],
		City:        sess.Data["city"],
		Salary:      salary,
		NeededCount: needed,
		StartDate:   &t,
	}
	if err := h.vacancy.Create(ctx, v); err != nil {
		h.log.Warn("create vacancy", zap.Error(err))
		h.sendText(msg.Chat.ID, "Ошибка.", nil)
		return
	}
	_ = h.session.Clear(ctx, msg.Chat.ID)
	h.sendText(msg.Chat.ID, "Вакансия создана.", employerMenuKeyboard())
}

func (h *Handler) handleApply(ctx context.Context, chatID int64, vacancyIDRaw string) {
	vacancyID, err := uuid.Parse(vacancyIDRaw)
	if err != nil {
		return
	}
	user, err := h.users.GetByTgID(ctx, chatID)
	if err != nil || user == nil {
		return
	}
	app, err := h.apps.Apply(ctx, vacancyID, user.ID)
	if err != nil {
		h.sendText(chatID, "Не удалось откликнуться.", nil)
		return
	}
	h.sendText(chatID, "Отклик отправлен работодателю.", nil)
	body := fmt.Sprintf(
		"Новый кандидат на «%s»\n\nИмя: %s\nГород: %s\nТелефон: %s\nДолжность: %s\nTG: @%s",
		app.VacancyTitle,
		app.CandidateName,
		app.CandidateCity,
		app.CandidatePhone,
		app.CandidateJob,
		app.CandidateUser,
	)
	m := tgbotapi.NewMessage(app.EmployerTgID, body)
	m.ReplyMarkup = applicantActionsKeyboard(app.ID.String())
	_, _ = h.api.Send(m)
}

func (h *Handler) handleHire(ctx context.Context, chatID int64, applicationIDRaw string) {
	applicationID, err := uuid.Parse(applicationIDRaw)
	if err != nil {
		return
	}
	app, vacancy, err := h.apps.Hire(ctx, applicationID)
	if err != nil {
		h.sendText(chatID, "Ошибка найма.", nil)
		return
	}
	h.sendText(chatID, fmt.Sprintf("Кандидат %s нанят.", app.CandidateName), nil)
	h.sendText(app.CandidateTgID, fmt.Sprintf("Тебя наняли на «%s».", app.VacancyTitle), nil)
	if vacancy.Status == models.VacancyClosed {
		h.sendText(chatID, "Вакансия закрыта — набрано нужное количество.", nil)
		h.promptReviews(ctx, app, vacancy)
	}
}

func (h *Handler) handleReject(ctx context.Context, chatID int64, applicationIDRaw string) {
	applicationID, err := uuid.Parse(applicationIDRaw)
	if err != nil {
		return
	}
	if err := h.apps.Reject(ctx, applicationID); err != nil {
		h.sendText(chatID, "Ошибка.", nil)
		return
	}
	h.sendText(chatID, "Кандидат отклонён.", nil)
}

func (h *Handler) handleCloseVacancy(ctx context.Context, chatID int64, vacancyIDRaw string) {
	vacancyID, err := uuid.Parse(vacancyIDRaw)
	if err != nil {
		return
	}
	employer, _ := h.users.GetByTgID(ctx, chatID)
	v, err := h.vacancy.Get(ctx, vacancyID)
	if err != nil || v == nil || employer == nil || v.EmployerID != employer.ID {
		h.sendText(chatID, "Вакансия не найдена.", nil)
		return
	}
	if err := h.vacancy.Close(ctx, vacancyID); err != nil {
		h.sendText(chatID, "Ошибка.", nil)
		return
	}
	h.sendText(chatID, "Вакансия закрыта.", employerMenuKeyboard())
}

func (h *Handler) showMyVacancies(ctx context.Context, chatID int64) {
	user, _ := h.users.GetByTgID(ctx, chatID)
	items, err := h.vacancy.ListEmployer(ctx, user.ID)
	if err != nil || len(items) == 0 {
		h.sendText(chatID, "Нет вакансий.", employerMenuKeyboard())
		return
	}
	for _, v := range items {
		text := fmt.Sprintf("«%s» — %s, %d BYN, %s (%d/%d)", v.Title, v.City, v.Salary, v.Status, v.FilledCount, v.NeededCount)
		h.sendText(chatID, text, vacancyActionsKeyboard(v.ID.String()))
	}
}

func (h *Handler) showApplicants(ctx context.Context, chatID int64, vacancyIDRaw string) {
	vacancyID, err := uuid.Parse(vacancyIDRaw)
	if err != nil {
		return
	}
	employer, _ := h.users.GetByTgID(ctx, chatID)
	v, _ := h.vacancy.Get(ctx, vacancyID)
	if v == nil || employer == nil || v.EmployerID != employer.ID {
		return
	}
	apps, err := h.apps.ListByVacancy(ctx, vacancyID)
	if err != nil || len(apps) == 0 {
		h.sendText(chatID, "Пока нет откликов.", vacancyActionsKeyboard(vacancyIDRaw))
		return
	}
	for _, a := range apps {
		text := fmt.Sprintf("%s — %s, %s, %s", a.CandidateName, a.CandidateCity, a.CandidatePhone, a.Status)
		h.sendText(chatID, text, applicantActionsKeyboard(a.ID.String()))
	}
}

func (h *Handler) promptReviews(ctx context.Context, app *models.ApplicationView, vacancy *models.Vacancy) {
	employer, _ := h.users.GetByTgID(ctx, app.EmployerTgID)
	candidate, _ := h.users.GetByTgID(ctx, app.CandidateTgID)
	if employer == nil || candidate == nil {
		return
	}
	h.sendText(app.EmployerTgID, "Оставь отзыв о кандидате (1-10):", reviewRatingKeyboard(vacancy.ID.String(), candidate.ID.String()))
	h.sendText(app.CandidateTgID, "Оставь отзыв о работодателе (1-10):", reviewRatingKeyboard(vacancy.ID.String(), employer.ID.String()))
}

func (h *Handler) handleReviewRating(ctx context.Context, chatID int64, payload string) {
	parts := strings.Split(payload, ":")
	if len(parts) != 3 {
		return
	}
	vacancyID, err := uuid.Parse(parts[0])
	if err != nil {
		return
	}
	toUserID, err := uuid.Parse(parts[1])
	if err != nil {
		return
	}
	rating, err := strconv.Atoi(parts[2])
	if err != nil || rating < 1 || rating > 10 {
		return
	}
	from, _ := h.users.GetByTgID(ctx, chatID)
	if from == nil {
		return
	}
	_ = h.session.SetStep(ctx, chatID, StepReviewText, map[string]string{
		"vacancy_id": vacancyID.String(),
		"to_user_id": toUserID.String(),
		"rating":     strconv.Itoa(rating),
	})
	h.sendText(chatID, "Комментарий (или «-» чтобы пропустить):", nil)
}

func (h *Handler) saveReviewText(ctx context.Context, msg *tgbotapi.Message, text string) {
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	from, _ := h.users.GetByTgID(ctx, msg.Chat.ID)
	vacancyID, _ := uuid.Parse(sess.Data["vacancy_id"])
	toUserID, _ := uuid.Parse(sess.Data["to_user_id"])
	rating, _ := strconv.Atoi(sess.Data["rating"])
	if text == "-" {
		text = ""
	}
	if err := h.reviews.Create(ctx, &models.Review{
		FromUserID: from.ID,
		ToUserID:   toUserID,
		VacancyID:  vacancyID,
		Rating:     rating,
		Text:       text,
	}); err != nil {
		h.sendText(msg.Chat.ID, "Не удалось сохранить отзыв.", nil)
		return
	}
	avg, count, _ := h.users.Rating(ctx, toUserID)
	_ = h.session.Clear(ctx, msg.Chat.ID)
	h.sendText(msg.Chat.ID, fmt.Sprintf("Спасибо! Рейтинг пользователя: %.1f (%d отзывов)", avg, count), nil)
}

func (h *Handler) sendRoleHome(ctx context.Context, chatID int64, user *models.User) {
	avg, count, _ := h.users.Rating(ctx, user.ID)
	switch user.Role {
	case models.RoleCandidate:
		h.sendText(chatID, fmt.Sprintf("Соискатель: %s\nГород: %s\nЗапрос: %s\nРейтинг: %.1f (%d)", user.Name, user.City, user.DesiredJob, avg, count), nil)
	case models.RoleEmployer:
		h.sendText(chatID, fmt.Sprintf("Работодатель: %s\nГород: %s\nРейтинг: %.1f (%d)", user.Name, user.City, avg, count), employerMenuKeyboard())
	}
}

func (h *Handler) ensureUser(ctx context.Context, msg *tgbotapi.Message) (*models.User, error) {
	existing, err := h.users.GetByTgID(ctx, msg.Chat.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if msg.From != nil {
			existing.Username = msg.From.UserName
			_ = h.users.Save(ctx, existing)
		}
		return existing, nil
	}
	u := &models.User{TgID: msg.Chat.ID}
	if msg.From != nil {
		u.Username = msg.From.UserName
	}
	if err := h.users.Save(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (h *Handler) profileComplete(u *models.User) bool {
	switch u.Role {
	case models.RoleCandidate:
		return u.Name != "" && u.City != "" && u.DesiredJob != "" && u.Phone != ""
	case models.RoleEmployer:
		return u.Name != ""
	default:
		return false
	}
}

func (h *Handler) sendText(chatID int64, text string, markup interface{}) {
	m := tgbotapi.NewMessage(chatID, text)
	if markup != nil {
		m.ReplyMarkup = markup
	}
	_, _ = h.api.Send(m)
}

func (h *Handler) answer(callbackID, text string) error {
	cb := tgbotapi.NewCallback(callbackID, text)
	_, err := h.api.Request(cb)
	return err
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
