package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/pg-analytics-service/pg-analytics-service/internal/application"
	"github.com/pg-analytics-service/pg-analytics-service/internal/domain"
)

const dateLayout = "2006-01-02"

type Handler struct {
	reports     *application.ReportService
	recompute   *application.RecomputeService
	audit       *application.AuditService
	health      *application.HealthService
	defaultDays int
}

func NewHandler(
	reports *application.ReportService,
	recompute *application.RecomputeService,
	audit *application.AuditService,
	health *application.HealthService,
	defaultDays int,
) *Handler {
	return &Handler{
		reports:     reports,
		recompute:   recompute,
		audit:       audit,
		health:      health,
		defaultDays: defaultDays,
	}
}

func (h *Handler) Routes(managementAPIKey string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Get("/api/v1/health", h.getHealth)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/reports", h.listReports)
		r.Get("/reports/{slug}", h.getReport)
		r.Get("/reports/{slug}/run", h.runReport)

		r.Group(func(r chi.Router) {
			r.Use(managementAuthMiddleware(managementAPIKey))
			r.Post("/recomputations", h.triggerRecompute)
			r.Get("/recomputations/{id}", h.getRecomputeRun)
			r.Get("/audit-entries", h.listAuditEntries)
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route was not found", nil)
	})

	return r
}

func (h *Handler) getHealth(w http.ResponseWriter, r *http.Request) {
	health := h.health.Check(r.Context())
	status := http.StatusOK
	if health.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, health, nil)
}

func (h *Handler) listReports(w http.ResponseWriter, r *http.Request) {
	filter := domain.ReportListFilter{
		Search: strings.TrimSpace(r.URL.Query().Get("search")),
		Limit:  parseIntWithDefault(r.URL.Query().Get("limit"), 20),
		Offset: parseIntWithDefault(r.URL.Query().Get("offset"), 0),
		Sort:   strings.TrimSpace(r.URL.Query().Get("sort")),
		Order:  strings.TrimSpace(r.URL.Query().Get("order")),
	}

	reports, pagination, err := h.reports.ListReports(r.Context(), filter)
	if err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, reports, map[string]any{"pagination": pagination})
}

func (h *Handler) getReport(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	report, err := h.reports.GetReport(r.Context(), slug)
	if err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, report, nil)
}

func (h *Handler) runReport(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	from, to, err := h.parseDateRange(r)
	if err != nil {
		handleError(w, err)
		return
	}

	params := domain.ReportRunParams{
		Window:    normalizeWindow(r.URL.Query().Get("window")),
		DateFrom:  from,
		DateTo:    to,
		Breakdown: strings.TrimSpace(r.URL.Query().Get("breakdown")),
		Limit:     parseIntWithDefault(r.URL.Query().Get("limit"), 50),
		Offset:    parseIntWithDefault(r.URL.Query().Get("offset"), 0),
		Source:    strings.TrimSpace(r.URL.Query().Get("source")),
		Status:    strings.TrimSpace(r.URL.Query().Get("status")),
	}

	result, err := h.reports.RunReport(r.Context(), slug, params)
	if err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result, map[string]any{"cache_hit": result.CacheHit})
}

type recomputeRequestBody struct {
	ReportSlug  string `json:"report_slug"`
	Window      string `json:"window"`
	DateFrom    string `json:"date_from"`
	DateTo      string `json:"date_to"`
	RequestedBy string `json:"requested_by"`
}

func (h *Handler) triggerRecompute(w http.ResponseWriter, r *http.Request) {
	var body recomputeRequestBody
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		handleError(w, domain.NewAppError(domain.ErrCodeInvalid, "invalid JSON body"))
		return
	}

	if strings.TrimSpace(body.ReportSlug) == "" {
		handleError(w, domain.NewAppError(domain.ErrCodeInvalid, "report_slug is required"))
		return
	}

	from, to, err := h.parseDateRangeWithValues(body.DateFrom, body.DateTo)
	if err != nil {
		handleError(w, err)
		return
	}

	requestedBy := strings.TrimSpace(body.RequestedBy)
	if requestedBy == "" {
		requestedBy = strings.TrimSpace(r.Header.Get("X-Actor"))
	}
	if requestedBy == "" {
		requestedBy = "management-api"
	}

	request := domain.RecomputeRequest{
		ReportSlug:   strings.TrimSpace(body.ReportSlug),
		Window:       normalizeWindow(body.Window),
		DateFrom:     from,
		DateTo:       to,
		RequestedBy:  requestedBy,
		RequestedVia: "http",
	}

	run, err := h.recompute.Trigger(r.Context(), request)
	if err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, run, nil)
}

func (h *Handler) getRecomputeRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.recompute.GetRun(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, run, nil)
}

func (h *Handler) listAuditEntries(w http.ResponseWriter, r *http.Request) {
	filter := domain.AuditFilter{
		Action: strings.TrimSpace(r.URL.Query().Get("action")),
		Actor:  strings.TrimSpace(r.URL.Query().Get("actor")),
		Limit:  parseIntWithDefault(r.URL.Query().Get("limit"), 20),
		Offset: parseIntWithDefault(r.URL.Query().Get("offset"), 0),
	}

	entries, pagination, err := h.audit.ListEntries(r.Context(), filter)
	if err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, entries, map[string]any{"pagination": pagination})
}

func (h *Handler) parseDateRange(r *http.Request) (time.Time, time.Time, error) {
	query := r.URL.Query()
	return h.parseDateRangeWithValues(query.Get("date_from"), query.Get("date_to"))
}

func (h *Handler) parseDateRangeWithValues(rawFrom, rawTo string) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, 0, -(h.defaultDays - 1))

	if strings.TrimSpace(rawTo) != "" {
		parsedTo, err := time.Parse(dateLayout, rawTo)
		if err != nil {
			return time.Time{}, time.Time{}, domain.NewAppError(domain.ErrCodeInvalid, "date_to must use YYYY-MM-DD format")
		}
		to = parsedTo.UTC()
	}

	if strings.TrimSpace(rawFrom) != "" {
		parsedFrom, err := time.Parse(dateLayout, rawFrom)
		if err != nil {
			return time.Time{}, time.Time{}, domain.NewAppError(domain.ErrCodeInvalid, "date_from must use YYYY-MM-DD format")
		}
		from = parsedFrom.UTC()
	}

	if strings.TrimSpace(rawFrom) == "" && strings.TrimSpace(rawTo) != "" {
		from = to.AddDate(0, 0, -(h.defaultDays - 1))
	}

	if from.After(to) {
		return time.Time{}, time.Time{}, domain.NewAppError(domain.ErrCodeInvalid, "date_from must be on or before date_to")
	}

	return from, to, nil
}

func parseIntWithDefault(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}

	return value
}

func normalizeWindow(raw string) string {
	window := strings.TrimSpace(strings.ToLower(raw))
	if window == "" {
		return domain.WindowDay
	}
	return window
}

func handleError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case domain.ErrCodeInvalid:
			writeError(w, http.StatusBadRequest, string(appErr.Code), appErr.Message, nil)
		case domain.ErrCodeNotFound:
			writeError(w, http.StatusNotFound, string(appErr.Code), appErr.Message, nil)
		case domain.ErrCodeUnauthorized:
			writeError(w, http.StatusUnauthorized, string(appErr.Code), appErr.Message, nil)
		case domain.ErrCodeConflict:
			writeError(w, http.StatusConflict, string(appErr.Code), appErr.Message, nil)
		case domain.ErrCodeUnavailable:
			writeError(w, http.StatusServiceUnavailable, string(appErr.Code), appErr.Message, nil)
		default:
			writeError(w, http.StatusInternalServerError, string(domain.ErrCodeInternal), appErr.Message, nil)
		}
		return
	}

	writeError(w, http.StatusInternalServerError, string(domain.ErrCodeInternal), fmt.Sprintf("internal error: %s", err.Error()), nil)
}
