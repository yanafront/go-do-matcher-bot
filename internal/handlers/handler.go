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
	if n.CandidateUUID != "" {
		id, err := uuid.Parse(n.CandidateUUID)
		if err == nil {
			candidate, _ := h.users.GetByTgID(ctx, n.CandidateID)
			if candidate != nil && !candidate.SearchActive {
				return nil
			}
			_ = h.users.IncrementMatchesReceived(ctx, id)
		}
	}
	pct := scorePercent(n.Score)
	text := fmt.Sprintf(
		"🎯 <b>Подходящая вакансия · %d%%</b>\n\n<b>%s</b>\n%s\n\n📍 %s · 💰 %s\n\nНажми «Откликнуться», если интересно — работодатель увидит твой профиль.",
		pct,
		escape(n.Title),
		escape(n.Description),
		escape(n.City),
		formatSalary(n.Salary),
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
	h.sendText(msg.Chat.ID, "👋 Привет! Здесь ищут работу и нанимают людей прямо в Telegram.\n\nВыбери, кто ты — и я помогу настроить профиль.", roleKeyboard())
}

func (h *Handler) handleMenu(ctx context.Context, msg *tgbotapi.Message) {
	user, err := h.users.GetByTgID(ctx, msg.Chat.ID)
	if err != nil || user == nil || user.Role == "" {
		h.handleStart(ctx, msg)
		return
	}
	_ = h.session.Clear(ctx, msg.Chat.ID)
	h.sendRoleHome(ctx, msg.Chat.ID, user)
}

func (h *Handler) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	if sess == nil || sess.Step == "" {
		user, _ := h.users.GetByTgID(ctx, msg.Chat.ID)
		if user != nil && user.Role != "" {
			h.sendText(msg.Chat.ID, "Главное меню — команда /menu 🙂", nil)
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
		h.sendText(msg.Chat.ID, "Нажми кнопку ниже, чтобы отправить номер 📱", contactKeyboard())
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
	case strings.HasPrefix(data, "accept:"):
		h.handleAccept(ctx, chatID, strings.TrimPrefix(data, "accept:"))
	case strings.HasPrefix(data, "apply:"):
		h.handleApply(ctx, chatID, strings.TrimPrefix(data, "apply:"))
	case strings.HasPrefix(data, "skip:"):
		h.sendText(chatID, "Ок, пропустили. Пришлю следующую, когда появится 🙂", nil)
	case strings.HasPrefix(data, "hire:"):
		h.handleHire(ctx, chatID, strings.TrimPrefix(data, "hire:"))
	case strings.HasPrefix(data, "reject:"):
		h.handleReject(ctx, chatID, strings.TrimPrefix(data, "reject:"))
	case strings.HasPrefix(data, "close:"):
		h.handleCloseVacancy(ctx, chatID, strings.TrimPrefix(data, "close:"))
	case data == "menu:create_vacancy":
		h.beginVacancyCreation(ctx, chatID, false)
	case data == "menu:create_one":
		h.beginVacancyCreation(ctx, chatID, true)
	case data == "menu:my_vacancies":
		h.showMyVacancies(ctx, chatID)
	case strings.HasPrefix(data, "menu:applicants:"):
		h.showApplicants(ctx, chatID, strings.TrimPrefix(data, "menu:applicants:"))
	case strings.HasPrefix(data, "review:"):
		h.handleReviewRating(ctx, chatID, strings.TrimPrefix(data, "review:"))
	case strings.HasPrefix(data, "skip_review:"):
		_ = h.session.Clear(ctx, chatID)
		h.sendText(chatID, "Без проблем! Если захочешь — оценишь позже.", nil)
	case data == "menu:toggle_search":
		h.toggleSearch(ctx, chatID)
	case data == "menu:toggle_hiring":
		h.toggleHiring(ctx, chatID)
	case data == "menu:edit_job":
		h.beginEditJob(ctx, chatID)
	case data == "menu:switch_employer":
		h.switchToEmployer(ctx, chatID, q.From)
	case data == "menu:switch_candidate":
		h.switchToCandidate(ctx, chatID, q.From)
	case strings.HasPrefix(data, "rate_prompt:"):
		h.handleRatePrompt(ctx, chatID, strings.TrimPrefix(data, "rate_prompt:"))
	case strings.HasPrefix(data, "menu:hired:"):
		h.showHiredTeam(ctx, chatID, strings.TrimPrefix(data, "menu:hired:"))
	case strings.HasPrefix(data, "needed:"):
		h.handleNeededPick(ctx, chatID, strings.TrimPrefix(data, "needed:"))
	case strings.HasPrefix(data, "pause_vac:"):
		h.setVacancyCollecting(ctx, chatID, strings.TrimPrefix(data, "pause_vac:"), false)
	case strings.HasPrefix(data, "resume_vac:"):
		h.setVacancyCollecting(ctx, chatID, strings.TrimPrefix(data, "resume_vac:"), true)
	}
}

func (h *Handler) startCandidate(ctx context.Context, chatID int64, from *tgbotapi.User) {
	user := &models.User{TgID: chatID, Role: models.RoleCandidate}
	if from != nil {
		user.Username = from.UserName
	}
	_ = h.users.Save(ctx, user)
	_ = h.session.SetStep(ctx, chatID, StepCandidateName, nil)
	h.sendText(chatID, "Как тебя зовут? 🙂", nil)
}

func (h *Handler) startEmployer(ctx context.Context, chatID int64, from *tgbotapi.User) {
	user := &models.User{TgID: chatID, Role: models.RoleEmployer}
	if from != nil {
		user.Username = from.UserName
	}
	_ = h.users.Save(ctx, user)
	_ = h.session.SetStep(ctx, chatID, StepEmployerName, nil)
	h.sendText(chatID, "Как называется компания или как к тебе обращаться?", nil)
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
	h.sendText(msg.Chat.ID, "Приятно познакомиться! В каком городе ищешь работу?", nil)
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
	h.sendText(msg.Chat.ID, "Какую должность ищешь?\nНапример: продавец, курьер, кассир", nil)
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
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	user.DesiredJob = job
	_ = h.users.Save(ctx, user)
	if sess != nil && sess.Data["edit_job"] == "1" {
		_ = h.session.Clear(ctx, msg.Chat.ID)
		h.sendText(msg.Chat.ID, fmt.Sprintf(
			"Готово! Теперь ищешь: <b>%s</b>\n\nБуду присылать подходящие вакансии.",
			escape(job),
		), h.candidateKB(ctx, msg.Chat.ID))
		return
	}
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepCandidatePhone, nil)
	h.sendText(msg.Chat.ID, "Последний шаг — номер телефона. Работодатель увидит его только после одобрения отклика.", contactKeyboard())
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
	h.sendText(msg.Chat.ID, "Готово! 🎉\n\nБуду присылать подходящие вакансии. Откликайся кнопкой — работодатель ответит в этом чате.", removeKeyboard())
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
	h.sendText(msg.Chat.ID, "В каком городе вакансия? (можно пропустить — напиши «-»)", nil)
}

func (h *Handler) saveEmployerCity(ctx context.Context, msg *tgbotapi.Message, city string) {
	user, _ := h.users.GetByTgID(ctx, msg.Chat.ID)
	if city == "-" {
		city = ""
	}
	user.City = city
	_ = h.users.Save(ctx, user)
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepEmployerJob, nil)
	h.sendText(msg.Chat.ID, "Кого ищешь? Напиши должность.", nil)
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
	h.sendText(msg.Chat.ID, "Зарплата в BYN (только число):", nil)
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
	h.sendText(msg.Chat.ID, "Когда выход на работу? Формат: 2026-06-17", nil)
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
	h.askNeededCount(ctx, msg.Chat.ID)
}

func (h *Handler) askNeededCount(ctx context.Context, chatID int64) {
	h.sendText(chatID, "Сколько людей нужно?\n\n👤 Чаще всего — <b>один человек</b>. Для смены выбери больше.", neededCountKeyboard())
}

func (h *Handler) handleNeededPick(ctx context.Context, chatID int64, pick string) {
	sess, _ := h.session.Get(ctx, chatID)
	if sess == nil {
		return
	}
	if pick == "custom" {
		h.sendText(chatID, fmt.Sprintf("Напиши число от 1 до %d.", models.MaxShiftSize), nil)
		return
	}
	count, err := strconv.Atoi(pick)
	if err != nil || count <= 0 || count > models.MaxShiftSize {
		h.sendText(chatID, "Выбери вариант из кнопок или напиши число.", neededCountKeyboard())
		return
	}
	switch sess.Step {
	case StepEmployerNeeded:
		h.finishEmployerNeeded(ctx, chatID, count)
	case StepVacancyNeeded:
		h.proceedVacancyNeeded(ctx, chatID, count)
	}
}

func (h *Handler) finishEmployerNeeded(ctx context.Context, chatID int64, count int) {
	user, _ := h.users.GetByTgID(ctx, chatID)
	sess, _ := h.session.Get(ctx, chatID)
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
		h.sendText(chatID, "Ошибка создания вакансии.", nil)
		return
	}
	_ = h.session.Clear(ctx, chatID)
	h.sendText(chatID, vacancyPublishedText(count), h.employerKB(ctx, chatID))
}

func (h *Handler) proceedVacancyNeeded(ctx context.Context, chatID int64, count int) {
	sess, _ := h.session.Get(ctx, chatID)
	sess.Data["needed"] = strconv.Itoa(count)
	_ = h.session.SetStep(ctx, chatID, StepVacancyStart, sess.Data)
	if isSingleHire(count) {
		h.sendText(chatID, "Последний шаг — когда выход на работу?\nФормат: 2026-06-17", nil)
		return
	}
	h.sendText(chatID, "Когда старт смены?\nФормат: 2026-06-17", nil)
}

func (h *Handler) parseNeededCount(raw string) (int, bool) {
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || count <= 0 || count > models.MaxShiftSize {
		return 0, false
	}
	return count, true
}

func (h *Handler) saveEmployerNeeded(ctx context.Context, msg *tgbotapi.Message, raw string) {
	count, ok := h.parseNeededCount(raw)
	if !ok {
		h.sendText(msg.Chat.ID, fmt.Sprintf("Введи число от 1 до %d или выбери кнопку.", models.MaxShiftSize), neededCountKeyboard())
		return
	}
	h.finishEmployerNeeded(ctx, msg.Chat.ID, count)
}

func (h *Handler) beginVacancyCreation(ctx context.Context, chatID int64, single bool) {
	data := map[string]string{}
	if single {
		data["quick_single"] = "1"
		data["needed"] = "1"
	}
	_ = h.session.SetStep(ctx, chatID, StepVacancyTitle, data)
	if single {
		h.sendText(chatID, "👤 <b>Быстрая вакансия</b> на одного человека.\n\nКак называется должность?", nil)
		return
	}
	h.sendText(chatID, "👥 <b>Смена / команда</b>\n\nКак называется вакансия?", nil)
}

func (h *Handler) saveVacancyTitle(ctx context.Context, msg *tgbotapi.Message, title string) {
	if title == "" {
		h.sendText(msg.Chat.ID, "Введи название.", nil)
		return
	}
	sess, _ := h.session.Get(ctx, msg.Chat.ID)
	sess.Data["title"] = title
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepVacancyDesc, sess.Data)
	h.sendText(msg.Chat.ID, "Кратко опиши задачи и условия:", nil)
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
	_ = h.session.Save(ctx, sess)
	if sess.Data["quick_single"] == "1" {
		_ = h.session.SetStep(ctx, msg.Chat.ID, StepVacancyStart, sess.Data)
		h.sendText(msg.Chat.ID, "Когда выход на работу?\nФормат: 2026-06-17", nil)
		return
	}
	_ = h.session.SetStep(ctx, msg.Chat.ID, StepVacancyNeeded, sess.Data)
	h.askNeededCount(ctx, msg.Chat.ID)
}

func (h *Handler) saveVacancyNeeded(ctx context.Context, msg *tgbotapi.Message, raw string) {
	count, ok := h.parseNeededCount(raw)
	if !ok {
		h.sendText(msg.Chat.ID, fmt.Sprintf("Введи число от 1 до %d или выбери кнопку.", models.MaxShiftSize), neededCountKeyboard())
		return
	}
	h.proceedVacancyNeeded(ctx, msg.Chat.ID, count)
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
	if needed <= 0 {
		needed = 1
	}
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
	h.sendText(msg.Chat.ID, vacancyPublishedText(needed), h.employerKB(ctx, msg.Chat.ID))
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
	if !user.SearchActive {
		h.sendText(chatID, "Сначала возобнови поиск в /menu 🙂", h.candidateKB(ctx, chatID))
		return
	}
	app, err := h.apps.Apply(ctx, vacancyID, user.ID)
	if err != nil {
		msg := "Не получилось откликнуться. Попробуй позже."
		switch err.Error() {
		case "vacancy is not collecting":
			msg = "Набор по этой вакансии приостановлен."
		case "employer paused hiring":
			msg = "Работодатель временно не принимает отклики."
		case "candidate search paused":
			msg = "Сначала возобнови поиск в /menu 🙂"
		case "vacancy is not open":
			msg = "Вакансия уже закрыта."
		}
		h.sendText(chatID, msg, nil)
		return
	}
	h.sendText(chatID, "✅ Отклик отправлен!\n\nЖди ответа работодателя — напишем, как только примет отклик.", nil)

	avg, count, _ := h.users.Rating(ctx, app.CandidateID)
	v, _ := h.vacancy.Get(ctx, vacancyID)
	single := v != nil && isSingleHire(v.NeededCount)
	body := fmt.Sprintf(
		"👤 <b>Новый отклик</b> на «%s»\n\nИмя: %s\nГород: %s\nИщет: %s\nРейтинг: %s\n\n<i>Контакты откроются после «Принять»</i>",
		escape(app.VacancyTitle),
		escape(app.CandidateName),
		escape(app.CandidateCity),
		escape(app.CandidateJob),
		formatRating(avg, count),
	)
	if single {
		body = fmt.Sprintf(
			"👤 <b>Отклик</b> на «%s»\n\n%s · %s\nИщет: %s · %s\n\n<i>Нравится? Нажми «Принять» → «Нанять — готово»</i>",
			escape(app.VacancyTitle),
			escape(app.CandidateName),
			escape(app.CandidateCity),
			escape(app.CandidateJob),
			formatRating(avg, count),
		)
	}
	m := tgbotapi.NewMessage(app.EmployerTgID, body)
	m.ParseMode = tgbotapi.ModeHTML
	m.ReplyMarkup = applicationKeyboard(models.AppSent, app.ID.String(), single)
	_, _ = h.api.Send(m)

	if v != nil && !v.Collecting {
		appCount, _ := h.vacancy.ActiveApplicationCount(ctx, vacancyID)
		h.sendText(app.EmployerTgID, fmt.Sprintf(
			"⏸ Набор по «%s» на паузе — %d откликов в очереди.\n\nРазбери текущих в «Отклики» или нажми «Возобновить».",
			app.VacancyTitle, appCount,
		), vacancyActionsKeyboard(v.ID.String(), false, false, v.NeededCount))
	}
}

func (h *Handler) handleAccept(ctx context.Context, chatID int64, applicationIDRaw string) {
	applicationID, err := uuid.Parse(applicationIDRaw)
	if err != nil {
		return
	}
	employer, _ := h.users.GetByTgID(ctx, chatID)
	app, err := h.apps.Accept(ctx, applicationID)
	if err != nil || app == nil {
		h.sendText(chatID, "Не удалось принять отклик.", nil)
		return
	}
	if employer == nil || app.EmployerTgID != chatID {
		return
	}

	v, _ := h.vacancy.Get(ctx, app.VacancyID)
	single := v != nil && isSingleHire(v.NeededCount)

	contact := fmt.Sprintf(
		"📇 <b>Контакты</b>\n\nИмя: %s\nТелефон: %s\nTelegram: %s",
		escape(app.CandidateName),
		escape(app.CandidatePhone),
		escape(tgLink(app.CandidateUser, app.CandidateTgID)),
	)
	if single {
		contact += "\n\nПодходит? Нажми <b>Нанять — готово</b> — вакансия закроется."
	} else {
		contact += "\n\nСвяжись с кандидатом. Если всё ок — нажми <b>Нанять</b>."
	}
	m := tgbotapi.NewMessage(chatID, contact)
	m.ParseMode = tgbotapi.ModeHTML
	m.ReplyMarkup = applicationKeyboard(models.AppAccepted, app.ID.String(), single)
	_, _ = h.api.Send(m)

	h.sendText(app.CandidateTgID, fmt.Sprintf(
		"🎉 Отличные новости!\n\nРаботодатель принял твой отклик на «%s».\nСкоро свяжется с тобой — держи телефон под рукой.",
		app.VacancyTitle,
	), nil)
}

func (h *Handler) handleHire(ctx context.Context, chatID int64, applicationIDRaw string) {
	applicationID, err := uuid.Parse(applicationIDRaw)
	if err != nil {
		return
	}
	app, vacancy, err := h.apps.Hire(ctx, applicationID)
	if err != nil {
		h.sendText(chatID, "Сначала прими отклик, потом можно нанять.", nil)
		return
	}
	remaining := vacancy.NeededCount - vacancy.FilledCount
	if isSingleHire(vacancy.NeededCount) && vacancy.Status == models.VacancyClosed {
		h.sendText(chatID, fmt.Sprintf(
			"🎉 Готово!\n\n<b>%s</b> нанят(а) на «%s».\nВакансия закрыта.",
			escape(app.CandidateName), escape(app.VacancyTitle),
		), h.employerKB(ctx, chatID))
	} else if vacancy.Status == models.VacancyClosed {
		h.sendText(chatID, fmt.Sprintf(
			"✅ %s в команде!\n\n🎉 Смена набрана: %d из %d.",
			app.CandidateName, vacancy.FilledCount, vacancy.NeededCount,
		), h.employerKB(ctx, chatID))
	} else {
		h.sendText(chatID, fmt.Sprintf(
			"✅ %s нанят(а)!\n\n👥 В смене: %d из %d · осталось %d.",
			app.CandidateName, vacancy.FilledCount, vacancy.NeededCount, remaining,
		), nil)
	}
	h.sendText(app.CandidateTgID, fmt.Sprintf(
		"🎊 Поздравляем!\n\nТебя наняли на «%s». Удачи на новом месте!",
		app.VacancyTitle,
	), nil)

	h.promptReviewPair(ctx, app, vacancy)

	if vacancy.Status != models.VacancyClosed {
		if resumed, _ := h.vacancy.MaybeResumeCollecting(ctx, vacancy.ID); resumed {
			h.sendText(chatID, fmt.Sprintf(
				"▶️ Набор снова открыт — ищем ещё %d человек в смену.\nОтклики смотри в «Мои вакансии».",
				remaining,
			), vacancyActionsKeyboard(vacancy.ID.String(), true, false, vacancy.NeededCount))
		}
		return
	}

	h.promptAllPendingReviews(ctx, vacancy)
}

func (h *Handler) handleReject(ctx context.Context, chatID int64, applicationIDRaw string) {
	applicationID, err := uuid.Parse(applicationIDRaw)
	if err != nil {
		return
	}
	app, err := h.apps.Get(ctx, applicationID)
	if err != nil || app == nil {
		h.sendText(chatID, "Отклик не найден.", nil)
		return
	}
	if err := h.apps.Reject(ctx, applicationID); err != nil {
		h.sendText(chatID, "Что-то пошло не так.", nil)
		return
	}
	h.sendText(chatID, "Отклик отклонён.", nil)
	if app.Status == models.AppSent || app.Status == models.AppAccepted {
		h.sendText(app.CandidateTgID, fmt.Sprintf(
			"К сожалению, по вакансии «%s» выбрали другого кандидата.\nНе расстраивайся — пришлю другие варианты!",
			app.VacancyTitle,
		), nil)
	}
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
	h.sendText(chatID, "Вакансия закрыта.", h.employerKB(ctx, chatID))
	if v != nil {
		h.promptAllPendingReviews(ctx, v)
	}
}

func (h *Handler) beginEditJob(ctx context.Context, chatID int64) {
	user, _ := h.users.GetByTgID(ctx, chatID)
	if user == nil || user.Role != models.RoleCandidate {
		return
	}
	_ = h.session.SetStep(ctx, chatID, StepCandidateJob, map[string]string{"edit_job": "1"})
	h.sendText(chatID, fmt.Sprintf(
		"Сейчас ищешь: <b>%s</b>\n\nНапиши новую должность.\nНапример: продавец, курьер, кассир",
		escape(user.DesiredJob),
	), nil)
}

func (h *Handler) switchToEmployer(ctx context.Context, chatID int64, from *tgbotapi.User) {
	user, _ := h.users.GetByTgID(ctx, chatID)
	if user == nil {
		h.startEmployer(ctx, chatID, from)
		return
	}
	if from != nil && from.UserName != "" {
		user.Username = from.UserName
	}
	user.Role = models.RoleEmployer
	_ = h.users.Save(ctx, user)
	_ = h.session.Clear(ctx, chatID)
	if h.profileComplete(user) {
		h.sendText(chatID, "Переключились в режим <b>работодателя</b> 💼\n\nМожно создавать вакансии и смотреть отклики.", h.employerKB(ctx, chatID))
		return
	}
	_ = h.session.SetStep(ctx, chatID, StepEmployerName, nil)
	h.sendText(chatID, "Как называется компания или как к тебе обращаться?", nil)
}

func (h *Handler) switchToCandidate(ctx context.Context, chatID int64, from *tgbotapi.User) {
	user, _ := h.users.GetByTgID(ctx, chatID)
	if user == nil {
		h.startCandidate(ctx, chatID, from)
		return
	}
	if from != nil && from.UserName != "" {
		user.Username = from.UserName
	}
	user.Role = models.RoleCandidate
	_ = h.users.Save(ctx, user)
	_ = h.session.Clear(ctx, chatID)
	if h.profileComplete(user) {
		h.sendText(chatID, "Переключились в режим <b>соискателя</b> 🔍\n\nБуду присылать подходящие вакансии.", h.candidateKB(ctx, chatID))
		return
	}
	h.continueCandidateProfile(ctx, chatID, user)
}

func (h *Handler) continueCandidateProfile(ctx context.Context, chatID int64, user *models.User) {
	switch {
	case user.Name == "":
		_ = h.session.SetStep(ctx, chatID, StepCandidateName, nil)
		h.sendText(chatID, "Как тебя зовут? 🙂", nil)
	case user.City == "":
		_ = h.session.SetStep(ctx, chatID, StepCandidateCity, nil)
		h.sendText(chatID, "В каком городе ищешь работу?", nil)
	case user.DesiredJob == "":
		_ = h.session.SetStep(ctx, chatID, StepCandidateJob, nil)
		h.sendText(chatID, "Какую должность ищешь?\nНапример: продавец, курьер, кассир", nil)
	default:
		_ = h.session.SetStep(ctx, chatID, StepCandidatePhone, nil)
		h.sendText(chatID, "Нужен номер телефона — работодатель увидит его только после одобрения отклика.", contactKeyboard())
	}
}

func (h *Handler) toggleSearch(ctx context.Context, chatID int64) {
	active, err := h.users.ToggleSearch(ctx, chatID)
	if err != nil {
		return
	}
	if active {
		h.sendText(chatID, "▶️ Поиск возобновлён!\n\nСнова буду присылать подходящие вакансии.", h.candidateKB(ctx, chatID))
		return
	}
	h.sendText(chatID, "⏸ Поиск на паузе.\n\nНовые вакансии присылать не буду — возобновить можно здесь же.", h.candidateKB(ctx, chatID))
}

func (h *Handler) toggleHiring(ctx context.Context, chatID int64) {
	paused, err := h.users.ToggleHiringPaused(ctx, chatID)
	if err != nil {
		return
	}
	if paused {
		h.sendText(chatID, "⏸ Набор по всем вакансиям на паузе.\n\nНовые отклики и подбор кандидатов остановлены.", h.employerKB(ctx, chatID))
		return
	}
	h.sendText(chatID, "▶️ Набор возобновлён!\n\nОткрытые вакансии снова принимают отклики (если по ним не стоит отдельная пауза).", h.employerKB(ctx, chatID))
}

func (h *Handler) setVacancyCollecting(ctx context.Context, chatID int64, vacancyIDRaw string, collecting bool) {
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
	if v.Status == models.VacancyClosed {
		h.sendText(chatID, "Вакансия уже закрыта.", nil)
		return
	}
	if err := h.vacancy.SetCollecting(ctx, vacancyID, collecting); err != nil {
		h.sendText(chatID, "Не удалось обновить статус.", nil)
		return
	}
	if collecting {
		h.sendText(chatID, fmt.Sprintf("▶️ Набор по «%s» возобновлён.", v.Title), vacancyActionsKeyboard(v.ID.String(), true, false, v.NeededCount))
		return
	}
	h.sendText(chatID, fmt.Sprintf("⏸ Набор по «%s» приостановлен.\n\nНовые отклики не принимаются.", v.Title), vacancyActionsKeyboard(v.ID.String(), false, false, v.NeededCount))
}

func (h *Handler) showMyVacancies(ctx context.Context, chatID int64) {
	user, _ := h.users.GetByTgID(ctx, chatID)
	items, err := h.vacancy.ListEmployer(ctx, user.ID)
	if err != nil || len(items) == 0 {
		h.sendText(chatID, "Пока нет вакансий. Создай первую — кнопка ниже 👇", h.employerKB(ctx, chatID))
		return
	}
	open, paused, closed := 0, 0, 0
	for _, v := range items {
		switch {
		case v.Status == models.VacancyClosed:
			closed++
		case !v.Collecting:
			paused++
		default:
			open++
		}
	}
	h.sendText(chatID, fmt.Sprintf(
		"📋 <b>Твои вакансии (%d)</b>\n🟢 набор: %d · ⏸ пауза: %d · 🔴 закрыто: %d",
		len(items), open, paused, closed,
	), nil)
	for _, v := range items {
		status := "🟢 открыта"
		if v.Status == models.VacancyClosed {
			status = "🔴 закрыта"
		} else if !v.Collecting {
			status = "⏸ пауза"
		}
		count, _ := h.vacancy.ActiveApplicationCount(ctx, v.ID)
		limit := models.ApplicationLimit(v.NeededCount)
		var text string
		if isSingleHire(v.NeededCount) {
			if v.Status == models.VacancyClosed {
				text = fmt.Sprintf("«%s» 👤 1 человек\n📍 %s · 💰 %s\n%s", v.Title, v.City, formatSalary(v.Salary), status)
			} else {
				text = fmt.Sprintf("«%s» 👤 1 человек\n📍 %s · 💰 %s\n%s · 📝 %d откл.", v.Title, v.City, formatSalary(v.Salary), status, count)
			}
		} else {
			text = fmt.Sprintf("«%s» (%s)\n📍 %s · 💰 %s\n%s · 👥 %d/%d · 📝 %d/%d",
				v.Title, teamLabel(v.NeededCount), v.City, formatSalary(v.Salary), status, v.FilledCount, v.NeededCount, count, limit)
		}
		h.sendText(chatID, text, vacancyActionsKeyboard(v.ID.String(), v.Collecting, v.Status == models.VacancyClosed, v.NeededCount))
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
		h.sendText(chatID, "Пока никто не откликнулся.", vacancyActionsKeyboard(vacancyIDRaw, v.Collecting, v.Status == models.VacancyClosed, v.NeededCount))
		return
	}
	h.sendText(chatID, applicantsHeader(v), nil)
	single := isSingleHire(v.NeededCount)
	var pending, hired int
	for _, a := range apps {
		if a.Status == models.AppHired {
			hired++
		} else if a.Status != models.AppRejected {
			pending++
		}
	}
	if hired > 0 && !single {
		h.sendText(chatID, fmt.Sprintf("✅ В смене (%d):", hired), nil)
		for _, a := range apps {
			if a.Status != models.AppHired {
				continue
			}
			h.sendHiredCard(ctx, chatID, employer.ID, a)
		}
	}
	if hired > 0 && single {
		for _, a := range apps {
			if a.Status == models.AppHired {
				h.sendHiredCard(ctx, chatID, employer.ID, a)
			}
		}
	}
	if pending > 0 {
		label := "📥 Отклики:"
		if single {
			label = "📥 Кандидат:"
		}
		h.sendText(chatID, fmt.Sprintf("%s (%d)", label, pending), nil)
		for _, a := range apps {
			if a.Status == models.AppRejected || a.Status == models.AppHired {
				continue
			}
			avg, count, _ := h.users.Rating(ctx, a.CandidateID)
			text := h.formatApplicantCard(a, avg, count)
			kb := applicationKeyboard(a.Status, a.ID.String(), single)
			h.sendText(chatID, text, kb)
		}
	}
}

func (h *Handler) showHiredTeam(ctx context.Context, chatID int64, vacancyIDRaw string) {
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
	if err != nil {
		return
	}
	var hired []models.ApplicationView
	for _, a := range apps {
		if a.Status == models.AppHired {
			hired = append(hired, a)
		}
	}
	if len(hired) == 0 {
		msg := "Пока никого не наняли в эту смену."
		if isSingleHire(v.NeededCount) {
			msg = "Пока никого не наняли."
		}
		h.sendText(chatID, msg, vacancyActionsKeyboard(vacancyIDRaw, v.Collecting, v.Status == models.VacancyClosed, v.NeededCount))
		return
	}
	if isSingleHire(v.NeededCount) {
		h.sendText(chatID, fmt.Sprintf("✅ «%s» — нанят", v.Title), nil)
	} else {
		h.sendText(chatID, fmt.Sprintf("✅ Команда «%s»: %d из %d", v.Title, len(hired), v.NeededCount), nil)
	}
	for _, a := range hired {
		h.sendHiredCard(ctx, chatID, employer.ID, a)
	}
}

func (h *Handler) sendHiredCard(ctx context.Context, chatID int64, employerID uuid.UUID, a models.ApplicationView) {
	avg, count, _ := h.users.Rating(ctx, a.CandidateID)
	text := h.formatApplicantCard(a, avg, count)
	reviewed, _ := h.reviews.Has(ctx, employerID, a.CandidateID, a.VacancyID)
	if reviewed {
		text += "\n<i>Ты уже оценил(а) этого кандидата</i>"
		h.sendText(chatID, text, nil)
		return
	}
	h.sendText(chatID, text, hiredRateKeyboard(a.VacancyID.String(), a.CandidateID.String()))
}

func (h *Handler) handleRatePrompt(ctx context.Context, chatID int64, payload string) {
	parts := strings.Split(payload, ":")
	if len(parts) != 2 {
		return
	}
	vacancyID, err := uuid.Parse(parts[0])
	if err != nil {
		return
	}
	candidateID, err := uuid.Parse(parts[1])
	if err != nil {
		return
	}
	employer, _ := h.users.GetByTgID(ctx, chatID)
	v, _ := h.vacancy.Get(ctx, vacancyID)
	if employer == nil || v == nil || v.EmployerID != employer.ID {
		return
	}
	candidate, _ := h.users.GetByID(ctx, candidateID)
	if candidate == nil {
		return
	}
	h.sendText(chatID, fmt.Sprintf("Оцени работу <b>%s</b> по вакансии «%s»:", escape(candidate.Name), escape(v.Title)), reviewRatingKeyboard(vacancyID.String(), candidateID.String()))
}

func (h *Handler) promptReviewPair(ctx context.Context, app *models.ApplicationView, vacancy *models.Vacancy) {
	employer, _ := h.users.GetByTgID(ctx, app.EmployerTgID)
	candidate, _ := h.users.GetByTgID(ctx, app.CandidateTgID)
	if employer == nil || candidate == nil {
		return
	}
	has, _ := h.reviews.Has(ctx, employer.ID, candidate.ID, vacancy.ID)
	if !has {
		h.sendText(app.EmployerTgID, fmt.Sprintf(
			"Оцени <b>%s</b> — как прошло знакомство?",
			escape(app.CandidateName),
		), reviewRatingKeyboard(vacancy.ID.String(), candidate.ID.String()))
	}
	hasReverse, _ := h.reviews.Has(ctx, candidate.ID, employer.ID, vacancy.ID)
	if !hasReverse {
		h.sendText(app.CandidateTgID, "Как тебе работодатель? Поставь оценку:", reviewRatingKeyboard(vacancy.ID.String(), employer.ID.String()))
	}
}

func (h *Handler) promptAllPendingReviews(ctx context.Context, vacancy *models.Vacancy) {
	employer, _ := h.users.GetByID(ctx, vacancy.EmployerID)
	if employer == nil {
		return
	}
	apps, err := h.apps.ListByVacancy(ctx, vacancy.ID)
	if err != nil {
		return
	}
	for _, a := range apps {
		if a.Status != models.AppHired {
			continue
		}
		has, _ := h.reviews.Has(ctx, employer.ID, a.CandidateID, vacancy.ID)
		if has {
			continue
		}
		h.sendText(employer.TgID, fmt.Sprintf(
			"Не забудь оценить <b>%s</b> по «%s»:",
			escape(a.CandidateName), escape(vacancy.Title),
		), reviewRatingKeyboard(vacancy.ID.String(), a.CandidateID.String()))
	}
}

func (h *Handler) formatApplicantCard(a models.ApplicationView, avg float64, reviewCount int) string {
	statusLabel := map[string]string{
		models.AppSent:     "⏳ ждёт решения",
		models.AppAccepted: "🟢 принят",
		models.AppHired:    "✅ нанят",
		models.AppRejected: "🔴 отклонён",
	}[a.Status]
	if statusLabel == "" {
		statusLabel = a.Status
	}
	base := fmt.Sprintf(
		"👤 <b>%s</b> · %s\n📍 %s · ищет: %s\n⭐ %s",
		escape(a.CandidateName),
		statusLabel,
		escape(a.CandidateCity),
		escape(a.CandidateJob),
		formatRating(avg, reviewCount),
	)
	if a.Status == models.AppAccepted || a.Status == models.AppHired {
		base += fmt.Sprintf("\n📞 %s · %s", escape(a.CandidatePhone), escape(tgLink(a.CandidateUser, a.CandidateTgID)))
	}
	return base
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
	h.sendText(chatID, "Хочешь добавить пару слов? Напиши текст или «-» чтобы пропустить.", nil)
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
		h.sendText(msg.Chat.ID, "Не удалось сохранить. Попробуй ещё раз.", nil)
		return
	}
	_ = h.session.Clear(ctx, msg.Chat.ID)
	h.sendText(msg.Chat.ID, "Спасибо! Отзыв сохранён 🙏", nil)
}

func (h *Handler) sendRoleHome(ctx context.Context, chatID int64, user *models.User) {
	user, _ = h.users.GetByTgID(ctx, chatID)
	if user == nil {
		return
	}
	switch user.Role {
	case models.RoleCandidate:
		searchLine := "▶️ поиск активен"
		if !user.SearchActive {
			searchLine = "⏸ поиск на паузе"
		}
		h.sendText(chatID, fmt.Sprintf(
			"Привет, %s! 👋\n\n📍 %s · ищешь: %s\n%s\n📊 Подобрано: %d · Выполнено работ: %d\n\nПрофессию можно сменить кнопкой ниже. Хочешь нанимать — «Нанимаю».",
			user.Name, user.City, user.DesiredJob, searchLine, user.MatchesReceived, user.JobsCompleted,
		), h.candidateKB(ctx, chatID))
	case models.RoleEmployer:
		items, _ := h.vacancy.ListEmployer(ctx, user.ID)
		active, paused := 0, 0
		for _, v := range items {
			if v.Status != models.VacancyOpen {
				continue
			}
			active++
			if !v.Collecting {
				paused++
			}
		}
		hiringLine := "▶️ набор активен"
		if user.HiringPaused {
			hiringLine = "⏸ все вакансии на паузе"
		}
		h.sendText(chatID, fmt.Sprintf(
			"Привет, %s! 👋\n\n📍 %s\n%s\n📊 Открытых вакансий: %d (на паузе: %d) · Закрыто заданий: %d\n\nВакансии — в «Мои вакансии». Ищешь работу — кнопка «Ищу работу».",
			user.Name, user.City, hiringLine, active, paused, user.JobsCompleted,
		), h.employerKB(ctx, chatID))
	}
}

func (h *Handler) candidateKB(ctx context.Context, chatID int64) tgbotapi.InlineKeyboardMarkup {
	u, _ := h.users.GetByTgID(ctx, chatID)
	if u == nil {
		return candidateMenuKeyboard(true)
	}
	return candidateMenuKeyboard(u.SearchActive)
}

func (h *Handler) employerKB(ctx context.Context, chatID int64) tgbotapi.InlineKeyboardMarkup {
	u, _ := h.users.GetByTgID(ctx, chatID)
	if u == nil {
		return employerMenuKeyboard(false)
	}
	return employerMenuKeyboard(u.HiringPaused)
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
	m.ParseMode = tgbotapi.ModeHTML
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
